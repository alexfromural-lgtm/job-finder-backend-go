// Package services contains business logic, mirroring src/services/ in the Node.js backend.
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

// dbUser is the full internal representation of a users row (includes password hash).
type dbUser struct {
	ID        string
	Name      string
	Email     string
	Password  string
	Roles     []string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// toResponse converts dbUser to the safe public DTO (no password).
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

// AuthService holds dependencies for all auth business logic.
// Mirrors the functions in src/services/auth.service.ts.
type AuthService struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

// NewAuthService constructs an AuthService.
func NewAuthService(pool *pgxpool.Pool, cfg *config.Config) *AuthService {
	return &AuthService{pool: pool, cfg: cfg}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (s *AuthService) getUserByEmail(ctx context.Context, email string) (*dbUser, error) {
	u := &dbUser{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, email, password, roles, is_active, created_at, updated_at
		 FROM users WHERE email = $1 LIMIT 1`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Roles, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *AuthService) getUserByID(ctx context.Context, id string) (*dbUser, error) {
	u := &dbUser{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, email, password, roles, is_active, created_at, updated_at
		 FROM users WHERE id = $1 LIMIT 1`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Roles, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *AuthService) checkEmailFree(ctx context.Context, email string) error {
	var exists bool
	_ = s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	if exists {
		return apperrors.New("User already exists", 409)
	}
	return nil
}

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
// Mirrors src/services/auth.service.ts → signupJobSeeker().
func (s *AuthService) SignupJobSeeker(ctx context.Context, name, email, password string) (at, rt string, err error) {
	if email == "" || password == "" {
		return "", "", apperrors.New("Email and password are required", 400)
	}
	if err = s.checkEmailFree(ctx, email); err != nil {
		return "", "", err
	}

	hashed, err := utils.HashPassword(password)
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
		name, email, hashed, []string{"JOB_SEEKER"}).
		Scan(&userID, &roles)
	if err != nil {
		return "", "", err
	}

	_, err = tx.Exec(ctx, `INSERT INTO job_seeker_profiles (user_id) VALUES ($1)`, userID)
	if err != nil {
		return "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", apperrors.New("Failed to commit transaction", 500)
	}

	return s.generateTokens(userID, roles)
}

// SignupRecruiter creates a user with the RECRUITER role and a recruiter profile.
// Mirrors src/services/auth.service.ts → signupRecruiter().
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

// Login verifies credentials and returns JWT tokens.
// Mirrors src/services/auth.service.ts → login().
func (s *AuthService) Login(ctx context.Context, email, password string) (at, rt string, err error) {
	if email == "" || password == "" {
		return "", "", apperrors.New("Email and password are required", 400)
	}

	user, err := s.getUserByEmail(ctx, email)
	if err != nil {
		// Don't reveal whether the email exists — same message for both cases
		return "", "", apperrors.New("Invalid email or password", 401)
	}
	if !user.IsActive {
		return "", "", apperrors.New("Account is deactivated", 403)
	}
	if err = utils.ComparePasswords(password, user.Password); err != nil {
		return "", "", apperrors.New("Invalid email or password", 401)
	}

	return s.generateTokens(user.ID, user.Roles)
}

// RefreshTokens verifies a refresh JWT and issues a fresh token pair.
// Mirrors src/services/auth.service.ts → refreshTokens().
func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (at, rt string, err error) {
	claims, err := utils.VerifyToken(refreshToken, s.cfg.RefreshTokenSecret)
	if err != nil {
		return "", "", apperrors.New("Invalid or expired refresh token", 401)
	}

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
// Mirrors src/services/auth.service.ts → getCurrentUser().
func (s *AuthService) GetCurrentUser(ctx context.Context, userID string) (models.UserResponse, error) {
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return models.UserResponse{}, apperrors.New("User not found", 404)
	}
	return user.toResponse(), nil
}

// UpgradeToRecruiter adds the RECRUITER role to an existing JOB_SEEKER and
// creates their recruiter profile. Re-issues tokens so the new role is active immediately.
// Mirrors src/services/auth.service.ts → upgradeToRecruiter().
func (s *AuthService) UpgradeToRecruiter(ctx context.Context, userID string, req models.UpgradeToRecruiterRequest) (models.UserResponse, string, string, error) {
	user, err := s.getUserByID(ctx, userID)
	if err != nil {
		return models.UserResponse{}, "", "", apperrors.New("User not found", 404)
	}

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
