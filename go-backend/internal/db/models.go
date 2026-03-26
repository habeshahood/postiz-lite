package db

import "time"

// State mirrors the Prisma "State" enum used on Post.
type State string

const (
	StateQueue     State = "QUEUE"
	StateDraft     State = "DRAFT"
	StatePublished State = "PUBLISHED"
	StateError     State = "ERROR"
)

// Provider mirrors the Prisma "Provider" enum used on User.
type Provider string

const (
	ProviderLocal    Provider = "LOCAL"
	ProviderGitHub   Provider = "GITHUB"
	ProviderGoogle   Provider = "GOOGLE"
	ProviderFarcaster Provider = "FARCASTER"
	ProviderWallet   Provider = "WALLET"
	ProviderGeneric  Provider = "GENERIC"
)

// Role mirrors the Prisma "Role" enum used on UserOrganization.
type Role string

const (
	RoleSuperAdmin Role = "SUPERADMIN"
	RoleAdmin      Role = "ADMIN"
	RoleUser       Role = "USER"
)

// Organization matches the Prisma Organization model.
// Column names are camelCase as Prisma does not apply @map on these fields.
type Organization struct {
	ID          string     `db:"id"`
	Name        string     `db:"name"`
	Description *string    `db:"description"`
	APIKey      *string    `db:"apiKey"`
	CreatedAt   time.Time  `db:"createdAt"`
	UpdatedAt   time.Time  `db:"updatedAt"`
}

// User matches the Prisma User model (core fields only).
type User struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	Password     *string   `db:"password"`
	ProviderName Provider  `db:"providerName"`
	Name         *string   `db:"name"`
	LastName     *string   `db:"lastName"`
	Timezone     int       `db:"timezone"`
	Activated    bool      `db:"activated"`
	CreatedAt    time.Time `db:"createdAt"`
	UpdatedAt    time.Time `db:"updatedAt"`
}

// UserOrganization matches the Prisma UserOrganization model.
type UserOrganization struct {
	ID             string    `db:"id"`
	UserID         string    `db:"userId"`
	OrganizationID string    `db:"organizationId"`
	Disabled       bool      `db:"disabled"`
	Role           Role      `db:"role"`
	CreatedAt      time.Time `db:"createdAt"`
	UpdatedAt      time.Time `db:"updatedAt"`
}

// Integration matches the Prisma Integration model.
type Integration struct {
	ID                    string     `db:"id"`
	InternalID            string     `db:"internalId"`
	OrganizationID        string     `db:"organizationId"`
	Name                  string     `db:"name"`
	Picture               *string    `db:"picture"`
	ProviderIdentifier    string     `db:"providerIdentifier"`
	Type                  string     `db:"type"`
	Token                 string     `db:"token"`
	Disabled              bool       `db:"disabled"`
	TokenExpiration       *time.Time `db:"tokenExpiration"`
	RefreshToken          *string    `db:"refreshToken"`
	Profile               *string    `db:"profile"`
	DeletedAt             *time.Time `db:"deletedAt"`
	CreatedAt             time.Time  `db:"createdAt"`
	UpdatedAt             *time.Time `db:"updatedAt"`
	InBetweenSteps        bool       `db:"inBetweenSteps"`
	RefreshNeeded         bool       `db:"refreshNeeded"`
	CustomInstanceDetails *string    `db:"customInstanceDetails"`
	AdditionalSettings    *string    `db:"additionalSettings"`
}

// Post matches the Prisma Post model.
type Post struct {
	ID             string     `db:"id"`
	State          State      `db:"state"`
	PublishDate    time.Time  `db:"publishDate"`
	OrganizationID string     `db:"organizationId"`
	IntegrationID  string     `db:"integrationId"`
	Content        string     `db:"content"`
	Delay          int        `db:"delay"`
	Group          string     `db:"group"`
	Title          *string    `db:"title"`
	Description    *string    `db:"description"`
	ParentPostID   *string    `db:"parentPostId"`
	ReleaseID      *string    `db:"releaseId"`
	ReleaseURL     *string    `db:"releaseURL"`
	Settings       *string    `db:"settings"`
	Image          *string    `db:"image"`
	Error          *string    `db:"error"`
	DeletedAt      *time.Time `db:"deletedAt"`
	CreatedAt      time.Time  `db:"createdAt"`
	UpdatedAt      time.Time  `db:"updatedAt"`
}

// Media matches the Prisma Media model.
type Media struct {
	ID             string     `db:"id"`
	Name           string     `db:"name"`
	OriginalName   *string    `db:"originalName"`
	Path           string     `db:"path"`
	OrganizationID string     `db:"organizationId"`
	FileSize       int        `db:"fileSize"`
	Type           string     `db:"type"`
	Thumbnail      *string    `db:"thumbnail"`
	DeletedAt      *time.Time `db:"deletedAt"`
	CreatedAt      time.Time  `db:"createdAt"`
	UpdatedAt      time.Time  `db:"updatedAt"`
}
