// Package services contains business logic, mirroring src/services/ in the Node.js backend.
package services

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
)

// RecruiterService handles all recruiter business logic.
// Mirrors src/services/recruiter.service.ts.
type RecruiterService struct {
	pool *pgxpool.Pool
}

// NewRecruiterService constructs a RecruiterService.
func NewRecruiterService(pool *pgxpool.Pool) *RecruiterService {
	return &RecruiterService{pool: pool}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// getRecruiterID resolves userID → recruiter_profiles.id.
func (s *RecruiterService) getRecruiterID(ctx context.Context, userID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM recruiter_profiles WHERE user_id = $1 LIMIT 1`, userID).Scan(&id)
	if err != nil {
		return "", apperrors.New("Recruiter profile not found", 404)
	}
	return id, nil
}

// scanRecruiterProfile scans a recruiter profile row (with user join).
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
// Mirrors recruiter.service.ts → getRecruiterProfile().
func (s *RecruiterService) GetRecruiterProfile(ctx context.Context, userID string) (models.RecruiterProfileResponse, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT r.id, r.user_id,
		        COALESCE(r.company_name,''), COALESCE(r.company_website,''),
		        COALESCE(r.description,''), COALESCE(r.industry,''),
		        r.created_at, r.updated_at,
		        u.name, u.email, u.roles
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
// Mirrors recruiter.service.ts → updateRecruiterProfile().
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

	// Fetch user info to attach
	_ = s.pool.QueryRow(ctx, `SELECT name, email, roles FROM users WHERE id = $1`, userID).
		Scan(&p.User.Name, &p.User.Email, &p.User.Roles)

	return p, nil
}

// GetApplicationsForJob returns all applications for a job, verifying recruiter ownership.
// Mirrors recruiter.service.ts → getApplicationsForJob().
func (s *RecruiterService) GetApplicationsForJob(ctx context.Context, userID, jobID string) ([]models.ApplicationForRecruiter, error) {
	recruiterID, err := s.getRecruiterID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Verify the recruiter owns this job
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

// UpdateApplicationStatus changes the status of an application, verifying recruiter ownership.
// Mirrors recruiter.service.ts → updateApplicationStatus().
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

	// Perform the update
	_, err = s.pool.Exec(ctx,
		`UPDATE applications SET status = $2, updated_at = NOW() WHERE id = $1`,
		applicationID, status)
	if err != nil {
		return models.ApplicationForRecruiter{}, err
	}

	// Fetch the full updated row with applicant info
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
