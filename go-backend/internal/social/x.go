package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/dghubble/oauth1"
	"github.com/habeshahood/postiz-lite/internal/db"
)

func init() {
	Register(&xProvider{})
}

type xProvider struct{}

func (x *xProvider) Identifier() string { return "x" }

func (x *xProvider) Post(ctx context.Context, post *db.Post, integration *db.Integration) (*PostResult, error) {
	apiKey := os.Getenv("X_API_KEY")
	apiSecret := os.Getenv("X_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("x: X_API_KEY and X_API_SECRET must be set")
	}

	parts := strings.SplitN(integration.Token, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("x: invalid token format, expected accessToken:accessSecret")
	}
	accessToken, accessSecret := parts[0], parts[1]

	cfg := oauth1.NewConfig(apiKey, apiSecret)
	token := oauth1.NewToken(accessToken, accessSecret)
	httpClient := cfg.Client(ctx, token)

	tweetID, err := postTweet(ctx, httpClient, post.Content)
	if err != nil {
		return nil, fmt.Errorf("x: post tweet: %w", err)
	}

	username, err := getUsername(ctx, httpClient)
	if err != nil {
		// Non-fatal: return a partial result with a fallback URL.
		slog.WarnContext(ctx, "x: could not fetch username, using tweet ID as fallback", "err", err)
		username = "i"
	}

	releaseURL := fmt.Sprintf("https://twitter.com/%s/status/%s", username, tweetID)
	return &PostResult{PostID: tweetID, ReleaseURL: releaseURL}, nil
}

// postTweet calls POST https://api.twitter.com/2/tweets and returns the tweet ID.
func postTweet(ctx context.Context, client *http.Client, content string) (string, error) {
	body, err := json.Marshal(map[string]string{"text": content})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitter.com/2/tweets", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("twitter API returned %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Data.ID == "" {
		return "", fmt.Errorf("twitter API returned empty tweet ID")
	}
	return result.Data.ID, nil
}

// getUsername calls GET https://api.twitter.com/2/users/me and returns the username.
func getUsername(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitter.com/2/users/me?user.fields=username", nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitter API returned %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Data.Username == "" {
		return "", fmt.Errorf("twitter API returned empty username")
	}
	return result.Data.Username, nil
}
