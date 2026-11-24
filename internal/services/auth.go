// ============================================================
// FILE: internal/services/auth.go
// ROLE: Authentication business logic (signup, login, refresh, upgrade).
//
// 📚 WHAT IS A SERVICE?
//   The service layer contains BUSINESS LOGIC — the rules of your application.
//   It sits between the HTTP handler (which decodes requests) and the database
//   (which stores data). Services are framework-agnostic: they work the same
//   whether called from an HTTP handler or a CLI command.
//
// 📝 LAYER RESPONSIBILITIES:
//   Handler   → HTTP concerns (decode, validate, write response)
//   Service   → Business rules (check duplicates, hash passwords, generate tokens)
//   Database  → Data persistence (SQL queries)
//
// 📝 COMPARISON with Node.js:
//   This mirrors src/services/auth.service.ts completely.
//   signupJobSeeker() → AuthService.SignupJobSeeker()
//   login()           → AuthService.Login()
//   refreshTokens()   → AuthService.RefreshTokens()
//
// 📝 TRANSACTIONS:
//   Some operations (signup) require multiple DB writes to succeed or fail together.
//   We use PostgreSQL transactions (pool.Begin → Exec → Commit/Rollback).
//   If any step fails, defer tx.Rollback() undoes all changes automatically.
// ============================================================
package services

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/utils"
)

// dbUser is the full internal representation of a users table row.
// It includes the hashed password — which is NEVER sent to the client.
// The toResponse() method produces the safe public version without the password.
//
// 📝 WHY NOT USE models.UserResponse DIRECTLY?
//   models.UserResponse has no Password field (safe to send to client).
//   dbUser has Password (needed for login verification).
//   Keeping them separate prevents accidentally leaking the hash.
type dbUser struct {
	ID        string
	Name      string
	Email     string
	Password  string    // bcrypt hash — never send this in responses!
	Roles     []string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// toResponse converts the internal dbUser to the safe public DTO.
// The Password field is intentionally omitted.
//
// 📝 time.RFC3339 formats timestamps like: "2024-01-15T10:30:00Z"
//   This is the ISO 8601 format expected by JSON APIs.
func (u *dbUser) toResponse() models.UserResponse {
	return models.UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Roles:     u.Roles,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
		UpdatedAt: u.UpdatedAt.Format(time.RFC3339),
	}
}

// AuthService holds the dependencies needed for all auth business logic.
// Using a struct groups related functions (methods) with their shared state (pool, cfg).
type AuthService struct {
	pool *pgxpool.Pool  // database connection pool
	cfg  *config.Config // JWT secrets + expiry durations
}

// NewAuthService constructs an AuthService.
// Called from main.go: services.NewAuthService(pool, cfg)
func NewAuthService(pool *pgxpool.Pool, cfg *config.Config) *AuthService {
	return &AuthService{pool: pool, cfg: cfg}
}

// ── private helpers ───────────────────────────────────────────────────────────

