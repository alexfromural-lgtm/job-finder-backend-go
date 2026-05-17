// Package services contains business logic, mirroring src/services/ in the Node.js backend.
package services

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
)

// JobSeekerService handles all job-seeker business logic.
// Mirrors src/services/jobseeker.service.ts.
type JobSeekerService struct {
	pool *pgxpool.Pool
}

// NewJobSeekerService constructs a JobSeekerService.
func NewJobSeekerService(pool *pgxpool.Pool) *JobSeekerService {
	return &JobSeekerService{pool: pool}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// getJobSeekerID resolves userID → job_seeker_profiles.id.
func (s *JobSeekerService) getJobSeekerID(ctx context.Context, userID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM job_seeker_profiles WHERE user_id = $1 LIMIT 1`, userID).Scan(&id)
	if err != nil {
		return "", apperrors.New("Job seeker profile not found", 404)
	}
	return id, nil
}

// scanProfile scans a profile row (with user join) into JobSeekerProfileResponse.
func scanProfile(row interface {
	Scan(...any) error
}) (models.JobSeekerProfileResponse, error) {
	var p models.JobSeekerProfileResponse
	err := row.Scan(
		&p.ID, &p.UserID, &p.Bio, &p.Location, &p.Skills,
		&p.Education, &p.Experience, &p.ResumeURL,
		&p.CreatedAt, &p.UpdatedAt,
		&p.User.Name, &p.User.Email, &p.User.Roles,
	)
	return p, err
}

// ── public service methods ────────────────────────────────────────────────────

// GetJobSeekerProfile returns the full profile with user info.
// Mirrors jobseeker.service.ts → getJobSeekerProfile().
func (s *JobSeekerService) GetJobSeekerProfile(ctx context.Context, userID string) (models.JobSeekerProfileResponse, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT p.id, p.user_id, COALESCE(p.bio,''), COALESCE(p.location,''), COALESCE(p.skills,'{}'),
		        COALESCE(p.education,''), COALESCE(p.experience,''), COALESCE(p.resume_url,''),
		        u.created_at, u.updated_at,
		        u.name, u.email, u.roles::text[]
		 FROM job_seeker_profiles p
		 JOIN users u ON u.id = p.user_id
		 WHERE p.user_id = $1 LIMIT 1`, userID)

	p, err := scanProfile(row)
	if err != nil {
		return models.JobSeekerProfileResponse{}, apperrors.New("Profile not found", 404)
	}
	return p, nil
}

// UpdateJobSeekerProfile applies partial updates using COALESCE.
// Mirrors jobseeker.service.ts → updateJobSeekerProfile().
func (s *JobSeekerService) UpdateJobSeekerProfile(ctx context.Context, userID string, req models.UpdateJobSeekerProfileRequest) (models.JobSeekerProfileResponse, error) {
	// Convert pointer fields to interface{} — nil → SQL NULL → COALESCE keeps existing value
	row := s.pool.QueryRow(ctx,
		`UPDATE job_seeker_profiles
		 SET bio        = COALESCE($2, bio),
		     location   = COALESCE($3, location),
		     skills     = COALESCE($4, skills),
		     education  = COALESCE($5, education),
		     experience = COALESCE($6, experience),
		     resume_url = COALESCE($7, resume_url),
		     updated_at = NOW()
		 WHERE user_id = $1
		 RETURNING id, user_id, COALESCE(bio,''), COALESCE(location,''), COALESCE(skills,'{}'),
		           COALESCE(education,''), COALESCE(experience,''), COALESCE(resume_url,''),
		           created_at, updated_at`,
		userID,
		req.Bio, req.Location, skillsArg(req.Skills),
		req.Education, req.Experience, req.ResumeURL)

	var p models.JobSeekerProfileResponse
	err := row.Scan(
		&p.ID, &p.UserID, &p.Bio, &p.Location, &p.Skills,
		&p.Education, &p.Experience, &p.ResumeURL,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return models.JobSeekerProfileResponse{}, apperrors.New("Profile not found", 404)
	}

	// Fetch user info for the response
	_ = s.pool.QueryRow(ctx, `SELECT name, email, roles FROM users WHERE id = $1`, userID).
		Scan(&p.User.Name, &p.User.Email, &p.User.Roles)

	return p, nil
}

