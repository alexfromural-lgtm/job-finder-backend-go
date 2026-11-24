// ============================================================
// FILE: internal/handlers/job.go
// ROLE: HTTP handlers for all /api/jobs/* endpoints.
//
// 📚 WHAT IS THIS FILE?
//   Job listing CRUD endpoints:
//   - Public: GET /api/jobs/all (search/filter/paginate), GET /api/jobs/:id
//   - Recruiter-only: POST /api/jobs, PUT /api/jobs/:id, DELETE /api/jobs/:id
//   - Recruiter-only: GET /api/jobs/recruiter (my posted jobs)
//
// 📝 ROUTE ORDER MATTERS IN CHI:
//   /recruiter must be declared BEFORE /{id} to prevent chi from treating
//   "recruiter" as a job UUID parameter.
//   Chi routes are matched in declaration order within a router group.
//
// 📝 QUERY PARAMETERS:
//   GET /api/jobs/all?search=engineer&category=backend&location=NYC&page=2&pageSize=5
//   r.URL.Query().Get("search") reads query params safely (empty string if absent).
//   queryInt() converts string → int with a fallback for invalid values.
//
// 📝 COMPARISON with Node.js:
//   job.controller.ts → JobHandler methods
//   job.route.ts      → Routes()
// ============================================================
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

// Routes creates the chi sub-router for all /api/jobs/* routes.
//
// 📝 ROUTE GROUPS WITH MIDDLEWARE:
//   r.Group(func(r chi.Router) {
//       r.Use(RequireAuth(...))         // apply auth to this group only
//       r.Use(AuthorizeRoles(...))      // apply role check on top
//       r.Get("/recruiter", handler)   // route in this group
//   })
//   Routes outside the Group() have no auth (public routes).
//
// 📝 WHY SPLIT INTO TWO GROUPS?
//   Group 1: /recruiter (no URL param) must come BEFORE /{id} (wildcard).
//   Group 2: /{id} routes (put/delete) are after the public GET /{id}.
//   This ordering prevents "recruiter" from being matched as a job ID.
func (h *JobHandler) Routes() http.Handler {
	r := chi.NewRouter()

	// ── Public routes ─────────────────────────────────────────────────────────
	r.Get("/all", h.GetAllJobs) // GET /api/jobs/all?search=...

	// ── Recruiter-only: list own jobs + create ────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(h.cfg.AccessTokenSecret))
		r.Use(middleware.AuthorizeRoles("RECRUITER"))

		r.Get("/recruiter", h.GetJobsByRecruiter) // MUST be before /{id}
		r.Post("/", h.CreateJob)
	})

	// ── Public: get a specific job by ID ─────────────────────────────────────
	// {id} is a wildcard — matches any string. Declared AFTER /recruiter so
	// "recruiter" is not captured as a job ID.
	r.Get("/{id}", h.GetJobByID)

	// ── Recruiter-only: update/delete a specific job ──────────────────────────
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
// Supports query parameters: ?search=, ?category=, ?location=, ?page=, ?pageSize=
//
// 📝 r.URL.Query():
//   Returns url.Values, a map[string][]string.
//   .Get("key") returns the first value for the key, or "" if absent.
//
// 📝 EXAMPLE REQUESTS:
//   GET /api/jobs/all                              → all active jobs (page 1, size 10)
//   GET /api/jobs/all?search=golang&page=2        → search + page 2
//   GET /api/jobs/all?category=backend&location=NYC → filtered
func (h *JobHandler) GetAllJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	category := q.Get("category")
	location := q.Get("location")
	page := queryInt(q.Get("page"), 1)        // default page = 1
	pageSize := queryInt(q.Get("pageSize"), 10) // default page size = 10

	result, err := h.svc.GetAllJobs(r.Context(), search, category, location, page, pageSize)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetJobByID godoc: GET /api/jobs/:id
// Returns a single job with its company name.
//
// 📝 chi.URLParam(r, "id") reads the {id} segment from the URL path.
//   Route: /api/jobs/{id}
//   URL:   /api/jobs/abc-123-def
//   Result: "abc-123-def"
func (h *JobHandler) GetJobByID(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	job, err := h.svc.GetJobByID(r.Context(), jobID)
	if err != nil {
		middleware.WriteError(w, err) // 404 if not found
		return
	}

	writeJSON(w, http.StatusOK, models.JobResponse{Job: job})
}

// GetJobsByRecruiter godoc: GET /api/jobs/recruiter (RECRUITER only)
// Returns all jobs posted by the authenticated recruiter.
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
// Creates a new job listing for the authenticated recruiter.
//
// 📝 REQUEST:
//   {
//     "title": "Senior Go Developer",
//     "description": "...",
//     "requirements": "...",
//     "location": "Remote",
//     "salaryRange": "$120k-$150k",    // optional
//     "category": "backend"            // optional
//   }
// 📝 RESPONSE: 201 {"message":"Job created successfully!","job":{...}}
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
// Applies partial updates to a job (PATCH semantics despite using PUT verb).
// Only the fields present in the request body are updated.
//
// 📝 WHY PUT WITH PATCH SEMANTICS?
//   The original Node.js API used PUT for partial updates.
//   We preserve this behavior for frontend compatibility even though
//   REST convention would use PATCH for partial updates.
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
		middleware.WriteError(w, err) // 403 if not owner, 404 if not found
		return
	}

	writeJSON(w, http.StatusOK, models.JobUpdatedResponse{
		Message: "Job updated successfully!",
		Job:     job,
	})
}

// DeleteJob godoc: DELETE /api/jobs/:id (RECRUITER only)
// Permanently deletes a job listing (hard delete).
// The service layer verifies the recruiter owns the job before deleting.
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

// queryInt safely parses a query-string value as an integer.
// Returns `fallback` if the value is empty, non-numeric, or less than 1.
//
// 📝 WHY A HELPER?
//   r.URL.Query().Get("page") always returns a string (or "").
//   strconv.Atoi("") would return an error; we want a default value instead.
//   This helper centralises the parsing logic.
//
// 📝 EXAMPLE:
//   queryInt("2", 1)   → 2
//   queryInt("", 1)    → 1  (default)
//   queryInt("abc", 1) → 1  (parse error → default)
//   queryInt("-5", 1)  → 1  (negative → default)
func queryInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s) // Atoi = ASCII to integer
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
