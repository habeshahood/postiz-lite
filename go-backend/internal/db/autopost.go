package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AutoPost matches the Prisma AutoPost model.
type AutoPost struct {
	ID              string     `db:"id" json:"id"`
	OrganizationID  string     `db:"organizationId" json:"organizationId"`
	Title           string     `db:"title" json:"title"`
	Content         *string    `db:"content" json:"content"`
	OnSlot          bool       `db:"onSlot" json:"onSlot"`
	SyncLast        bool       `db:"syncLast" json:"syncLast"`
	URL             string     `db:"url" json:"url"`
	LastURL         string     `db:"lastUrl" json:"lastUrl"`
	Active          bool       `db:"active" json:"active"`
	AddPicture      bool       `db:"addPicture" json:"addPicture"`
	GenerateContent bool       `db:"generateContent" json:"generateContent"`
	Integrations    string     `db:"integrations" json:"integrations"`
	DeletedAt       *time.Time `db:"deletedAt" json:"deletedAt"`
	CreatedAt       time.Time  `db:"createdAt" json:"createdAt"`
	UpdatedAt       time.Time  `db:"updatedAt" json:"updatedAt"`
}

const autoPostCols = `id, "organizationId", title, content, "onSlot", "syncLast", url,
	"lastUrl", active, "addPicture", "generateContent", integrations, "deletedAt", "createdAt", "updatedAt"`

func scanAutoPost(row interface{ Scan(...any) error }) (*AutoPost, error) {
	a := &AutoPost{}
	err := row.Scan(
		&a.ID,
		&a.OrganizationID,
		&a.Title,
		&a.Content,
		&a.OnSlot,
		&a.SyncLast,
		&a.URL,
		&a.LastURL,
		&a.Active,
		&a.AddPicture,
		&a.GenerateContent,
		&a.Integrations,
		&a.DeletedAt,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	return a, err
}

// ListAutoPosts returns all non-deleted auto-post rules for an org.
func (s *Store) ListAutoPosts(ctx context.Context, orgID string) ([]*AutoPost, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "AutoPost"
		WHERE "deletedAt" IS NULL AND "organizationId" = $1
		ORDER BY "createdAt" DESC`, autoPostCols)

	rows, err := s.pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("ListAutoPosts: %w", err)
	}
	defer rows.Close()

	var out []*AutoPost
	for rows.Next() {
		a, err := scanAutoPost(rows)
		if err != nil {
			return nil, fmt.Errorf("ListAutoPosts scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAutoPost inserts a new AutoPost row from a body map.
func (s *Store) CreateAutoPost(ctx context.Context, orgID string, body map[string]any) (*AutoPost, error) {
	id := uuid.New().String()

	title, _ := body["title"].(string)
	url, _ := body["url"].(string)
	onSlot, _ := body["onSlot"].(bool)
	syncLast, _ := body["syncLast"].(bool)
	active, _ := body["active"].(bool)
	addPicture, _ := body["addPicture"].(bool)
	generateContent, _ := body["generateContent"].(bool)
	integrations, _ := body["integrations"].(string)
	if integrations == "" {
		integrations = "[]"
	}

	var content *string
	if v, ok := body["content"].(string); ok && v != "" {
		content = &v
	}

	q := fmt.Sprintf(`
		INSERT INTO "AutoPost"
			(id, "organizationId", title, content, "onSlot", "syncLast", url, "lastUrl",
			 active, "addPicture", "generateContent", integrations, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, $9, $10, $11, NOW(), NOW())
		RETURNING %s`, autoPostCols)

	a, err := scanAutoPost(s.pool.QueryRow(ctx, q,
		id, orgID, title, content, onSlot, syncLast, url,
		active, addPicture, generateContent, integrations,
	))
	if err != nil {
		return nil, fmt.Errorf("CreateAutoPost: %w", err)
	}
	return a, nil
}

// UpdateAutoPost updates an existing AutoPost row from a body map.
func (s *Store) UpdateAutoPost(ctx context.Context, orgID, id string, body map[string]any) (*AutoPost, error) {
	title, _ := body["title"].(string)
	url, _ := body["url"].(string)
	onSlot, _ := body["onSlot"].(bool)
	syncLast, _ := body["syncLast"].(bool)
	active, _ := body["active"].(bool)
	addPicture, _ := body["addPicture"].(bool)
	generateContent, _ := body["generateContent"].(bool)
	integrations, _ := body["integrations"].(string)
	if integrations == "" {
		integrations = "[]"
	}

	var content *string
	if v, ok := body["content"].(string); ok && v != "" {
		content = &v
	}

	q := fmt.Sprintf(`
		UPDATE "AutoPost"
		SET title = $1, content = $2, "onSlot" = $3, "syncLast" = $4, url = $5,
		    active = $6, "addPicture" = $7, "generateContent" = $8, integrations = $9,
		    "updatedAt" = NOW()
		WHERE id = $10 AND "organizationId" = $11 AND "deletedAt" IS NULL
		RETURNING %s`, autoPostCols)

	a, err := scanAutoPost(s.pool.QueryRow(ctx, q,
		title, content, onSlot, syncLast, url,
		active, addPicture, generateContent, integrations, id, orgID,
	))
	if err != nil {
		return nil, fmt.Errorf("UpdateAutoPost: %w", err)
	}
	return a, nil
}

// DeleteAutoPost soft-deletes an auto-post rule.
func (s *Store) DeleteAutoPost(ctx context.Context, orgID, id string) error {
	const q = `
		UPDATE "AutoPost"
		SET "deletedAt" = NOW(), "updatedAt" = NOW()
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, id, orgID)
	if err != nil {
		return fmt.Errorf("DeleteAutoPost: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DeleteAutoPost: auto-post %s not found for org %s", id, orgID)
	}
	return nil
}

// SetAutoPostActive toggles the active field on an auto-post rule.
func (s *Store) SetAutoPostActive(ctx context.Context, orgID, id string, active bool) error {
	const q = `
		UPDATE "AutoPost"
		SET active = $1, "updatedAt" = NOW()
		WHERE id = $2 AND "organizationId" = $3 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, active, id, orgID)
	if err != nil {
		return fmt.Errorf("SetAutoPostActive: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("SetAutoPostActive: auto-post %s not found for org %s", id, orgID)
	}
	return nil
}
