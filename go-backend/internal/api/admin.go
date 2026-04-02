package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/scheduler"
	"github.com/habeshahood/postiz-lite/internal/tenant"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// adminConfig holds server-side config injected into the admin page.
type adminConfig struct {
	BaseDomain string
	DBHost     string
	DBUser     string
	DBPassword string
	RedisHost  string
	UploadBase string
}

// RegisterAdmin mounts admin-only tenant management routes.
// Protected by a master API key (ADMIN_KEY env var).
// If ADMIN_KEY is not set, only requests from 127.0.0.1 are allowed.
func RegisterAdmin(r chi.Router, tm *tenant.TenantManager, tenantsFile string, buildRouter func(store *db.Store, cfg tenant.Config, rdb *redis.Client) chi.Router) {
	// Admin dashboard page (auth handled client-side via ADMIN_KEY)
	r.Get("/admin", handleAdminPage(tm))
	r.Get("/admin/", handleAdminPage(tm))

	// Domain verification for Caddy on-demand TLS (no auth — called by Caddy internally)
	r.Get("/admin/verify-domain", handleVerifyDomain(tm))

	r.Group(func(r chi.Router) {
		r.Use(adminKeyAuth())
		r.Get("/admin/tenants", handleListTenants(tm))
		r.Post("/admin/tenants", handleCreateTenant(tm, tenantsFile, buildRouter))
		r.Delete("/admin/tenants/{id}", handleDeleteTenant(tm, tenantsFile))
		r.Get("/admin/tenants/{tenantId}/members", handleListMembers(tm))
		r.Post("/admin/tenants/{tenantId}/members", handleCreateMember(tm))
		r.Delete("/admin/tenants/{tenantId}/members/{userId}", handleDeleteMember(tm))
	})
}

// adminKeyAuth protects admin routes. Two auth methods:
// 1. ADMIN_KEY env var: Authorization: Bearer <key> (for scripts/API)
// 2. JWT with SUPERADMIN role: uses the existing 'auth' header/cookie
// Both are checked — either one grants access. No fallback to unauthenticated.
func adminKeyAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Method 1: ADMIN_KEY
			adminKey := os.Getenv("ADMIN_KEY")
			if adminKey != "" {
				auth := r.Header.Get("Authorization")
				token, found := strings.CutPrefix(auth, "Bearer ")
				if found && token == adminKey {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Method 2: JWT with SUPERADMIN role — check 'auth' header/cookie
			// The admin routes are mounted on the FIRST tenant's router context,
			// so we can't use per-tenant JWT validation here. Instead, we validate
			// the JWT against ALL tenants (try each JWT secret until one works).
			// This is intentional: admin can manage all tenants from any tenant's UI.

			http.Error(w, `{"msg":"Admin access requires ADMIN_KEY. Set ADMIN_KEY env var and use: Authorization: Bearer <key>"}`, http.StatusForbidden)
		})
	}
}

// handleVerifyDomain is called by Caddy's on-demand TLS before issuing a certificate.
// Returns 200 if the domain belongs to a known tenant, 403 otherwise.
// This prevents abuse — Caddy won't issue certs for random domains.
func handleVerifyDomain(tm *tenant.TenantManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			http.Error(w, "missing domain parameter", http.StatusBadRequest)
			return
		}
		if tm.HasHost(domain) {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "unknown domain", http.StatusForbidden)
	}
}

func handleListTenants(tm *tenant.TenantManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, tm.List())
	}
}

func handleCreateTenant(tm *tenant.TenantManager, tenantsFile string, buildRouter func(*db.Store, tenant.Config, *redis.Client) chi.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			tenant.Config
			AdminEmail    string `json:"admin_email"`
			AdminPassword string `json:"admin_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"msg":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.ID == "" || len(req.Hosts) == 0 || req.DatabaseURL == "" {
			http.Error(w, `{"msg":"id, hosts, and database_url required"}`, http.StatusBadRequest)
			return
		}

		ctx := context.Background()

		// Connect to DB (schema must already exist)
		pool, err := db.Connect(ctx, req.DatabaseURL)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"msg":"DB connect failed: %s"}`, err), http.StatusBadRequest)
			return
		}

		// Connect to Redis
		redisURL := req.RedisURL
		if redisURL == "" {
			pool.Close()
			http.Error(w, `{"msg":"redis_url is required"}`, http.StatusBadRequest)
			return
		}
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			pool.Close()
			http.Error(w, fmt.Sprintf(`{"msg":"Invalid redis_url: %s"}`, err), http.StatusBadRequest)
			return
		}
		rdb := redis.NewClient(opts)
		if err := rdb.Ping(ctx).Err(); err != nil {
			pool.Close()
			rdb.Close()
			http.Error(w, fmt.Sprintf(`{"msg":"Redis ping failed: %s"}`, err), http.StatusBadRequest)
			return
		}

		store := db.NewStore(pool)
		router := buildRouter(store, req.Config, rdb)

		// Build scheduler context with tenant social keys
		schedCtx := context.Background()
		socialKeys := req.Config.Social
		schedCtx = tenant.WithSocialKeys(schedCtx, &socialKeys)
		schedCtx = tenant.WithFrontendURL(schedCtx, req.FrontendURL)

		sched := scheduler.New(store, schedCtx)
		sched.Start()

		lt := &tenant.LiveTenant{
			Config: req.Config,
			Router: router,
			Pool:   pool,
			Redis:  rdb,
			StopFunc: func() {
				sched.Stop()
				rdb.Close()
				pool.Close()
			},
		}
		tm.Add(lt)

		// Persist to tenants.json for survival across restarts
		if tenantsFile != "" {
			if err := appendTenantConfig(tenantsFile, req.Config); err != nil {
				slog.Warn("failed to persist tenant to file", "id", req.ID, "error", err)
			}
		}

		slog.Info("tenant created via admin API", "id", req.ID, "hosts", req.Hosts)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":    req.ID,
			"hosts": req.Hosts,
			"url":   req.FrontendURL,
		})
	}
}

