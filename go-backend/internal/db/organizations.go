package db

import (
	"context"
	"fmt"
)

// GetOrgByAPIKey returns the Organization whose apiKey matches the given key.
// Returns an error if no row is found or if the key is blank.
func (s *Store) GetOrgByAPIKey(ctx context.Context, apiKey string) (*Organization, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("apiKey must not be empty")
	}

	const q = `
		SELECT id, name, description, "apiKey", "createdAt", "updatedAt"
		FROM "Organization"
		WHERE "apiKey" = $1
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, apiKey)
	org := &Organization{}
	err := row.Scan(
		&org.ID,
		&org.Name,
		&org.Description,
		&org.APIKey,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrgByAPIKey: %w", err)
	}
	return org, nil
}

// GetOrgByID returns the Organization with the given id.
func (s *Store) GetOrgByID(ctx context.Context, id string) (*Organization, error) {
	const q = `
		SELECT id, name, description, "apiKey", "createdAt", "updatedAt"
		FROM "Organization"
		WHERE id = $1
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, id)
	org := &Organization{}
	err := row.Scan(
		&org.ID,
		&org.Name,
		&org.Description,
		&org.APIKey,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOrgByID: %w", err)
	}
	return org, nil
}
