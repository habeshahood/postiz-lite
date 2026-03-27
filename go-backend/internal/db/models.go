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
	ID             string     `db:"id" json:"id"`
	State          State      `db:"state" json:"state"`
	PublishDate    time.Time  `db:"publishDate" json:"publishDate"`
	OrganizationID string     `db:"organizationId" json:"organizationId"`
	IntegrationID  string     `db:"integrationId" json:"integrationId"`
	Content        string     `db:"content" json:"content"`
	Delay          int        `db:"delay" json:"delay"`
	Group          string     `db:"group" json:"group"`
	Title          *string    `db:"title" json:"title"`
	Description    *string    `db:"description" json:"description"`
	ParentPostID   *string    `db:"parentPostId" json:"parentPostId"`
	ReleaseID      *string    `db:"releaseId" json:"releaseId"`
	ReleaseURL     *string    `db:"releaseURL" json:"releaseURL"`
	Settings       *string    `db:"settings" json:"settings"`
	Image          *string    `db:"image" json:"image"`
	Error          *string    `db:"error" json:"error"`
	DeletedAt      *time.Time `db:"deletedAt" json:"deletedAt"`
	CreatedAt      time.Time  `db:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time  `db:"updatedAt" json:"updatedAt"`
}

// Media matches the Prisma Media model.
type Media struct {
	ID             string     `db:"id" json:"id"`
	Name           string     `db:"name" json:"name"`
	OriginalName   *string    `db:"originalName" json:"originalName"`
	Path           string     `db:"path" json:"path"`
	OrganizationID string     `db:"organizationId" json:"organizationId"`
	FileSize       int        `db:"fileSize" json:"fileSize"`
	Type           string     `db:"type" json:"type"`
	Thumbnail      *string    `db:"thumbnail" json:"thumbnail"`
	DeletedAt      *time.Time `db:"deletedAt" json:"deletedAt"`
	CreatedAt      time.Time  `db:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time  `db:"updatedAt" json:"updatedAt"`
}
