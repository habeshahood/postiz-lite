package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/tenant"
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

		socialKeys := tenant.GetSocialKeys(r.Context())
		frontendURL := tenant.GetFrontendURL(r.Context())

		var accessToken, refreshToken, userID, userName, picture, username string
		var expiresIn int

		switch integration {
		case "youtube":
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeYouTubeToken(body.Code, socialKeys, frontendURL)
		case "facebook", "instagram":
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeFacebookToken(body.Code, body.Refresh, socialKeys, frontendURL+"/integrations/social/"+integration)
			if err == nil {
				// Fetch the user's Pages and use the first page's token for posting
				pageToken, pageID, pageName, pagePic := fetchFacebookPage(accessToken)
				if pageToken != "" {
					accessToken = pageToken
					userID = pageID
					userName = pageName
					if pagePic != "" {
						picture = pagePic
					}
				}
				// Use a distinct internalId for Instagram vs Facebook (same page, different provider)
				if integration == "instagram" {
					userID = "ig_" + userID
				}
			}
		case "tiktok":
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeTikTokToken(body.Code, body.CodeVerifier, socialKeys, frontendURL)
		case "x":
			codeVerifier, _ := rdb.Get(ctx, "login:"+body.State).Result()
			accessToken, refreshToken, userID, userName, picture, username, expiresIn, err = exchangeXToken(body.Code, codeVerifier, socialKeys)
		case "telegram":
			// Telegram: use the shared bot token from tenant config
			var botToken string
			if socialKeys != nil {
				botToken = socialKeys.TelegramBotToken
			}
			if botToken == "" {
				http.Error(w, `{"msg":"Telegram bot not configured"}`, http.StatusBadRequest)
				return
			}
			accessToken = botToken
			// Get bot info from Telegram API
			botResp, botErr := http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getMe", botToken))
			if botErr == nil {
				defer botResp.Body.Close()
				var botInfo struct {
					OK     bool `json:"ok"`
					Result struct {
						ID        int64  `json:"id"`
						FirstName string `json:"first_name"`
						Username  string `json:"username"`
					} `json:"result"`
				}
				json.NewDecoder(botResp.Body).Decode(&botInfo)
				if botInfo.OK {
					userID = fmt.Sprintf("tg_%d", botInfo.Result.ID)
					userName = botInfo.Result.FirstName
					username = botInfo.Result.Username
				}
			}
			if userID == "" {
				userID = "tg_" + randomState()[:8]
				userName = "Telegram"
			}
			expiresIn = 999999999
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
			if len(userID) >= 8 {
				userName = "Channel_" + userID[:8]
			} else {
				userName = "Channel_" + userID
			}
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

func exchangeYouTubeToken(code string, keys *tenant.SocialKeys, frontendURL string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	var clientID, clientSecret string
	if keys != nil {
		clientID = keys.YouTubeClientID
		clientSecret = keys.YouTubeSecret
	}
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

func exchangeFacebookToken(code, refresh string, keys *tenant.SocialKeys, redirectURI string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	var appID, appSecret string
	if keys != nil {
		appID = keys.FacebookAppID
		appSecret = keys.FacebookSecret
	}
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

// fetchFacebookPage gets the first Page the user manages and returns its page token.
func fetchFacebookPage(userToken string) (pageToken, pageID, pageName, pagePicture string) {
	resp, err := http.Get(fmt.Sprintf("https://graph.facebook.com/v20.0/me/accounts?fields=id,name,access_token,picture&access_token=%s", userToken))
	if err != nil {
		return "", "", "", ""
	}
	defer resp.Body.Close()
	var pages struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			AccessToken string `json:"access_token"`
			Picture     struct{ Data struct{ URL string } } `json:"picture"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&pages)
	if len(pages.Data) == 0 {
		return "", "", "", ""
	}
	p := pages.Data[0]
	return p.AccessToken, p.ID, p.Name, p.Picture.Data.URL
}

// ── TikTok token exchange ────────────────────────────────────

func exchangeTikTokToken(code, codeVerifier string, keys *tenant.SocialKeys, frontendURL string) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	var clientID, clientSecret string
	if keys != nil {
		clientID = keys.TikTokClientID
		clientSecret = keys.TikTokSecret
	}
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

	// Get user info (only request fields available in user.info.basic scope)
	req2, _ := http.NewRequest("GET", "https://open.tiktokapis.com/v2/user/info/?fields=open_id,avatar_url,display_name", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	resp2, err2 := http.DefaultClient.Do(req2)
	if err2 != nil {
		slog.Error("TikTok user info request failed", "error", err2)
		// Still return the token — we just won't have user details
		return tokenResp.AccessToken, tokenResp.RefreshToken, "tiktok_user", "TikTok User", "", "", 82800, nil
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	slog.Info("TikTok user info response", "body", string(body2[:min(len(body2), 500)]))

	var userResp struct {
		Data struct {
			User struct {
				OpenID      string `json:"open_id"`
				AvatarURL   string `json:"avatar_url"`
				DisplayName string `json:"display_name"`
			} `json:"user"`
		} `json:"data"`
	}
	json.Unmarshal(body2, &userResp)

	uid := userResp.Data.User.OpenID
	if uid == "" {
		uid = "tiktok_" + randomState()[:8]
	}
	displayName := userResp.Data.User.DisplayName
	if displayName == "" {
		displayName = "TikTok"
	}
	return tokenResp.AccessToken, tokenResp.RefreshToken, uid, displayName, userResp.Data.User.AvatarURL, "", 82800, nil
}

// ── X / Twitter token exchange (OAuth 1.0a) ──────────────────

func exchangeXToken(code, codeVerifier string, keys *tenant.SocialKeys) (accessToken, refreshToken, userID, userName, picture, username string, expiresIn int, err error) {
	if codeVerifier == "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("missing code verifier (OAuth state expired)")
	}

	var clientID, clientSecret string
	if keys != nil {
		clientID = keys.XAPIKey
		clientSecret = keys.XAPISecret
	}

	// OAuth 2.0 PKCE token exchange
	formData := url.Values{
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {"https://postiz.1h00d.com/integrations/social/x"},
		"code_verifier": {codeVerifier},
	}

	req, _ := http.NewRequest("POST", "https://api.twitter.com/2/oauth2/token", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := http.DefaultClient.Do(req)
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
		return "", "", "", "", "", "", 0, fmt.Errorf("X OAuth2: %s — %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", "", "", "", "", "", 0, fmt.Errorf("X OAuth2 empty token: %s", string(raw[:min(len(raw), 200)]))
	}

	// Get user info
	req2, _ := http.NewRequest("GET", "https://api.twitter.com/2/users/me?user.fields=profile_image_url,name,username", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	resp2, err2 := http.DefaultClient.Do(req2)
	if err2 != nil {
		slog.Error("X user info failed", "error", err2)
		return tokenResp.AccessToken, tokenResp.RefreshToken, "x_user", "X User", "", "", tokenResp.ExpiresIn, nil
	}
	defer resp2.Body.Close()

	var userResp struct {
		Data struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Username        string `json:"username"`
			ProfileImageURL string `json:"profile_image_url"`
		} `json:"data"`
	}
	json.NewDecoder(resp2.Body).Decode(&userResp)

	return tokenResp.AccessToken, tokenResp.RefreshToken, userResp.Data.ID, userResp.Data.Name, userResp.Data.ProfileImageURL, userResp.Data.Username, tokenResp.ExpiresIn, nil
}
