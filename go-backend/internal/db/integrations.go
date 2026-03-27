package db

import (
	"context"
	"fmt"
	"time"

	"github.com/lucsky/cuid"
)

const integrationCols = `id, "internalId", "organizationId", name, picture, "providerIdentifier",
		type, token, disabled, "tokenExpiration", "refreshToken", profile, "deletedAt",
		"createdAt", "updatedAt", "inBetweenSteps", "refreshNeeded", "customInstanceDetails", "additionalSettings"`

func scanIntegration(row interface{ Scan(...any) error }) (*Integration, error) {
	i := &Integration{}
	err := row.Scan(
		&i.ID,
		&i.InternalID,
		&i.OrganizationID,
		&i.Name,
		&i.Picture,
		&i.ProviderIdentifier,
		&i.Type,
		&i.Token,
		&i.Disabled,
		&i.TokenExpiration,
		&i.RefreshToken,
		&i.Profile,
		&i.DeletedAt,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.InBetweenSteps,
		&i.RefreshNeeded,
		&i.CustomInstanceDetails,
		&i.AdditionalSettings,
	)
	return i, err
}

// ListIntegrations returns all non-deleted integrations for an organization.
func (s *Store) ListIntegrations(ctx context.Context, orgID string) ([]*Integration, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Integration"
		WHERE "organizationId" = $1 AND "deletedAt" IS NULL
		ORDER BY "createdAt" DESC`, integrationCols)

	rows, err := s.pool.Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("ListIntegrations: %w", err)
	}
	defer rows.Close()

	var out []*Integration
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, fmt.Errorf("ListIntegrations scan: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// GetIntegrationByID returns a single integration by its id, scoped to an org.
func (s *Store) GetIntegrationByID(ctx context.Context, id, orgID string) (*Integration, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Integration"
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL
		LIMIT 1`, integrationCols)

	i, err := scanIntegration(s.pool.QueryRow(ctx, q, id, orgID))
	if err != nil {
		return nil, fmt.Errorf("GetIntegrationByID: %w", err)
	}
	return i, nil
}

// CreateIntegration inserts a new Integration row.
// Prisma uses cuid() for Integration.id.
func (s *Store) CreateIntegration(ctx context.Context,
	orgID, internalID, name, providerIdentifier, integrationType, token string,
	refreshToken *string, tokenExpiration *time.Time,
) (*Integration, error) {
	id := cuid.New()

	q := fmt.Sprintf(`
		INSERT INTO "Integration"
			(id, "internalId", "organizationId", name, "providerIdentifier", type, token,
			 "refreshToken", "tokenExpiration", disabled, "inBetweenSteps", "refreshNeeded",
			 "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, false, false, NOW(), NOW())
		RETURNING %s`, integrationCols)

	i, err := scanIntegration(s.pool.QueryRow(ctx, q,
		id, internalID, orgID, name, providerIdentifier, integrationType, token,
		refreshToken, tokenExpiration,
	))
	if err != nil {
		return nil, fmt.Errorf("CreateIntegration: %w", err)
	}
	return i, nil
}

// UpdateToken updates the OAuth token and refresh token for an integration.
func (s *Store) UpdateToken(ctx context.Context, id, orgID, token string, refreshToken *string, tokenExpiration *time.Time) error {
	const q = `
		UPDATE "Integration"
		SET token = $1, "refreshToken" = $2, "tokenExpiration" = $3, "refreshNeeded" = false, "updatedAt" = NOW()
		WHERE id = $4 AND "organizationId" = $5 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, token, refreshToken, tokenExpiration, id, orgID)
	if err != nil {
		return fmt.Errorf("UpdateToken: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdateToken: integration %s not found for org %s", id, orgID)
	}
	return nil
}

// GetExpiringIntegrations returns integrations whose token expires within the given duration,
// or that have refreshNeeded=true. Only returns integrations with a non-null refreshToken.
func (s *Store) GetExpiringIntegrations(ctx context.Context, within time.Duration) ([]*Integration, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Integration"
		WHERE "deletedAt" IS NULL
		  AND "refreshToken" IS NOT NULL
		  AND "refreshToken" != ''
		  AND (
		      ("tokenExpiration" IS NOT NULL AND "tokenExpiration" < $1)
		      OR "refreshNeeded" = true
		  )`, integrationCols)

	rows, err := s.pool.Query(ctx, q, time.Now().Add(within))
	if err != nil {
		return nil, fmt.Errorf("GetExpiringIntegrations: %w", err)
	}
	defer rows.Close()

	var out []*Integration
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, fmt.Errorf("GetExpiringIntegrations scan: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// UpdateIntegrationTokens updates the token fields for an integration by ID (no org scope).
// Used by the scheduler's token-refresh job.
func (s *Store) UpdateIntegrationTokens(ctx context.Context, id, token string, refreshToken *string, expiration *time.Time) error {
	const q = `
		UPDATE "Integration"
		SET token = $1, "refreshToken" = $2, "tokenExpiration" = $3,
		    "refreshNeeded" = false, "updatedAt" = NOW()
		WHERE id = $4`

	_, err := s.pool.Exec(ctx, q, token, refreshToken, expiration, id)
	if err != nil {
		return fmt.Errorf("UpdateIntegrationTokens: %w", err)
	}
	return nil
}

// DeleteIntegration soft-deletes an integration by setting deletedAt.
func (s *Store) DeleteIntegration(ctx context.Context, id, orgID string) error {
	const q = `
		UPDATE "Integration"
		SET "deletedAt" = NOW(), "updatedAt" = NOW()
		WHERE id = $1 AND "organizationId" = $2 AND "deletedAt" IS NULL`

	tag, err := s.pool.Exec(ctx, q, id, orgID)
	if err != nil {
		return fmt.Errorf("DeleteIntegration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DeleteIntegration: integration %s not found for org %s", id, orgID)
	}
	return nil
}
