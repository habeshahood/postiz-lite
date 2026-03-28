package db

import (
	"context"
	"fmt"
	"time"

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

// GetUserRoleInOrg returns the user's Role within a specific organization.
func (s *Store) GetUserRoleInOrg(ctx context.Context, userID, orgID string) (Role, error) {
	const q = `
		SELECT role
		FROM "UserOrganization"
		WHERE "userId" = $1 AND "organizationId" = $2 AND disabled = false
		LIMIT 1`

	var role Role
	err := s.pool.QueryRow(ctx, q, userID, orgID).Scan(&role)
	if err != nil {
		return "", fmt.Errorf("GetUserRoleInOrg: %w", err)
	}
	return role, nil
}

// UserOrg is a joined result for the user's organizations list.
type UserOrg struct {
	OrgID    string  `json:"id"`
	OrgName  string  `json:"name"`
	Role     Role    `json:"role"`
	Disabled bool    `json:"disabled"`
	APIKey   *string `json:"-"`
}

// GetUserOrganizations returns all orgs the user belongs to (with role).
func (s *Store) GetUserOrganizations(ctx context.Context, userID string) ([]UserOrg, error) {
	const q = `
		SELECT o.id, o.name, uo.role, uo.disabled, o."apiKey"
		FROM "UserOrganization" uo
		JOIN "Organization" o ON o.id = uo."organizationId"
		WHERE uo."userId" = $1 AND uo.disabled = false
		ORDER BY uo."createdAt" ASC`

	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("GetUserOrganizations: %w", err)
	}
	defer rows.Close()

	var out []UserOrg
	for rows.Next() {
		var uo UserOrg
		if err := rows.Scan(&uo.OrgID, &uo.OrgName, &uo.Role, &uo.Disabled, &uo.APIKey); err != nil {
			return nil, fmt.Errorf("GetUserOrganizations scan: %w", err)
		}
		out = append(out, uo)
	}
	return out, rows.Err()
}

// ListUsers returns all users in the organization.
func (s *Store) ListUsers(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.name, u."lastName", u.activated, uo.role, u."createdAt"
		FROM "User" u
		JOIN "UserOrganization" uo ON uo."userId" = u.id
		WHERE uo."organizationId" = $1 AND uo.disabled = false
		ORDER BY u."createdAt" DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []map[string]any
	for rows.Next() {
		var id, email string
		var name, lastName *string
		var activated bool
		var role string
		var createdAt time.Time
		if err := rows.Scan(&id, &email, &name, &lastName, &activated, &role, &createdAt); err != nil {
			continue
		}
		users = append(users, map[string]any{
			"id": id, "email": email, "name": name, "lastName": lastName,
			"activated": activated, "role": role, "createdAt": createdAt,
		})
	}
	if users == nil {
		users = []map[string]any{}
	}
	return users, nil
}

// GetPostsByDateRange returns posts within a date range for the calendar view.
func (s *Store) GetPostsByDateRange(ctx context.Context, orgID, from, to string) ([]*Post, error) {
	q := fmt.Sprintf(`
		SELECT %s
		FROM "Post"
		WHERE "organizationId" = $1
		  AND "publishDate" >= $2::timestamp
		  AND "publishDate" <= $3::timestamp
		  AND "deletedAt" IS NULL
		ORDER BY "publishDate" ASC`, postCols)

	rows, err := s.pool.Query(ctx, q, orgID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetPostsByDateRange: %w", err)
	}
	defer rows.Close()

	var out []*Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("GetPostsByDateRange scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
