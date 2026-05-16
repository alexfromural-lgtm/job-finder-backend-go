// Package models contains request/response DTO structs used by handlers and services.
package models

import "time"

// ── Recruiter request bodies ───────────────────────────────────────────────────

// UpdateRecruiterProfileRequest mirrors updateRecruiterProfileSchema (recruiter-profile.schema.ts).
// All fields optional — nil → COALESCE keeps existing value.
type UpdateRecruiterProfileRequest struct {
	CompanyName    *string `json:"companyName"    validate:"omitempty,min=1"`
	CompanyWebsite *string `json:"companyWebsite" validate:"omitempty,url"`
	Description    *string `json:"description"    validate:"omitempty"`
	Industry       *string `json:"industry"       validate:"omitempty"`
}

// UpdateApplicationStatusRequest mirrors applicationStatusSchema (application.schema.ts).
// Status must be one of the four valid enum values.
type UpdateApplicationStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=submitted shortlisted rejected under_review"`
}

// ── Recruiter response bodies ──────────────────────────────────────────────────

// RecruiterProfileResponse is the full recruiter_profiles row with user info joined.
// Mirrors the Prisma include: { user: { select: { name, email, roles } } }
type RecruiterProfileResponse struct {
	ID             string      `json:"id"`
	UserID         string      `json:"userId"`
	CompanyName    string      `json:"companyName"`
	CompanyWebsite string      `json:"companyWebsite"`
	Description    string      `json:"description"`
	Industry       string      `json:"industry"`
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	User           UserSummary `json:"user"`
}

// RecruiterProfileResponseWrapper wraps a single profile.
// Matches Node.js: res.json({ profile })
type RecruiterProfileResponseWrapper struct {
	Profile RecruiterProfileResponse `json:"profile"`
}

// ApplicantSummary is the subset of job-seeker info returned inside an application.
// Mirrors the Prisma include: jobSeeker.user.{name, email} + jobSeeker.{resumeUrl, bio, skills}
type ApplicantSummary struct {
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	ResumeURL string   `json:"resumeUrl"`
	Bio       string   `json:"bio"`
	Skills    []string `json:"skills"`
}

// ApplicationForRecruiter is an application row with full applicant info.
// Mirrors the Prisma include shape in getApplicationsForJob().
type ApplicationForRecruiter struct {
	ID          string           `json:"id"`
	JobID       string           `json:"jobId"`
	JobSeekerID string           `json:"jobSeekerId"`
	CoverLetter string           `json:"coverLetter"`
	Status      string           `json:"status"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
	JobSeeker   ApplicantSummary `json:"jobSeeker"`
}

// ApplicationsForJobResponse wraps the list.
// Matches Node.js: res.json({ applications })
type ApplicationsForJobResponse struct {
	Applications []ApplicationForRecruiter `json:"applications"`
}

// UpdatedApplicationResponse wraps the result of a status update.
// Matches Node.js: res.json({ application })
type UpdatedApplicationResponse struct {
	Application ApplicationForRecruiter `json:"application"`
}
