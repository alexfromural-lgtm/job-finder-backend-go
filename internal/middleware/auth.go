// Package middleware contains HTTP middleware for the chi router.
package middleware

import (
	"context"
	"net/http"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/utils"
)

// contextKey is an unexported type to avoid collisions with other packages
// that also store values in context.Context.
type contextKey string

const (
	// ContextKeyUserID is the context key for the authenticated user's ID.
	ContextKeyUserID contextKey = "userID"
	// ContextKeyRoles is the context key for the authenticated user's roles.
	ContextKeyRoles contextKey = "roles"
)

// RequireAuth reads the "accessToken" HttpOnly cookie, verifies the JWT, and
// injects the userID and roles into the request context.
// Mirrors Node.js requireAuth from src/middleware/auth.middleware.ts.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("accessToken")
			if err != nil {
				// Cookie absent or malformed
				WriteError(w, apperrors.New("Missing access token", http.StatusUnauthorized))
				return
			}

			claims, err := utils.VerifyToken(cookie.Value, secret)
			if err != nil {
				WriteError(w, apperrors.New("Invalid or expired access token", http.StatusUnauthorized))
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyRoles, claims.Roles)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthorizeRoles returns a middleware that checks whether the authenticated
// user holds at least one of the required roles. RequireAuth must run first.
// Mirrors Node.js authorizeRoles() from src/middleware/auth.middleware.ts.
func AuthorizeRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRoles, ok := r.Context().Value(ContextKeyRoles).([]string)
			if !ok {
				WriteError(w, apperrors.New("Access denied.", http.StatusForbidden))
				return
			}
			for _, role := range userRoles {
				if _, found := allowed[role]; found {
					next.ServeHTTP(w, r)
					return
				}
			}
			WriteError(w, apperrors.New("Access denied.", http.StatusForbidden))
		})
	}
}

// GetUserID retrieves the authenticated user's ID from a request context.
// Returns ("", false) if RequireAuth has not run or the value is missing.
func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ContextKeyUserID).(string)
	return id, ok
}

// GetRoles retrieves the authenticated user's roles from a request context.
func GetRoles(ctx context.Context) ([]string, bool) {
	roles, ok := ctx.Value(ContextKeyRoles).([]string)
	return roles, ok
}
