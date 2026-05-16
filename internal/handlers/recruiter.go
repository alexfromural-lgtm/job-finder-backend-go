// Package handlers contains HTTP handlers for the chi router.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
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

// Routes mounts all /api/recruiter endpoints. All routes require RECRUITER role.
// Mirrors recruiter.route.ts.
func (h *RecruiterHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
	r.Use(middleware.AuthorizeRoles("RECRUITER"))

	// Profile
	r.Get("/profile", h.GetProfile)
	r.Patch("/profile", h.UpdateProfile)

	// Application management
	r.Get("/jobs/{jobId}/applications", h.GetApplicationsForJob)
	r.Patch("/applications/{id}/status", h.UpdateApplicationStatus)

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GetProfile godoc: GET /api/recruiter/profile
// Mirrors getProfile controller in recruiter.controller.ts.
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
// Mirrors updateProfile controller in recruiter.controller.ts.
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
// Verifies recruiter ownership of the job before returning applicants.
// Mirrors getApplicationsForJob controller in recruiter.controller.ts.
func (h *RecruiterHandler) GetApplicationsForJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	jobID := chi.URLParam(r, "jobId")

	apps, err := h.svc.GetApplicationsForJob(r.Context(), userID, jobID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.ApplicationsForJobResponse{Applications: apps})
}

// UpdateApplicationStatus godoc: PATCH /api/recruiter/applications/:id/status
// Verifies recruiter owns the job the application belongs to.
// Mirrors updateApplicationStatus controller in recruiter.controller.ts.
func (h *RecruiterHandler) UpdateApplicationStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	appID := chi.URLParam(r, "id")

	body, err := middleware.DecodeAndValidate[models.UpdateApplicationStatusRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	application, err := h.svc.UpdateApplicationStatus(r.Context(), userID, appID, body.Status)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.UpdatedApplicationResponse{Application: application})
}
