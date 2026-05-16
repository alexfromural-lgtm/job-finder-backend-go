-- name: ListActiveJobs :many
-- Supports optional search (?search=), category (?category=), location (?location=)
-- All filters are optional — pass empty string to skip.
SELECT
  j.id, j.recruiter_id, j.title, j.description, j.requirements,
  j.location, j.salary_range, j.category, j.is_active, j.created_at, j.updated_at,
  r.company_name
FROM jobs j
JOIN recruiter_profiles r ON r.id = j.recruiter_id
WHERE
  j.is_active = TRUE
  AND ($1::text = '' OR (
        j.title       ILIKE '%' || $1 || '%'
     OR j.description ILIKE '%' || $1 || '%'
     OR j.requirements ILIKE '%' || $1 || '%'
  ))
  AND ($2::text = '' OR j.category = $2)
  AND ($3::text = '' OR j.location ILIKE '%' || $3 || '%')
ORDER BY j.created_at DESC;

-- name: GetJobByID :one
SELECT
  j.id, j.recruiter_id, j.title, j.description, j.requirements,
  j.location, j.salary_range, j.category, j.is_active, j.created_at, j.updated_at,
  r.company_name
FROM jobs j
JOIN recruiter_profiles r ON r.id = j.recruiter_id
WHERE j.id = $1
LIMIT 1;

-- name: GetJobsByRecruiterID :many
SELECT
  j.id, j.recruiter_id, j.title, j.description, j.requirements,
  j.location, j.salary_range, j.category, j.is_active, j.created_at, j.updated_at,
  r.company_name
FROM jobs j
JOIN recruiter_profiles r ON r.id = j.recruiter_id
WHERE j.recruiter_id = $1
ORDER BY j.created_at DESC;

-- name: CreateJob :one
INSERT INTO jobs (recruiter_id, title, description, requirements, location, salary_range, category)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateJob :one
UPDATE jobs
SET
  title        = $2,
  description  = $3,
  requirements = $4,
  location     = $5,
  salary_range = $6,
  category     = $7
WHERE id = $1
RETURNING *;

-- name: DeleteJob :exec
DELETE FROM jobs WHERE id = $1;

-- name: GetJobRecruiterID :one
SELECT recruiter_id FROM jobs WHERE id = $1 LIMIT 1;