// ApplyToJob executes the apply-to-job work synchronously (called by the queue worker).
// Mirrors jobseeker.service.ts → applyToJob().
func (s *JobSeekerService) ApplyToJob(ctx context.Context, userID, jobID, coverLetter string) (models.ApplicationRow, error) {
	var app models.ApplicationRow
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return app, err
	}

	// Check job exists and is active
	var isActive bool
	err = s.pool.QueryRow(ctx, `SELECT is_active FROM jobs WHERE id = $1 LIMIT 1`, jobID).Scan(&isActive)
	if err != nil {
		return app, apperrors.New("Job not found", 404)
	}
	if !isActive {
		return app, apperrors.New("This job is no longer active", 410)
	}

	// Check for duplicate application
	var exists bool
	_ = s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM applications WHERE job_id=$1 AND job_seeker_id=$2)`,
		jobID, jobSeekerID).Scan(&exists)
	if exists {
		return app, apperrors.New("You have already applied to this job", 409)
	}

	err = s.pool.QueryRow(ctx,
		`INSERT INTO applications (job_id, job_seeker_id, cover_letter) VALUES ($1, $2, $3)
		 RETURNING id, job_id, job_seeker_id, COALESCE(cover_letter, ''), status, created_at, updated_at`,
		jobID, jobSeekerID, coverLetter).Scan(
		&app.ID, &app.JobID, &app.JobSeekerID, &app.CoverLetter, &app.Status, &app.CreatedAt, &app.UpdatedAt,
	)
	return app, err
}

// GetApplications returns all applications for the job seeker with job summary.
// Mirrors jobseeker.service.ts → getApplications().
func (s *JobSeekerService) GetApplications(ctx context.Context, userID string) ([]models.ApplicationRow, error) {
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT a.id, a.job_id, a.job_seeker_id,
		        COALESCE(a.cover_letter,''), a.status, a.created_at, a.updated_at,
		        j.id, j.title, COALESCE(j.location,''), COALESCE(j.salary_range,''),
		        COALESCE(j.category,''), r.company_name
		 FROM applications a
		 JOIN jobs j ON j.id = a.job_id
		 JOIN recruiter_profiles r ON r.id = j.recruiter_id
		 WHERE a.job_seeker_id = $1
		 ORDER BY a.created_at DESC`, jobSeekerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	apps := make([]models.ApplicationRow, 0)
	for rows.Next() {
		var a models.ApplicationRow
		err := rows.Scan(
			&a.ID, &a.JobID, &a.JobSeekerID,
			&a.CoverLetter, &a.Status, &a.CreatedAt, &a.UpdatedAt,
			&a.Job.ID, &a.Job.Title, &a.Job.Location, &a.Job.SalaryRange,
			&a.Job.Category, &a.Job.CompanyName,
		)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, nil
}

// WithdrawApplication deletes an application, enforcing ownership.
// Mirrors jobseeker.service.ts → withdrawApplication().
func (s *JobSeekerService) WithdrawApplication(ctx context.Context, userID, applicationID string) error {
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify it exists and belongs to this seeker
	var ownerID string
	err = s.pool.QueryRow(ctx,
		`SELECT job_seeker_id FROM applications WHERE id = $1 LIMIT 1`, applicationID).Scan(&ownerID)
	if err != nil {
		return apperrors.New("Application not found", 404)
	}
	if ownerID != jobSeekerID {
		return apperrors.New("You are not authorized to withdraw this application", 403)
	}

	_, err = s.pool.Exec(ctx, `DELETE FROM applications WHERE id = $1`, applicationID)
	return err
}

// SaveJob executes the save-job work synchronously (called by the queue worker).
// Mirrors jobseeker.service.ts → saveJob().
func (s *JobSeekerService) SaveJob(ctx context.Context, userID, jobID string) (models.SavedJobRow, error) {
	var saved models.SavedJobRow
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return saved, err
	}

	// Check job exists
	var exists bool
	_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1)`, jobID).Scan(&exists)
	if !exists {
		return saved, apperrors.New("Job not found", 404)
	}

	err = s.pool.QueryRow(ctx,
		`INSERT INTO saved_jobs (job_id, job_seeker_id) VALUES ($1, $2)
		 ON CONFLICT (job_id, job_seeker_id) DO UPDATE SET job_id = EXCLUDED.job_id
		 RETURNING id, job_id, job_seeker_id, saved_at`,
		jobID, jobSeekerID).Scan(
		&saved.ID, &saved.JobID, &saved.JobSeekerID, &saved.SavedAt,
	)
	return saved, err
}

// GetSavedJobs returns all saved jobs for the seeker with job summary.
// Mirrors jobseeker.service.ts → getSavedJobs().
func (s *JobSeekerService) GetSavedJobs(ctx context.Context, userID string) ([]models.SavedJobRow, error) {
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.job_id, s.job_seeker_id, s.saved_at,
		        j.id, j.title, COALESCE(j.location,''), COALESCE(j.salary_range,''),
		        COALESCE(j.category,''), j.is_active, r.company_name
		 FROM saved_jobs s
		 JOIN jobs j ON j.id = s.job_id
		 JOIN recruiter_profiles r ON r.id = j.recruiter_id
		 WHERE s.job_seeker_id = $1
		 ORDER BY s.saved_at DESC`, jobSeekerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	saved := make([]models.SavedJobRow, 0)
	for rows.Next() {
		var s models.SavedJobRow
		err := rows.Scan(
			&s.ID, &s.JobID, &s.JobSeekerID, &s.SavedAt,
			&s.Job.ID, &s.Job.Title, &s.Job.Location, &s.Job.SalaryRange,
			&s.Job.Category, &s.Job.IsActive, &s.Job.CompanyName,
		)
		if err != nil {
			return nil, err
		}
		saved = append(saved, s)
	}
	return saved, nil
}

// UnsaveJob removes a job from the seeker's saved list.
// Mirrors jobseeker.service.ts → unsaveJob().
func (s *JobSeekerService) UnsaveJob(ctx context.Context, userID, jobID string) error {
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM saved_jobs WHERE job_id = $1 AND job_seeker_id = $2`,
		jobID, jobSeekerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.New("Saved job not found", 404)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// skillsArg converts a nil slice to nil (SQL NULL) and a non-nil slice to itself,
// so COALESCE(skills, existing_skills) works correctly.
func skillsArg(skills []string) interface{} {
	if skills == nil {
		return nil
	}
	return skills
}
