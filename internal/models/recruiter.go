// ============================================================
// FILE: internal/models/recruiter.go
// ROLE: Request & Response Data Transfer Objects (DTOs) for Recruiters.
//
// 📚 WHAT THIS FILE COVERS:
//   - Recruiter company profile updates.
//   - Candidate application status updates (shortlisting, rejecting, reviewing).
//   - Viewing job applications with candidate profiles attached.
// ============================================================
package models

import "time"

// ── Recruiter Request Bodies ───────────────────────────────────────────────────

// UpdateRecruiterProfileRequest defines the partial update payload for a recruiter's company profile.
//
// 💡 POINTER FIELDS (*string):
//   All fields are optional pointers. If the recruiter updates only companyName:
//   `{"companyName": "Acme Global"}`
//   Then CompanyName != nil, and all other fields are nil (left untouched in DB).
type UpdateRecruiterProfileRequest struct {
	CompanyName    *string `json:"companyName"    validate:"omitempty,min=1"`
	CompanyWebsite *string `json:"companyWebsite" validate:"omitempty,url"`
	Description    *string `json:"description"    validate:"omitempty"`
	Industry       *string `json:"industry"       validate:"omitempty"`
}

// UpdateApplicationStatusRequest defines the payload when a recruiter updates an applicant's status.
//
// 🏷️ VALIDATION:
//   `oneof=submitted shortlisted rejected under_review`
//   Ensures status must be one of the four allowed enum values, rejecting any invalid strings.
//
// 📝 EXAMPLE INCOMING JSON:
//   { "status": "shortlisted" }
type UpdateApplicationStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=submitted shortlisted rejected under_review"`
}

// ── Recruiter Response Bodies ──────────────────────────────────────────────────

// RecruiterProfileResponse represents the complete recruiter profile with user details attached.
//
// 📝 EXAMPLE OUTGOING JSON:
//   {
//     "id": "r1a2b3...",
//     "userId": "u1a2b3...",
//     "companyName": "Acme Corp",
//     "companyWebsite": "https://acme.com",
//     "description": "Tech consultancy",
//     "industry": "Software",
//     "createdAt": "2026-11-23T19:00:00Z",
//     "updatedAt": "2026-11-23T19:00:00Z",
//     "user": {
//       "name": "Jane Recruiter",
//       "email": "jane@acme.com",
//       "roles": ["RECRUITER"]
//     }
//   }
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

// RecruiterProfileResponseWrapper wraps a single recruiter profile.
// Matches Node.js format: res.json({ profile })
type RecruiterProfileResponseWrapper struct {
	Profile RecruiterProfileResponse `json:"profile"`
}

// ApplicantSummary contains applicant candidate information
// attached to an application when viewed by the job poster.
//
// 📝 DATA SOURCE:
//   Obtained via SQL JOIN between `applications`, `job_seeker_profiles`, and `users`.
type ApplicantSummary struct {
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	ResumeURL string   `json:"resumeUrl"`
	Bio       string   `json:"bio"`
	Skills    []string `json:"skills"`
}

// ApplicationForRecruiter represents an application from the recruiter's perspective,
// including full applicant profile info.
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

// ApplicationsForJobResponse wraps the list of candidate applications for a specific job.
// Matches Node.js format: res.json({ applications })
type ApplicationsForJobResponse struct {
	Applications []ApplicationForRecruiter `json:"applications"`
}

// UpdatedApplicationResponse is returned after updating an application's status.
// Matches Node.js format: res.json({ application })
type UpdatedApplicationResponse struct {
	Application ApplicationForRecruiter `json:"application"`
}
