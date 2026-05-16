// Package handlers contains HTTP handlers for the chi router.
package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
)

// JobHandler groups all /api/jobs HTTP handlers.
type JobHandler struct {
	svc *services.JobService
	cfg *config.Config
}

// NewJobHandler constructs a JobHandler.
func NewJobHandler(svc *services.JobService, cfg *config.Config) *JobHandler {
	return &JobHandler{svc: svc, cfg: cfg}
}

// Routes mounts all /api/jobs endpoints on a fresh chi.Router.
// Route order matters: /recruiter must be declared before /:id to prevent wildcard capture.
// Mirrors job.route.ts.
func (h *JobHandler) Routes() http.Handler {
	r := chi.NewRouter()

	// ── Public ────────────────────────────────────────────────────────────────
	r.Get("/all", h.GetAllJobs)

	// ── RECRUITER-only ────────────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
		r.Use(middleware.AuthorizeRoles("RECRUITER"))

		r.Get("/recruiter", h.GetJobsByRecruiter) // must be before /:id
		r.Post("/", h.CreateJob)
	})

	// ── Public + wildcard (must be declared AFTER /recruiter) ─────────────────
	r.Get("/{id}", h.GetJobByID)

	// ── RECRUITER-only with job ID ─────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
		r.Use(middleware.AuthorizeRoles("RECRUITER"))

		r.Put("/{id}", h.UpdateJob)
		r.Delete("/{id}", h.DeleteJob)
	})

	return r
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GetAllJobs godoc: GET /api/jobs/all
// Supports ?search=, ?category=, ?location=, ?page=, ?pageSize= query params.
// Mirrors getAllJobs controller in job.controller.ts.
func (h *JobHandler) GetAllJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	category := q.Get("category")
	location := q.Get("location")
	page := queryInt(q.Get("page"), 1)
	pageSize := queryInt(q.Get("pageSize"), 10)

	result, err := h.svc.GetAllJobs(r.Context(), search, category, location, page, pageSize)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetJobByID godoc: GET /api/jobs/:id
// Mirrors getJobById controller in job.controller.ts.
func (h *JobHandler) GetJobByID(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	job, err := h.svc.GetJobByID(r.Context(), jobID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.JobResponse{Job: job})
}

// GetJobsByRecruiter godoc: GET /api/jobs/recruiter (RECRUITER only)
// Mirrors getJobsByRecruiter controller in job.controller.ts.
func (h *JobHandler) GetJobsByRecruiter(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	jobs, err := h.svc.GetJobsByRecruiter(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.JobsResponse{Jobs: jobs})
}

// CreateJob godoc: POST /api/jobs (RECRUITER only)
// Mirrors createJob controller in job.controller.ts.
func (h *JobHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	body, err := middleware.DecodeAndValidate[models.CreateJobRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	job, err := h.svc.CreateJob(r.Context(), userID, body)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, models.JobCreatedResponse{
		Message: "Job created successfully!",
		Job:     job,
	})
}

// UpdateJob godoc: PUT /api/jobs/:id (RECRUITER only)
// Accepts a partial body — only supplied fields are updated.
// Mirrors updateJob controller in job.controller.ts.
func (h *JobHandler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	jobID := chi.URLParam(r, "id")

	body, err := middleware.DecodeAndValidate[models.UpdateJobRequest](r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	job, err := h.svc.UpdateJob(r.Context(), userID, jobID, body)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.JobUpdatedResponse{
		Message: "Job updated successfully!",
		Job:     job,
	})
}

// DeleteJob godoc: DELETE /api/jobs/:id (RECRUITER only)
// Mirrors deleteJob controller in job.controller.ts.
func (h *JobHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		middleware.WriteError(w, errUnauth())
		return
	}

	jobID := chi.URLParam(r, "id")

	if err := h.svc.DeleteJob(r.Context(), userID, jobID); err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "Job deleted successfully!"})
}

// ── internal helpers ──────────────────────────────────────────────────────────

// queryInt parses a query-string integer, returning fallback on error.
func queryInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
