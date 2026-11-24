// ============================================================
// FILE: internal/models/auth.go
// ROLE: Request & Response Data Transfer Objects (DTOs) for authentication.
//
// 📚 WHAT ARE DTOs?
//   DTOs (Data Transfer Objects) are Go structs designed specifically
//   for serializing and deserializing data sent over HTTP as JSON.
//
// 🏷️ STRUCT TAGS EXPLAINED:
//   In Go, struct fields can have "tags" enclosed in backticks:
//     `json:"name" validate:"required,min=1"`
//   - `json:"name"`: Tells Go's encoding/json package to map the JSON key "name"
//     to this struct field during unmarshaling and marshaling.
//   - `validate:"..."`: Used by go-playground/validator to enforce rules
//     like minimum length, valid email syntax, or required fields.
//
// 📝 COMPARISON WITH NODE.JS:
//   In TypeScript/Node.js, you might define:
//     const signupSchema = z.object({
//       email: z.string().email(),
//       password: z.string().min(6)
//     })
//   In Go, the struct and tags replace the Zod schema!
// ============================================================
package models

// ── Auth Request Bodies ────────────────────────────────────────────────────────

// SignupJobSeekerRequest defines the JSON body required for job seeker registration.
//
// 📝 EXAMPLE INCOMING JSON:
//   {
//     "name": "Jane Doe",
//     "email": "jane@example.com",
//     "password": "secretPassword123"
//   }
type SignupJobSeekerRequest struct {
	// Name must be at least 1 character long
	Name string `json:"name" validate:"required,min=1"`

	// Email must conform to standard RFC email format
	Email string `json:"email" validate:"required,email"`

	// Password must be at least 6 characters long
	Password string `json:"password" validate:"required,min=6"`
}

// SignupRecruiterRequest defines the JSON body required for recruiter registration.
// It includes user credentials PLUS company profile details in a single step.
//
// 📝 EXAMPLE INCOMING JSON:
//   {
//     "name": "Alice Smith",
//     "email": "alice@acme.com",
//     "password": "strongPassword!",
//     "companyName": "Acme Corp",
//     "companyWebsite": "https://acme.com",
//     "description": "Building cool tools",
//     "industry": "Technology"
//   }
type SignupRecruiterRequest struct {
	Name           string `json:"name"           validate:"required,min=1"`
	Email          string `json:"email"          validate:"required,email"`
	Password       string `json:"password"       validate:"required,min=6"`
	CompanyName    string `json:"companyName"    validate:"required,min=1"`
	// omitempty: if the user omits companyWebsite, don't validate it. But if provided, it must be a URL.
	CompanyWebsite string `json:"companyWebsite" validate:"omitempty,url"`
	Description    string `json:"description"    validate:"omitempty"`
	Industry       string `json:"industry"       validate:"omitempty"`
}

// UpgradeToRecruiterRequest carries company profile fields for an existing user
// upgrading their JOB_SEEKER account to a RECRUITER role.
//
// 📝 NOTE:
//   Name, Email, and Password are NOT repeated here because the user
//   is already authenticated and exists in the `users` table.
type UpgradeToRecruiterRequest struct {
	CompanyName    string `json:"companyName"    validate:"required,min=1"`
	CompanyWebsite string `json:"companyWebsite" validate:"omitempty,url"`
	Description    string `json:"description"    validate:"omitempty"`
	Industry       string `json:"industry"       validate:"omitempty"`
}

// LoginRequest defines the credentials payload for POST /api/auth/login.
//
// 📝 EXAMPLE INCOMING JSON:
//   {
//     "email": "jane@example.com",
//     "password": "secretPassword123"
//   }
type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=1"`
}

// ── Auth Response Bodies ───────────────────────────────────────────────────────

// MessageResponse provides a standard generic JSON success message.
//
// 📝 EXAMPLE OUTGOING JSON:
//   { "message": "Signed up successfully" }
type MessageResponse struct {
	Message string `json:"message"`
}

// UserResponse represents the sanitized public user profile.
//
// ⚠️ SECURITY NOTE:
//   Notice that `Password` is strictly NOT included in this struct!
//   This prevents sensitive password hashes from ever being serialized into JSON.
//
// 📝 EXAMPLE OUTGOING JSON:
//   {
//     "id": "c1f7a28e-...",
//     "name": "Jane Doe",
//     "email": "jane@example.com",
//     "roles": ["JOB_SEEKER"],
//     "isActive": true,
//     "createdAt": "2026-11-23T19:00:00Z",
//     "updatedAt": "2026-11-23T19:00:00Z"
//   }
type UserResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
	IsActive  bool     `json:"isActive"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}

// MeResponse wraps the current authenticated user DTO.
// Matches the React frontend expectation: response.data.user
type MeResponse struct {
	User UserResponse `json:"user"`
}

// UpgradeResponse wraps the updated user DTO after account upgrade.
type UpgradeResponse struct {
	User UserResponse `json:"user"`
}
