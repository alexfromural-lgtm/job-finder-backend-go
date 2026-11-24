// ============================================================
// FILE: internal/handlers/recruiter.go
// ROLE: HTTP handlers for all /api/recruiter/* endpoints.
//
// 📚 WHAT IS THIS FILE?
//   All recruiter-facing HTTP endpoints live here:
//   - Profile management (GET/PATCH their company profile)
//   - Application management (view applicants, change their status)
//
// 📝 ROUTE PROTECTION:
//   ALL routes in this handler require:
//     1. RequireAuth  — valid JWT (must be logged in)
//     2. AuthorizeRoles("RECRUITER") — must have RECRUITER role
//   Both are applied in Routes() so individual handlers don't need to repeat the check.
//
// 📝 errUnauth() HELPER:
//   A tiny local helper that returns a standard 401 AppError.
//   Keeps the handlers DRY — no need to retype apperrors.New("...", 401) everywhere.
//
// 📝 COMPARISON with Node.js:
//   recruiter.controller.ts → RecruiterHandler methods
//   recruiter.route.ts      → Routes()
// ============================================================
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
)

// RecruiterHandler groups all /api/recruiter HTTP handlers.
type RecruiterHandler struct {
	svc *services.RecruiterService
	cfg *config.Config
}

// NewRecruiterHandler constructs a RecruiterHandler.
func NewRecruiterHandler(svc *services.RecruiterService, cfg *config.Config) *RecruiterHandler {
	return &RecruiterHandler{svc: svc, cfg: cfg}
}

// errUnauth returns a standard 401 Unauthorized AppError.
// Package-level (not method) so all handlers in this package can use it.
//
// 📝 WHY A HELPER FUNCTION?
//   Avoids repeating: apperrors.New("Not authenticated", 401) in every handler.
//   Single point of change if the message ever needs updating.
func errUnauth() error {
	return apperrors.New("Not authenticated", 401)
}

// Routes creates the chi sub-router for all /api/recruiter/* routes.
// ALL routes require authentication and the RECRUITER role.
//
// 📝 WHY USE r.Use() HERE INSTEAD OF HANDLER-LEVEL CHECKS?
//   r.Use() applies middleware to EVERY route in this group at the router level.
//   This is safer than manually calling GetUserID() in each handler:
//   if a developer forgets the check in a new handler, it's still protected.
//
// 📝 ROUTE MAP:
//   GET  /api/recruiter/profile                     → GetProfile
//   PATCH /api/recruiter/profile                    → UpdateProfile
//   GET  /api/recruiter/jobs/:jobId/applications    → GetApplicationsForJob
//   PATCH /api/recruiter/applications/:id/status    → UpdateApplicationStatus
func (h *RecruiterHandler) Routes() http.Handler {
	r := chi.NewRouter()

	// Apply auth + role check to ALL routes in this group
	r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
	r.Use(middleware.AuthorizeRoles("RECRUITER"))

	// Profile management
	r.Get("/profile", h.GetProfile)
	r.Patch("/profile", h.UpdateProfile)

	// Application management (jobs the recruiter posted)
	r.Get("/jobs/{jobId}/applications", h.GetApplicationsForJob)
	r.Patch("/applications/{id}/status", h.UpdateApplicationStatus)

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GetProfile godoc: GET /api/recruiter/profile
// Returns the authenticated recruiter's company profile.
//
// 📝 RESPONSE:
//   {
//     "profile": {
//       "id": "...", "companyName": "TechCorp", ...,
//       "user": {"name": "Bob", "email": "bob@techcorp.com", "roles": ["RECRUITER"]}
//     }
//   }
func (h *RecruiterHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	profile, err := h.svc.GetRecruiterProfile(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.RecruiterProfileResponseWrapper{Profile: profile})
}

// UpdateProfile godoc: PATCH /api/recruiter/profile
// Updates the recruiter's company profile (partial update — only provided fields change).
//
// 📝 REQUEST EXAMPLE:
//   {"companyName": "New Name", "industry": "FinTech"}
//   Only companyName and industry change; other fields keep their current values.
func (h *RecruiterHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	body, err := middleware.DecodeAndValidate[models.UpdateRecruiterProfileRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	profile, err := h.svc.UpdateRecruiterProfile(r.Context(), userID, body)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.RecruiterProfileResponseWrapper{Profile: profile})
}

// GetApplicationsForJob godoc: GET /api/recruiter/jobs/:jobId/applications
// Returns all applications for a job posted by the authenticated recruiter.
// The service layer verifies the recruiter owns the job before returning data.
//
// 📝 chi.URLParam extracts :jobId from the route pattern.
//   Route: /jobs/{jobId}/applications
//   URL:   /api/recruiter/jobs/abc-123/applications
//   chi.URLParam(r, "jobId") → "abc-123"
func (h *RecruiterHandler) GetApplicationsForJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	jobID := chi.URLParam(r, "jobId")

	apps, err := h.svc.GetApplicationsForJob(r.Context(), userID, jobID)
	if err != nil {
		middleware.WriteError(w, err) // e.g. 403 if recruiter doesn't own the job
		return
	}

	writeJSON(w, http.StatusOK, models.ApplicationsForJobResponse{Applications: apps})
}

// UpdateApplicationStatus godoc: PATCH /api/recruiter/applications/:id/status
// Changes the status of a job application.
//
// 📝 VALID STATUS VALUES (enforced by struct validation):
//   submitted | shortlisted | rejected | under_review
//
// 📝 REQUEST:  {"status": "shortlisted"}
// 📝 RESPONSE: 200 {"application": {updated application object}}
func (h *RecruiterHandler) UpdateApplicationStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	appID := chi.URLParam(r, "id") // the application UUID

	body, err := middleware.DecodeAndValidate[models.UpdateApplicationStatusRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	application, err := h.svc.UpdateApplicationStatus(r.Context(), userID, appID, body.Status)
	if err != nil {
		middleware.WriteError(w, err) // e.g. 403 if recruiter doesn't own the job
		return
	}

	writeJSON(w, http.StatusOK, models.UpdatedApplicationResponse{Application: application})
}