// getUserByEmail queries the users table by email.
// pool.QueryRow returns a *pgx.Row; .Scan copies column values into variables.
//
// 📝 PARAMETERISED QUERY ($1):
//   Never use string formatting for SQL: "WHERE email = '" + email + "'"
//   That would allow SQL injection attacks.
//   $1 is a placeholder; pgx safely quotes and escapes the value.
//
// 📝 COLUMN ORDER:
//   The SELECT column order MUST match the Scan() argument order exactly.
//   Mismatched order = wrong values silently scanned into wrong fields.
func (s *AuthService) getUserByEmail(ctx context.Context, email string) (*dbUser, error) {
	u := &dbUser{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, email, password, roles, is_active, created_at, updated_at
		 FROM users WHERE email = $1 LIMIT 1`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Roles, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, err // err is pgx.ErrNoRows if not found
}

// getUserByID queries the users table by UUID.
// Same pattern as getUserByEmail but different WHERE clause.
func (s *AuthService) getUserByID(ctx context.Context, id string) (*dbUser, error) {
	u := &dbUser{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, email, password, roles, is_active, created_at, updated_at
		 FROM users WHERE id = $1 LIMIT 1`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Roles, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// checkEmailFree returns an AppError if the email is already registered.
// Used to give a friendly 409 Conflict before attempting the INSERT.
//
// 📝 SELECT EXISTS:
//   More efficient than SELECT * — PostgreSQL stops scanning at the first match.
//   Returns TRUE/FALSE — a boolean value scanned into `exists`.
func (s *AuthService) checkEmailFree(ctx context.Context, email string) error {
	var exists bool
	_ = s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	if exists {
		return apperrors.New("User already exists", 409)
	}
	return nil
}

// generateTokens creates both an access token and a refresh token for a user.
// Called after every successful authentication event (signup, login, refresh, upgrade).
//
// 📝 MULTIPLE RETURN VALUES:
//   Go functions can return multiple values: (accessToken, refreshToken, error)
//   This avoids having to create a wrapper struct just to return 3 values.
func (s *AuthService) generateTokens(userID string, roles []string) (accessToken, refreshToken string, err error) {
	at, err := utils.GenerateAccessToken(userID, roles, s.cfg.AccessTokenSecret, s.cfg.AccessTokenExpiry)
	if err != nil {
		return "", "", apperrors.New("Failed to generate access token", 500)
	}
	rt, err := utils.GenerateRefreshToken(userID, roles, s.cfg.RefreshTokenSecret, s.cfg.RefreshTokenExpiry)
	if err != nil {
		return "", "", apperrors.New("Failed to generate refresh token", 500)
	}
	return at, rt, nil
}

// ── public service methods ────────────────────────────────────────────────────

// SignupJobSeeker creates a user with the JOB_SEEKER role and a blank profile.
// Uses a transaction: both inserts (users + job_seeker_profiles) must succeed together.
//
// 📝 TRANSACTION PATTERN:
//   tx, _ := pool.Begin(ctx)      // start transaction
//   defer tx.Rollback(ctx)        // ← runs on error (before Commit) OR success (no-op after Commit)
//   tx.Exec / tx.QueryRow(...)    // transactional queries
//   tx.Commit(ctx)                // make changes permanent
//
//   If anything fails before Commit, Rollback() undoes all changes.
//   If Commit succeeds, the deferred Rollback() is a no-op.
func (s *AuthService) SignupJobSeeker(ctx context.Context, name, email, password string) (at, rt string, err error) {
	if email == "" || password == "" {
		return "", "", apperrors.New("Email and password are required", 400)
	}
	if err = s.checkEmailFree(ctx, email); err != nil {
		return "", "", err
	}

	// Hash the password BEFORE storing it. Never store plain-text passwords.
	hashed, err := utils.HashPassword(password)
	if err != nil {
		return "", "", apperrors.New("Failed to hash password", 500)
	}

	// Begin a database transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", apperrors.New("Failed to start transaction", 500)
	}
	// defer + nolint: Rollback is intentionally called even on success (it's a no-op after Commit)
	defer tx.Rollback(ctx) //nolint:errcheck

	var userID string
	var roles []string
	// RETURNING clause fetches the auto-generated id and roles in the same query as INSERT.
	// No need for a separate SELECT after INSERT.
	err = tx.QueryRow(ctx,
		`INSERT INTO users (name, email, password, roles)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, roles`,
		name, email, hashed, []string{"JOB_SEEKER"}).
		Scan(&userID, &roles)
	if err != nil {
		return "", "", err
	}

	// Also create an empty job_seeker_profiles row linked to the new user.
	// Without this, GetJobSeekerProfile would return 404.
	_, err = tx.Exec(ctx, `INSERT INTO job_seeker_profiles (user_id) VALUES ($1)`, userID)
	if err != nil {
		return "", "", err
	}

	// Commit makes both INSERTs permanent in the database.
	if err = tx.Commit(ctx); err != nil {
		return "", "", apperrors.New("Failed to commit transaction", 500)
	}

	return s.generateTokens(userID, roles)
}

// SignupRecruiter creates a user with the RECRUITER role and a recruiter profile.
// Same transaction pattern as SignupJobSeeker, but with more profile fields.
func (s *AuthService) SignupRecruiter(ctx context.Context, req models.SignupRecruiterRequest) (at, rt string, err error) {
	if req.Email == "" || req.Password == "" {
		return "", "", apperrors.New("Email and password are required", 400)
	}
	if err = s.checkEmailFree(ctx, req.Email); err != nil {
		return "", "", err
	}

	hashed, err := utils.HashPassword(req.Password)
	if err != nil {
		return "", "", apperrors.New("Failed to hash password", 500)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", apperrors.New("Failed to start transaction", 500)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var userID string
	var roles []string
	err = tx.QueryRow(ctx,
		`INSERT INTO users (name, email, password, roles)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, roles`,
		req.Name, req.Email, hashed, []string{"RECRUITER"}).
		Scan(&userID, &roles)
	if err != nil {
		return "", "", err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO recruiter_profiles (user_id, company_name, company_website, industry, description)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, req.CompanyName, req.CompanyWebsite, req.Industry, req.Description)
	if err != nil {
		return "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", apperrors.New("Failed to commit transaction", 500)
	}

	return s.generateTokens(userID, roles)
}

// Login verifies email + password and issues new JWT tokens.
//
// 📝 SECURITY: same error message for "wrong email" and "wrong password".
//   Returning different messages ("no such user" vs "wrong password") lets
//   attackers enumerate registered email addresses. We always say "Invalid
//   email or password" to prevent user enumeration.
func (s *AuthService) Login(ctx context.Context, email, password string) (at, rt string, err error) {
	if email == "" || password == "" {
		return "", "", apperrors.New("Email and password are required", 400)
	}

	user, err := s.getUserByEmail(ctx, email)
	if err != nil {
		// Don't reveal whether the email is registered
		return "", "", apperrors.New("Invalid email or password", 401)
	}
	if !user.IsActive {
		return "", "", apperrors.New("Account is deactivated", 403)
	}

	// utils.ComparePasswords runs bcrypt — intentionally slow to resist brute force
	if err = utils.ComparePasswords(password, user.Password); err != nil {
		return "", "", apperrors.New("Invalid email or password", 401)
	}

	return s.generateTokens(user.ID, user.Roles)
}

// RefreshTokens verifies the refresh JWT and issues a new token pair.
// Called by the /api/auth/refresh endpoint when the access token expires.
//
// 📝 FLOW:
//   1. Verify the refresh JWT (signature + expiry)
//   2. Look up the user to check they're still active (not deactivated since token was issued)
//   3. Issue new access + refresh tokens
func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (at, rt string, err error) {
	claims, err := utils.VerifyToken(refreshToken, s.cfg.RefreshTokenSecret)
	if err != nil {
		return "", "", apperrors.New("Invalid or expired refresh token", 401)
	}

	// Re-fetch user to confirm they're still active
	user, err := s.getUserByID(ctx, claims.UserID)
	if err != nil {
		return "", "", apperrors.New("User not found", 404)
	}
	if !user.IsActive {
		return "", "", apperrors.New("Account is deactivated", 403)
	}

	return s.generateTokens(user.ID, user.Roles)
}

// GetCurrentUser returns the safe public profile for the authenticated user.
// Called by GET /api/auth/me — the userID comes from the JWT (not the request body).
func (s *AuthService) GetCurrentUser(ctx context.Context, userID string) (models.UserResponse, error) {
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return models.UserResponse{}, apperrors.New("User not found", 404)
	}
	return user.toResponse(), nil
}

// UpgradeToRecruiter adds the RECRUITER role to an existing JOB_SEEKER.
// Creates a recruiter_profiles row and re-issues tokens with the new role.
//
// 📝 RETURNS 4 VALUES: (userResponse, accessToken, refreshToken, error)
//   Multiple returns avoid a wrapper struct and keep the caller code clean.
//
// 📝 WHY RE-ISSUE TOKENS?
//   The existing JWT says roles=["JOB_SEEKER"]. After upgrade, the user has
//   roles=["JOB_SEEKER","RECRUITER"]. The old JWT would be rejected by RECRUITER
//   routes. New tokens with the updated roles let the user access recruiter
//   endpoints immediately without logging out and back in.
func (s *AuthService) UpgradeToRecruiter(ctx context.Context, userID string, req models.UpgradeToRecruiterRequest) (models.UserResponse, string, string, error) {
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return models.UserResponse{}, "", "", apperrors.New("User not found", 404)
	}

	// Check role eligibility
	hasJobSeeker := false
	hasRecruiter := false
	for _, r := range user.Roles {
		if r == "JOB_SEEKER" {
			hasJobSeeker = true
		}
		if r == "RECRUITER" {
			hasRecruiter = true
		}
	}
	if !hasJobSeeker {
		return models.UserResponse{}, "", "", apperrors.New("Only Job Seekers can upgrade to Recruiter", 403)
	}
	if hasRecruiter {
		return models.UserResponse{}, "", "", apperrors.New("User already has a recruiter profile", 409)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.UserResponse{}, "", "", apperrors.New("Failed to start transaction", 500)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// append adds to the slice — newRoles = ["JOB_SEEKER", "RECRUITER"]
	newRoles := append(user.Roles, "RECRUITER")
	var updatedUser dbUser
	err = tx.QueryRow(ctx,
		`UPDATE users SET roles = $2, updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, name, email, password, roles, is_active, created_at, updated_at`,
		userID, newRoles).
		Scan(&updatedUser.ID, &updatedUser.Name, &updatedUser.Email, &updatedUser.Password,
			&updatedUser.Roles, &updatedUser.IsActive, &updatedUser.CreatedAt, &updatedUser.UpdatedAt)
	if err != nil {
		return models.UserResponse{}, "", "", err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO recruiter_profiles (user_id, company_name, company_website, industry, description)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, req.CompanyName, req.CompanyWebsite, req.Industry, req.Description)
	if err != nil {
		return models.UserResponse{}, "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return models.UserResponse{}, "", "", apperrors.New("Failed to commit transaction", 500)
	}

	at, rt, err := s.generateTokens(updatedUser.ID, updatedUser.Roles)
	if err != nil {
		return models.UserResponse{}, "", "", err
	}

	return updatedUser.toResponse(), at, rt, nil
}
