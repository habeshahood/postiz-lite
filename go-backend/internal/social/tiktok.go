package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/habeshahood/postiz-lite/internal/db"
)

func init() {
	Register(&tiktokProvider{})
}

type tiktokProvider struct{}

func (t *tiktokProvider) Identifier() string { return "tiktok" }

// tiktokPostSettings mirrors the JSON stored in post.Settings.
// Only the fields we need are decoded; the rest are ignored.
type tiktokPostSettings struct {
	Media []struct {
		URL  string `json:"url"`
		Type string `json:"type"` // "video", "image", etc.
	} `json:"media"`
}

// tiktokInitRequest is the body sent to the video/init endpoint.
type tiktokInitRequest struct {
	PostInfo   tiktokPostInfo   `json:"post_info"`
	SourceInfo tiktokSourceInfo `json:"source_info"`
}

type tiktokPostInfo struct {
	Title        string `json:"title"`
	PrivacyLevel string `json:"privacy_level"`
}

type tiktokSourceInfo struct {
	Source   string `json:"source"`
	VideoURL string `json:"video_url"`
}

// tiktokInitResponse is the envelope returned by the video/init endpoint.
type tiktokInitResponse struct {
	Data struct {
		PublishID string `json:"publish_id"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (t *tiktokProvider) Post(ctx context.Context, post *db.Post, integration *db.Integration) (*PostResult, error) {
	videoURL, err := t.resolveVideoURL(post)
	if err != nil {
		return nil, err
	}

	title := post.Content
	if post.Title != nil && *post.Title != "" {
		title = *post.Title
	}

	publishID, err := t.initVideoUpload(ctx, integration.Token, title, videoURL)
	if err != nil {
		return nil, err
	}

	username := ""
	if integration.Profile != nil {
		username = *integration.Profile
	}

	releaseURL := fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", username, publishID)

	slog.InfoContext(ctx, "tiktok: post published",
		"publish_id", publishID,
		"release_url", releaseURL,
	)

	return &PostResult{
		PostID:     publishID,
		ReleaseURL: releaseURL,
	}, nil
}

// resolveVideoURL finds the first video URL from the post's Settings or Image field.
func (t *tiktokProvider) resolveVideoURL(post *db.Post) (string, error) {
	if post.Settings != nil && *post.Settings != "" {
		var settings tiktokPostSettings
		if err := json.Unmarshal([]byte(*post.Settings), &settings); err == nil {
			for _, m := range settings.Media {
				if strings.HasPrefix(m.Type, "video") {
					return m.URL, nil
				}
			}
		}
	}

	// Fallback: treat post.Image as a video URL if it looks like a video path.
	if post.Image != nil && *post.Image != "" {
		img := *post.Image
		lower := strings.ToLower(img)
		if strings.Contains(lower, ".mp4") ||
			strings.Contains(lower, ".mov") ||
			strings.Contains(lower, ".webm") {
			return img, nil
		}
	}

	return "", fmt.Errorf("tiktok: video content is required — no video media found on this post")
}

// initVideoUpload calls POST /v2/post/publish/video/init/ and returns the publish_id.
func (t *tiktokProvider) initVideoUpload(ctx context.Context, token, title, videoURL string) (string, error) {
	const endpoint = "https://open.tiktokapis.com/v2/post/publish/video/init/"

	reqBody := tiktokInitRequest{
		PostInfo: tiktokPostInfo{
			Title:        title,
			PrivacyLevel: "SELF_ONLY",
		},
		SourceInfo: tiktokSourceInfo{
			Source:   "PULL_FROM_URL",
			VideoURL: videoURL,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("tiktok: marshal init request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("tiktok: create init request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tiktok: init request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("tiktok: read init response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tiktok: init endpoint returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var initResp tiktokInitResponse
	if err := json.Unmarshal(raw, &initResp); err != nil {
		return "", fmt.Errorf("tiktok: decode init response: %w", err)
	}

	if initResp.Error.Code != "" && initResp.Error.Code != "ok" {
		return "", fmt.Errorf("tiktok: API error %s: %s", initResp.Error.Code, initResp.Error.Message)
	}

	if initResp.Data.PublishID == "" {
		return "", fmt.Errorf("tiktok: API returned empty publish_id (raw: %s)", string(raw))
	}

	return initResp.Data.PublishID, nil
}
