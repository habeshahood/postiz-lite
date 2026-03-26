package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/habeshahood/postiz-lite/internal/db"
)

const UserContextKey contextKey = "user"

// JWTAuth validates JWT tokens from the Next.js frontend session cookies.
func JWTAuth(secret string, store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				http.Error(w, `{"msg":"Missing token"}`, http.StatusUnauthorized)
				return
			}

			claims := &jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"msg":"Invalid token"}`, http.StatusUnauthorized)
				return
			}

			userID, _ := (*claims)["id"].(string)
			orgID, _ := (*claims)["orgId"].(string)

			if userID == "" {
				http.Error(w, `{"msg":"Invalid token claims"}`, http.StatusUnauthorized)
				return
			}

			user, err := store.GetUserByID(r.Context(), userID)
			if err != nil {
				http.Error(w, `{"msg":"User not found"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, UserContextKey, user)
			if orgID != "" {
				org, err := store.GetOrgByID(ctx, orgID)
				if err == nil {
					ctx = context.WithValue(ctx, OrgContextKey, org)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	// Check Authorization header first
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Check cookie (Next.js stores JWT in auth cookie)
	if cookie, err := r.Cookie("auth"); err == nil {
		return cookie.Value
	}
	return ""
}

// GetUser extracts the User from the request context.
func GetUser(r *http.Request) *db.User {
	user, _ := r.Context().Value(UserContextKey).(*db.User)
	return user
}
