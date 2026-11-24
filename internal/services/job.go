// ============================================================
// FILE: internal/services/job.go
// ROLE: Job listing CRUD business logic.
//
// 📚 KEY CONCEPTS IN THIS FILE:
//
//   PAGINATION:
//     Returning all jobs in one query would be slow for large datasets.
//     We return `pageSize` rows at a time, offset by (page-1)*pageSize.
//     The response also includes total count + totalPages for the frontend.
//
//   DYNAMIC WHERE CLAUSES:
//     The search/filter query uses $1::text = '' OR ... to conditionally
//     apply filters. When the user sends no filter, $1 = '' and the
//     entire OR branch short-circuits to TRUE (no filtering).
//     This avoids building the SQL string dynamically for read queries.
//
//   DYNAMIC UPDATE:
//     UpdateJob uses pointer fields (*string) in the request struct.
//     nil pointer = "don't update this field"
//     non-nil pointer = "update to this value"
//     We build the SET clause dynamically by checking each pointer.
//
//   OWNERSHIP CHECKS:
//     Recruiter operations (create/update/delete job) first verify that
//     the authenticated user owns the recruiter_profiles row, then verify
//     the recruiter owns the specific job.
//
// 📝 COMPARISON with Node.js:
//   job.service.ts → JobService struct methods
// ============================================================
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
type JobService struct {
	pool *pgxpool.Pool
}

