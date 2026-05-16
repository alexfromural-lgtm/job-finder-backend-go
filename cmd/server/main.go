// cmd/server/main.go — Application entry point.
// Wires configuration, DB pool, chi router, middleware, and the asynq worker server.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/klauspost/compress/gzhttp"
	"github.com/rs/cors"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/config"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/db"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/handlers"
	appMiddleware "github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/queue"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
)

func main() {
	// ── 1. Load & validate environment variables ──────────────────────────────
	cfg := config.Load()

	// ── 2. Open PostgreSQL connection pool (pgx/v5) ───────────────────────────
	pool := db.Open(cfg.DatabaseURL)
	defer db.Close(pool)

	// ── 3. Init asynq queue client (Redis) ────────────────────────────────────
	qClient, err := queue.NewClient(cfg.RedisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[queue] %v\n", err)
		os.Exit(1)
	}
	defer qClient.Close()

	// ── 4. Init asynq worker (started after jobSeekerSvc is created below) ────
	worker, err := queue.NewWorkerServer(cfg.RedisURL, cfg.QueueConcurrency)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[worker] %v\n", err)
		os.Exit(1)
	}
	defer worker.Shutdown()

	// ── 5. Build HTTP router (chi) ────────────────────────────────────────────
	r := chi.NewRouter()

	// ── Security headers (Phase 9) ────────────────────────────────────────────
	// Mirrors Node.js: app.use(helmet())
	r.Use(appMiddleware.SecureHeaders)
	if cfg.IsProduction() {
		r.Use(appMiddleware.SecureHeadersProduction)
	}

	// ── Chi standard middleware ───────────────────────────────────────────────
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(30 * time.Second))

	// ── CORS — mirrors Node.js: cors({ origin: CORS_ORIGIN, credentials: true })
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{cfg.CORSOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true, // Required for HttpOnly cookie auth across origins
		MaxAge:           300,
	}).Handler)

	// ── Health check ──────────────────────────────────────────────────────────
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	// ── Route groups ──────────────────────────────────────────────────────────

	// Phase 3: Auth routes
	// Rate limiting note: chi's built-in Throttle is in-process.
	// For distributed rate limiting, wire ulule/limiter with a Redis store here.
	authSvc := services.NewAuthService(pool, cfg)
	authHandler := handlers.NewAuthHandler(authSvc, cfg)
	r.Mount("/api/auth", authHandler.Routes())

	// Phase 4: Jobs routes
	jobSvc := services.NewJobService(pool)
	jobHandler := handlers.NewJobHandler(jobSvc, cfg)
	r.Mount("/api/jobs", jobHandler.Routes())

	// Phase 5: Job Seeker routes
	jobSeekerSvc := services.NewJobSeekerService(pool)
	jobSeekerHandler := handlers.NewJobSeekerHandler(jobSeekerSvc, cfg, qClient)
	r.Mount("/api/jobseeker", jobSeekerHandler.Routes())

	// Wire jobSeekerSvc into worker now that it's constructed
	// Mirrors Node.js: import "./queue/worker" at server boot
	if err := worker.Start(jobSeekerSvc); err != nil {
		fmt.Fprintf(os.Stderr, "[worker] failed to start: %v\n", err)
		os.Exit(1)
	}

	// Phase 6: Recruiter routes
	recruiterSvc := services.NewRecruiterService(pool)
	recruiterHandler := handlers.NewRecruiterHandler(recruiterSvc, cfg)
	r.Mount("/api/recruiter", recruiterHandler.Routes())

	// Phase 7: Queue status routes
	queueHandler, err := handlers.NewQueueHandler(cfg.RedisURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[queue] inspector: %v\n", err)
		os.Exit(1)
	}
	r.Mount("/api/queue", queueHandler.Routes())

	// ── 6. HTTP server with optional gzip (Phase 9) ───────────────────────────
	// Gzip compression: only in production — mirrors Node.js: if (IS_PRODUCTION) app.use(compression())
	var handler http.Handler = r
	if cfg.IsProduction() {
		handler = gzhttp.GzipHandler(r)
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		fmt.Printf("[server] Listening on http://localhost:%s [%s]\n", cfg.Port, cfg.GoEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "[server] ListenAndServe error: %v\n", err)
			os.Exit(1)
		}
	}()

	// ── 7. Graceful shutdown on SIGINT / SIGTERM ──────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("[server] Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[server] Shutdown error: %v\n", err)
	}
	fmt.Println("[server] Stopped.")
}
