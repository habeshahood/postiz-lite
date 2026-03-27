package social

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/tenant"
)

// RefreshResult holds the new tokens after a refresh.
type RefreshResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // seconds
}

// RefreshToken refreshes the OAuth token for the given integration.
// Returns nil if the provider doesn't support refresh.
func RefreshToken(ctx context.Context, integration *db.Integration) (*RefreshResult, error) {
	switch integration.ProviderIdentifier {
	case "tiktok":
		return refreshTikTok(ctx, integration)
	case "facebook", "instagram":
		return refreshFacebook(ctx, integration)
	case "youtube":
		return refreshYouTube(ctx, integration)
	default:
		return nil, nil // X, Bluesky don't need refresh
	}
}

func refreshTikTok(ctx context.Context, integration *db.Integration) (*RefreshResult, error) {
	keys := tenant.GetSocialKeys(ctx)
	if keys == nil || keys.TikTokClientID == "" {
		return nil, fmt.Errorf("tiktok: no client credentials in tenant context")
	}

	if integration.RefreshToken == nil || *integration.RefreshToken == "" {
		return nil, fmt.Errorf("tiktok: no refresh token")
	}

	form := url.Values{
		"client_key":    {keys.TikTokClientID},
		"client_secret": {keys.TikTokSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {*integration.RefreshToken},
	}

	resp, err := http.Post("https://open.tiktokapis.com/v2/oauth/token/",
		"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("tiktok refresh: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("tiktok refresh decode: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("tiktok refresh: %s: %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("tiktok refresh: empty access token in response")
	}

	return &RefreshResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
	}, nil
}

func refreshFacebook(ctx context.Context, integration *db.Integration) (*RefreshResult, error) {
	keys := tenant.GetSocialKeys(ctx)
	if keys == nil || keys.FacebookAppID == "" {
		return nil, fmt.Errorf("facebook: no app credentials in tenant context")
	}

	// Facebook long-lived token exchange using the current access token
	u := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		keys.FacebookAppID, keys.FacebookSecret, integration.Token,
	)

	resp, err := http.Get(u)
	if err != nil {
		return nil, fmt.Errorf("facebook refresh: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("facebook refresh decode: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("facebook refresh: empty access token")
	}

	return &RefreshResult{
		AccessToken: result.AccessToken,
		ExpiresIn:   result.ExpiresIn,
	}, nil
}

func refreshYouTube(ctx context.Context, integration *db.Integration) (*RefreshResult, error) {
	keys := tenant.GetSocialKeys(ctx)
	if keys == nil || keys.YouTubeClientID == "" {
		return nil, fmt.Errorf("youtube: no client credentials in tenant context")
	}

	if integration.RefreshToken == nil || *integration.RefreshToken == "" {
		return nil, fmt.Errorf("youtube: no refresh token")
	}

	form := url.Values{
		"client_id":     {keys.YouTubeClientID},
		"client_secret": {keys.YouTubeSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {*integration.RefreshToken},
	}

	resp, err := http.Post("https://oauth2.googleapis.com/token",
		"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("youtube refresh: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("youtube refresh decode: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("youtube refresh: %s: %s", result.Error, result.ErrorDesc)
	}

	return &RefreshResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken, // may be empty; Google reuses the old refresh token
		ExpiresIn:    result.ExpiresIn,
	}, nil
}
