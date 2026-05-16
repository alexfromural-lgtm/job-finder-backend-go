// Package services contains business logic, mirroring src/services/ in the Node.js backend.
package services

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
)

// JobService handles all job-related business logic.
// Mirrors src/services/job.service.ts.
type JobService struct {
	pool *pgxpool.Pool
}

// NewJobService constructs a JobService.
func NewJobService(pool *pgxpool.Pool) *JobService {
	return &JobService{pool: pool}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// scanJob scans a pgx.Row / pgx.Rows into a JobRow.
// Column order must match the SELECT in jobs.sql.
func scanJob(s pgx.Row) (models.JobRow, error) {
	var j models.JobRow
	var salaryRange, category string
	err := s.Scan(
		&j.ID, &j.RecruiterID, &j.Title, &j.Description, &j.Requirements,
		&j.Location, &salaryRange, &category,
		&j.IsActive, &j.CreatedAt, &j.UpdatedAt,
		&j.CompanyName,
	)
	j.SalaryRange = salaryRange
	j.Category = category
	return j, err
}

// getRecruiterProfileID resolves a userID → recruiter_profiles.id.
// Returns AppError 404 if no profile exists.
// Mirrors validateRecruiterProfile() from src/utils/job.utils.ts.
func (s *JobService) getRecruiterProfileID(ctx context.Context, userID string) (string, error) {
	var recruiterID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM recruiter_profiles WHERE user_id = $1 LIMIT 1`, userID).
		Scan(&recruiterID)
	if err != nil {
		return "", apperrors.New("Recruiter profile not found. Please create one first.", 404)
	}
	return recruiterID, nil
}

// ── public service methods ────────────────────────────────────────────────────

// GetAllJobs returns a paginated, optionally-filtered list of active jobs.
// Mirrors src/services/job.service.ts → getAllJobs().
func (s *JobService) GetAllJobs(ctx context.Context, search, category, location string, page, pageSize int) (models.JobsListResponse, error) {
	// Clamp + default pagination
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// Count total matching rows first
	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM jobs j
		 WHERE j.is_active = TRUE
		   AND ($1::text = '' OR (j.title ILIKE '%'||$1||'%' OR j.description ILIKE '%'||$1||'%' OR j.requirements ILIKE '%'||$1||'%'))
		   AND ($2::text = '' OR j.category = $2)
		   AND ($3::text = '' OR j.location ILIKE '%'||$3||'%')`,
		search, category, location).Scan(&total)
	if err != nil {
		return models.JobsListResponse{}, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT j.id, j.recruiter_id, j.title, j.description, j.requirements,
		        j.location, j.salary_range, j.category, j.is_active, j.created_at, j.updated_at,
		        r.company_name
		 FROM jobs j
		 JOIN recruiter_profiles r ON r.id = j.recruiter_id
		 WHERE j.is_active = TRUE
		   AND ($1::text = '' OR (j.title ILIKE '%'||$1||'%' OR j.description ILIKE '%'||$1||'%' OR j.requirements ILIKE '%'||$1||'%'))
		   AND ($2::text = '' OR j.category = $2)
		   AND ($3::text = '' OR j.location ILIKE '%'||$3||'%')
		 ORDER BY j.created_at DESC
		 LIMIT $4 OFFSET $5`,
		search, category, location, pageSize, offset)
	if err != nil {
		return models.JobsListResponse{}, err
	}
	defer rows.Close()

	jobs := make([]models.JobRow, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return models.JobsListResponse{}, err
		}
		jobs = append(jobs, j)
	}

	return models.JobsListResponse{
		Jobs: jobs,
		Meta: models.JobsListMeta{
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
		},
	}, nil
}

// GetJobByID returns a single job by its UUID, with recruiter company info.
// Returns AppError 404 if not found.
// Mirrors src/services/job.service.ts → getJobById().
func (s *JobService) GetJobByID(ctx context.Context, jobID string) (models.JobRow, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT j.id, j.recruiter_id, j.title, j.description, j.requirements,
		        j.location, j.salary_range, j.category, j.is_active, j.created_at, j.updated_at,
		        r.company_name
		 FROM jobs j
		 JOIN recruiter_profiles r ON r.id = j.recruiter_id
		 WHERE j.id = $1 LIMIT 1`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return models.JobRow{}, apperrors.New("Job not found", 404)
	}
	return job, nil
}

