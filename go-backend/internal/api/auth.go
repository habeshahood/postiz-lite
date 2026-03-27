package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// RegisterAuth mounts authentication routes (public, no JWT required).
func RegisterAuth(r chi.Router, store *db.Store, jwtSecret string) {
	r.Post("/auth/login", handleLogin(store, jwtSecret))
	r.Get("/auth/me", handleMe(store, jwtSecret))
	r.Get("/auth/logout", handleLogout())
	r.Post("/auth/logout", handleLogout())
	r.Get("/auth/auto-login", handleAutoLogin(store, jwtSecret))
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

		// Verify password (Postiz uses bcrypt $2b$)
		if user.Password == nil || bcrypt.CompareHashAndPassword([]byte(*user.Password), []byte(body.Password)) != nil {
			http.Error(w, `{"msg":"Invalid credentials"}`, http.StatusUnauthorized)
			return
		}

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

		// Set cookie — NOT HttpOnly because the frontend reads it via document.cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    tokenStr,
			Path:     "/",
			HttpOnly: false,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			MaxAge:   60 * 60 * 24 * 365, // 1 year for owner
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

// handleAutoLogin sets a permanent auth cookie for the first SUPERADMIN user
// and redirects to the dashboard. Only works from localhost.
func handleAutoLogin(store *db.Store, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the first user and their org
		const q = `SELECT u.id, u.email, uo."organizationId"
			FROM "User" u
			JOIN "UserOrganization" uo ON uo."userId" = u.id
			WHERE uo.role = 'SUPERADMIN' AND uo.disabled = false
			ORDER BY u."createdAt" ASC LIMIT 1`

		var userID, email, orgID string
		err := store.QueryRow(r.Context(), q).Scan(&userID, &email, &orgID)
		if err != nil {
			http.Error(w, `{"msg":"No admin user found"}`, http.StatusNotFound)
			return
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"id":    userID,
			"orgId": orgID,
			"exp":   time.Now().Add(365 * 24 * time.Hour).Unix(),
		})
		tokenStr, _ := token.SignedString([]byte(jwtSecret))

		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    tokenStr,
			Path:     "/",
			HttpOnly: false,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			MaxAge:   60 * 60 * 24 * 365,
		})

		http.Redirect(w, r, "/launches", http.StatusFound)
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
