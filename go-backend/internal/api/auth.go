package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
)

// RegisterAuth mounts authentication routes (public, no JWT required).
func RegisterAuth(r chi.Router, store *db.Store, jwtSecret string) {
	r.Post("/auth/login", handleLogin(store, jwtSecret))
	r.Get("/auth/me", handleMe(store, jwtSecret))
	r.Get("/auth/logout", handleLogout())
	r.Post("/auth/logout", handleLogout())
}

func handleLogin(store *db.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Provider string `json:"providerName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"msg":"Invalid request"}`, http.StatusBadRequest)
			return
		}

		user, err := store.GetUserByEmail(r.Context(), body.Email)
		if err != nil {
			http.Error(w, `{"msg":"Invalid credentials"}`, http.StatusUnauthorized)
			return
		}

		// TODO: verify password with argon2 (Postiz uses argon2id)
		_ = user

		// Get user's first org
		orgID, _ := store.GetUserDefaultOrgID(r.Context(), user.ID)

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"id":    user.ID,
			"orgId": orgID,
			"exp":   time.Now().Add(24 * time.Hour * 30).Unix(),
		})

		tokenStr, err := token.SignedString([]byte(jwtSecret))
		if err != nil {
			http.Error(w, `{"msg":"Token generation failed"}`, http.StatusInternalServerError)
			return
		}

		// Set cookie (same as Postiz)
		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    tokenStr,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			MaxAge:   60 * 60 * 24 * 30,
		})

		writeJSON(w, http.StatusOK, map[string]string{"token": tokenStr})
	}
}

func handleMe(store *db.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from cookie or header (same order as middleware)
		tokenStr := ""
		if auth := r.Header.Get("Authorization"); auth != "" {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		} else if auth := r.Header.Get("auth"); auth != "" {
			tokenStr = auth
		} else if cookie, err := r.Cookie("auth"); err == nil {
			tokenStr = cookie.Value
		}

		if tokenStr == "" {
			http.Error(w, `{"msg":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}

		claims := &jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			http.Error(w, `{"msg":"Invalid token"}`, http.StatusUnauthorized)
			return
		}

		userID, _ := (*claims)["id"].(string)
		user, err := store.GetUserByID(r.Context(), userID)
		if err != nil {
			http.Error(w, `{"msg":"User not found"}`, http.StatusUnauthorized)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		})
	}
}

func handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:   "auth",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
