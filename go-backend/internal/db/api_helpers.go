package db

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/lucsky/cuid"
)

// CreatePostFromAPI creates a Post from the raw JSON body sent by the Public API.
// Expected fields in body:
//
//	integrationId  string  (required)
//	content        string  (required)
//	publishDate    string  (required, RFC3339)
//	title          string  (optional)
//	settings       string  (optional, JSON blob)
//	image          string  (optional)
//	parentPostId   string  (optional)
func (s *Store) CreatePostFromAPI(ctx context.Context, orgID string, body map[string]interface{}) (*Post, error) {
	integrationID, _ := body["integrationId"].(string)
	content, _ := body["content"].(string)
	publishDateStr, _ := body["publishDate"].(string)

	if integrationID == "" {
		return nil, fmt.Errorf("CreatePostFromAPI: integrationId is required")
	}
	if content == "" {
		return nil, fmt.Errorf("CreatePostFromAPI: content is required")
	}
	if publishDateStr == "" {
		return nil, fmt.Errorf("CreatePostFromAPI: publishDate is required")
	}

	publishDate, err := time.Parse(time.RFC3339, publishDateStr)
	if err != nil {
		return nil, fmt.Errorf("CreatePostFromAPI: invalid publishDate %q: %w", publishDateStr, err)
	}

	var title, settings, image, parentPostID *string
	if v, ok := body["title"].(string); ok && v != "" {
		title = &v
	}
	if v, ok := body["settings"].(string); ok && v != "" {
		settings = &v
	}
	if v, ok := body["image"].(string); ok && v != "" {
		image = &v
	}
	if v, ok := body["parentPostId"].(string); ok && v != "" {
		parentPostID = &v
	}

	// Each logical post group gets a shared group ID (cuid). For single posts
	// the group is the post's own id (set after generation).
	groupID := cuid.New()

	return s.CreatePost(ctx, orgID, integrationID, content, groupID, publishDate, title, settings, image, parentPostID)
}

// SaveMediaFromUpload streams a multipart file to the configured upload
// directory (UPLOAD_DIR env var, default "./uploads"), then persists a Media
// row. The caller is responsible for closing src.
func (s *Store) SaveMediaFromUpload(ctx context.Context, orgID, filename string, src io.Reader) (*Media, error) {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("SaveMediaFromUpload: mkdir %s: %w", uploadDir, err)
	}

	// Build a collision-safe stored name: uuid + original extension
	ext := filepath.Ext(filename)
	storedName := uuid.New().String() + ext
	storedPath := filepath.Join(uploadDir, storedName)

	f, err := os.Create(storedPath)
	if err != nil {
		return nil, fmt.Errorf("SaveMediaFromUpload: create file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, src)
	if err != nil {
		os.Remove(storedPath)
		return nil, fmt.Errorf("SaveMediaFromUpload: write file: %w", err)
	}

	// Infer media type from extension
	mediaType := "image"
	if mt := mime.TypeByExtension(ext); mt != "" {
		if len(mt) >= 5 && mt[:5] == "video" {
			mediaType = "video"
		}
	}

	orig := filename
	return s.SaveMedia(ctx, orgID, storedName, storedPath, mediaType, &orig, int(written))
}
