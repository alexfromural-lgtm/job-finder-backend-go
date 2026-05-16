// Package models contains request/response DTO structs used by handlers and services.
package models

import "time"

// ── Job Seeker request bodies ──────────────────────────────────────────────────

// UpdateJobSeekerProfileRequest mirrors updateJobSeekerProfileSchema (jobseeker.schema.ts).
// All fields are optional — nil means "keep existing value" (COALESCE in SQL).
type UpdateJobSeekerProfileRequest struct {
	Bio        *string  `json:"bio"        validate:"omitempty,max=500"`
	Location   *string  `json:"location"   validate:"omitempty,max=100"`
	Skills     []string `json:"skills"     validate:"omitempty"`
	Education  *string  `json:"education"  validate:"omitempty"`
	Experience *string  `json:"experience" validate:"omitempty"`
	ResumeURL  *string  `json:"resumeUrl"  validate:"omitempty,url"`
}

// ApplyToJobRequest carries the optional cover letter for a job application.
// Mirrors applicationSchema from application.schema.ts.
type ApplyToJobRequest struct {
	CoverLetter string `json:"coverLetter" validate:"omitempty,max=2000"`
}

// ── Job Seeker response bodies ─────────────────────────────────────────────────

// UserSummary is the safe subset of user fields included inside profile responses.
type UserSummary struct {
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

// JobSeekerProfileResponse is the full profile row with user info joined.
// Mirrors the Prisma include: { user: { select: { name, email, roles } } }
type JobSeekerProfileResponse struct {
	ID         string      `json:"id"`
	UserID     string      `json:"userId"`
	Bio        string      `json:"bio"`
	Location   string      `json:"location"`
	Skills     []string    `json:"skills"`
	Education  string      `json:"education"`
	Experience string      `json:"experience"`
	ResumeURL  string      `json:"resumeUrl"`
	CreatedAt  time.Time   `json:"createdAt"`
	UpdatedAt  time.Time   `json:"updatedAt"`
	User       UserSummary `json:"user"`
}

// ProfileResponse wraps the profile — matches Node.js: res.json({ profile })
type ProfileResponse struct {
	Profile JobSeekerProfileResponse `json:"profile"`
}

// JobSummaryInApplication is the minimal job info inside an application row.
type JobSummaryInApplication struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Location    string `json:"location"`
	SalaryRange string `json:"salaryRange"`
	Category    string `json:"category"`
	CompanyName string `json:"companyName"`
}

// ApplicationRow represents a job application with embedded job summary.
// Mirrors the Prisma include shape in getApplications().
type ApplicationRow struct {
	ID          string                  `json:"id"`
	JobID       string                  `json:"jobId"`
	JobSeekerID string                  `json:"jobSeekerId"`
	CoverLetter string                  `json:"coverLetter"`
	Status      string                  `json:"status"`
	CreatedAt   time.Time               `json:"createdAt"`
	UpdatedAt   time.Time               `json:"updatedAt"`
	Job         JobSummaryInApplication `json:"job"`
}

// ApplicationsResponse wraps a list — matches Node.js: res.json({ applications })
type ApplicationsResponse struct {
	Applications []ApplicationRow `json:"applications"`
}

// JobSummaryInSaved is the minimal job info inside a saved-job row.
type JobSummaryInSaved struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Location    string `json:"location"`
	SalaryRange string `json:"salaryRange"`
	Category    string `json:"category"`
	IsActive    bool   `json:"isActive"`
	CompanyName string `json:"companyName"`
}

// SavedJobRow represents a saved-job entry with embedded job summary.
type SavedJobRow struct {
	ID          string            `json:"id"`
	JobID       string            `json:"jobId"`
	JobSeekerID string            `json:"jobSeekerId"`
	SavedAt     time.Time         `json:"savedAt"`
	Job         JobSummaryInSaved `json:"job"`
}

// SavedJobsResponse wraps the list — matches Node.js: res.json({ savedJobs })
type SavedJobsResponse struct {
	SavedJobs []SavedJobRow `json:"savedJobs"`
}

// QueuedResponse is returned for async operations (202 Accepted).
// Mirrors Node.js: res.status(202).json({ queueJobId: job.id, status: "queued" })
type QueuedResponse struct {
	QueueJobID string `json:"queueJobId"`
	Status     string `json:"status"`
}
