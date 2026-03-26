package social

import (
	"context"
	"fmt"
	"sync"

	"github.com/habeshahood/postiz-lite/internal/db"
)

// PostResult is returned by a provider after successfully publishing a post.
type PostResult struct {
	PostID     string // Platform-specific post ID
	ReleaseURL string // Public URL on the platform
}

// Provider is the interface every social media provider must implement.
// Only the Post method is required for Phase 2 — OAuth flows come in Phase 4.
type Provider interface {
	// Identifier returns the providerIdentifier string stored in the Integration table
	// (e.g. "x", "bluesky", "tiktok", "facebook", "youtube").
	Identifier() string

	// Post publishes content to the platform and returns the platform post ID + URL.
	Post(ctx context.Context, post *db.Post, integration *db.Integration) (*PostResult, error)
}

// Registry maps providerIdentifier strings to Provider implementations.
var (
	registry   = map[string]Provider{}
	registryMu sync.RWMutex
)

// Register adds a provider to the global registry. Call from init() in each provider file.
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.Identifier()] = p
}

// Get returns the provider for the given identifier, or an error if not found.
func Get(identifier string) (Provider, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[identifier]
	if !ok {
		return nil, fmt.Errorf("social: no provider registered for %q", identifier)
	}
	return p, nil
}

// Identifiers returns all registered provider identifiers.
func Identifiers() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	return ids
}
