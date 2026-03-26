package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/middleware"
)

// RegisterInternal mounts all JWT-protected internal API routes (used by Next.js frontend).
func RegisterInternal(r chi.Router, store *db.Store) {
	// Posts
	r.Get("/posts", handleInternalListPosts(store))
	r.Post("/posts", handleInternalCreatePost(store))

	// Integrations
	r.Get("/integrations", handleInternalListIntegrations(store))

	// Settings
	r.Get("/user/self", handleSelf(store))
}

func handleInternalListPosts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		posts, err := store.ListPosts(r.Context(), org.ID)
		if err != nil {
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, posts)
	}
}

func handleInternalCreatePost(store *db.Store) http.HandlerFunc {
	return handleCreatePost(store) // reuse public handler
}

func handleInternalListIntegrations(store *db.Store) http.HandlerFunc {
	return handleListIntegrations(store) // reuse public handler
}

func handleSelf(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUser(r)
		if user == nil {
			http.Error(w, `{"msg":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		})
	}
}
