package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Comment matches the Prisma Comments model.
type Comment struct {
	ID             string     `db:"id" json:"id"`
	Content        string     `db:"content" json:"content"`
	OrganizationID string     `db:"organizationId" json:"organizationId"`
	PostID         string     `db:"postId" json:"postId"`
	UserID         string     `db:"userId" json:"userId"`
	CreatedAt      time.Time  `db:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time  `db:"updatedAt" json:"updatedAt"`
	DeletedAt      *time.Time `db:"deletedAt" json:"deletedAt"`
}

const commentCols = `id, content, "organizationId", "postId", "userId", "createdAt", "updatedAt", "deletedAt"`

func scanComment(row interface{ Scan(...any) error }) (*Comment, error) {
	c := &Comment{}
	err := row.Scan(
		&c.ID,
		&c.Content,
		&c.OrganizationID,
		&c.PostID,
		&c.UserID,
		&c.CreatedAt,
		&c.UpdatedAt,
		&c.DeletedAt,
	)
	return c, err
}

// ListComments returns all non-deleted comments for a post scoped to an org, oldest first.
func (s *Store) ListComments(ctx context.Context, postID, orgID string) ([]*Comment, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Comments"
		WHERE "postId" = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL
		ORDER BY "createdAt" ASC`, commentCols)

	rows, err := s.pool.Query(ctx, q, postID, orgID)
	if err != nil {
		return nil, fmt.Errorf("ListComments: %w", err)
	}
	defer rows.Close()

	var out []*Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("ListComments scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateComment inserts a new Comment row and returns the created record.
func (s *Store) CreateComment(ctx context.Context, orgID, postID, userID, content string) (*Comment, error) {
	id := uuid.New().String()

	q := fmt.Sprintf(`
		INSERT INTO "Comments" (id, content, "organizationId", "postId", "userId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING %s`, commentCols)

	c, err := scanComment(s.pool.QueryRow(ctx, q, id, content, orgID, postID, userID))
	if err != nil {
		return nil, fmt.Errorf("CreateComment: %w", err)
	}
	return c, nil
}
