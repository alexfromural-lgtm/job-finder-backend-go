-- name: GetJobSeekerProfileByID :one
SELECT * FROM job_seeker_profiles WHERE id = $1 LIMIT 1;

-- name: UpdateJobSeekerProfile :one
UPDATE job_seeker_profiles
SET
  bio        = COALESCE($2, bio),
  location   = COALESCE($3, location),
  skills     = COALESCE($4, skills),
  education  = COALESCE($5, education),
  experience = COALESCE($6, experience),
  resume_url = COALESCE($7, resume_url)
WHERE user_id = $1
RETURNING *;

-- name: CreateApplication :one
INSERT INTO applications (job_id, job_seeker_id, cover_letter)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetApplicationsByJobSeekerID :many
SELECT
  a.id, a.job_id, a.job_seeker_id, a.cover_letter, a.status,
  a.created_at, a.updated_at,
  j.title AS job_title, j.location AS job_location
FROM applications a
JOIN jobs j ON j.id = a.job_id
WHERE a.job_seeker_id = $1
ORDER BY a.created_at DESC;

-- name: GetApplicationByID :one
SELECT * FROM applications WHERE id = $1 LIMIT 1;

-- name: DeleteApplication :exec
DELETE FROM applications WHERE id = $1 AND job_seeker_id = $2;

-- name: ApplicationExists :one
SELECT EXISTS (
  SELECT 1 FROM applications
  WHERE job_id = $1 AND job_seeker_id = $2
) AS exists;

-- name: CreateSavedJob :one
INSERT INTO saved_jobs (job_id, job_seeker_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetSavedJobsByJobSeekerID :many
SELECT
  s.id, s.job_id, s.job_seeker_id, s.saved_at,
  j.title AS job_title, j.location AS job_location, j.salary_range, j.category
FROM saved_jobs s
JOIN jobs j ON j.id = s.job_id
WHERE s.job_seeker_id = $1
ORDER BY s.saved_at DESC;

-- name: DeleteSavedJob :exec
DELETE FROM saved_jobs WHERE job_id = $1 AND job_seeker_id = $2;

-- name: SavedJobExists :one
SELECT EXISTS (
  SELECT 1 FROM saved_jobs
  WHERE job_id = $1 AND job_seeker_id = $2
) AS exists;
