-- name: UpdateRecruiterProfile :one
UPDATE recruiter_profiles
SET
  company_name    = COALESCE($2, company_name),
  company_website = COALESCE($3, company_website),
  description     = COALESCE($4, description),
  industry        = COALESCE($5, industry)
WHERE user_id = $1
RETURNING *;

-- name: GetApplicationsByJobID :many
SELECT
  a.id, a.job_id, a.job_seeker_id, a.cover_letter, a.status,
  a.created_at, a.updated_at,
  u.name AS applicant_name, u.email AS applicant_email,
  p.resume_url, p.bio, p.skills
FROM applications a
JOIN job_seeker_profiles p ON p.id = a.job_seeker_id
JOIN users u ON u.id = p.user_id
WHERE a.job_id = $1
ORDER BY a.created_at DESC;

-- name: UpdateApplicationStatus :one
UPDATE applications
SET status = $2
WHERE id = $1
RETURNING *;
