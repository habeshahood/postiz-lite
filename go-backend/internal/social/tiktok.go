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

func (t *tiktokProvider) Post(ctx context.Context, post *db.Post, integration *db.Integration) (*PostResult, error) {
	videoURL, err := t.resolveVideoURL(post)
	if err != nil {
		return nil, err
	}

	title := post.Content
	if post.Title != nil && *post.Title != "" {
		title = *post.Title
	}

	// Step 1: Download the video
	slog.InfoContext(ctx, "tiktok: downloading video", "url", videoURL)
	videoResp, err := http.Get(videoURL)
	if err != nil {
		return nil, fmt.Errorf("tiktok: download video: %w", err)
	}
	defer videoResp.Body.Close()
	videoBytes, err := io.ReadAll(videoResp.Body)
	if err != nil {
		return nil, fmt.Errorf("tiktok: read video: %w", err)
	}
	videoSize := len(videoBytes)
	slog.InfoContext(ctx, "tiktok: video downloaded", "size", videoSize)

	// Step 2: Init upload with FILE_UPLOAD source (DIRECT_POST method)
	uploadURL, publishID, err := t.initDirectPost(ctx, integration.Token, title, int64(videoSize))
	if err != nil {
		return nil, err
	}

	// Step 3: Upload the video to TikTok's upload URL
	if err := t.uploadChunk(ctx, uploadURL, videoBytes); err != nil {
		return nil, err
	}

	// Step 4: Poll for completion
	finalURL, err := t.pollPublishStatus(ctx, integration.Token, publishID, integration.Profile)
	if err != nil {
		slog.WarnContext(ctx, "tiktok: poll failed, using fallback URL", "error", err)
		username := ""
		if integration.Profile != nil {
			username = *integration.Profile
		}
		finalURL = fmt.Sprintf("https://www.tiktok.com/@%s", username)
	}

	slog.InfoContext(ctx, "tiktok: post published", "publish_id", publishID, "release_url", finalURL)

	return &PostResult{
		PostID:     publishID,
		ReleaseURL: finalURL,
	}, nil
}

// resolveVideoURL finds the first video URL from the post's Settings or Image field.
func (t *tiktokProvider) resolveVideoURL(post *db.Post) (string, error) {
	// Check settings.media array
	if post.Settings != nil && *post.Settings != "" {
		var settings struct {
			Media []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"media"`
		}
		if err := json.Unmarshal([]byte(*post.Settings), &settings); err == nil {
			for _, m := range settings.Media {
				if strings.Contains(m.Path, "mp4") || strings.Contains(m.Path, "mov") || strings.HasPrefix(m.Type, "video") {
					return m.Path, nil
				}
			}
		}
		// Settings might itself be a JSON array of media
		if strings.HasPrefix(strings.TrimSpace(*post.Settings), "[") {
			var mediaList []struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(*post.Settings), &mediaList); err == nil {
				for _, m := range mediaList {
					if strings.Contains(strings.ToLower(m.Path), "mp4") || strings.Contains(strings.ToLower(m.Path), "mov") {
						return m.Path, nil
					}
				}
			}
		}
	}

	if post.Image != nil && *post.Image != "" {
		img := *post.Image
		// Image can be a JSON array of media objects: [{"id":"...","path":"https://...mp4"}]
		if strings.HasPrefix(strings.TrimSpace(img), "[") {
			var mediaList []struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(img), &mediaList); err == nil {
				for _, m := range mediaList {
					if strings.Contains(strings.ToLower(m.Path), "mp4") || strings.Contains(strings.ToLower(m.Path), "mov") {
						return m.Path, nil
					}
				}
			}
		}
		// Plain URL
		lower := strings.ToLower(img)
		if strings.Contains(lower, ".mp4") || strings.Contains(lower, ".mov") || strings.Contains(lower, ".webm") {
			return img, nil
		}
	}

	return "", fmt.Errorf("tiktok: video content required — no video found on post")
}

// initDirectPost uses the DIRECT_POST / FILE_UPLOAD method.
// Returns the upload URL and publish_id.
func (t *tiktokProvider) initDirectPost(ctx context.Context, token, title string, videoSize int64) (uploadURL, publishID string, err error) {
	// Use /inbox/video/init/ — works for unaudited apps (sends to creator inbox)
	const endpoint = "https://open.tiktokapis.com/v2/post/publish/inbox/video/init/"

	reqBody := map[string]any{
		"post_info": map[string]any{
			"title":                  title,
			"privacy_level":          "SELF_ONLY",
			"disable_duet":           false,
			"disable_comment":        false,
			"disable_stitch":         false,
			"brand_content_toggle":   false,
			"brand_organic_toggle":   false,
			"is_aigc":               false,
		},
		"source_info": map[string]any{
			"source":     "FILE_UPLOAD",
			"video_size": videoSize,
			"chunk_size": videoSize, // single chunk
			"total_chunk_count": 1,
		},
	}

	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("tiktok: init request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("tiktok: init returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var initResp struct {
		Data struct {
			PublishID string `json:"publish_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(raw, &initResp)

	if initResp.Error.Code != "" && initResp.Error.Code != "ok" {
		return "", "", fmt.Errorf("tiktok: %s: %s", initResp.Error.Code, initResp.Error.Message)
	}
	if initResp.Data.UploadURL == "" {
		return "", "", fmt.Errorf("tiktok: empty upload_url (raw: %s)", string(raw))
	}

	return initResp.Data.UploadURL, initResp.Data.PublishID, nil
}

// uploadChunk uploads the video bytes to TikTok's upload URL.
func (t *tiktokProvider) uploadChunk(ctx context.Context, uploadURL string, videoBytes []byte) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(videoBytes))
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(videoBytes)-1, len(videoBytes)))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tiktok: upload chunk: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tiktok: upload returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	return nil
}

// pollPublishStatus checks the publish status until complete.
func (t *tiktokProvider) pollPublishStatus(ctx context.Context, token, publishID string, profile *string) (string, error) {
	const endpoint = "https://open.tiktokapis.com/v2/post/publish/status/fetch/"

	body, _ := json.Marshal(map[string]string{"publish_id": publishID})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tiktok: status fetch: %w", err)
	}
	defer resp.Body.Close()

	var status struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&status)

	username := ""
	if profile != nil {
		username = *profile
	}
	return fmt.Sprintf("https://www.tiktok.com/@%s", username), nil
}
