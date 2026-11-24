# 🎓 Job Finder Backend (Go) — Complete Learning Guide

Welcome to the **Job Finder Backend (Go)**! This branch (`learning`) is designed as an interactive, line-by-line learning resource. Every file has been annotated with extensive comments, analogies, and small examples comparing standard Go idioms with Node.js/TypeScript patterns.

---

## 🏛️ Architecture Overview

The application follows clean architectural layering with **Dependency Injection**:

```
                       HTTP Request
                            │
                            ▼
              ┌───────────────────────────┐
              │   chi HTTP Router & Mux   │
              │  (cmd/server/main.go)     │
              └─────────────┬─────────────┘
                            │
               Global Middleware Pipeline
        [SecureHeaders] ──► [Logger] ──► [CORS]
                            │
                            ▼
             Route Group Middleware Pipeline
            [RequireAuth] ──► [AuthorizeRoles]
                            │
                            ▼
              ┌───────────────────────────┐
              │       HTTP Handlers       │
              │   (internal/handlers/*)   │
              │ - Decodes JSON Body (DTO) │
              │ - Validates Struct Tags   │
              │ - Writes JSON Response    │
              └─────────────┬─────────────┘
                            │
              ┌─────────────┴─────────────┐
              │                           │ (Async actions)
              ▼                           ▼
┌───────────────────────────┐ ┌───────────────────────────┐
│     Business Services     │ │    Redis Task Queue       │
│   (internal/services/*)   │ │    (internal/queue/*)     │
│ - Check business rules    │ │ - Enqueue "apply-to-job"  │
│ - Password hashing        │ │ - Enqueue "save-job"      │
│ - Issue JWT tokens        │ └─────────────┬─────────────┘
└─────────────┬─────────────┘               │ (Background Consumer)
              │                             ▼
              │                ┌───────────────────────────┐
              │                │    Asynq Worker Server    │
              │                │ - Concurrently executes   │
              │                │   task handlers           │
              │                └────────────┬──────────────┘
              │                             │
              ▼                             ▼
┌─────────────────────────────────────────────────────────┐
│        PostgreSQL Connection Pool (pgx/v5)              │
│                (internal/db/db.go)                      │
└─────────────────────────────────────────────────────────┘
```

---

## 📂 Project Structure & Learning Map

| File Path | Responsibility | Key Concepts Covered |
| :--- | :--- | :--- |
| [`cmd/server/main.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/cmd/server/main.go) | Application entry point | Server lifecycle, graceful shutdown with channels & OS signals, Chi router, dependency injection. |
| [`cmd/seed/main.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/cmd/seed/main.go) | Standalone database seeder | Migrations on startup, Foreign-Key safe truncations, mock user creation, bcrypt hashing. |
| [`internal/config/config.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/config/config.go) | Environment configuration | Fail-fast validation, custom duration parsing (`"7d"`), type-safe config struct. |
| [`internal/db/db.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/db/db.go) | Database connection pool | `pgxpool.Pool` lifecycle, connection reuse, health check ping on startup. |
| [`internal/errors/app_error.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/errors/app_error.go) | Typed application errors | Custom error types implementing Go's `error` interface, HTTP status code propagation. |
| [`internal/utils/hash.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/utils/hash.go) | Cryptographic password hashing | Bcrypt slow hashing, salt generation, constant-time comparison against timing attacks. |
| [`internal/utils/token.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/utils/token.go) | JWT token generation & verification | HMAC-SHA256 signing, claims embedding, custom claims + registered claims. |
| [`internal/utils/cookie.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/utils/cookie.go) | HttpOnly Cookie management | Cookie security attributes (`HttpOnly`, `SameSite=Lax`, `Secure`, `MaxAge`). |
| [`internal/middleware/auth.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/middleware/auth.go) | Authentication & RBAC | Request context propagation (`context.WithValue`), token verification, role-based checks. |
| [`internal/middleware/validate.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/middleware/validate.go) | Generic DTO validation | Go Generics (`[T any]`), JSON streaming decoder, struct-tag validation with validator/v10. |
| [`internal/middleware/error_handler.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/middleware/error_handler.go) | Central error classifier | `errors.As` and `errors.Is`, PostgreSQL error code `23505` (unique constraint), generic 500 fallback. |
| [`internal/middleware/security.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/middleware/security.go) | HTTP Security Headers | Clickjacking prevention (`X-Frame-Options`), MIME sniffing (`X-Content-Type-Options`), HSTS. |
| [`internal/queue/tasks.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/queue/tasks.go) | Queue task types & payloads | Task type constants, JSON payload serialization schemas. |
| [`internal/queue/client.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/queue/client.go) | Queue producer client | Redis task enqueueing, task retention periods for status polling. |
| [`internal/queue/worker.go`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/queue/worker.go) | Queue consumer worker | Task mux dispatcher, concurrent worker pool, task result writing, graceful draining. |
| [`internal/handlers/*`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/handlers) | HTTP Handlers | Decoding requests, invoking services, handling HTTP verbs, returning JSON and status codes. |
| [`internal/services/*`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/services) | Business Logic Layer | Parameterized SQL queries, transactions (`tx.Begin/Commit/Rollback`), dynamic update queries. |
| [`internal/models/*`](file:///c:/TestPrj/job-finder/job-finder-backend-go/internal/models) | Data Transfer Objects (DTOs) | Struct tags for JSON & validation, pointer fields for optional PATCH updates. |

---

## 🔑 Key Patterns & How to Learn From Them

### 1. Pointer Fields for Partial Updates (PATCH Semantics)
In Go, primitive types like `string` have zero-values (`""`). If a client sends:
```json
{ "title": "Senior Engineer" }
```
A normal struct would have `Description = ""`, making it impossible to know whether the user wants to erase the description or leave it untouched.
By using `*string` (pointer to string):
- `Description == nil`: client omitted the field (keep current value in database).
- `*Description == "Senior Engineer"`: client explicitly provided a new value.

### 2. Context Value Propagation
In Go, you never attach variables to global objects. To pass the authenticated user's ID from middleware to handlers:
```go
// In middleware:
ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
next.ServeHTTP(w, r.WithContext(ctx))

// In handler:
userID, ok := middleware.GetUserID(r.Context())
```

### 3. Asynchronous Operations (HTTP 202 Accepted)
When an operation might take time (e.g. applying for a job, generating notifications):
1. **Handler** enqueues a task to Redis and responds immediately with `202 Accepted` and a `queueJobId`.
2. **Worker** picks up the task from Redis and executes it in the background.
3. **Frontend** polls `GET /api/queue/job/:taskId` until `status == "completed"`.

---

## 🚀 Running the Project

```bash
# 1. Start all containers (Postgres, Redis, API, pgAdmin)
docker compose up -d

# 2. Run the database seeder to populate sample data
docker compose run --rm backend go run ./cmd/seed

# 3. Test the healthcheck endpoint
curl http://localhost:5002/health
# Output: {"status":"ok"}
```
