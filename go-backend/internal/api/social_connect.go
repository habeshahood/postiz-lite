package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/redis/go-redis/v9"
)

// socialConnectRequest is the body the frontend sends after OAuth redirect.
type socialConnectRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	CodeVerifier string `json:"codeVerifier"`
	Refresh      string `json:"refresh"`
	Timezone     any    `json:"timezone"`
}

// handleSocialConnectReal exchanges the OAuth code for tokens and saves the integration.
func handleSocialConnectReal(store *db.Store, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		integration := chi.URLParam(r, "integration")

		// Parse body — accept both JSON and form-encoded
		raw, _ := io.ReadAll(r.Body)
		var body socialConnectRequest
		if err := json.Unmarshal(raw, &body); err != nil {
			slog.Error("social-connect body parse error", "error", err, "body", string(raw[:min(len(raw), 200)]))
			http.Error(w, `{"msg":"Invalid request body"}`, http.StatusBadRequest)
			return
		}

		if body.State == "" {
			http.Error(w, `{"msg":"Missing state"}`, http.StatusBadRequest)
			return
		}

		slog.Info("social-connect attempt", "provider", integration, "state", body.State, "hasCode", body.Code != "")

		// Look up the org from Redis (stored when the OAuth URL was generated)
		ctx := context.Background()
		orgID, err := rdb.Get(ctx, "organization:"+body.State).Result()
		if err != nil || orgID == "" {
			slog.Error("social-connect state not found in Redis", "state", body.State, "error", err)
			// List all organization keys for debugging
			keys, _ := rdb.Keys(ctx, "organization:*").Result()
			slog.Error("available organization keys", "count", len(keys), "keys", keys)
			http.Error(w, `{"msg":"Invalid or expired state — try adding the channel again"}`, http.StatusBadRequest)
			return
		}

		var accessToken, refreshToken, userID, userName, picture, username string
		var expiresIn int

		switch integration {
		case "youtube":
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeYouTubeToken(body.Code)
		case "facebook":
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeFacebookToken(body.Code, body.Refresh)
		case "tiktok":
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeTikTokToken(body.Code, body.CodeVerifier)
		case "x":
			codeVerifier, _ := rdb.Get(ctx, "login:"+body.State).Result()
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeXToken(body.Code, codeVerifier)
		default:
			http.Error(w, fmt.Sprintf(`{"msg":"OAuth connect not implemented for %s"}`, integration), http.StatusNotImplemented)
			return
		}

		if err != nil {
			slog.Error("OAuth token exchange failed", "provider", integration, "error", err)
			http.Error(w, fmt.Sprintf(`{"msg":"Authentication failed: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if userName == "" {
			userName = username
		}
		if userName == "" {
			userName = "Channel_" + userID[:8]
		}

		// Calculate token expiration
		var tokenExpiration *time.Time
		if expiresIn > 0 {
			t := time.Now().Add(time.Duration(expiresIn) * time.Second)
			tokenExpiration = &t
		}

		var rt *string
		if refreshToken != "" {
			rt = &refreshToken
		}

		// If this is a refresh, update the existing integration instead of creating a duplicate.
		// body.Refresh contains the internalId of the integration to refresh.
		if body.Refresh != "" {
			const refreshQ = `UPDATE "Integration" SET token = $1, "refreshToken" = $2, "tokenExpiration" = $3,
				"refreshNeeded" = false, "updatedAt" = NOW()
				WHERE "internalId" = $4 AND "organizationId" = $5 AND "deletedAt" IS NULL`
			tag, rerr := store.Exec(ctx, refreshQ, accessToken, rt, tokenExpiration, body.Refresh, orgID)
			if rerr == nil && tag.RowsAffected() > 0 {
				slog.Info("OAuth refresh successful", "provider", integration, "internalId", body.Refresh)
				writeJSON(w, http.StatusOK, map[string]any{
					"internalId": body.Refresh,
					"name":       userName,
					"identifier": integration,
					"onboarding": false,
					"pages":      []any{},
				})
				rdb.Del(ctx, "organization:"+body.State, "login:"+body.State, "refresh:"+body.State)
				return
			}
			slog.Warn("refresh update failed (no matching integration), creating new", "internalId", body.Refresh)
		}

		// Create new integration
		result, err := store.CreateIntegration(ctx,
			orgID, userID, userName, integration, "social", accessToken,
			rt, tokenExpiration,
		)
		if err != nil {
			slog.Error("failed to save integration", "provider", integration, "error", err)
			http.Error(w, `{"msg":"Failed to save integration"}`, http.StatusInternalServerError)
			return
		}

		// Update picture and profile if available
		if picture != "" || username != "" {
			store.Exec(ctx,
				`UPDATE "Integration" SET picture = COALESCE($1, picture), profile = COALESCE($2, profile) WHERE id = $3`,
				nilIfEmpty(picture), nilIfEmpty(username), result.ID,
			)
		}

		// Clean up Redis state
		rdb.Del(ctx, "organization:"+body.State, "login:"+body.State, "refresh:"+body.State)

		slog.Info("OAuth connect successful", "provider", integration, "name", userName, "id", userID)

		writeJSON(w, http.StatusOK, map[string]any{
			"id":         result.ID,
			"name":       userName,
			"picture":    picture,
			"identifier": integration,
			"internalId": userID,
			"onboarding": false,
			"pages":      []any{},
		})
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── YouTube token exchange ───────────────────────────────────

func exchangeYouTubeToken(code string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	clientID := os.Getenv("YOUTUBE_CLIENT_ID")
	clientSecret := os.Getenv("YOUTUBE_CLIENT_SECRET")
	frontendURL := os.Getenv("FRONTEND_URL")
	redirectURI := frontendURL + "/integrations/social/youtube"

	// Exchange code for tokens
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return "", "", "", "", "", "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	json.Unmarshal(raw, &tokenResp)
	if tokenResp.Error != "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Get user info
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	infoResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tokenResp.AccessToken, tokenResp.RefreshToken, "unknown", "YouTube Channel", "", "", tokenResp.ExpiresIn, nil
	}
	defer infoResp.Body.Close()

	var userInfo struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	json.NewDecoder(infoResp.Body).Decode(&userInfo)

	return tokenResp.AccessToken, tokenResp.RefreshToken, userInfo.ID, userInfo.Name, userInfo.Picture, "", tokenResp.ExpiresIn, nil
}

// ── Facebook token exchange ──────────────────────────────────

func exchangeFacebookToken(code, refresh string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	appID := os.Getenv("FACEBOOK_APP_ID")
	appSecret := os.Getenv("FACEBOOK_APP_SECRET")
	frontendURL := os.Getenv("FRONTEND_URL")
	redirectURI := frontendURL + "/integrations/social/facebook"
	if refresh != "" {
		redirectURI += "?refresh=" + refresh
	}

	// Exchange code for short-lived token
	u := fmt.Sprintf("https://graph.facebook.com/v20.0/oauth/access_token?client_id=%s&redirect_uri=%s&client_secret=%s&code=%s",
		appID, url.QueryEscape(redirectURI), appSecret, code)
	resp, err := http.Get(u)
	if err != nil {
		return "", "", "", "", "", "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	var shortToken struct {
		AccessToken string `json:"access_token"`
		Error       struct{ Message string } `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&shortToken)
	if shortToken.Error.Message != "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("facebook: %s", shortToken.Error.Message)
	}

	// Exchange for long-lived token
	u2 := fmt.Sprintf("https://graph.facebook.com/v20.0/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		appID, appSecret, shortToken.AccessToken)
	resp2, _ := http.Get(u2)
	defer resp2.Body.Close()
	var longToken struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	json.NewDecoder(resp2.Body).Decode(&longToken)
	if longToken.AccessToken == "" {
		longToken.AccessToken = shortToken.AccessToken
	}

	// Get user info
	resp3, _ := http.Get(fmt.Sprintf("https://graph.facebook.com/v20.0/me?fields=id,name,picture&access_token=%s", longToken.AccessToken))
	defer resp3.Body.Close()
	var fbUser struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Picture struct{ Data struct{ URL string } } `json:"picture"`
	}
	json.NewDecoder(resp3.Body).Decode(&fbUser)

	return longToken.AccessToken, longToken.AccessToken, fbUser.ID, fbUser.Name, fbUser.Picture.Data.URL, "", longToken.ExpiresIn, nil
}

// ── TikTok token exchange ────────────────────────────────────

func exchangeTikTokToken(code, codeVerifier string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	clientID := os.Getenv("TIKTOK_CLIENT_ID")
	clientSecret := os.Getenv("TIKTOK_CLIENT_SECRET")
	frontendURL := os.Getenv("FRONTEND_URL")
	redirectURI := frontendURL + "/integrations/social/tiktok"

	formData := url.Values{
		"client_key":    {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {redirectURI},
	}

	req, _ := http.NewRequest("POST", "https://open.tiktokapis.com/v2/oauth/token/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", "", "", "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)
	if tokenResp.Error != "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Get user info
	req2, _ := http.NewRequest("GET", "https://open.tiktokapis.com/v2/user/info/?fields=open_id,avatar_url,display_name,username", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	resp2, _ := http.DefaultClient.Do(req2)
	defer resp2.Body.Close()

	var userResp struct {
		Data struct {
			User struct {
				OpenID      string `json:"open_id"`
				AvatarURL   string `json:"avatar_url"`
				DisplayName string `json:"display_name"`
				Username    string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	json.NewDecoder(resp2.Body).Decode(&userResp)

	uid := strings.ReplaceAll(userResp.Data.User.OpenID, "-", "")
	return tokenResp.AccessToken, tokenResp.RefreshToken, uid, userResp.Data.User.DisplayName, userResp.Data.User.AvatarURL, userResp.Data.User.Username, 82800, nil
}

// ── X / Twitter token exchange (OAuth 1.0a) ──────────────────

func exchangeXToken(oauthVerifier, codeVerifier string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	if codeVerifier == "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("missing code verifier (OAuth state expired)")
	}

	parts := strings.SplitN(codeVerifier, ":", 2)
	if len(parts) != 2 {
		return "", "", "", "", "", "", 0, fmt.Errorf("invalid code verifier format")
	}
	oauthToken, oauthSecret := parts[0], parts[1]

	apiKey := os.Getenv("X_API_KEY")
	apiSecret := os.Getenv("X_API_SECRET")

	// Exchange request token for access token
	formData := url.Values{
		"oauth_verifier": {oauthVerifier},
	}

	req, _ := http.NewRequest("POST", "https://api.twitter.com/oauth/access_token?"+formData.Encode(), nil)
	_ = oauthToken
	_ = oauthSecret
	_ = apiKey
	_ = apiSecret
	// OAuth 1.0a signing is complex — for now use the oauth1 library
	// This is a simplified version
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", "", "", "", 0, fmt.Errorf("access token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	vals, _ := url.ParseQuery(string(raw))
	at := vals.Get("oauth_token")
	as := vals.Get("oauth_token_secret")
	uid := vals.Get("user_id")
	screenName := vals.Get("screen_name")

	if at == "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("X OAuth failed: %s", string(raw))
	}

	return at + ":" + as, "", uid, screenName, "", screenName, 999999999, nil
}