// NewJobService constructs a JobService.
func NewJobService(pool *pgxpool.Pool) *JobService {
	return &JobService{pool: pool}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// scanJob scans a pgx.Row or pgx.Rows into a JobRow.
// The pgx.Row interface is satisfied by both pgx.Row and pgx.Rows,
// so this helper works for both single-row and multi-row queries.
//
// 📝 WHY A SCAN HELPER?
//   The same SELECT column list appears in GetAllJobs, GetJobByID, GetJobsByRecruiter, etc.
//   A single scanJob function keeps the scan logic in one place — if the schema changes,
//   update one function instead of five.
//
// 📝 COLUMN ORDER MUST MATCH SELECT:
//   &j.ID, &j.RecruiterID, &j.Title, ... must be in the EXACT order returned by SQL.
func scanJob(s pgx.Row) (models.JobRow, error) {
	var j models.JobRow
	var salaryRange, category string // scan nullable columns into plain strings
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

// getRecruiterProfileID resolves a userID to the recruiter_profiles.id.
// Every job is owned by a recruiter_profiles row (not directly by a user).
// Returns 404 AppError if the user has no recruiter profile.
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

// GetAllJobs returns a paginated, filtered list of active jobs.
//
// 📝 QUERY PARAMETERS (from HTTP request, parsed in handler):
//   search   → full-text substring search across title, description, requirements
//   category → exact match on job.category column
//   location → substring search on job.location column
//   page     → page number (1-indexed)
//   pageSize → results per page (clamped to 1-100, default 10)
//
// 📝 PAGINATION MATH:
//   page=2, pageSize=10 → offset = (2-1)*10 = 10 → skip first 10 rows
//   totalPages = ceil(total / pageSize)
//     total=25, pageSize=10 → ceil(25/10) = ceil(2.5) = 3 pages
//
// 📝 $1::text = '' OR condition:
//   This PostgreSQL trick: when $1 == "", the whole OR is TRUE (no filter).
//   When $1 == "engineer", only rows where title ILIKE '%engineer%' pass.
//   ILIKE is case-insensitive LIKE.
func (s *JobService) GetAllJobs(ctx context.Context, search, category, location string, page, pageSize int) (models.JobsListResponse, error) {
	// Clamp pagination parameters to safe defaults
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize // e.g. page=3 → offset=20

	// First query: count total matching rows (for pagination metadata)
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

	// Second query: fetch the actual page of jobs with company info joined
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
	defer rows.Close() // always close rows to return the connection to the pool

	// Iterate rows and collect into a slice
	jobs := make([]models.JobRow, 0) // make([]T, 0) creates an empty (non-nil) slice
	for rows.Next() {
		// rows.Next() advances the cursor; returns false when no more rows
		j, err := scanJob(rows) // rows satisfies pgx.Row interface
		if err != nil {
			return models.JobsListResponse{}, err
		}
		jobs = append(jobs, j) // append grows the slice dynamically
	}

	return models.JobsListResponse{
		Jobs: jobs,
		Meta: models.JobsListMeta{
			Total:    total,
			Page:     page,
			PageSize: pageSize,
			// math.Ceil rounds up: 11 jobs / 10 per page = 2 pages
			TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
		},
	}, nil
}

// GetJobByID returns a single job by UUID, with company name joined.
// Returns 404 AppError if the job doesn't exist.
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

// GetJobsByRecruiter returns all jobs posted by the authenticated recruiter.
// First resolves userID → recruiter_profiles.id, then queries jobs by that ID.
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

// CreateJob inserts a new job listing for the authenticated recruiter.
// After INSERT, re-fetches the full row (with company name join) using GetJobByID.
//
// 📝 WHY RETURNING id THEN RE-FETCH?
//   The INSERT RETURNING only returns the new row's columns from the jobs table.
//   company_name comes from a JOIN with recruiter_profiles — not available in RETURNING.
//   So we INSERT → get id → SELECT with JOIN to get the full response shape.
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

	return s.GetJobByID(ctx, j.ID)
}

// UpdateJob applies partial field updates to a job, verifying recruiter ownership.
// Only non-nil fields in the request are updated (PATCH semantics).
//
// 📝 DYNAMIC SQL WITH $N PLACEHOLDERS:
//   We build the SET clause by checking which *string fields are non-nil.
//   args[0] = jobID ($1 in SQL)
//   For each non-nil field, we append "field = $N" and the value.
//   argIdx tracks the current placeholder number.
//
// 📝 EXAMPLE:
//   Request: {"title": "Senior Engineer"}  (only title provided)
//   SQL:     UPDATE jobs SET title = $2, updated_at = NOW() WHERE id = $1
//   Args:    [jobID, "Senior Engineer"]
func (s *JobService) UpdateJob(ctx context.Context, userID, jobID string, req models.UpdateJobRequest) (models.JobRow, error) {
	recruiterID, err := s.getRecruiterProfileID(ctx, userID)
	if err != nil {
		return models.JobRow{}, err
	}

	// Verify the recruiter owns this job
	var ownerID string
	err = s.pool.QueryRow(ctx, `SELECT recruiter_id FROM jobs WHERE id = $1 LIMIT 1`, jobID).Scan(&ownerID)
	if err != nil {
		return models.JobRow{}, apperrors.New("Job not found", 404)
	}
	if ownerID != recruiterID {
		return models.JobRow{}, apperrors.New("You are not authorized to update this job", 403)
	}

	// Build SET clause dynamically
	setClauses := []string{}
	args := []any{jobID} // $1 is always the jobID (used in WHERE)
	argIdx := 2          // next $N placeholder starts at 2

	// Check each field — if the pointer is non-nil, include the field in the update
	if req.Title != nil {
		setClauses = append(setClauses, "title = $"+argN(argIdx))
		args = append(args, *req.Title) // *req.Title dereferences the pointer to get the string value
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
		// Nothing to update — return the current state
		return s.GetJobByID(ctx, jobID)
	}

	// Always update updated_at timestamp
	setClauses = append(setClauses, "updated_at = NOW()")

	// Build the final SQL: "UPDATE jobs SET title = $2, updated_at = NOW() WHERE id = $1"
	query := "UPDATE jobs SET " + strings.Join(setClauses, ", ") + " WHERE id = $1"

	// args... unpacks the slice as variadic arguments: Exec(ctx, sql, arg1, arg2, ...)
	_, err = s.pool.Exec(ctx, query, args...)
	if err != nil {
		return models.JobRow{}, err
	}

	return s.GetJobByID(ctx, jobID)
}

// DeleteJob removes a job, verifying recruiter ownership first.
// Hard delete — no soft delete / is_active flag for now.
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
// strconv.Itoa is faster than fmt.Sprintf for simple int→string conversion.
//
// 📝 EXAMPLE: argN(3) → "3"  → used in "salary_range = $3"
func argN(n int) string { return strconv.Itoa(n) }
