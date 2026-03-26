package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// GetUserByEmail returns the first User matching the given email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	const q = `
		SELECT id, email, password, "providerName", name, "lastName", timezone, activated, "createdAt", "updatedAt"
		FROM "User"
		WHERE email = $1
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, email)
	u := &User{}
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Password,
		&u.ProviderName,
		&u.Name,
		&u.LastName,
		&u.Timezone,
		&u.Activated,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetUserByEmail: %w", err)
	}
	return u, nil
}

// GetUserByID returns the User with the given id.
func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	const q = `
		SELECT id, email, password, "providerName", name, "lastName", timezone, activated, "createdAt", "updatedAt"
		FROM "User"
		WHERE id = $1
		LIMIT 1`

	row := s.pool.QueryRow(ctx, q, id)
	u := &User{}
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Password,
		&u.ProviderName,
		&u.Name,
		&u.LastName,
		&u.Timezone,
		&u.Activated,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetUserByID: %w", err)
	}
	return u, nil
}

// CreateUser inserts a new User row and returns the created record.
// Prisma uses uuid() for User.id, so we generate a UUID here.
// password should be an argon2 hash or nil for OAuth-only users.
func (s *Store) CreateUser(ctx context.Context, email string, hashedPassword *string, provider Provider, timezone int) (*User, error) {
	id := uuid.New().String()

	const q = `
		INSERT INTO "User" (id, email, password, "providerName", timezone, activated, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW())
		RETURNING id, email, password, "providerName", name, "lastName", timezone, activated, "createdAt", "updatedAt"`

	row := s.pool.QueryRow(ctx, q, id, email, hashedPassword, string(provider), timezone)
	u := &User{}
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Password,
		&u.ProviderName,
		&u.Name,
		&u.LastName,
		&u.Timezone,
		&u.Activated,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateUser: %w", err)
	}
	return u, nil
}

// GetUserDefaultOrgID returns the organizationId of the first (oldest) active
// UserOrganization membership for the given user. Used when minting a JWT.
func (s *Store) GetUserDefaultOrgID(ctx context.Context, userID string) (string, error) {
	const q = `
		SELECT "organizationId"
		FROM "UserOrganization"
		WHERE "userId" = $1 AND disabled = false
		ORDER BY "createdAt" ASC
		LIMIT 1`

	var orgID string
	err := s.pool.QueryRow(ctx, q, userID).Scan(&orgID)
	if err != nil {
		return "", fmt.Errorf("GetUserDefaultOrgID: %w", err)
	}
	return orgID, nil
}
