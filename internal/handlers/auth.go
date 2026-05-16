// Package handlers contains HTTP handlers for the chi router.
// Each handler decodes/validates its request, calls the appropriate service,
// and writes a JSON response — mirroring the pattern in src/controllers/.
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

// AuthHandler groups all /api/auth HTTP handlers.
type AuthHandler struct {
	svc *services.AuthService
	cfg *config.Config
}

// NewAuthHandler creates an AuthHandler wired to the given service and config.
func NewAuthHandler(svc *services.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{svc: svc, cfg: cfg}
}

// writeJSON serialises v as JSON and sets the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Routes mounts all /api/auth endpoints on a fresh chi.Router and returns it.
// This is called from main.go: r.Mount("/api/auth", handlers.NewAuthHandler(...).Routes())
func (h *AuthHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware.NoCache)

	// Public — rate limiting is applied per-route (Phase 9); for now, open
	r.Post("/signup/jobseeker", h.SignupJobSeeker)
	r.Post("/signup/recruiter", h.SignupRecruiter)
	r.Post("/login", h.Login)
	r.Post("/logout", h.Logout)
	r.Post("/refresh", h.Refresh)

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
		r.Get("/me", h.GetMe)
		r.Post("/upgrade/recruiter",
			middleware.AuthorizeRoles("JOB_SEEKER")(http.HandlerFunc(h.UpgradeToRecruiter)).ServeHTTP)
	})

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// SignupJobSeeker godoc: POST /api/auth/signup/jobseeker
// Mirrors signupJobSeeker controller in auth.controller.ts.
func (h *AuthHandler) SignupJobSeeker(w http.ResponseWriter, r *http.Request) {
	body, err := middleware.DecodeAndValidate[models.SignupJobSeekerRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	at, rt, err := h.svc.SignupJobSeeker(r.Context(), body.Name, body.Email, body.Password)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusCreated, models.MessageResponse{Message: "Signed up successfully"})
}

// SignupRecruiter godoc: POST /api/auth/signup/recruiter
// Mirrors signupRecruiter controller in auth.controller.ts.
func (h *AuthHandler) SignupRecruiter(w http.ResponseWriter, r *http.Request) {
	body, err := middleware.DecodeAndValidate[models.SignupRecruiterRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

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
// Mirrors login controller in auth.controller.ts.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	body, err := middleware.DecodeAndValidate[models.LoginRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	at, rt, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Logged in successfully"})
}

// Logout godoc: POST /api/auth/logout
// Mirrors logout controller — just clears both HttpOnly cookies.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	utils.ClearAuthCookies(w)
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Logged out successfully"})
}

// Refresh godoc: POST /api/auth/refresh
// Mirrors refreshTokens controller in auth.controller.ts.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
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

// GetMe godoc: GET /api/auth/me (requires RequireAuth middleware)
// Mirrors getMe controller in auth.controller.ts.
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
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

// UpgradeToRecruiter godoc: POST /api/auth/upgrade/recruiter (requires RequireAuth + JOB_SEEKER role)
// Mirrors upgradeToRecruiter controller in auth.controller.ts.
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

	// Re-issue cookies so the RECRUITER role is active immediately (no re-login required)
	utils.SetAccessTokenCookie(w, at, h.cfg.AccessTokenExpiry, h.cfg.IsProduction())
	utils.SetRefreshTokenCookie(w, rt, h.cfg.RefreshTokenExpiry, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, models.UpgradeResponse{User: user})
}


