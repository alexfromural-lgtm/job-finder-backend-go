// ============================================================
// FILE: internal/services/jobseeker.go
// ROLE: Job-seeker profile, application, and saved-jobs business logic.
//
// 📚 KEY CONCEPTS IN THIS FILE:
//
//   COALESCE in SQL:
//     COALESCE(value, fallback) returns the first non-NULL value.
//     We use it on nullable columns so Go always receives a string (never nil).
//     Example: COALESCE(bio, '') → empty string instead of NULL
//
//   COALESCE for PATCH updates:
//     COALESCE($2, existing_column) → if $2 is NULL, keep existing value.
//     This lets us implement PATCH semantics: only provided fields are updated.
//
//   ON CONFLICT DO UPDATE (upsert):
//     SaveJob uses ON CONFLICT to silently ignore duplicate saves:
//     if the job is already saved, the INSERT becomes a no-op UPDATE.
//
//   Queue integration:
//     ApplyToJob and SaveJob are called by the queue WORKER (not directly by handlers).
//     The handler enqueues a task; the worker calls these service methods.
//
// 📝 COMPARISON with Node.js:
//   jobseeker.service.ts → JobSeekerService struct methods
// ============================================================
package services

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
)

// JobSeekerService handles all job-seeker business logic.
type JobSeekerService struct {
	pool *pgxpool.Pool
}

// NewJobSeekerService constructs a JobSeekerService.
func NewJobSeekerService(pool *pgxpool.Pool) *JobSeekerService {
	return &JobSeekerService{pool: pool}
}

// ── private helpers ───────────────────────────────────────────────────────────

