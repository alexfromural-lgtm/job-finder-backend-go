// Package models contains request/response DTO structs used by handlers and services.
// Struct tags drive both JSON decoding and go-playground/validator validation,
// mirroring the Zod schemas in src/validators/.
package models

// ── Auth request bodies ────────────────────────────────────────────────────────

// SignupJobSeekerRequest mirrors the Node.js signupSchema (jobseeker.schema.ts).
type SignupJobSeekerRequest struct {
	Name     string `json:"name"     validate:"required,min=1"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

// SignupRecruiterRequest mirrors recruiterSignupSchema (recruiter.schema.ts).
type SignupRecruiterRequest struct {
	Name           string `json:"name"           validate:"required,min=1"`
	Email          string `json:"email"          validate:"required,email"`
	Password       string `json:"password"       validate:"required,min=6"`
	CompanyName    string `json:"companyName"    validate:"required,min=1"`
	CompanyWebsite string `json:"companyWebsite" validate:"omitempty,url"`
	Description    string `json:"description"    validate:"omitempty"`
	Industry       string `json:"industry"       validate:"omitempty"`
}

// UpgradeToRecruiterRequest carries just the company profile fields — the user
// already exists, so name/email/password are not required again.
type UpgradeToRecruiterRequest struct {
	CompanyName    string `json:"companyName"    validate:"required,min=1"`
	CompanyWebsite string `json:"companyWebsite" validate:"omitempty,url"`
	Description    string `json:"description"    validate:"omitempty"`
	Industry       string `json:"industry"       validate:"omitempty"`
}

// LoginRequest mirrors loginSchema (auth.schema.ts).
type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=1"`
}

// ── Auth response bodies ───────────────────────────────────────────────────────

// MessageResponse is the envelope for simple success responses.
// Matches Node.js: res.json({ message: "..." })
type MessageResponse struct {
	Message string `json:"message"`
}

// UserResponse is the safe subset of user fields returned by GET /api/auth/me
// and POST /api/auth/upgrade/recruiter.
// Matches the Prisma select shape in getCurrentUser().
type UserResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
	IsActive  bool     `json:"isActive"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

// MeResponse wraps the user — matches Node.js: res.json({ user })
type MeResponse struct {
	User UserResponse `json:"user"`
}

// UpgradeResponse wraps the updated user — matches Node.js: res.json({ user })
type UpgradeResponse struct {
	User UserResponse `json:"user"`
}
