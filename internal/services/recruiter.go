// ============================================================
// FILE: internal/services/recruiter.go
// ROLE: Recruiter profile and application management business logic.
//
// 📚 KEY CONCEPTS IN THIS FILE:
//
//   RECRUITER PROFILE vs USER:
//     The users table stores credentials (email, password, roles).
//     The recruiter_profiles table stores company info (company_name, etc.).
//     They are linked by user_id. This separation allows one user to have
//     both a job_seeker_profiles AND a recruiter_profiles row (upgrade flow).
//
//   OWNERSHIP CHAIN:
//     To view applications for a job:
//       User → recruiter_profiles.id → jobs.recruiter_id → applications.job_id
//     The service walks this chain to verify the recruiter owns the job.
//
//   APPLICATION STATUS:
//     Status is an enum: submitted | shortlisted | rejected | under_review
//     The request model uses `validate:"oneof=..."` to enforce the allowed values.
//
// 📝 COMPARISON with Node.js:
//   recruiter.service.ts → RecruiterService struct methods
// ============================================================
package services

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
)

// RecruiterService handles all recruiter business logic.
type RecruiterService struct {
	pool *pgxpool.Pool
}

// NewRecruiterService constructs a RecruiterService.
func NewRecruiterService(pool *pgxpool.Pool) *RecruiterService {
	return &RecruiterService{pool: pool}
}

// ── private helpers ───────────────────────────────────────────────────────────

// getRecruiterID resolves userID → recruiter_profiles.id.
// All recruiter operations start here to get the profile ID.
func (s *RecruiterService) getRecruiterID(ctx context.Context, userID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM recruiter_profiles WHERE user_id = $1 LIMIT 1`, userID).Scan(&id)
	if err != nil {
		return "", apperrors.New("Recruiter profile not found", 404)
	}
	return id, nil
}

// scanRecruiterProfile scans a profile row with user JOIN.
// Uses the same interface trick as scanProfile in jobseeker.go.
//
// 📝 WHY SCAN INTO NESTED STRUCT?
//   models.RecruiterProfileResponse has a User field (UserSummary struct).
//   We scan user fields (name, email, roles) directly into p.User.Name, etc.
//   This gives us the full response in one SQL query instead of two.
func scanRecruiterProfile(row interface{ Scan(...any) error }) (models.RecruiterProfileResponse, error) {
	var p models.RecruiterProfileResponse
	err := row.Scan(
		&p.ID, &p.UserID,
		&p.CompanyName, &p.CompanyWebsite, &p.Description, &p.Industry,
		&p.CreatedAt, &p.UpdatedAt,
		&p.User.Name, &p.User.Email, &p.User.Roles,
	)
	return p, err
}

// ── public service methods ────────────────────────────────────────────────────

// GetRecruiterProfile returns the full recruiter profile with user info joined.
//
// 📝 roles::text[]:
//   The users.roles column is a PostgreSQL array of a custom enum type.
//   ::text[] casts it to a text array so pgx can scan it into []string.
func (s *RecruiterService) GetRecruiterProfile(ctx context.Context, userID string) (models.RecruiterProfileResponse, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT r.id, r.user_id,
		        COALESCE(r.company_name,''), COALESCE(r.company_website,''),
		        COALESCE(r.description,''), COALESCE(r.industry,''),
		        u.created_at, u.updated_at,
		        u.name, u.email, u.roles::text[]
		 FROM recruiter_profiles r
		 JOIN users u ON u.id = r.user_id
		 WHERE r.user_id = $1 LIMIT 1`, userID)

	p, err := scanRecruiterProfile(row)
	if err != nil {
		return models.RecruiterProfileResponse{}, apperrors.New("Profile not found", 404)
	}
	return p, nil
}

