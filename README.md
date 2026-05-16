# Job Finder API — Go Backend

A **100% wire-compatible** Go rewrite of the Node.js/Express Job Finder backend, built with [`chi`](https://github.com/go-chi/chi), [`pgx/v5`](https://github.com/jackc/pgx), and [`asynq`](https://github.com/hibiken/asynq).

## Tech Stack

| Concern | Go implementation |
|---|---|
| HTTP router | `go-chi/chi/v5` |
| Database driver | `jackc/pgx/v5` (raw SQL, no ORM) |
| Auth (JWT) | `golang-jwt/jwt/v5` |
| Password hashing | `golang.org/x/crypto/bcrypt` |
| Request validation | `go-playground/validator/v10` |
| Async queue | `hibiken/asynq` (Redis-backed) |
| CORS | `rs/cors` |
| Gzip (production) | `klauspost/compress/gzhttp` |

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [Go 1.22+](https://go.dev/dl/) (for local development only)

## Quick Start

```bash
# 1. Copy and configure environment
cp .env.sample .env
# Edit .env — set ACCESS_TOKEN_SECRET, REFRESH_TOKEN_SECRET (≥16 chars each)

# 2. Start all services (PostgreSQL, Redis, pgAdmin, API)
docker compose up --build

# 3. (Optional) Seed the database with demo data
docker compose run --rm backend go run ./cmd/seed
```

The API is now available at **http://localhost:5002**.

## Development (without Docker)

```bash
# Start infrastructure only
docker compose up db redis -d

# Run the API server locally
go run ./cmd/server

# Seed the database
go run ./cmd/seed
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | ✅ | — | PostgreSQL connection string |
| `ACCESS_TOKEN_SECRET` | ✅ | — | JWT access token secret (≥16 chars) |
| `REFRESH_TOKEN_SECRET` | ✅ | — | JWT refresh token secret (≥16 chars) |
| `REDIS_URL` | ✅ | — | Redis connection URL |
| `ACCESS_TOKEN_EXPIRES_IN` | | `15m` | Access token TTL |
| `REFRESH_TOKEN_EXPIRES_IN` | | `7d` | Refresh token TTL |
| `PORT` | | `5002` | HTTP server port |
| `GO_ENV` | | `development` | Set to `production` to enable gzip + HSTS |
| `CORS_ORIGIN` | | `http://localhost:3000` | Allowed CORS origin |
| `QUEUE_CONCURRENCY` | | `5` | asynq worker concurrency |

## API Routes

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/auth/signup/jobseeker` | — | Register as job seeker |
| `POST` | `/api/auth/signup/recruiter` | — | Register as recruiter |
| `POST` | `/api/auth/login` | — | Login (sets HttpOnly cookies) |
| `POST` | `/api/auth/logout` | — | Clear auth cookies |
| `POST` | `/api/auth/refresh` | — | Refresh access token |
| `GET` | `/api/auth/me` | JWT | Current user info |
| `POST` | `/api/auth/upgrade/recruiter` | JWT | Upgrade to recruiter role |
| `GET` | `/api/jobs/all` | — | List jobs (filterable, paginated) |
| `GET` | `/api/jobs/:id` | — | Get single job |
| `GET` | `/api/jobs/recruiter` | RECRUITER | Recruiter's own jobs |
| `POST` | `/api/jobs` | RECRUITER | Create job |
| `PUT` | `/api/jobs/:id` | RECRUITER | Update job (partial) |
| `DELETE` | `/api/jobs/:id` | RECRUITER | Delete job |
| `GET` | `/api/jobseeker/profile` | JOB_SEEKER | Get profile |
| `PATCH` | `/api/jobseeker/profile` | JOB_SEEKER | Update profile |
| `POST` | `/api/jobseeker/apply/:jobId` | JOB_SEEKER | Apply to job (async → 202) |
| `GET` | `/api/jobseeker/applications` | JOB_SEEKER | My applications |
| `DELETE` | `/api/jobseeker/applications/:id` | JOB_SEEKER | Withdraw application |
| `POST` | `/api/jobseeker/saved/:jobId` | JOB_SEEKER | Save job (async → 202) |
| `GET` | `/api/jobseeker/saved` | JOB_SEEKER | My saved jobs |
| `DELETE` | `/api/jobseeker/saved/:jobId` | JOB_SEEKER | Unsave job |
| `GET` | `/api/recruiter/profile` | RECRUITER | Recruiter profile |
| `PATCH` | `/api/recruiter/profile` | RECRUITER | Update recruiter profile |
| `GET` | `/api/recruiter/jobs/:jobId/applications` | RECRUITER | Applications for a job |
| `PATCH` | `/api/recruiter/applications/:id/status` | RECRUITER | Update application status |
| `GET` | `/api/queue/job/:taskId` | — | Poll async task status |
| `GET` | `/health` | — | Health check |

## Demo Seed Credentials

After running `go run ./cmd/seed`:

| Role | Email | Password |
|---|---|---|
| Admin | admin@example1.com | admin |
| Recruiter | recruiter@example.com | recruiter123 |
| Job Seeker | seeker@example.com | seeker123 |

## Project Structure

```
cmd/
  server/main.go      # Entry point — wires all components
  seed/main.go        # Standalone DB seeder
internal/
  config/             # Env var parsing + validation (fail-fast)
  db/                 # pgx connection pool + migrations + SQL queries
  errors/             # AppError type with HTTP status
  handlers/           # HTTP handlers (auth, jobs, jobseeker, recruiter, queue)
  middleware/         # RequireAuth, AuthorizeRoles, DecodeAndValidate, SecureHeaders
  models/             # Request/response DTOs
  queue/              # asynq client, worker server, task definitions
  services/           # Business logic (auth, jobs, jobseeker, recruiter)
  utils/              # JWT, bcrypt, cookie helpers
```
