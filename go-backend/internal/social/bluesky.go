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
	"time"

	"github.com/habeshahood/postiz-lite/internal/db"
)

func init() {
	Register(&blueskyProvider{})
}

type blueskyProvider struct{}

func (b *blueskyProvider) Identifier() string { return "bluesky" }

// blueskyInstanceDetails is the shape stored in integration.CustomInstanceDetails.
type blueskyInstanceDetails struct {
	Service    string `json:"service"`
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// blueskySessionResponse is the response from com.atproto.server.createSession.
type blueskySessionResponse struct {
	AccessJwt  string `json:"accessJwt"`
	RefreshJwt string `json:"refreshJwt"`
	Handle     string `json:"handle"`
	DID        string `json:"did"`
}

// blueskyCreateRecordResponse is the response from com.atproto.repo.createRecord.
type blueskyCreateRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

func (b *blueskyProvider) Post(ctx context.Context, post *db.Post, integration *db.Integration) (*PostResult, error) {
	const defaultService = "https://bsky.social"

	var (
		serviceURL string
		accessJWT  string
		did        string
	)

	did = integration.InternalID

	// Try to parse CustomInstanceDetails as plain JSON to get credentials.
	// If available, perform a fresh login to obtain an access JWT.
	if integration.CustomInstanceDetails != nil && *integration.CustomInstanceDetails != "" {
		var details blueskyInstanceDetails
		if err := json.Unmarshal([]byte(*integration.CustomInstanceDetails), &details); err != nil {
			slog.WarnContext(ctx, "bluesky: failed to parse CustomInstanceDetails as JSON, falling back to Token",
				"integration_id", integration.ID,
				"error", err,
			)
		} else {
			serviceURL = details.Service
			if serviceURL == "" {
				serviceURL = defaultService
			}

			jwt, sessionDID, err := blueskyCreateSession(ctx, serviceURL, details.Identifier, details.Password)
			if err != nil {
				return nil, fmt.Errorf("bluesky: createSession failed: %w", err)
			}
			accessJWT = jwt
			// Prefer the DID returned by the session over the stored InternalID.
			if sessionDID != "" {
				did = sessionDID
			}
		}
	}

	// Fall back to Token as a pre-obtained access JWT (no login step needed).
	if accessJWT == "" {
		accessJWT = integration.Token
		serviceURL = defaultService
		slog.InfoContext(ctx, "bluesky: using Token as access JWT", "integration_id", integration.ID)
	}

	uri, err := blueskyCreateRecord(ctx, serviceURL, accessJWT, did, post.Content)
	if err != nil {
		return nil, fmt.Errorf("bluesky: createRecord failed: %w", err)
	}

	// uri format: at://did:plc:.../app.bsky.feed.post/<rkey>
	rkey := atURIRkey(uri)
	releaseURL := fmt.Sprintf("https://bsky.app/profile/%s/post/%s", did, rkey)

	slog.InfoContext(ctx, "bluesky: post published",
		"integration_id", integration.ID,
		"uri", uri,
		"release_url", releaseURL,
	)

	return &PostResult{
		PostID:     rkey,
		ReleaseURL: releaseURL,
	}, nil
}

// blueskyCreateSession calls com.atproto.server.createSession and returns
// the accessJwt and the user's DID.
func blueskyCreateSession(ctx context.Context, serviceURL, identifier, password string) (accessJWT string, did string, err error) {
	body, err := json.Marshal(map[string]string{
		"identifier": identifier,
		"password":   password,
	})
	if err != nil {
		return "", "", fmt.Errorf("marshal session request: %w", err)
	}

	url := strings.TrimRight(serviceURL, "/") + "/xrpc/com.atproto.server.createSession"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("createSession HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var session blueskySessionResponse
	if err := json.Unmarshal(raw, &session); err != nil {
		return "", "", fmt.Errorf("unmarshal session response: %w", err)
	}

	return session.AccessJwt, session.DID, nil
}

// blueskyCreateRecord calls com.atproto.repo.createRecord and returns the AT URI.
func blueskyCreateRecord(ctx context.Context, serviceURL, accessJWT, did, text string) (string, error) {
	record := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(map[string]any{
		"repo":       did,
		"collection": "app.bsky.feed.post",
		"record":     record,
	})
	if err != nil {
		return "", fmt.Errorf("marshal createRecord request: %w", err)
	}

	url := strings.TrimRight(serviceURL, "/") + "/xrpc/com.atproto.repo.createRecord"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessJWT)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("createRecord HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result blueskyCreateRecordResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("unmarshal createRecord response: %w", err)
	}

	if result.URI == "" {
		return "", fmt.Errorf("createRecord response missing uri field")
	}

	return result.URI, nil
}

// atURIRkey extracts the rkey (final path segment) from an AT URI.
// e.g. "at://did:plc:abc/app.bsky.feed.post/3jwdwj2ctlk26" → "3jwdwj2ctlk26"
func atURIRkey(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) == 0 {
		return uri
	}
	return parts[len(parts)-1]
}
