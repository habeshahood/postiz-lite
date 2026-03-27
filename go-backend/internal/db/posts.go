package db

import (
	"context"
	"fmt"
	"time"

	"github.com/lucsky/cuid"
)

const postCols = `id, state, "publishDate", "organizationId", "integrationId", content,
		delay, "group", title, description, "parentPostId", "releaseId", "releaseURL",
		settings, image, error, "deletedAt", "createdAt", "updatedAt"`

func scanPost(row interface{ Scan(...any) error }) (*Post, error) {
	p := &Post{}
	err := row.Scan(
		&p.ID,
		&p.State,
		&p.PublishDate,
		&p.OrganizationID,
		&p.IntegrationID,
		&p.Content,
		&p.Delay,
		&p.Group,
		&p.Title,
		&p.Description,
		&p.ParentPostID,
		&p.ReleaseID,
		&p.ReleaseURL,
		&p.Settings,
		&p.Image,
		&p.Error,
		&p.DeletedAt,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	return p, err
}

// CreatePost inserts a new Post row and returns the created record.
// Prisma uses cuid() for Post.id; group is also caller-supplied (Prisma sets it per post group).
func (s *Store) CreatePost(ctx context.Context,
	orgID, integrationID, content, group string,
	publishDate time.Time,
	title, settings, image, parentPostID *string,
) (*Post, error) {
	id := cuid.New()

	q := fmt.Sprintf(`
		INSERT INTO "Post"
			(id, state, "publishDate", "organizationId", "integrationId", content,
			 delay, "group", title, settings, image, "parentPostId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, $9, $10, $11, NOW(), NOW())
		RETURNING %s`, postCols)

	p, err := scanPost(s.pool.QueryRow(ctx, q,
		id, string(StateQueue), publishDate, orgID, integrationID, content,
		group, title, settings, image, parentPostID,
	))
	if err != nil {
		return nil, fmt.Errorf("CreatePost: %w", err)
	}
	return p, nil
}

// GetPost returns a single post by id scoped to an org.
func (s *Store) GetPost(ctx context.Context, id, orgID string) (*Post, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Post"
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL
		LIMIT 1`, postCols)

	p, err := scanPost(s.pool.QueryRow(ctx, q, id, orgID))
	if err != nil {
		return nil, fmt.Errorf("GetPost: %w", err)
	}
	return p, nil
}

// ListPosts returns all non-deleted posts for an org, newest first.
func (s *Store) ListPosts(ctx context.Context, orgID string) ([]*Post, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Post"
		WHERE "organizationId" = $1 AND "deletedAt" IS NULL
		ORDER BY "publishDate" DESC
		LIMIT 500`, postCols)

	rows, err := s.pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("ListPosts: %w", err)
	}
	defer rows.Close()

	var out []*Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("ListPosts scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePost soft-deletes a post.
func (s *Store) DeletePost(ctx context.Context, id, orgID string) error {
	const q = `
		UPDATE "Post"
		SET "deletedAt" = NOW(), "updatedAt" = NOW()
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, id, orgID)
	if err != nil {
		return fmt.Errorf("DeletePost: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DeletePost: post %s not found for org %s", id, orgID)
	}
	return nil
}

// GetQueuedPosts returns all QUEUE-state posts whose publishDate is in the past
// and have not been soft-deleted. Used by the scheduler.
func (s *Store) GetQueuedPosts(ctx context.Context) ([]*Post, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Post"
		WHERE state = 'QUEUE' AND "publishDate" <= NOW() AND "deletedAt" IS NULL
		ORDER BY "publishDate" ASC`, postCols)

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("GetQueuedPosts: %w", err)
	}
	defer rows.Close()

	var out []*Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("GetQueuedPosts scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdatePostContent updates the content, image, and settings of an existing post.
func (s *Store) UpdatePostContent(ctx context.Context, id, orgID, content string, image, settings *string) (*Post, error) {
	q := fmt.Sprintf(`
		UPDATE "Post"
		SET content = $1, image = $2, settings = $3, "updatedAt" = NOW()
		WHERE id = $4 AND "organizationId" = $5 AND "deletedAt" IS NULL
		RETURNING %s`, postCols)

	p, err := scanPost(s.pool.QueryRow(ctx, q, content, image, settings, id, orgID))
	if err != nil {
		return nil, fmt.Errorf("UpdatePostContent: %w", err)
	}
	return p, nil
}

// UpdatePostState transitions a post to the given state and optionally records
// an error message (used when transitioning to StateError).
func (s *Store) UpdatePostState(ctx context.Context, id string, state State, errMsg *string) error {
	const q = `
		UPDATE "Post"
		SET state = $1, error = $2, "updatedAt" = NOW()
		WHERE id = $3 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, string(state), errMsg, id)
	if err != nil {
		return fmt.Errorf("UpdatePostState: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdatePostState: post %s not found", id)
	}
	return nil
}

// UpdatePostPublished marks a post as PUBLISHED and records the platform post ID + URL.
func (s *Store) UpdatePostPublished(ctx context.Context, id, releaseID, releaseURL string) error {
	const q = `
		UPDATE "Post"
		SET state = 'PUBLISHED', "releaseId" = $1, "releaseURL" = $2, "updatedAt" = NOW()
		WHERE id = $3 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, releaseID, releaseURL, id)
	if err != nil {
		return fmt.Errorf("UpdatePostPublished: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdatePostPublished: post %s not found", id)
	}
	return nil
}
