package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

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
		case "instagram":
			authURL, codeVerifier, state, err = generateInstagramAuthURL(frontendURL, socialKeys)
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

// ── X / Twitter (OAuth 2.0 PKCE) ────────────────────────────

func generateXAuthURL(frontendURL string, keys *tenant.SocialKeys) (authURL, codeVerifier, state string, err error) {
	var clientID string
	if keys != nil {
		clientID = keys.XAPIKey
	}
	if clientID == "" {
		return "", "", "", fmt.Errorf("X_API_KEY (OAuth 2.0 Client ID) must be set")
	}

	state = randomState()
	codeVerifier = randomState() + randomState() // PKCE needs 43-128 chars

	// S256 code challenge
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	redirectURI := frontendURL + "/integrations/social/x"
	scopes := "tweet.read tweet.write users.read offline.access"

	u, _ := url.Parse("https://twitter.com/i/oauth2/authorize")
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scopes)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()

	return u.String(), codeVerifier, state, nil
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

	scopes := "pages_show_list,pages_manage_posts"

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

// ── Instagram (via Facebook OAuth 2.0) ──────────────────────

func generateInstagramAuthURL(frontendURL string, keys *tenant.SocialKeys) (authURL, codeVerifier, state string, err error) {
	var appID string
	if keys != nil {
		appID = keys.FacebookAppID
	}
	if appID == "" {
		return "", "", "", fmt.Errorf("FACEBOOK_APP_ID must be set for Instagram")
	}

	state = randomState()
	codeVerifier = randomState()
	redirectURI := frontendURL + "/integrations/social/instagram"

	scopes := "pages_show_list,instagram_basic,instagram_content_publish"

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
