// ============================================================
// FILE: internal/middleware/auth.go
// ROLE: JWT authentication and role-based authorisation middleware.
//
// 📚 WHAT IS MIDDLEWARE?
//   Middleware is a function that sits BETWEEN the router and the handler:
//   Request → [Middleware 1] → [Middleware 2] → [Handler] → Response
//
//   Each middleware can:
//     a) Inspect / modify the request or response
//     b) Call next.ServeHTTP(w, r) to pass control to the next layer
//     c) Return early (write a response itself) to block the request
//
// 📝 COMPARISON with Node.js:
//   const requireAuth = (req, res, next) => {
//     const token = req.cookies.accessToken
//     if (!token) return res.status(401).json({ error: "..." })
//     const claims = jwt.verify(token, secret)
//     req.user = claims
//     next()
//   }
//   → Go version: RequireAuth(secret) returns an http.Handler wrapper
//
// 📝 CONTEXT KEY PATTERN:
//   Go's context.Context is a key-value store passed through the request chain.
//   We store the authenticated userID in the context so handlers can read it
//   without global variables or thread-local storage.
//   This is the idiomatic Go equivalent of Node.js `req.user`.
// ============================================================
package middleware

import (
	"context"
	"net/http"
	"slices"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/utils"
)

// contextKey is a private type used as a context key.
// Using a named type (instead of a plain string) prevents collisions:
// another package's context.WithValue("userID", ...) won't clash with ours
// because they can't create a value of our unexported contextKey type.
//
// 📝 KEY CONCEPT — context key type safety:
//   var key contextKey = "userID"
//   ctx.Value(key)         → returns our value (correct key type)
//   ctx.Value("userID")    → returns nil  (different key type, different slot)
type contextKey string

// ctxUserID is the actual key used to store/retrieve the authenticated user's ID.
const ctxUserID contextKey = "userID"

// ctxUserRoles is the key used to store/retrieve the authenticated user's roles.
const ctxUserRoles contextKey = "userRoles"

// RequireAuth returns a middleware that:
//   1. Reads the "accessToken" HttpOnly cookie from the request
//   2. Verifies the JWT signature and expiry
//   3. Stores the userID + roles in the request context
//   4. Calls the next handler if valid; returns 401 if not
//
// 📝 USAGE (in a route definition):
//   r.Group(func(r chi.Router) {
//       r.Use(middleware.RequireAuth(cfg.AccessTokenSecret))
//       r.Get("/profile", handler.GetProfile)
//   })
//
// 📝 HOW TO READ THE USER IN A HANDLER:
//   userID, ok := middleware.GetUserID(r.Context())
//   if !ok { /* return 401 */ }
func RequireAuth(accessSecret string) func(http.Handler) http.Handler {
	// This is a closure — it "captures" accessSecret from the outer scope
	// and uses it every time a request comes through.
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// r.Cookie("accessToken") reads the named cookie from the request.
			// Returns http.ErrNoCookie if the cookie is absent.
			cookie, err := r.Cookie("accessToken")
			if err != nil {
				// No cookie → not authenticated
				WriteError(w, apperrors.New("Authentication required", 401))
				return // stop here — do NOT call next
			}

			// Verify the JWT: check signature + expiry
			// cookie.Value is the raw JWT string, e.g. "eyJhbGciOi..."
			claims, err := utils.VerifyToken(cookie.Value, accessSecret)
			if err != nil {
				// Expired, tampered, or malformed token
				WriteError(w, err) // WriteError handles jwt.ErrTokenExpired → 401
				return
			}

			// Token is valid — store user identity in the request context.
			// context.WithValue creates a NEW context derived from the existing one,
			// with an additional key-value pair attached.
			//
			// 📝 WHY NOT modify r directly?
			//   http.Request is shared across middleware; modifying it directly
			//   causes data races. context is the safe, idiomatic Go pattern.
			ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
			ctx = context.WithValue(ctx, ctxUserRoles, claims.Roles)

			// r.WithContext(ctx) returns a copy of r with the new context.
			// next.ServeHTTP passes control to the next middleware or handler.
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthorizeRoles returns a middleware that only allows users whose roles
// contain AT LEAST ONE of the allowed roles.
//
// 📝 EXAMPLE:
//   r.Use(middleware.AuthorizeRoles("RECRUITER"))
//   // Only users with the RECRUITER role can reach these routes.
//   // JOB_SEEKER users get 403 Forbidden.
//
// 📝 HOW IT COMPOSES WITH RequireAuth:
//   RequireAuth runs first (puts roles in context)
//   AuthorizeRoles runs second (checks the roles)
func AuthorizeRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read the roles stored by RequireAuth middleware.
			// If RequireAuth wasn't called first, roles will be nil.
			roles, _ := r.Context().Value(ctxUserRoles).([]string)

			// slices.ContainsFunc iterates allowedRoles and checks if any
			// of them appear in the user's roles slice.
			//
			// 📝 EQUIVALENT JAVASCRIPT:
			//   allowedRoles.some(role => userRoles.includes(role))
			hasRole := slices.ContainsFunc(allowedRoles, func(allowed string) bool {
				return slices.Contains(roles, allowed)
			})

			if !hasRole {
				WriteError(w, apperrors.New("Access forbidden", 403))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID extracts the authenticated user's ID from a request context.
// Returns ("", false) if RequireAuth middleware wasn't applied to this route.
//
// 📝 USAGE IN HANDLERS:
//   userID, ok := middleware.GetUserID(r.Context())
//   if !ok {
//       middleware.WriteError(w, apperrors.New("Unauthorized", 401))
//       return
//   }
//   // userID is now a valid UUID string like "clx1234..."
func GetUserID(ctx context.Context) (string, bool) {
	// ctx.Value(ctxUserID) returns interface{} (any value or nil).
	// The .(string) type assertion converts it to string.
	// The second return value `ok` is false if the value is nil or not a string.
	id, ok := ctx.Value(ctxUserID).(string)
	return id, ok
}
