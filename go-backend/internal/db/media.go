package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

const mediaCols = `id, name, "originalName", path, "organizationId", "fileSize", type, thumbnail, "deletedAt", "createdAt", "updatedAt"`

func scanMedia(row interface{ Scan(...any) error }) (*Media, error) {
	m := &Media{}
	err := row.Scan(
		&m.ID,
		&m.Name,
		&m.OriginalName,
		&m.Path,
		&m.OrganizationID,
		&m.FileSize,
		&m.Type,
		&m.Thumbnail,
		&m.DeletedAt,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	return m, err
}

// SaveMedia inserts a new Media row and returns the created record.
// Prisma uses uuid() for Media.id.
func (s *Store) SaveMedia(ctx context.Context, orgID, name, path, mediaType string, originalName *string, fileSize int) (*Media, error) {
	id := uuid.New().String()

	q := fmt.Sprintf(`
		INSERT INTO "Media" (id, name, "originalName", path, "organizationId", "fileSize", type, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING %s`, mediaCols)

	m, err := scanMedia(s.pool.QueryRow(ctx, q,
		id, name, originalName, path, orgID, fileSize, mediaType,
	))
	if err != nil {
		return nil, fmt.Errorf("SaveMedia: %w", err)
	}
	return m, nil
}

// GetMedia returns a single non-deleted Media record by id scoped to an org.
func (s *Store) GetMedia(ctx context.Context, id, orgID string) (*Media, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Media"
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL
		LIMIT 1`, mediaCols)

	m, err := scanMedia(s.pool.QueryRow(ctx, q, id, orgID))
	if err != nil {
		return nil, fmt.Errorf("GetMedia: %w", err)
	}
	return m, nil
}
