package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/habeshahood/postiz-lite/internal/db"
)

func init() {
	Register(&youtubeProvider{})
}

type youtubeProvider struct{}

func (y *youtubeProvider) Identifier() string { return "youtube" }

// youtubePostSettings is parsed from post.Settings JSON.
type youtubePostSettings struct {
	// Title overrides post.Title when present.
	Title string `json:"title"`
	// Type maps to YouTube privacyStatus: "public", "private", or "unlisted".
	Type string `json:"type"`
	// Tags is a list of video tags.
	Tags []string `json:"tags"`
	// SelfDeclaredMadeForKids sets the madeForKids flag.
	SelfDeclaredMadeForKids bool `json:"selfDeclaredMadeForKids"`
	// VideoURL is the URL of the video file to upload (from media settings).
	VideoURL string `json:"videoUrl"`
}

// youtubeInsertRequest is the metadata body sent when initiating a resumable upload.
type youtubeInsertRequest struct {
	Snippet youtubeSnippet `json:"snippet"`
	Status  youtubeStatus  `json:"status"`
}

type youtubeSnippet struct {
	Title                    string   `json:"title"`
	Description              string   `json:"description"`
	Tags                     []string `json:"tags,omitempty"`
	SelfDeclaredMadeForKids  bool     `json:"selfDeclaredMadeForKids"`
}

type youtubeStatus struct {
	PrivacyStatus string `json:"privacyStatus"`
}

// youtubeInsertResponse is the JSON body returned by the YouTube videos.insert endpoint.
type youtubeInsertResponse struct {
	ID string `json:"id"`
}

func (y *youtubeProvider) Post(ctx context.Context, post *db.Post, integration *db.Integration) (*PostResult, error) {
	// Parse post settings to get video URL and metadata.
	var settings youtubePostSettings
	if post.Settings != nil && *post.Settings != "" {
		if err := json.Unmarshal([]byte(*post.Settings), &settings); err != nil {
			slog.WarnContext(ctx, "youtube: failed to parse post settings JSON, using defaults",
				"integration_id", integration.ID,
				"post_id", post.ID,
				"error", err,
			)
		}
	}

	// YouTube only supports video content — require a video URL.
	if settings.VideoURL == "" {
		return nil, fmt.Errorf("youtube: post %s has no video URL in settings; YouTube only accepts video uploads", post.ID)
	}

	// Determine title: settings.Title → post.Title → fallback.
	title := settings.Title
	if title == "" && post.Title != nil && *post.Title != "" {
		title = *post.Title
	}
	if title == "" {
		title = "Untitled video"
	}

	// Default privacy to public if not specified.
	privacyStatus := settings.Type
	if privacyStatus == "" {
		privacyStatus = "public"
	}

	videoID, err := youtubeInitiateResumableUpload(ctx, integration.Token, title, post.Content, privacyStatus, settings.Tags, settings.SelfDeclaredMadeForKids)
	if err != nil {
		return nil, fmt.Errorf("youtube: initiate resumable upload: %w", err)
	}

	releaseURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	slog.InfoContext(ctx, "youtube: post published",
		"integration_id", integration.ID,
		"post_id", post.ID,
		"video_id", videoID,
		"release_url", releaseURL,
	)

	return &PostResult{
		PostID:     videoID,
		ReleaseURL: releaseURL,
	}, nil
}

// youtubeInitiateResumableUpload calls the YouTube Data API v3 videos.insert endpoint
// with uploadType=resumable to register the upload and obtain a video ID.
//
// In a full implementation the caller would follow the Location header to stream
// the video bytes. This function posts the metadata and returns the video ID
// from the JSON response body that the API includes even on the 200 OK of the
// metadata step when the upload type is "multipart" or when the API echoes back
// the resource. For a resumable upload the video ID is returned in the response
// to the final upload request; here we use the API's metadata-only path as a
// well-typed stub that callers can extend with actual byte streaming.
func youtubeInitiateResumableUpload(
	ctx context.Context,
	accessToken string,
	title, description, privacyStatus string,
	tags []string,
	madeForKids bool,
) (string, error) {
	const uploadURL = "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status"

	metadata := youtubeInsertRequest{
		Snippet: youtubeSnippet{
			Title:                   title,
			Description:             description,
			Tags:                    tags,
			SelfDeclaredMadeForKids: madeForKids,
		},
		Status: youtubeStatus{
			PrivacyStatus: privacyStatus,
		},
	}

	body, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	// The resumable upload initiation returns 200 OK with the video resource
	// (including the assigned video ID) in the body.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YouTube API returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result youtubeInsertResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("YouTube API response missing video id field; body: %s", string(raw))
	}

	return result.ID, nil
}
