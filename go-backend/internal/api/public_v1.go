package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
	"github.com/habeshahood/postiz-lite/internal/middleware"
)

// RegisterPublicV1 mounts all /api/public/v1/* routes.
// These are the endpoints our Rust postiz_client.rs calls.
func RegisterPublicV1(r chi.Router, store *db.Store) {
	r.Get("/is-connected", handleIsConnected())
	r.Get("/integrations", handleListIntegrations(store))
	r.Post("/posts", handleCreatePost(store))
	r.Get("/posts", handleListPosts(store))
	r.Delete("/posts/{id}", handleDeletePost(store))
	r.Post("/upload", handleUpload(store))
	r.Get("/notifications", handleGetNotifications(store))
	r.Delete("/integrations/{id}", handleDeleteIntegration(store))
}

func handleIsConnected() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
	}
}

func handleListIntegrations(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		integrations, err := store.ListIntegrations(r.Context(), org.ID)
		if err != nil {
			slog.Error("list integrations", "error", err)
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}

		// Match Postiz response shape exactly
		type integrationResponse struct {
			ID         string  `json:"id"`
			Name       string  `json:"name"`
			Identifier string  `json:"identifier"`
			Picture    *string `json:"picture"`
			Disabled   bool    `json:"disabled"`
			Profile    *string `json:"profile"`
		}

		result := make([]integrationResponse, 0, len(integrations))
		for _, i := range integrations {
			result = append(result, integrationResponse{
				ID:         i.ID,
				Name:       i.Name,
				Identifier: i.ProviderIdentifier,
				Picture:    i.Picture,
				Disabled:   i.Disabled,
				Profile:    i.Profile,
			})
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func handleCreatePost(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"msg":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}

		post, err := store.CreatePostFromAPI(r.Context(), org.ID, body)
		if err != nil {
			slog.Error("create post", "error", err)
			http.Error(w, `{"msg":"Failed to create post"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, post)
	}
}

func handleListPosts(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		posts, err := store.ListPosts(r.Context(), org.ID)
		if err != nil {
			slog.Error("list posts", "error", err)
			http.Error(w, `{"msg":"Internal error"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"posts": posts})
	}
}

func handleDeletePost(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		postID := chi.URLParam(r, "id")

		if err := store.DeletePost(r.Context(), postID, org.ID); err != nil {
			slog.Error("delete post", "error", err)
			http.Error(w, `{"msg":"Failed to delete"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func handleUpload(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}

		// Parse multipart form (max 100MB)
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, `{"msg":"File too large or invalid"}`, http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"msg":"No file provided"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		media, err := store.SaveMediaFromUpload(r.Context(), org.ID, header.Filename, file)
		if err != nil {
			slog.Error("upload", "error", err)
			http.Error(w, `{"msg":"Upload failed"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, media)
	}
}

func handleGetNotifications(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		if org == nil {
			http.Error(w, `{"msg":"No org"}`, http.StatusUnauthorized)
			return
		}
		// Stub — return empty list for now
		writeJSON(w, http.StatusOK, []interface{}{})
	}
}

func handleDeleteIntegration(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := middleware.GetOrg(r)
		integrationID := chi.URLParam(r, "id")

		if err := store.DeleteIntegration(r.Context(), integrationID, org.ID); err != nil {
			slog.Error("delete integration", "error", err)
			http.Error(w, `{"msg":"Failed to delete"}`, http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
