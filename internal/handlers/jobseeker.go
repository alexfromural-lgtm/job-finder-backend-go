// Package handlers contains HTTP handlers for the chi router.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/queue"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
)

// JobSeekerHandler groups all /api/jobseeker HTTP handlers.
type JobSeekerHandler struct {
	svc    *services.JobSeekerService
	cfg    *config.Config
	qClient *queue.Client
}

// NewJobSeekerHandler constructs a JobSeekerHandler.
func NewJobSeekerHandler(svc *services.JobSeekerService, cfg *config.Config, qClient *queue.Client) *JobSeekerHandler {
	return &JobSeekerHandler{svc: svc, cfg: cfg, qClient: qClient}
}

// Routes mounts all /api/jobseeker endpoints. All routes require JOB_SEEKER role.
// Mirrors jobseeker.route.ts.
func (h *JobSeekerHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
	r.Use(middleware.AuthorizeRoles("JOB_SEEKER"))

	// Profile
	r.Get("/profile", h.GetProfile)
	r.Patch("/profile", h.UpdateProfile)

	// Applications
	r.Post("/apply/{jobId}", h.ApplyToJob)
	r.Get("/applications", h.GetApplications)
	r.Delete("/applications/{id}", h.WithdrawApplication)

	// Saved jobs
	r.Post("/saved/{jobId}", h.SaveJob)
	r.Get("/saved", h.GetSavedJobs)
	r.Delete("/saved/{jobId}", h.UnsaveJob)

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GetProfile godoc: GET /api/jobseeker/profile
func (h *JobSeekerHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	profile, err := h.svc.GetJobSeekerProfile(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.ProfileResponse{Profile: profile})
}

// UpdateProfile godoc: PATCH /api/jobseeker/profile
func (h *JobSeekerHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	body, err := middleware.DecodeAndValidate[models.UpdateJobSeekerProfileRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	profile, err := h.svc.UpdateJobSeekerProfile(r.Context(), userID, body)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.ProfileResponse{Profile: profile})
}

// ApplyToJob godoc: POST /api/jobseeker/apply/:jobId
// Enqueues an apply-to-job task and returns 202 immediately.
// Mirrors Node.js: dbWriteQueue.add({ type: "apply-to-job", ... })
func (h *JobSeekerHandler) ApplyToJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	jobID := chi.URLParam(r, "jobId")

	body, err := middleware.DecodeAndValidate[models.ApplyToJobRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	taskID, err := h.qClient.EnqueueApplyToJob(queue.ApplyToJobPayload{
		UserID:      userID,
		JobID:       jobID,
		CoverLetter: body.CoverLetter,
	})
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, models.QueuedResponse{QueueJobID: taskID, Status: "queued"})
}

// GetApplications godoc: GET /api/jobseeker/applications
func (h *JobSeekerHandler) GetApplications(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	apps, err := h.svc.GetApplications(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.ApplicationsResponse{Applications: apps})
}

// WithdrawApplication godoc: DELETE /api/jobseeker/applications/:id
func (h *JobSeekerHandler) WithdrawApplication(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	appID := chi.URLParam(r, "id")

	if err := h.svc.WithdrawApplication(r.Context(), userID, appID); err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Application withdrawn successfully"})
}

// SaveJob godoc: POST /api/jobseeker/saved/:jobId
// Enqueues a save-job task and returns 202 immediately.
// Mirrors Node.js: dbWriteQueue.add({ type: "save-job", ... })
func (h *JobSeekerHandler) SaveJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	jobID := chi.URLParam(r, "jobId")

	taskID, err := h.qClient.EnqueueSaveJob(queue.SaveJobPayload{
		UserID: userID,
		JobID:  jobID,
	})
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, models.QueuedResponse{QueueJobID: taskID, Status: "queued"})
}

// GetSavedJobs godoc: GET /api/jobseeker/saved
func (h *JobSeekerHandler) GetSavedJobs(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	saved, err := h.svc.GetSavedJobs(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.SavedJobsResponse{SavedJobs: saved})
}

// UnsaveJob godoc: DELETE /api/jobseeker/saved/:jobId
func (h *JobSeekerHandler) UnsaveJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}
	jobID := chi.URLParam(r, "jobId")

	if err := h.svc.UnsaveJob(r.Context(), userID, jobID); err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Job removed from saved list"})
}
