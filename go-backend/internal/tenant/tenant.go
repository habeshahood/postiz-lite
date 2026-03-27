package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// SocialKeys holds per-tenant OAuth app credentials.
type SocialKeys struct {
	XAPIKey          string `json:"x_api_key"`
	XAPISecret       string `json:"x_api_secret"`
	TikTokClientID   string `json:"tiktok_client_id"`
	TikTokSecret     string `json:"tiktok_client_secret"`
	YouTubeClientID  string `json:"youtube_client_id"`
	YouTubeSecret    string `json:"youtube_client_secret"`
	FacebookAppID    string `json:"facebook_app_id"`
	FacebookSecret   string `json:"facebook_app_secret"`
}

// Config is one tenant's JSON-serializable configuration.
type Config struct {
	ID          string     `json:"id"`
	Hosts       []string   `json:"hosts"`
	DatabaseURL string     `json:"database_url"`
	RedisURL    string     `json:"redis_url"`
	JWTSecret   string     `json:"jwt_secret"`
	FrontendURL string     `json:"frontend_url"`
	UploadDir   string     `json:"upload_dir"`
	Social      SocialKeys `json:"social"`
}

// Tenant is a fully-initialized tenant with live DB/Redis connections.
type Tenant struct {
	Config
	DB    *pgxpool.Pool
	Redis *redis.Client
}

// LoadConfigs reads and parses a tenants.json file.
func LoadConfigs(path string) ([]Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tenant: read %s: %w", path, err)
	}
	var configs []Config
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("tenant: parse %s: %w", path, err)
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("tenant: %s has no tenants", path)
	}
	// Normalize hosts to lowercase
	for i := range configs {
		for j := range configs[i].Hosts {
			configs[i].Hosts[j] = strings.ToLower(configs[i].Hosts[j])
		}
	}
	return configs, nil
}

// --- Context injection for per-request tenant data ---

type ctxKey string

const (
	socialKeysKey  ctxKey = "tenantSocialKeys"
	frontendURLKey ctxKey = "tenantFrontendURL"
	uploadDirKey   ctxKey = "tenantUploadDir"
	tenantIDKey    ctxKey = "tenantID"
)

func WithSocialKeys(ctx context.Context, keys *SocialKeys) context.Context {
	return context.WithValue(ctx, socialKeysKey, keys)
}

func GetSocialKeys(ctx context.Context) *SocialKeys {
	keys, _ := ctx.Value(socialKeysKey).(*SocialKeys)
	return keys
}

func WithFrontendURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, frontendURLKey, url)
}

func GetFrontendURL(ctx context.Context) string {
	s, _ := ctx.Value(frontendURLKey).(string)
	return s
}

func WithUploadDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, uploadDirKey, dir)
}

func GetUploadDir(ctx context.Context) string {
	s, _ := ctx.Value(uploadDirKey).(string)
	return s
}

func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantIDKey, id)
}

func GetTenantID(ctx context.Context) string {
	s, _ := ctx.Value(tenantIDKey).(string)
	return s
}
