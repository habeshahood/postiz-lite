package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Webhook matches the Prisma Webhook model.
type Webhook struct {
	ID             string  `db:"id" json:"id"`
	Name           string  `db:"name" json:"name"`
	OrganizationID string  `db:"organizationId" json:"organizationId"`
	URL            string  `db:"url" json:"url"`
	DeletedAt      *string `db:"deletedAt" json:"deletedAt"`
	CreatedAt      string  `db:"createdAt" json:"createdAt"`
	UpdatedAt      string  `db:"updatedAt" json:"updatedAt"`
}

const webhookCols = `id, name, "organizationId", url, "deletedAt", "createdAt", "updatedAt"`

func scanWebhook(row interface{ Scan(...any) error }) (*Webhook, error) {
	w := &Webhook{}
	err := row.Scan(&w.ID, &w.Name, &w.OrganizationID, &w.URL, &w.DeletedAt, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// ListWebhooks returns all non-deleted webhooks for an org.
func (s *Store) ListWebhooks(ctx context.Context, orgID string) ([]*Webhook, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Webhook"
		WHERE "deletedAt" IS NULL AND "organizationId" = $1
		ORDER BY "createdAt" DESC`, webhookCols)

	rows, err := s.pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("ListWebhooks: %w", err)
	}
	defer rows.Close()

	var out []*Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, fmt.Errorf("ListWebhooks scan: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// CreateWebhook inserts a new Webhook row and returns the created record.
func (s *Store) CreateWebhook(ctx context.Context, orgID, name, url string) (*Webhook, error) {
	id := uuid.New().String()

	q := fmt.Sprintf(`
		INSERT INTO "Webhook" (id, name, "organizationId", url, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING %s`, webhookCols)

	w, err := scanWebhook(s.pool.QueryRow(ctx, q, id, name, orgID, url))
	if err != nil {
		return nil, fmt.Errorf("CreateWebhook: %w", err)
	}
	return w, nil
}

// UpdateWebhook updates the name and url of an existing webhook.
func (s *Store) UpdateWebhook(ctx context.Context, orgID, id, name, url string) error {
	const q = `
		UPDATE "Webhook"
		SET name = $1, url = $2, "updatedAt" = NOW()
		WHERE id = $3 AND "organizationId" = $4 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, name, url, id, orgID)
	if err != nil {
		return fmt.Errorf("UpdateWebhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdateWebhook: webhook %s not found for org %s", id, orgID)
	}
	return nil
}

// DeleteWebhook soft-deletes a webhook by setting deletedAt.
func (s *Store) DeleteWebhook(ctx context.Context, orgID, id string) error {
	const q = `
		UPDATE "Webhook"
		SET "deletedAt" = NOW(), "updatedAt" = NOW()
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, id, orgID)
	if err != nil {
		return fmt.Errorf("DeleteWebhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DeleteWebhook: webhook %s not found for org %s", id, orgID)
	}
	return nil
}