// GetJobsByRecruiter returns all jobs owned by the recruiter profile for userID.
// Mirrors src/services/job.service.ts → getJobsByRecruiter().
func (s *JobService) GetJobsByRecruiter(ctx context.Context, userID string) ([]models.JobRow, error) {
	recruiterID, err := s.getRecruiterProfileID(ctx, userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT j.id, j.recruiter_id, j.title, j.description, j.requirements,
		        j.location, j.salary_range, j.category, j.is_active, j.created_at, j.updated_at,
		        r.company_name
		 FROM jobs j
		 JOIN recruiter_profiles r ON r.id = j.recruiter_id
		 WHERE j.recruiter_id = $1
		 ORDER BY j.created_at DESC`, recruiterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]models.JobRow, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// CreateJob inserts a new job for the authenticated recruiter.
// Mirrors src/services/job.service.ts → createJob().
func (s *JobService) CreateJob(ctx context.Context, userID string, req models.CreateJobRequest) (models.JobRow, error) {
	recruiterID, err := s.getRecruiterProfileID(ctx, userID)
	if err != nil {
		return models.JobRow{}, err
	}

	var j models.JobRow
	err = s.pool.QueryRow(ctx,
		`INSERT INTO jobs (recruiter_id, title, description, requirements, location, salary_range, category)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		recruiterID, req.Title, req.Description, req.Requirements, req.Location, req.SalaryRange, req.Category).
		Scan(&j.ID)
	if err != nil {
		return models.JobRow{}, err
	}

	// Fetch the full row with company_name join
	return s.GetJobByID(ctx, j.ID)
}

// UpdateJob applies partial field updates to a job, enforcing recruiter ownership.
// Mirrors src/services/job.service.ts → updateJob().
func (s *JobService) UpdateJob(ctx context.Context, userID, jobID string, req models.UpdateJobRequest) (models.JobRow, error) {
	recruiterID, err := s.getRecruiterProfileID(ctx, userID)
	if err != nil {
		return models.JobRow{}, err
	}

	// Verify ownership
	var ownerID string
	err = s.pool.QueryRow(ctx, `SELECT recruiter_id FROM jobs WHERE id = $1 LIMIT 1`, jobID).Scan(&ownerID)
	if err != nil {
		return models.JobRow{}, apperrors.New("Job not found", 404)
	}
	if ownerID != recruiterID {
		return models.JobRow{}, apperrors.New("You are not authorized to update this job", 403)
	}

	// Build SET clause dynamically from non-nil fields
	setClauses := []string{}
	args := []any{jobID} // $1 = jobID
	argIdx := 2

	if req.Title != nil {
		setClauses = append(setClauses, "title = $"+argN(argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = $"+argN(argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Requirements != nil {
		setClauses = append(setClauses, "requirements = $"+argN(argIdx))
		args = append(args, *req.Requirements)
		argIdx++
	}
	if req.Location != nil {
		setClauses = append(setClauses, "location = $"+argN(argIdx))
		args = append(args, *req.Location)
		argIdx++
	}
	if req.SalaryRange != nil {
		setClauses = append(setClauses, "salary_range = $"+argN(argIdx))
		args = append(args, *req.SalaryRange)
		argIdx++
	}
	if req.Category != nil {
		setClauses = append(setClauses, "category = $"+argN(argIdx))
		args = append(args, *req.Category)
		argIdx++
	}

	if len(setClauses) == 0 {
		// Nothing to update — return the current job unchanged
		return s.GetJobByID(ctx, jobID)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := "UPDATE jobs SET " + strings.Join(setClauses, ", ") + " WHERE id = $1"

	_, err = s.pool.Exec(ctx, query, args...)
	if err != nil {
		return models.JobRow{}, err
	}

	return s.GetJobByID(ctx, jobID)
}

// DeleteJob removes a job, enforcing recruiter ownership.
// Mirrors src/services/job.service.ts → deleteJob().
func (s *JobService) DeleteJob(ctx context.Context, userID, jobID string) error {
	recruiterID, err := s.getRecruiterProfileID(ctx, userID)
	if err != nil {
		return err
	}

	var ownerID string
	err = s.pool.QueryRow(ctx, `SELECT recruiter_id FROM jobs WHERE id = $1 LIMIT 1`, jobID).Scan(&ownerID)
	if err != nil {
		return apperrors.New("Job not found", 404)
	}
	if ownerID != recruiterID {
		return apperrors.New("You are not authorized to delete this job", 403)
	}

	_, err = s.pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID)
	return err
}

// argN converts an int to its string representation for SQL $N placeholders.
func argN(n int) string { return strconv.Itoa(n) }
