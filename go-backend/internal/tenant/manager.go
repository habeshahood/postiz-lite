package tenant

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// LiveTenant is a running tenant with its router and resources.
type LiveTenant struct {
	Config
	Router   chi.Router
	Pool     *pgxpool.Pool
	Redis    *redis.Client
	StopFunc func() // stops scheduler, closes connections
}

// TenantManager holds live tenants and supports runtime add/remove.
type TenantManager struct {
	mu      sync.RWMutex
	tenants []*LiveTenant
	hostMap map[string]*LiveTenant
}

func NewTenantManager() *TenantManager {
	return &TenantManager{
		hostMap: make(map[string]*LiveTenant),
	}
}

// Add registers a new live tenant. Thread-safe.
func (tm *TenantManager) Add(lt *LiveTenant) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tenants = append(tm.tenants, lt)
	for _, host := range lt.Hosts {
		tm.hostMap[strings.ToLower(host)] = lt
	}
}

// Remove stops and removes a tenant by ID. Thread-safe.
func (tm *TenantManager) Remove(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, lt := range tm.tenants {
		if lt.ID == id {
			for _, host := range lt.Hosts {
				delete(tm.hostMap, strings.ToLower(host))
			}
			if lt.StopFunc != nil {
				lt.StopFunc()
			}
			tm.tenants = append(tm.tenants[:i], tm.tenants[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("tenant %s not found", id)
}

// List returns all tenant configs (no secrets).
func (tm *TenantManager) List() []map[string]any {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]map[string]any, len(tm.tenants))
	for i, lt := range tm.tenants {
		result[i] = map[string]any{
			"id":           lt.ID,
			"hosts":        lt.Hosts,
			"frontend_url": lt.FrontendURL,
		}
	}
	return result
}

// HasHost returns true if the given domain is registered to any tenant.
// Used by Caddy on-demand TLS to verify domains before issuing certificates.
func (tm *TenantManager) HasHost(domain string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	host := strings.ToLower(domain)
	if _, ok := tm.hostMap[host]; ok {
		return true
	}
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		if _, ok := tm.hostMap[host[:idx]]; ok {
			return true
		}
	}
	return false
}

// ServeHTTP dispatches requests by Host header. This is the main handler.
func (tm *TenantManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Check X-Forwarded-Host first (SSR/proxy), then Host
	host := strings.ToLower(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.ToLower(r.Host)
	}

	// Exact match
	if lt, ok := tm.hostMap[host]; ok {
		lt.Router.ServeHTTP(w, r)
		return
	}
	// Strip port and retry
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		if lt, ok := tm.hostMap[host[:idx]]; ok {
			lt.Router.ServeHTTP(w, r)
			return
		}
	}
	// Fallback to first tenant (localhost dev)
	if len(tm.tenants) > 0 {
		tm.tenants[0].Router.ServeHTTP(w, r)
		return
	}
	http.Error(w, `{"msg":"No tenants configured"}`, http.StatusNotFound)
}

// GetTenant returns a tenant by ID, or nil if not found.
func (tm *TenantManager) GetTenant(id string) *LiveTenant {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, lt := range tm.tenants {
		if lt.ID == id {
			return lt
		}
	}
	return nil
}

// Tenants returns the live tenant slice (read-only snapshot for shutdown).
func (tm *TenantManager) Tenants() []*LiveTenant {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]*LiveTenant, len(tm.tenants))
	copy(out, tm.tenants)
	return out
}
