-- name: GetUserByEmail :one
SELECT id, name, email, password, roles, is_active, created_at, updated_at
FROM users
WHERE email = $1
LIMIT 1;

-- name: GetUserByID :one
SELECT id, name, email, password, roles, is_active, created_at, updated_at
FROM users
WHERE id = $1
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (name, email, password, roles)
VALUES ($1, $2, $3, $4)
RETURNING id, name, email, roles, is_active, created_at, updated_at;

-- name: UpdateUserRoles :one
UPDATE users
SET roles = $2
WHERE id = $1
RETURNING id, name, email, roles, is_active, created_at, updated_at;

-- name: CreateJobSeekerProfile :one
INSERT INTO job_seeker_profiles (user_id)
VALUES ($1)
RETURNING *;

-- name: GetJobSeekerProfileByUserID :one
SELECT * FROM job_seeker_profiles WHERE user_id = $1 LIMIT 1;

-- name: CreateRecruiterProfile :one
INSERT INTO recruiter_profiles (user_id, company_name, company_website, industry, description)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRecruiterProfileByUserID :one
SELECT * FROM recruiter_profiles WHERE user_id = $1 LIMIT 1;

-- name: RecruiterProfileExistsByUserID :one
SELECT EXISTS (SELECT 1 FROM recruiter_profiles WHERE user_id = $1) AS exists;
