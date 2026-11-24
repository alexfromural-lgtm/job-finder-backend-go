// ============================================================
// FILE: internal/handlers/jobseeker.go
// ROLE: HTTP handlers for all /api/jobseeker/* endpoints.
//
// 📚 WHAT IS THIS FILE?
//   Job-seeker facing HTTP endpoints:
//   - Profile: GET/PATCH their personal profile
//   - Applications: apply (async), view, withdraw
//   - Saved Jobs: save (async), view, unsave
//
// 📝 ASYNC OPERATIONS (202 ACCEPTED):
//   ApplyToJob and SaveJob use the task queue instead of calling the service directly.
//   The handler:
//     1. Enqueues the task into Redis via h.qClient.EnqueueXxx()
//     2. Returns HTTP 202 Accepted immediately with the task ID
//     3. The queue worker picks up the task and calls the service in the background
//
//   This pattern keeps the HTTP response fast (< 10ms) even if the DB is slow.
//   The client polls GET /api/queue/job/:taskId to check completion.
//
// 📝 WHY A QUEUE CLIENT IN THE HANDLER?
//   This handler has two dependencies:
//   - svc  (*JobSeekerService)  for synchronous operations (get profile, etc.)
//   - qClient (*queue.Client)   for async operations (apply, save)
//
// 📝 COMPARISON with Node.js:
//   jobseeker.controller.ts → JobSeekerHandler methods
//   jobseeker.route.ts      → Routes()
// ============================================================
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
// Unlike RecruiterHandler, it also holds a queue client for async operations.
type JobSeekerHandler struct {
	svc     *services.JobSeekerService // synchronous service (profile, applications list)
	cfg     *config.Config
	qClient *queue.Client              // queue client (apply, save → async)
}

// NewJobSeekerHandler constructs a JobSeekerHandler with all dependencies.
func NewJobSeekerHandler(svc *services.JobSeekerService, cfg *config.Config, qClient *queue.Client) *JobSeekerHandler {
	return &JobSeekerHandler{svc: svc, cfg: cfg, qClient: qClient}
}

// Routes creates the chi sub-router for all /api/jobseeker/* routes.
// ALL routes require authentication and the JOB_SEEKER role.
//
// 📝 ROUTE MAP:
//   GET  /api/jobseeker/profile              → GetProfile
//   PATCH /api/jobseeker/profile             → UpdateProfile
//   POST  /api/jobseeker/apply/:jobId        → ApplyToJob (async → 202)
//   GET  /api/jobseeker/applications         → GetApplications
//   DELETE /api/jobseeker/applications/:id   → WithdrawApplication
//   POST  /api/jobseeker/saved/:jobId        → SaveJob (async → 202)
//   GET  /api/jobseeker/saved                → GetSavedJobs
//   DELETE /api/jobseeker/saved/:jobId       → UnsaveJob
func (h *JobSeekerHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
	r.Use(middleware.AuthorizeRoles("JOB_SEEKER"))

	// Profile management
	r.Get("/profile", h.GetProfile)
	r.Patch("/profile", h.UpdateProfile)

	// Job applications
	r.Post("/apply/{jobId}", h.ApplyToJob)          // enqueues async task
	r.Get("/applications", h.GetApplications)        // returns list
	r.Delete("/applications/{id}", h.WithdrawApplication)

	// Saved jobs
	r.Post("/saved/{jobId}", h.SaveJob)             // enqueues async task
	r.Get("/saved", h.GetSavedJobs)                 // returns list
	r.Delete("/saved/{jobId}", h.UnsaveJob)

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GetProfile godoc: GET /api/jobseeker/profile
// Returns the authenticated job seeker's full profile with user info.
//
// 📝 RESPONSE:
//   {
//     "profile": {
//       "id":"...", "bio":"...", "skills":["Go","React"],
//       "user":{"name":"Alice","email":"alice@example.com","roles":["JOB_SEEKER"]}
//     }
//   }
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
// Partially updates the job seeker's profile. Only provided fields are changed.
//
// 📝 REQUEST (all fields optional):
//   {
//     "bio": "Go developer with 5 years experience",
//     "skills": ["Go", "PostgreSQL", "Docker"],
//     "resumeUrl": "https://..."
//   }
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
// Enqueues an "apply-to-job" task and returns 202 Accepted immediately.
//
// 📝 WHY 202 ACCEPTED?
//   202 Accepted = "I've received your request and queued it for processing".
//   201 Created would imply the resource was created immediately.
//   The actual DB write happens asynchronously (in the queue worker).
//
// 📝 REQUEST:  {"coverLetter": "..."} (optional)
// 📝 RESPONSE: 202 {"queueJobId":"abc-123","status":"queued"}
// 📝 THEN POLL: GET /api/queue/job/abc-123 to check if it completed
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

	// Enqueue the task into Redis (fast — no DB call here)
	taskID, err := h.qClient.EnqueueApplyToJob(queue.ApplyToJobPayload{
		UserID:      userID,
		JobID:       jobID,
		CoverLetter: body.CoverLetter,
	})
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	// Return 202 with the task ID so the client can poll for completion
	writeJSON(w, http.StatusAccepted, models.QueuedResponse{QueueJobID: taskID, Status: "queued"})
}

// GetApplications godoc: GET /api/jobseeker/applications
// Returns all applications submitted by the authenticated job seeker.
// Each application includes a job summary (title, location, company, etc.).
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
// Removes an application. Only the applicant can withdraw their own application.
//
// 📝 OWNERSHIP CHECK:
//   The service layer verifies the application belongs to this user.
//   Another job seeker cannot withdraw someone else's application.
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
// Enqueues a "save-job" task and returns 202 Accepted immediately.
// Same async pattern as ApplyToJob.
//
// 📝 IDEMPOTENT:
//   Saving a job you already saved is safe (the worker uses ON CONFLICT upsert).
//   Multiple saves of the same job result in one saved_jobs row.
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
// Returns all jobs saved by the authenticated job seeker.
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
// Removes a job from the authenticated job seeker's saved list.
//
// 📝 404 IF NOT SAVED:
//   The service returns 404 if the job wasn't in the saved list.
//   This follows REST convention: DELETE on a non-existent resource = 404.
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
