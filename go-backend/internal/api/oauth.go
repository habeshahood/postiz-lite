package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/dghubble/oauth1"
	"github.com/go-chi/chi/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/middleware"
	"github.com/habeshahood/postiz-lite/internal/tenant"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a Redis client from the REDIS_URL env var.
func NewRedisClient() *redis.Client {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("invalid REDIS_URL", "error", err)
		return nil
	}
	return redis.NewClient(opts)
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// handleSocialOAuthURLReal generates OAuth URLs for each platform, stores
// state in Redis, and returns the URL for the frontend to redirect to.
func handleSocialOAuthURLReal(store *db.Store, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		integration := chi.URLParam(r, "integration")
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		frontendURL := tenant.GetFrontendURL(r.Context())
		if frontendURL == "" {
			frontendURL = "https://postiz.1h00d.com"
		}

		refresh := r.URL.Query().Get("refresh")
		onboarding := r.URL.Query().Get("onboarding")

		var authURL, codeVerifier, state string
		var err error

		socialKeys := tenant.GetSocialKeys(r.Context())
		switch integration {
		case "x":
			authURL, codeVerifier, state, err = generateXAuthURL(frontendURL, socialKeys)
		case "bluesky":
			// Bluesky uses custom fields (identifier + password), no OAuth redirect
			state = randomState()
			codeVerifier = randomState()
			authURL = state // Frontend handles bluesky differently — shows a form
		case "tiktok":
			authURL, codeVerifier, state, err = generateTikTokAuthURL(frontendURL, socialKeys)
		case "youtube":
			authURL, codeVerifier, state, err = generateYouTubeAuthURL(frontendURL, socialKeys)
		case "facebook":
			authURL, codeVerifier, state, err = generateFacebookAuthURL(frontendURL, socialKeys)
		default:
			// For other platforms, return a placeholder
			state = randomState()
			codeVerifier = randomState()
			authURL = ""
			slog.Warn("OAuth not yet implemented for provider", "provider", integration)
		}

		if err != nil {
			slog.Error("failed to generate auth URL", "provider", integration, "error", err)
			writeJSON(w, http.StatusOK, map[string]any{"err": true})
			return
		}

		// Store state in Redis (same keys as NestJS backend)
		ctx := context.Background()
		ttl := time.Hour
		if rdb != nil && state != "" {
			rdb.Set(ctx, "organization:"+state, org.ID, ttl)
			rdb.Set(ctx, "login:"+state, codeVerifier, ttl)
			if refresh != "" {
				rdb.Set(ctx, "refresh:"+state, refresh, ttl)
			}
			if onboarding == "true" {
				rdb.Set(ctx, "onboarding:"+state, "true", ttl)
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":          authURL,
			"codeVerifier": codeVerifier,
			"state":        state,
		})
	}
}

// ── X / Twitter (OAuth 1.0a) ─────────────────────────────────

func generateXAuthURL(frontendURL string, keys *tenant.SocialKeys) (authURL, codeVerifier, state string, err error) {
	var apiKey, apiSecret string
	if keys != nil {
		apiKey = keys.XAPIKey
		apiSecret = keys.XAPISecret
	}
	if apiKey == "" || apiSecret == "" {
		return "", "", "", fmt.Errorf("X_API_KEY and X_API_SECRET must be set")
	}

	callbackURL := frontendURL + "/integrations/social/x"

	config := oauth1.Config{
		ConsumerKey:    apiKey,
		ConsumerSecret: apiSecret,
		CallbackURL:    callbackURL,
		Endpoint: oauth1.Endpoint{
			RequestTokenURL: "https://api.twitter.com/oauth/request_token",
			AuthorizeURL:    "https://api.twitter.com/oauth/authenticate",
			AccessTokenURL:  "https://api.twitter.com/oauth/access_token",
		},
	}

	requestToken, requestSecret, err := config.RequestToken()
	if err != nil {
		return "", "", "", fmt.Errorf("X request token: %w", err)
	}

	authorizationURL, err := config.AuthorizationURL(requestToken)
	if err != nil {
		return "", "", "", fmt.Errorf("X auth URL: %w", err)
	}

	return authorizationURL.String(), requestToken + ":" + requestSecret, requestToken, nil
}

// ── TikTok (OAuth 2.0 PKCE) ─────────────────────────────────

func generateTikTokAuthURL(frontendURL string, keys *tenant.SocialKeys) (authURL, codeVerifier, state string, err error) {
	var clientID string
	if keys != nil {
		clientID = keys.TikTokClientID
	}
	if clientID == "" {
		return "", "", "", fmt.Errorf("TIKTOK_CLIENT_ID must be set")
	}

	state = randomState()
	codeVerifier = randomState()
	redirectURI := frontendURL + "/integrations/social/tiktok"

	scopes := "user.info.basic,video.publish,video.upload,video.list"

	u, _ := url.Parse("https://www.tiktok.com/v2/auth/authorize/")
	q := u.Query()
	q.Set("client_key", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("scope", scopes)
	u.RawQuery = q.Encode()

	return u.String(), codeVerifier, state, nil
}

// ── YouTube (Google OAuth 2.0) ───────────────────────────────

func generateYouTubeAuthURL(frontendURL string, keys *tenant.SocialKeys) (authURL, codeVerifier, state string, err error) {
	var clientID string
	if keys != nil {
		clientID = keys.YouTubeClientID
	}
	if clientID == "" {
		return "", "", "", fmt.Errorf("YOUTUBE_CLIENT_ID must be set")
	}

	state = randomState()
	codeVerifier = randomState()
	redirectURI := frontendURL + "/integrations/social/youtube"

	scopes := "https://www.googleapis.com/auth/youtube https://www.googleapis.com/auth/youtube.upload https://www.googleapis.com/auth/userinfo.profile https://www.googleapis.com/auth/userinfo.email"

	u, _ := url.Parse("https://accounts.google.com/o/oauth2/v2/auth")
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("scope", scopes)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	u.RawQuery = q.Encode()

	return u.String(), codeVerifier, state, nil
}

// ── Facebook (OAuth 2.0) ─────────────────────────────────────

func generateFacebookAuthURL(frontendURL string, keys *tenant.SocialKeys) (authURL, codeVerifier, state string, err error) {
	var appID string
	if keys != nil {
		appID = keys.FacebookAppID
	}
	if appID == "" {
		return "", "", "", fmt.Errorf("FACEBOOK_APP_ID must be set")
	}

	state = randomState()
	codeVerifier = randomState()
	redirectURI := frontendURL + "/integrations/social/facebook"

	scopes := "pages_show_list,pages_read_engagement,pages_manage_posts,pages_manage_metadata,read_insights,business_management"

	u, _ := url.Parse("https://www.facebook.com/v20.0/dialog/oauth")
	q := u.Query()
	q.Set("client_id", appID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("scope", scopes)
	u.RawQuery = q.Encode()

	return u.String(), codeVerifier, state, nil
}
