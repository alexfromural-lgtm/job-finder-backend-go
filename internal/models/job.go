// Package models contains request/response DTO structs used by handlers and services.
package models

import "time"

// ── Job request bodies ─────────────────────────────────────────────────────────

// CreateJobRequest mirrors jobSchema from src/validators/job.schema.ts.
type CreateJobRequest struct {
	Title        string `json:"title"        validate:"required,min=1,max=100"`
	Description  string `json:"description"  validate:"required,min=1"`
	Requirements string `json:"requirements" validate:"required,min=1"`
	Location     string `json:"location"     validate:"required,min=1"`
	SalaryRange  string `json:"salaryRange"  validate:"omitempty"`
	Category     string `json:"category"     validate:"omitempty"`
}

// UpdateJobRequest is a partial version — all fields optional.
// Mirrors jobSchema.partial() used on PUT /:id.
type UpdateJobRequest struct {
	Title        *string `json:"title"        validate:"omitempty,min=1,max=100"`
	Description  *string `json:"description"  validate:"omitempty,min=1"`
	Requirements *string `json:"requirements" validate:"omitempty,min=1"`
	Location     *string `json:"location"     validate:"omitempty,min=1"`
	SalaryRange  *string `json:"salaryRange"  validate:"omitempty"`
	Category     *string `json:"category"     validate:"omitempty"`
}

// ── Job response bodies ────────────────────────────────────────────────────────

// JobRow is the flattened job row returned by all job queries
// (includes company_name from the JOIN with recruiter_profiles).
type JobRow struct {
	ID           string    `json:"id"`
	RecruiterID  string    `json:"recruiterId"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Requirements string    `json:"requirements"`
	Location     string    `json:"location"`
	SalaryRange  string    `json:"salaryRange"`
	Category     string    `json:"category"`
	IsActive     bool      `json:"isActive"`
	CompanyName  string    `json:"companyName"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// JobsResponse wraps a list of jobs — matches Node.js: res.json({ jobs })
type JobsResponse struct {
	Jobs []JobRow `json:"jobs"`
}

// JobResponse wraps a single job — matches Node.js: res.json({ job })
type JobResponse struct {
	Job JobRow `json:"job"`
}

// JobCreatedResponse is returned by POST /api/jobs.
// Matches Node.js: res.json({ message: "...", job })
type JobCreatedResponse struct {
	Message string `json:"message"`
	Job     JobRow `json:"job"`
}

// JobUpdatedResponse is returned by PUT /api/jobs/:id.
type JobUpdatedResponse struct {
	Message string `json:"message"`
	Job     JobRow `json:"job"`
}

// JobsListResponse is returned by GET /api/jobs/all with pagination metadata.
// Matches Node.js: res.json({ jobs, meta: { total, page, pageSize, totalPages } })
type JobsListResponse struct {
	Jobs []JobRow      `json:"jobs"`
	Meta JobsListMeta  `json:"meta"`
}

// JobsListMeta holds pagination info mirroring the Node.js meta object.
type JobsListMeta struct {
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalPages int `json:"totalPages"`
}
