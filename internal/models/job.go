// ============================================================
// FILE: internal/models/job.go
// ROLE: Request & Response Data Transfer Objects (DTOs) for Job Listings.
//
// 📚 KEY CONCEPTS:
//   1. POINTERS FOR PARTIAL UPDATES (*string):
//      In Go, a normal `string` defaults to `""` (empty string) if omitted in JSON.
//      We cannot distinguish between "user sent empty string" vs "user didn't send field".
//      By using a pointer `*string`:
//        - field omitted in JSON  --> pointer is nil (don't update)
//        - field is `"value"`     --> pointer has address of "value" (update it)
//   2. PAGINATION METADATA:
//      Standard metadata (total, page, pageSize, totalPages) allows
//      frontend components (like pagination bars) to render properly.
// ============================================================
package models

import "time"

// ── Job Request Bodies ─────────────────────────────────────────────────────────

// CreateJobRequest defines the JSON schema for publishing a new job listing.
//
// 📝 EXAMPLE INCOMING JSON:
//   {
//     "title": "Go Backend Engineer",
//     "description": "Join our platform team building high-throughput APIs.",
//     "requirements": "3+ years Go experience, PostgreSQL, Docker",
//     "location": "Remote, US",
//     "salaryRange": "$130k - $160k",
//     "category": "Engineering"
//   }
type CreateJobRequest struct {
	Title        string `json:"title"        validate:"required,min=1,max=100"`
	Description  string `json:"description"  validate:"required,min=1"`
	Requirements string `json:"requirements" validate:"required,min=1"`
	Location     string `json:"location"     validate:"required,min=1"`
	SalaryRange  string `json:"salaryRange"  validate:"omitempty"`
	Category     string `json:"category"     validate:"omitempty"`
}

// UpdateJobRequest allows partial updates to an existing job listing.
//
// 💡 WHY POINTERS (*string)?
//   Pointers can be `nil`. If the client only wants to update the title:
//   `{"title": "Senior Go Engineer"}`
//   Then Title != nil, while Description == nil, Location == nil, etc.
//   This allows our service to update only the fields that were actually provided!
type UpdateJobRequest struct {
	Title        *string `json:"title"        validate:"omitempty,min=1,max=100"`
	Description  *string `json:"description"  validate:"omitempty,min=1"`
	Requirements *string `json:"requirements" validate:"omitempty,min=1"`
	Location     *string `json:"location"     validate:"omitempty,min=1"`
	SalaryRange  *string `json:"salaryRange"  validate:"omitempty"`
	Category     *string `json:"category"     validate:"omitempty"`
}

// ── Job Response Bodies ────────────────────────────────────────────────────────

// JobRow represents a complete job listing including the company name
// obtained via SQL JOIN with `recruiter_profiles`.
//
// 📝 EXAMPLE OUTGOING JSON:
//   {
//     "id": "e4b1c8f0-...",
//     "recruiterId": "a1b2c3d4-...",
//     "title": "Go Backend Engineer",
//     "description": "...",
//     "requirements": "...",
//     "location": "Remote",
//     "salaryRange": "$130k - $160k",
//     "category": "Engineering",
//     "isActive": true,
//     "companyName": "TechCorp Inc.",
//     "createdAt": "2026-11-23T19:00:00Z",
//     "updatedAt": "2026-11-23T19:00:00Z"
//   }
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

// JobsResponse wraps an array of job listings for recruiter job views.
// Matches Node.js response format: res.json({ jobs })
type JobsResponse struct {
	Jobs []JobRow `json:"jobs"`
}

// JobResponse wraps a single job listing.
// Matches Node.js response format: res.json({ job })
type JobResponse struct {
	Job JobRow `json:"job"`
}

// JobCreatedResponse is returned on HTTP 201 Created when a job is posted.
type JobCreatedResponse struct {
	Message string `json:"message"`
	Job     JobRow `json:"job"`
}

// JobUpdatedResponse is returned on HTTP 200 OK after updating a job.
type JobUpdatedResponse struct {
	Message string `json:"message"`
	Job     JobRow `json:"job"`
}

// JobsListResponse contains a paginated list of jobs along with metadata.
// Returned by GET /api/jobs/all.
//
// 📝 EXAMPLE OUTGOING JSON:
//   {
//     "jobs": [ ... ],
//     "meta": {
//       "total": 45,
//       "page": 1,
//       "pageSize": 10,
//       "totalPages": 5
//     }
//   }
type JobsListResponse struct {
	Jobs []JobRow     `json:"jobs"`
	Meta JobsListMeta `json:"meta"`
}

// JobsListMeta carries pagination numbers for front-end pagination controls.
type JobsListMeta struct {
	Total      int `json:"total"`      // Total matching records in database
	Page       int `json:"page"`       // Current page number (1-based)
	PageSize   int `json:"pageSize"`   // Maximum items per page
	TotalPages int `json:"totalPages"` // Calculated: Ceil(Total / PageSize)
}