// UpdateRecruiterProfile applies partial updates using COALESCE.
// Same PATCH pattern as UpdateJobSeekerProfile: nil pointer = keep existing.
//
// 📝 RETURNING without user JOIN:
//   The UPDATE RETURNING clause only returns columns from recruiter_profiles.
//   We do a separate SELECT to fetch user info (name, email, roles).
//   This keeps the UPDATE simple; the extra SELECT is a small price for clarity.
func (s *RecruiterService) UpdateRecruiterProfile(ctx context.Context, userID string, req models.UpdateRecruiterProfileRequest) (models.RecruiterProfileResponse, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE recruiter_profiles
		 SET company_name    = COALESCE($2, company_name),
		     company_website = COALESCE($3, company_website),
		     description     = COALESCE($4, description),
		     industry        = COALESCE($5, industry),
		     updated_at      = NOW()
		 WHERE user_id = $1
		 RETURNING id, user_id,
		           COALESCE(company_name,''), COALESCE(company_website,''),
		           COALESCE(description,''), COALESCE(industry,''),
		           created_at, updated_at`,
		userID, req.CompanyName, req.CompanyWebsite, req.Description, req.Industry)

	var p models.RecruiterProfileResponse
	err := row.Scan(
		&p.ID, &p.UserID,
		&p.CompanyName, &p.CompanyWebsite, &p.Description, &p.Industry,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return models.RecruiterProfileResponse{}, apperrors.New("Profile not found", 404)
	}

	// Fetch user info separately (UPDATE RETURNING doesn't include the JOIN)
	_ = s.pool.QueryRow(ctx, `SELECT name, email, roles::text[] FROM users WHERE id = $1`, userID).
		Scan(&p.User.Name, &p.User.Email, &p.User.Roles)

	return p, nil
}

// GetApplicationsForJob returns all applications for a job, verifying recruiter ownership.
//
// 📝 OWNERSHIP VERIFICATION STEPS:
//   1. getRecruiterID(userID) → recruiter_profiles.id
//   2. SELECT recruiter_id FROM jobs WHERE id = jobID
//   3. If ownerID != recruiterID → 403 Forbidden
//
// 📝 WHY THIS MATTERS:
//   Without the ownership check, any authenticated recruiter could view
//   applications for jobs posted by other recruiters — a data leak.
func (s *RecruiterService) GetApplicationsForJob(ctx context.Context, userID, jobID string) ([]models.ApplicationForRecruiter, error) {
	recruiterID, err := s.getRecruiterID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var ownerID string
	err = s.pool.QueryRow(ctx, `SELECT recruiter_id FROM jobs WHERE id = $1 LIMIT 1`, jobID).Scan(&ownerID)
	if err != nil {
		return nil, apperrors.New("Job not found", 404)
	}
	if ownerID != recruiterID {
		return nil, apperrors.New("You are not authorized to view applications for this job", 403)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.job_id, a.job_seeker_id,
		        COALESCE(a.cover_letter,''), a.status, a.created_at, a.updated_at,
		        u.name, u.email,
		        COALESCE(p.resume_url,''), COALESCE(p.bio,''), COALESCE(p.skills,'{}')
		 FROM applications a
		 JOIN job_seeker_profiles p ON p.id = a.job_seeker_id
		 JOIN users u ON u.id = p.user_id
		 WHERE a.job_id = $1
		 ORDER BY a.created_at DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	apps := make([]models.ApplicationForRecruiter, 0)
	for rows.Next() {
		var a models.ApplicationForRecruiter
		// Scan into nested struct: &a.JobSeeker.Name, &a.JobSeeker.Email, etc.
		err := rows.Scan(
			&a.ID, &a.JobID, &a.JobSeekerID,
			&a.CoverLetter, &a.Status, &a.CreatedAt, &a.UpdatedAt,
			&a.JobSeeker.Name, &a.JobSeeker.Email,
			&a.JobSeeker.ResumeURL, &a.JobSeeker.Bio, &a.JobSeeker.Skills,
		)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, nil
}

// UpdateApplicationStatus changes the status of an application, verifying ownership.
//
// 📝 STATUS VALUES (from models.UpdateApplicationStatusRequest validate tag):
//   "submitted"   → initial state when seeker applies
//   "under_review" → recruiter has started reviewing
//   "shortlisted" → recruiter likes the candidate
//   "rejected"    → recruiter decided not to proceed
//
// 📝 TWO-STEP PATTERN:
//   1. UPDATE applications SET status = $2 WHERE id = $1
//   2. SELECT the full row (with applicant info JOIN) to return
//   We do this because UPDATE RETURNING doesn't support JOINs in PostgreSQL.
func (s *RecruiterService) UpdateApplicationStatus(ctx context.Context, userID, applicationID, status string) (models.ApplicationForRecruiter, error) {
	recruiterID, err := s.getRecruiterID(ctx, userID)
	if err != nil {
		return models.ApplicationForRecruiter{}, err
	}

	// Verify the recruiter owns the job this application belongs to
	var jobOwnerID string
	err = s.pool.QueryRow(ctx,
		`SELECT j.recruiter_id
		 FROM applications a
		 JOIN jobs j ON j.id = a.job_id
		 WHERE a.id = $1 LIMIT 1`, applicationID).Scan(&jobOwnerID)
	if err != nil {
		return models.ApplicationForRecruiter{}, apperrors.New("Application not found", 404)
	}
	if jobOwnerID != recruiterID {
		return models.ApplicationForRecruiter{}, apperrors.New("You are not authorized to update this application", 403)
	}

	// Step 1: Update the status
	_, err = s.pool.Exec(ctx,
		`UPDATE applications SET status = $2, updated_at = NOW() WHERE id = $1`,
		applicationID, status)
	if err != nil {
		return models.ApplicationForRecruiter{}, err
	}

	// Step 2: Fetch the full updated application with applicant info
	row := s.pool.QueryRow(ctx,
		`SELECT a.id, a.job_id, a.job_seeker_id,
		        COALESCE(a.cover_letter,''), a.status, a.created_at, a.updated_at,
		        u.name, u.email,
		        COALESCE(p.resume_url,''), COALESCE(p.bio,''), COALESCE(p.skills,'{}')
		 FROM applications a
		 JOIN job_seeker_profiles p ON p.id = a.job_seeker_id
		 JOIN users u ON u.id = p.user_id
		 WHERE a.id = $1 LIMIT 1`, applicationID)

	var a models.ApplicationForRecruiter
	err = row.Scan(
		&a.ID, &a.JobID, &a.JobSeekerID,
		&a.CoverLetter, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		&a.JobSeeker.Name, &a.JobSeeker.Email,
		&a.JobSeeker.ResumeURL, &a.JobSeeker.Bio, &a.JobSeeker.Skills,
	)
	if err != nil {
		return models.ApplicationForRecruiter{}, err
	}
	return a, nil
}
