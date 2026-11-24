// ============================================================
// FILE: internal/models/jobseeker.go
// ROLE: Request & Response Data Transfer Objects (DTOs) for Job Seekers.
//
// 📚 WHAT THIS FILE COVERS:
//   - Job seeker profile updates (bio, location, skills, resume).
//   - Job application submissions and withdrawal.
//   - Saved jobs (bookmarks).
//   - Nested relationships (e.g. including Job details inside an Application).
// ============================================================
package models

import "time"

// ── Job Seeker Request Bodies ──────────────────────────────────────────────────

// UpdateJobSeekerProfileRequest defines the partial update payload for a job seeker.
//
// 💡 POINTER FIELDS (*string):
//   Using pointers allows distinguishing between:
//   - Field omitted in JSON (value is nil -> keep existing database value via COALESCE)
//   - Field provided as "" (empty string -> clear database field)
//
// 📝 EXAMPLE INCOMING JSON:
//   {
//     "bio": "Full-stack engineer specializing in Go and React",
//     "location": "London, UK",
//     "skills": ["Go", "Docker", "PostgreSQL", "React"],
//     "resumeUrl": "https://storage.example.com/resumes/my-resume.pdf"
//   }
type UpdateJobSeekerProfileRequest struct {
	Bio        *string  `json:"bio"        validate:"omitempty,max=500"`
	Location   *string  `json:"location"   validate:"omitempty,max=100"`
	Skills     []string `json:"skills"     validate:"omitempty"`
	Education  *string  `json:"education"  validate:"omitempty"`
	Experience *string  `json:"experience" validate:"omitempty"`
	ResumeURL  *string  `json:"resumeUrl"  validate:"omitempty,url"`
}

// ApplyToJobRequest contains the cover letter when submitting a job application.
//
// 📝 EXAMPLE INCOMING JSON:
//   {
//     "coverLetter": "Hello, I am enthusiastic about applying for this role because..."
//   }
type ApplyToJobRequest struct {
	CoverLetter string `json:"coverLetter" validate:"omitempty,max=2000"`
}

// ── Job Seeker Response Bodies ─────────────────────────────────────────────────

// UserSummary embeds safe user account fields inside profile/application DTOs.
type UserSummary struct {
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

// JobSeekerProfileResponse represents the full job seeker profile
// combined with the underlying user account details.
//
// 📝 EXAMPLE OUTGOING JSON:
//   {
//     "id": "b3e2a1...",
//     "userId": "c5d4...",
//     "bio": "Software developer",
//     "location": "San Francisco",
//     "skills": ["Go", "AWS"],
//     "education": "BS Computer Science",
//     "experience": "3 years",
//     "resumeUrl": "https://example.com/resume.pdf",
//     "createdAt": "2026-11-23T19:00:00Z",
//     "updatedAt": "2026-11-23T19:00:00Z",
//     "user": {
//       "name": "Alex",
//       "email": "alex@example.com",
//       "roles": ["JOB_SEEKER"]
//     }
//   }
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

// ProfileResponse wraps the single job seeker profile response.
type ProfileResponse struct {
	Profile JobSeekerProfileResponse `json:"profile"`
}

// JobSummaryInApplication represents the embedded job details returned
// when a job seeker views their list of job applications.
type JobSummaryInApplication struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Location    string `json:"location"`
	SalaryRange string `json:"salaryRange"`
	Category    string `json:"category"`
	CompanyName string `json:"companyName"`
}

// ApplicationRow represents an application record including embedded job info.
//
// 📝 STATUS VALUES:
//   "submitted" | "under_review" | "shortlisted" | "rejected"
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

// ApplicationsResponse wraps the list of a job seeker's applications.
type ApplicationsResponse struct {
	Applications []ApplicationRow `json:"applications"`
}

// JobSummaryInSaved represents the embedded job details inside a saved job entry.
type JobSummaryInSaved struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Location    string `json:"location"`
	SalaryRange string `json:"salaryRange"`
	Category    string `json:"category"`
	IsActive    bool   `json:"isActive"`
	CompanyName string `json:"companyName"`
}

// SavedJobRow represents a bookmarked/saved job record.
type SavedJobRow struct {
	ID          string            `json:"id"`
	JobID       string            `json:"jobId"`
	JobSeekerID string            `json:"jobSeekerId"`
	SavedAt     time.Time         `json:"savedAt"`
	Job         JobSummaryInSaved `json:"job"`
}

// SavedJobsResponse wraps the list of saved jobs.
type SavedJobsResponse struct {
	SavedJobs []SavedJobRow `json:"savedJobs"`
}

// QueuedResponse is returned with HTTP 202 Accepted when a task
// (such as applying to a job or saving a job) is pushed onto Redis.
//
// 📝 EXAMPLE OUTGOING JSON:
//   {
//     "queueJobId": "e2f07297-76fa-4b8c-b062-8175d658bfe0",
//     "status": "queued"
//   }
type QueuedResponse struct {
	QueueJobID string `json:"queueJobId"`
	Status     string `json:"status"`
}
