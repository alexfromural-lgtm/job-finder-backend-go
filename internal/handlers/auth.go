// ============================================================
// FILE: internal/handlers/auth.go
// ROLE: HTTP handlers for all /api/auth/* endpoints.
//
// 📚 WHAT IS A HANDLER?
//   A handler is a function that receives an HTTP request and writes a response.
//   In Go the signature is: func(w http.ResponseWriter, r *http.Request)
//   - w = ResponseWriter: you write status code, headers, and body INTO it
//   - r = *Request: contains URL, method, headers, body, cookies, context
//
// 📝 THE HANDLER PATTERN IN THIS PROJECT:
//   Every handler follows the same 3-step pattern:
//     1. DECODE   → middleware.DecodeAndValidate[RequestType](r)
//     2. DELEGATE → h.svc.MethodName(r.Context(), ...)
//     3. RESPOND  → writeJSON(w, statusCode, responseBody)
//   Error at any step → middleware.WriteError(w, err) + return
//
// 📝 COMPARISON with Node.js controllers:
//   const login = async (req, res) => {
//     const { email, password } = loginSchema.parse(req.body)   ← step 1
//     const { at, rt } = await authService.login(email, password) ← step 2
//     setAccessTokenCookie(res, at)
//     res.json({ message: "Logged in" })                        ← step 3
//   }
//
// 📝 DEPENDENCY INJECTION:
//   AuthHandler holds a *AuthService (not a global).
//   This makes it easy to test: pass a fake service → test just the handler.
// ============================================================
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/utils"
)

// AuthHandler groups all /api/auth HTTP handlers and their dependencies.
// The struct is the Go equivalent of a Node.js controller class.
type AuthHandler struct {
	svc *services.AuthService // business logic layer
	cfg *config.Config        // needed for cookie expiry + production flag
}

// NewAuthHandler creates an AuthHandler with its dependencies injected.
// Called from main.go: handlers.NewAuthHandler(authSvc, cfg)
func NewAuthHandler(svc *services.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{svc: svc, cfg: cfg}
}

// writeJSON is a package-level helper that:
//   1. Sets Content-Type: application/json
//   2. Writes the HTTP status code
//   3. JSON-encodes v into the response body
//
// 📝 IMPORTANT: WriteHeader MUST be called before writing the body.
//   Once the body is written, the status code is locked.
//
// 📝 EXAMPLE:
//   writeJSON(w, http.StatusOK, models.MessageResponse{Message: "ok"})
//   // Response: HTTP 200 {"message":"ok"}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // _ discards the write error (nothing to do if connection dropped)
}

// Routes creates a sub-router for all /api/auth/* endpoints and returns it.
// main.go mounts it: r.Mount("/api/auth", authHandler.Routes())
//
// 📝 WHY A SUB-ROUTER?
//   chi.NewRouter() scoped to /api/auth keeps route definitions close to
//   the handler that handles them. It also lets us apply middleware only
//   to this group (e.g. NoCache applies to ALL auth routes here).
func (h *AuthHandler) Routes() http.Handler {
	r := chi.NewRouter()

	// NoCache adds Cache-Control: no-store to auth responses.
	// Auth responses (tokens, user data) must never be cached by browsers or proxies.
	r.Use(chiMiddleware.NoCache)

	// ── Public routes (no authentication required) ───────────────────────────
	// r.Post(pattern, handlerFunc) registers a handler for HTTP POST requests.
	r.Post("/signup/jobseeker", h.SignupJobSeeker) // create job-seeker account
	r.Post("/signup/recruiter", h.SignupRecruiter) // create recruiter account
	r.Post("/login", h.Login)                      // verify credentials → issue tokens
	r.Post("/logout", h.Logout)                    // clear auth cookies
	r.Post("/refresh", h.Refresh)                  // issue new token pair from refresh token

	// ── Protected routes (JWT required) ─────────────────────────────────────
	// r.Group() creates a sub-group that shares middleware.
	// All routes inside the func callback inherit RequireAuth.
	r.Group(func(r chi.Router) {
		// RequireAuth reads the accessToken cookie, verifies the JWT,
		// and stores the userID in the request context.
		r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))

		r.Get("/me", h.GetMe) // return the authenticated user's profile

		// UpgradeToRecruiter also requires the JOB_SEEKER role.
		// AuthorizeRoles wraps the handler inline — a bit verbose but explicit.
		r.Post("/upgrade/recruiter",
			middleware.AuthorizeRoles("JOB_SEEKER")(http.HandlerFunc(h.UpgradeToRecruiter)).ServeHTTP)
	})

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// SignupJobSeeker godoc: POST /api/auth/signup/jobseeker
// Creates a new user with the JOB_SEEKER role, issues JWT pair.
//
// 📝 REQUEST:  {"name":"Alice","email":"alice@example.com","password":"secret123"}
// 📝 RESPONSE: 201 {"message":"Signed up successfully"} + cookies set
func (h *AuthHandler) SignupJobSeeker(w http.ResponseWriter, r *http.Request) {
	// Step 1: Decode JSON body + validate struct tags
	body, err := middleware.DecodeAndValidate[models.SignupJobSeekerRequest](r)
	if err != nil {
		middleware.WriteError(w, err) // e.g. 400 "Validation failed: email is required"
		return
	}

	// Step 2: Call service (business logic + DB write)
	at, rt, err := h.svc.SignupJobSeeker(r.Context(), body.Name, body.Email, body.Password)
	if err != nil {
		middleware.WriteError(w, err) // e.g. 409 "User already exists"
		return
	}

	// Step 3: Set cookies + respond
	// SetAccessTokenCookie sets the short-lived access JWT as HttpOnly cookie
	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	// SetRefreshTokenCookie sets the long-lived refresh JWT as HttpOnly cookie
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusCreated, models.MessageResponse{Message: "Signed up successfully"})
}

