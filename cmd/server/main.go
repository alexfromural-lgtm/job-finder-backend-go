// ============================================================
// FILE: cmd/server/main.go
// ROLE: Application entry point (the "main" program).
//
// 📚 WHAT IS THIS FILE?
//   Every Go program starts in a function called main() inside
//   a package called "main". This file wires together all the
//   pieces of the server: config, database, queue, router, and
//   HTTP server — then keeps running until a signal arrives.
//
// 📐 ARCHITECTURE OVERVIEW
//   Config ──► DB Pool ──► Services ──► Handlers ──► chi Router ──► HTTP Server
//                     └──► Queue Client ──► JobSeekerHandler
//                     └──► Queue Worker ──► Background processor
// ============================================================
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// chi — lightweight HTTP router for Go.
	// Think of it like Express.js for Go: r.Get("/path", handler)
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	// gzhttp — transparent gzip compression for all responses.
	// Equivalent to the compression npm package in Express.
	"github.com/klauspost/compress/gzhttp"

	// cors — Cross-Origin Resource Sharing middleware.
	// Equivalent to the cors npm package: app.use(cors({...}))
	"github.com/rs/cors"

	// Internal packages — our own code organised by responsibility.
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/db"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/handlers"
	appMiddleware "github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/queue"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
)

// ── main ──────────────────────────────────────────────────────────────────────
// main() is called automatically when the binary starts.
// It NEVER returns during normal operation — the os.Signal block at the
// bottom keeps it alive until Ctrl-C or a container SIGTERM arrives.
func main() {
	// ── 1. Load & validate environment variables ──────────────────────────────
	// config.Load() reads the .env file (or real env vars) and panics early
	// if required values are missing. Fast-fail is intentional: a misconfigured
	// server should crash immediately, not silently misbehave.
	//
	// 📝 EXAMPLE: if .env has PORT=5002, cfg.Port == "5002"
	cfg := config.Load()

	// ── 2. Open PostgreSQL connection pool ────────────────────────────────────
	// db.Open() creates a *pgxpool.Pool — a managed set of reusable database
	// connections. Using a pool is critical for performance: opening a new TCP
	// connection to PostgreSQL for every request would be ~10–100ms overhead.
	//
	// 📝 EXAMPLE: pool holds 10 idle connections ready to serve queries instantly.
	//
	// defer pool.Close() — Go's defer runs when main() exits (graceful shutdown),
	// ensuring all connections are properly closed before the process ends.
	pool := db.Open(cfg.DatabaseURL)
	defer db.Close(pool)

	// ── 3. Init asynq queue client (Redis) ───────────────────────────────────
	// The queue client is used to ENQUEUE tasks (producer side).
	// It serialises a job struct to JSON and pushes it into Redis.
	//
	// 📝 EXAMPLE:
	//   qClient.EnqueueApplyToJob(payload) → task pushed to Redis
	//   The worker on the other end picks it up and calls svc.ApplyToJob()
	qClient, err := queue.NewClient(cfg.RedisURL)
	if err != nil {
		// fmt.Fprintf(os.Stderr, ...) writes to standard error (not stdout).
		// This is the Go convention for error messages — they go to stderr
		// so they can be separated from normal output in logs.
		fmt.Fprintf(os.Stderr, "[queue] %v\n", err)
		os.Exit(1) // non-zero exit tells the container orchestrator to restart
	}
	defer qClient.Close() // closes the Redis connection cleanly on shutdown

	// ── 4. Init asynq worker server ──────────────────────────────────────────
	// The worker server is the CONSUMER side of the queue.
	// It runs in the background and executes tasks that arrive in Redis.
	//
	// 📝 ANALOGY: Think of the queue client as a "ticket dispenser" and the
	// worker as the "ticket processor" — they share the same Redis "counter".
	//
	// Note: worker.Start(jobSeekerSvc) is called LATER (step 5b) because
	// jobSeekerSvc is not yet constructed at this point.
	worker, err := queue.NewWorkerServer(cfg.RedisURL, cfg.QueueConcurrency)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[worker] %v\n", err)
		os.Exit(1)
	}
	defer worker.Shutdown() // gracefully drains in-flight tasks on shutdown

	// ── 5. Build the HTTP router ──────────────────────────────────────────────
	// chi.NewRouter() creates a fresh router — a table that maps
	// HTTP method + path pattern → handler function.
	//
	// 📝 EXAMPLE:
	//   r.Get("/health", handlerFn)  maps GET /health → handlerFn
	//   r.Mount("/api/auth", ...)    delegates all /api/auth/* routes to a sub-router
	r := chi.NewRouter()

	// ── 5a. Global middleware (applied to EVERY request) ─────────────────────
	// Middleware is a function that wraps a handler:
	//   request → middleware → (next middleware) → handler → response
	//
	// 📝 ANALOGY: Like Express: app.use(helmet()), app.use(cors()), etc.

	// Security HTTP headers (Content-Security-Policy, X-Frame-Options, etc.)
	// Equivalent to Node.js: app.use(helmet())
	r.Use(appMiddleware.SecureHeaders)
	if cfg.IsProduction() {
		// HSTS (Strict-Transport-Security) is only added in production
		// to avoid locking local dev to HTTPS.
		r.Use(appMiddleware.SecureHeadersProduction)
	}

	// Chi's built-in middleware stack:
	r.Use(chiMiddleware.RequestID)                // adds X-Request-ID header to every request
	r.Use(chiMiddleware.RealIP)                   // reads X-Forwarded-For → r.RemoteAddr
	r.Use(chiMiddleware.Logger)                   // logs: "GET /api/jobs → 200 in 3ms"
	r.Use(chiMiddleware.Recoverer)                // catches panics and returns 500 instead of crashing
	r.Use(chiMiddleware.Timeout(30 * time.Second)) // cancels the request context after 30s

	// CORS — tells browsers which origins are allowed to call our API.
	// Without this, the browser blocks cross-origin fetch() calls.
	//
	// 📝 EXAMPLE: Frontend at http://localhost:3000 calls GET /api/jobs.
	// Without CORS the browser blocks it. With AllowedOrigins:["http://localhost:3000"]
	// the browser sees Access-Control-Allow-Origin: http://localhost:3000 and allows it.
	//
	// AllowCredentials: true is required for HttpOnly cookie auth (our JWT strategy).
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{cfg.CORSOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true, // needed for Set-Cookie across origins
		MaxAge:           300,  // browser can cache the CORS preflight for 5 minutes
	}).Handler)

	// ── 5b. Health check endpoint ─────────────────────────────────────────────
	// A simple route with an inline handler (anonymous function).
	// Load balancers / Kubernetes hit this to decide if the pod is healthy.
	//
	// 📝 EXAMPLE: curl http://localhost:5002/health → {"status":"ok"}
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		// w is the ResponseWriter — you write your response into it.
		// r is the *Request — it carries URL, headers, body, context.
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	// ── 5c. Route groups (each mounted at a prefix) ───────────────────────────
	// Pattern: construct service → inject into handler → mount handler's routes.
	//
	// This is "dependency injection" — handlers don't create their own deps;
	// they receive them. Makes testing easy: swap real deps for fakes in tests.

	// Auth routes — /api/auth/signup, /api/auth/login, etc.
	authSvc := services.NewAuthService(pool, cfg)      // business logic layer
	authHandler := handlers.NewAuthHandler(authSvc, cfg) // HTTP layer
	r.Mount("/api/auth", authHandler.Routes())           // wires /api/auth/* to chi sub-router

	// Job routes — /api/jobs (CRUD)
	jobSvc := services.NewJobService(pool)
	jobHandler := handlers.NewJobHandler(jobSvc, cfg)
	r.Mount("/api/jobs", jobHandler.Routes())

	// Job seeker routes — /api/jobseeker/profile, /apply/:jobId, etc.
	// Also receives qClient to enqueue async tasks (apply-to-job, save-job).
	jobSeekerSvc := services.NewJobSeekerService(pool)
	jobSeekerHandler := handlers.NewJobSeekerHandler(jobSeekerSvc, cfg, qClient)
	r.Mount("/api/jobseeker", jobSeekerHandler.Routes())

	// Start the queue worker NOW that jobSeekerSvc is available.
	// worker.Start() registers the task handlers and launches the processing loop.
	if err := worker.Start(jobSeekerSvc); err != nil {
		fmt.Fprintf(os.Stderr, "[worker] failed to start: %v\n", err)
		os.Exit(1)
	}

	// Recruiter routes — /api/recruiter/profile, job application management
	recruiterSvc := services.NewRecruiterService(pool)
	recruiterHandler := handlers.NewRecruiterHandler(recruiterSvc, cfg)
	r.Mount("/api/recruiter", recruiterHandler.Routes())

	// Queue status routes — /api/queue/job/:taskId (polling endpoint)
	queueHandler, err := handlers.NewQueueHandler(cfg.RedisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[queue handler] %v\n", err)
		os.Exit(1)
	}
	r.Mount("/api/queue", queueHandler.Routes())

	// ── 6. Wrap the router with gzip compression ──────────────────────────────
	// gzhttp.GzipHandler checks the Accept-Encoding: gzip request header.
	// If the client supports gzip, responses are compressed transparently.
	//
	// 📝 EXAMPLE: a 10KB JSON payload compresses to ~1.5KB, saving bandwidth.
	handler := gzhttp.GzipHandler(r)

	// ── 7. Create and start the HTTP server ───────────────────────────────────
	// http.Server is the standard library's HTTP server.
	// We set timeouts explicitly to prevent slow-client attacks (Slowloris).
	//
	// 📝 EXAMPLE: a client that connects but never sends the request body will
	// be dropped after ReadHeaderTimeout instead of holding a goroutine forever.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // time allowed to read request headers
		WriteTimeout:      35 * time.Second, // time allowed to write the response
		IdleTimeout:       120 * time.Second, // keep-alive connection idle limit
	}

	// Start the server in a goroutine so we can listen for OS signals below.
	//
	// 📝 KEY CONCEPT — goroutines:
	//   go func() { ... }() launches a lightweight concurrent function.
	//   Unlike threads, goroutines are managed by Go's scheduler, not the OS.
	//   Thousands of them can run concurrently with minimal memory (~2KB each).
	go func() {
		fmt.Printf("[server] listening on :%s (env=%s)\n", cfg.Port, cfg.GoEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// ErrServerClosed is expected during graceful shutdown — not an error.
			fmt.Fprintf(os.Stderr, "[server] fatal: %v\n", err)
			os.Exit(1)
		}
	}()

	// ── 8. Graceful shutdown on OS signal ────────────────────────────────────
	// quit is a channel that blocks until it receives a signal.
	//
	// 📝 KEY CONCEPT — channels:
	//   A channel (chan) is a typed pipe between goroutines.
	//   make(chan os.Signal, 1) creates a buffered channel (capacity 1).
	//   signal.Notify wires OS signals to this channel.
	//   <-quit blocks here until a SIGINT (Ctrl-C) or SIGTERM arrives.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // blocks until signal received

	fmt.Println("[server] shutdown signal received, draining connections…")

	// Give in-flight requests up to 30 seconds to finish before hard-closing.
	// context.WithTimeout creates a context that cancels itself after 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// srv.Shutdown(ctx) stops accepting new connections and waits for active
	// ones to complete (or the context deadline to expire).
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[server] shutdown error: %v\n", err)
	}
	fmt.Println("[server] stopped cleanly")
}