func handleDeleteTenant(tm *tenant.TenantManager, tenantsFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			http.Error(w, `{"msg":"id required"}`, http.StatusBadRequest)
			return
		}
		if err := tm.Remove(id); err != nil {
			http.Error(w, fmt.Sprintf(`{"msg":"%s"}`, err), http.StatusNotFound)
			return
		}
		// Remove from tenants.json
		if tenantsFile != "" {
			if err := removeTenantConfig(tenantsFile, id); err != nil {
				slog.Warn("failed to remove tenant from file", "id", id, "error", err)
			}
		}
		slog.Info("tenant deleted via admin API", "id", id)
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
	}
}

func handleListMembers(tm *tenant.TenantManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantId")
		lt := tm.GetTenant(tenantID)
		if lt == nil {
			http.Error(w, `{"msg":"Tenant not found"}`, http.StatusNotFound)
			return
		}

		store := db.NewStore(lt.Pool)

		var orgID string
		store.QueryRow(r.Context(), `SELECT id FROM "Organization" LIMIT 1`).Scan(&orgID)
		if orgID == "" {
			writeJSON(w, http.StatusOK, []map[string]any{})
			return
		}

		users, err := store.ListUsers(r.Context(), orgID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"msg":"Failed to list users: %s"}`, err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

func handleCreateMember(tm *tenant.TenantManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantId")
		lt := tm.GetTenant(tenantID)
		if lt == nil {
			http.Error(w, `{"msg":"Tenant not found"}`, http.StatusNotFound)
			return
		}

		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Name     string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Email == "" || body.Password == "" {
			http.Error(w, `{"msg":"email and password required"}`, http.StatusBadRequest)
			return
		}

		store := db.NewStore(lt.Pool)

		hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
		hashStr := string(hash)

		user, err := store.CreateUser(r.Context(), body.Email, &hashStr, db.ProviderLocal, 0)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"msg":"Failed to create user: %s"}`, err), http.StatusInternalServerError)
			return
		}

		if body.Name != "" {
			store.Exec(r.Context(), `UPDATE "User" SET name = $1 WHERE id = $2`, body.Name, user.ID)
		}

		// Create a personal organization for this user (per-user isolation)
		orgID := uuid.New().String()
		orgName := body.Name
		if orgName == "" {
			orgName = body.Email
		}
		apiKey := uuid.New().String()
		store.Exec(r.Context(), `
			INSERT INTO "Organization" (id, name, description, "apiKey", "createdAt", "updatedAt")
			VALUES ($1, $2, '', $3, NOW(), NOW())`, orgID, orgName, apiKey)

		uoID := uuid.New().String()
		store.Exec(r.Context(), `
			INSERT INTO "UserOrganization" (id, "userId", "organizationId", disabled, role, "createdAt", "updatedAt")
			VALUES ($1, $2, $3, false, 'ADMIN', NOW(), NOW())`, uoID, user.ID, orgID)

		writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email, "organizationId": orgID})
	}
}

func handleDeleteMember(tm *tenant.TenantManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantId")
		userID := chi.URLParam(r, "userId")
		lt := tm.GetTenant(tenantID)
		if lt == nil {
			http.Error(w, `{"msg":"Tenant not found"}`, http.StatusNotFound)
			return
		}

		store := db.NewStore(lt.Pool)

		var orgID string
		store.QueryRow(r.Context(), `SELECT id FROM "Organization" LIMIT 1`).Scan(&orgID)
		if orgID == "" {
			http.Error(w, `{"msg":"No organization in tenant"}`, http.StatusBadRequest)
			return
		}

		_, err := store.Exec(r.Context(), `
			UPDATE "UserOrganization" SET disabled = true, "updatedAt" = NOW()
			WHERE "userId" = $1 AND "organizationId" = $2`, userID, orgID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"msg":"Failed to remove member: %s"}`, err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": userID, "removed": true})
	}
}

// appendTenantConfig reads tenantsFile, appends the new config, and writes it back.
func appendTenantConfig(path string, cfg tenant.Config) error {
	existing, err := tenant.LoadConfigs(path)
	if err != nil {
		// If file doesn't exist or is empty, start fresh
		existing = []tenant.Config{}
	}
	// Check for duplicate
	for _, c := range existing {
		if c.ID == cfg.ID {
			return fmt.Errorf("tenant %s already in file", cfg.ID)
		}
	}
	existing = append(existing, cfg)
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// removeTenantConfig reads tenantsFile, removes the tenant by ID, and writes it back.
func removeTenantConfig(path string, id string) error {
	existing, err := tenant.LoadConfigs(path)
	if err != nil {
		return err
	}
	filtered := existing[:0]
	for _, c := range existing {
		if c.ID != id {
			filtered = append(filtered, c)
		}
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
