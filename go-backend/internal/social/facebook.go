package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/habeshahood/postiz-lite/internal/db"
)

const (
	fbGraphBase = "https://graph.facebook.com/v20.0"
)

func init() {
	Register(&facebookProvider{})
}

type facebookProvider struct{}

func (f *facebookProvider) Identifier() string { return "facebook" }

// fbFeedRequest is the JSON body sent to /{page_id}/feed.
type fbFeedRequest struct {
	Message   string `json:"message"`
	Published bool   `json:"published"`
}

// fbFeedResponse is what the Graph API returns on success.
type fbFeedResponse struct {
	ID           string `json:"id"`
	PermalinkURL string `json:"permalink_url"`
}

// fbErrorDetail is the inner error object in Graph API error responses.
type fbErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

// fbErrorResponse wraps the Graph API error envelope.
type fbErrorResponse struct {
	Error fbErrorDetail `json:"error"`
}

func (f *facebookProvider) Post(
	ctx context.Context,
	post *db.Post,
	integration *db.Integration,
) (*PostResult, error) {
	pageID := integration.InternalID
	token := integration.Token

	endpoint := fmt.Sprintf("%s/%s/feed", fbGraphBase, url.PathEscape(pageID))

	body, err := json.Marshal(fbFeedRequest{
		Message:   post.Content,
		Published: true,
	})
	if err != nil {
		return nil, fmt.Errorf("facebook: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint+"?access_token="+url.QueryEscape(token),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("facebook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("facebook: http request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("facebook: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr fbErrorResponse
		if jsonErr := json.Unmarshal(raw, &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf(
				"facebook: API error (code %d, type %s): %s",
				apiErr.Error.Code,
				apiErr.Error.Type,
				apiErr.Error.Message,
			)
		}
		return nil, fmt.Errorf("facebook: unexpected status %d: %s", resp.StatusCode, string(raw))
	}

	var result fbFeedResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("facebook: decode response: %w", err)
	}

	releaseURL := result.PermalinkURL
	if releaseURL == "" {
		// Fallback: construct URL from the returned composite ID (page_post_id).
		// The Graph API returns IDs in the form "<pageID>_<postID>".
		postID := result.ID
		if idx := strings.Index(postID, "_"); idx >= 0 {
			postID = postID[idx+1:]
		}
		releaseURL = fmt.Sprintf("https://www.facebook.com/%s/posts/%s", pageID, postID)
	}

	slog.InfoContext(ctx, "facebook: post published",
		"post_id", result.ID,
		"page_id", pageID,
		"release_url", releaseURL,
	)

	return &PostResult{
		PostID:     result.ID,
		ReleaseURL: releaseURL,
	}, nil
}
