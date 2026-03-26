package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/habeshahood/postiz-lite/internal/db"
)

type contextKey string

const OrgContextKey contextKey = "org"

// APIKeyAuth validates the Authorization header against Organization.apiKey.
// Postiz uses: Authorization: {API_KEY} (no "Bearer" prefix).
func APIKeyAuth(store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("Authorization")
			apiKey = strings.TrimPrefix(apiKey, "Bearer ")
			apiKey = strings.TrimSpace(apiKey)

			if apiKey == "" {
				http.Error(w, `{"msg":"Missing API key"}`, http.StatusUnauthorized)
				return
			}

			org, err := store.GetOrgByAPIKey(r.Context(), apiKey)
			if err != nil {
				http.Error(w, `{"msg":"Invalid API key"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), OrgContextKey, org)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetOrg extracts the Organization from the request context.
func GetOrg(r *http.Request) *db.Organization {
	org, _ := r.Context().Value(OrgContextKey).(*db.Organization)
	return org
}