// SignupRecruiter godoc: POST /api/auth/signup/recruiter
// Creates a new user with the RECRUITER role and a recruiter profile.
//
// 📝 REQUEST:  {"name":"Bob","email":"bob@example.com","password":"secret123",
//               "companyName":"TechCorp","companyWebsite":"https://tech.com",...}
// 📝 RESPONSE: 201 {"message":"Signed up successfully"} + cookies set
func (h *AuthHandler) SignupRecruiter(w http.ResponseWriter, r *http.Request) {
	body, err := middleware.DecodeAndValidate[models.SignupRecruiterRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	// Pass the full request struct to the service (it has more fields than job-seeker signup)
	at, rt, err := h.svc.SignupRecruiter(r.Context(), body)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusCreated, models.MessageResponse{Message: "Signed up successfully"})
}

// Login godoc: POST /api/auth/login
// Verifies email + password and issues a new JWT pair.
//
// 📝 REQUEST:  {"email":"alice@example.com","password":"secret123"}
// 📝 RESPONSE: 200 {"message":"Logged in successfully"} + cookies set
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	body, err := middleware.DecodeAndValidate[models.LoginRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	at, rt, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		middleware.WriteError(w, err) // e.g. 401 "Invalid email or password"
		return
	}

	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Logged in successfully"})
}

// Logout godoc: POST /api/auth/logout
// Clears both auth cookies (effectively logging the user out).
// No request body needed; no DB call needed.
//
// 📝 HOW "LOGOUT" WORKS WITH COOKIES:
//   The server responds with Set-Cookie headers that expire the cookies.
//   The browser deletes them and stops sending them on future requests.
//   The old access token is still technically valid until its expiry (15min),
//   but without the cookie the browser won't send it.
//   For immediate invalidation you'd maintain a token denylist in Redis.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	utils.ClearAuthCookies(w) // expire both cookies
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Logged out successfully"})
}

// Refresh godoc: POST /api/auth/refresh
// Issues a new access + refresh token pair from the refresh cookie.
//
// 📝 WHY THIS EXISTS:
//   The access token expires every 15 minutes (security).
//   Instead of forcing the user to log in again, the client silently
//   calls this endpoint with the 7-day refresh token to get a new access token.
//
// 📝 FLOW:
//   Browser detects 401 → calls POST /api/auth/refresh (refresh cookie auto-sent)
//   → new access + refresh cookies set → retry original request
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// Read the refreshToken cookie (not the accessToken)
	cookie, err := r.Cookie("refreshToken")
	if err != nil {
		middleware.WriteError(w, apperrors.New("No refresh token provided", 401))
		return
	}

	at, rt, err := h.svc.RefreshTokens(r.Context(), cookie.Value)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Token refreshed"})
}

// GetMe godoc: GET /api/auth/me (protected — requires RequireAuth middleware)
// Returns the authenticated user's profile (no password).
//
// 📝 HOW userID IS AVAILABLE:
//   RequireAuth middleware verified the JWT and stored claims.UserID
//   in the request context. GetUserID() retrieves it from the context.
//
// 📝 RESPONSE: 200 {"user":{"id":"...","name":"Alice","email":"...","roles":[...],...}}
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	// Extract the authenticated user's ID from the request context.
	// middleware.RequireAuth put it there after verifying the JWT.
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, apperrors.New("Not authenticated", 401))
		return
	}

	user, err := h.svc.GetCurrentUser(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MeResponse{User: user})
}

// UpgradeToRecruiter godoc: POST /api/auth/upgrade/recruiter
// (requires RequireAuth + AuthorizeRoles("JOB_SEEKER"))
// Adds the RECRUITER role to an existing job-seeker account and
// creates their recruiter profile. Re-issues tokens immediately.
//
// 📝 USE CASE:
//   A job-seeker decides to also post jobs. Rather than creating a new account,
//   they "upgrade" — their account gets the RECRUITER role added to it.
//
// 📝 REQUEST:  {"companyName":"My Company","companyWebsite":"https://..."}
// 📝 RESPONSE: 200 {"user":{...updated user with RECRUITER role...}} + new cookies
func (h *AuthHandler) UpgradeToRecruiter(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, apperrors.New("Not authenticated", 401))
		return
	}

	body, err := middleware.DecodeAndValidate[models.UpgradeToRecruiterRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	user, at, rt, err := h.svc.UpgradeToRecruiter(r.Context(), userID, body)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	// Re-issue cookies so the new RECRUITER role is reflected in the JWT immediately.
	// Without this, the user would need to logout + login to see the new role.
	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, models.UpgradeResponse{User: user})
}