// getJobSeekerID resolves userID → job_seeker_profiles.id.
// Returns 404 if the user has no job seeker profile.
//
// 📝 WHY NOT USE userID DIRECTLY?
//   The jobs table links to job_seeker_profiles.id, not users.id directly.
//   This level of indirection allows one user to potentially have both
//   a seeker profile and a recruiter profile (the upgrade flow).
func (s *JobSeekerService) getJobSeekerID(ctx context.Context, userID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM job_seeker_profiles WHERE user_id = $1 LIMIT 1`, userID).Scan(&id)
	if err != nil {
		return "", apperrors.New("Job seeker profile not found", 404)
	}
	return id, nil
}

// scanProfile scans a profile row (with user JOIN) into JobSeekerProfileResponse.
// The interface{ Scan(...any) error } trick lets this function work for both
// pool.QueryRow (single row) and rows (multi-row iteration).
//
// 📝 INTERFACE PARAMETER:
//   func scanProfile(row interface{ Scan(...any) error })
//   Any type with a Scan method works here — both *pgx.Row and *pgx.Rows satisfy this.
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

// GetJobSeekerProfile returns the full profile with user info joined.
//
// 📝 COALESCE USAGE:
//   COALESCE(p.bio, '')         → "" if bio is NULL (new user, no bio yet)
//   COALESCE(p.skills, '{}')    → empty array "{}" if skills is NULL
//   The Go struct receives strings/slices, not nullable types — simpler to work with.
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
//
// 📝 HOW PARTIAL UPDATES WORK:
//   Request struct uses pointer fields (*string, []string).
//   nil = not provided by client = keep existing value.
//   non-nil = provided = update to this value.
//
//   SQL: SET bio = COALESCE($2, bio)
//   - $2 = nil (SQL NULL) → COALESCE returns existing bio (unchanged)
//   - $2 = "I am a developer" → COALESCE returns "I am a developer" (updated)
//
// 📝 RETURNING clause:
//   After UPDATE, we use RETURNING to get the new values in one round-trip.
//   No need for a separate SELECT after UPDATE.
func (s *JobSeekerService) UpdateJobSeekerProfile(ctx context.Context, userID string, req models.UpdateJobSeekerProfileRequest) (models.JobSeekerProfileResponse, error) {
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

	// Fetch user info for the response (not returned by the UPDATE)
	_ = s.pool.QueryRow(ctx, `SELECT name, email, roles FROM users WHERE id = $1`, userID).
		Scan(&p.User.Name, &p.User.Email, &p.User.Roles)

	return p, nil
}

// ApplyToJob creates a job application record.
// Called by the queue WORKER (not the HTTP handler directly).
//
// 📝 WHY ASYNC VIA QUEUE?
//   The handler enqueues the task and returns 202 immediately.
//   The worker calls ApplyToJob in the background.
//   This keeps the HTTP response fast and allows retries on DB failure.
//
// 📝 CHECKS PERFORMED:
//   1. Job seeker has a profile
//   2. Job exists and is still active
//   3. User hasn't already applied to this job (409 if duplicate)
func (s *JobSeekerService) ApplyToJob(ctx context.Context, userID, jobID, coverLetter string) (models.ApplicationRow, error) {
	var app models.ApplicationRow
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return app, err
	}

	// Verify job is active
	var isActive bool
	err = s.pool.QueryRow(ctx, `SELECT is_active FROM jobs WHERE id = $1 LIMIT 1`, jobID).Scan(&isActive)
	if err != nil {
		return app, apperrors.New("Job not found", 404)
	}
	if !isActive {
		return app, apperrors.New("This job is no longer active", 410) // 410 Gone
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
//
// 📝 JOIN PATTERN:
//   applications a JOIN jobs j JOIN recruiter_profiles r
//   The result includes job title, location, salary, category, and company name —
//   everything the frontend needs to display the application list without extra calls.
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
// Only the job seeker who created the application can withdraw it.
//
// 📝 OWNERSHIP CHECK PATTERN:
//   1. SELECT job_seeker_id FROM applications WHERE id = $1
//   2. Compare against the authenticated user's jobSeekerID
//   3. 403 Forbidden if they don't match
func (s *JobSeekerService) WithdrawApplication(ctx context.Context, userID, applicationID string) error {
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return err
	}

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

// SaveJob saves a job to the seeker's list (upsert — idempotent).
// Called by the queue WORKER after a save-job task is dequeued.
//
// 📝 ON CONFLICT DO UPDATE (upsert):
//   If the row already exists (same job_id + job_seeker_id), the INSERT
//   would fail with a unique constraint error. Instead, we do a no-op UPDATE.
//   "ON CONFLICT ... DO UPDATE SET job_id = EXCLUDED.job_id" re-sets job_id to itself
//   (effectively a no-op) so the query succeeds and RETURNING still works.
//   This makes SaveJob idempotent: calling it twice doesn't create duplicates.
func (s *JobSeekerService) SaveJob(ctx context.Context, userID, jobID string) (models.SavedJobRow, error) {
	var saved models.SavedJobRow
	jobSeekerID, err := s.getJobSeekerID(ctx, userID)
	if err != nil {
		return saved, err
	}

	// Verify the job exists
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
// Same JOIN pattern as GetApplications — includes job details in one query.
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
//
// 📝 tag.RowsAffected():
//   pool.Exec returns a CommandTag with metadata about the query result.
//   RowsAffected() == 0 means no row matched the WHERE clause (job wasn't saved).
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

// skillsArg converts a nil []string to nil (SQL NULL) so COALESCE works correctly.
// When skills == nil (not provided in PATCH), we pass SQL NULL → COALESCE keeps existing.
// When skills == []string{} (empty array), we pass the empty array → COALESCE updates to {}.
//
// 📝 WHY interface{}?
//   pgx accepts both []string and nil as the $4 parameter.
//   Returning interface{} lets us return either without a type mismatch.
func skillsArg(skills []string) interface{} {
	if skills == nil {
		return nil // → SQL NULL → COALESCE keeps existing value
	}
	return skills // → SQL array → COALESCE uses this value
}
