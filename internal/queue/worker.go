// ============================================================
// FILE: internal/queue/worker.go
// ROLE: Asynq worker server — the CONSUMER side of the task queue.
//
// 📚 WHAT IS THE WORKER?
//   The worker is a background goroutine that continuously polls Redis for
//   pending tasks, picks them up one at a time (up to Concurrency in parallel),
//   and runs the registered handler function for each task type.
//
// 📝 HOW TASK ROUTING WORKS:
//   The ServeMux (asynq.NewServeMux) is like a mini-router for tasks.
//   It maps: task type string → handler function
//
//   "apply-to-job" → makeApplyHandler(jobSeekerSvc)
//   "save-job"     → makeSaveHandler(jobSeekerSvc)
//
//   When a task arrives in Redis, asynq calls the matching handler.
//   If no handler matches, the task is retried later.
//
// 📝 COMPARISON with Node.js worker.ts:
//   dbWriteQueue.process(CONCURRENCY, async (job) => {
//     switch (job.data.type) {
//       case "apply-to-job": await jobSeekerService.applyToJob(...); break;
//       case "save-job":     await jobSeekerService.saveJob(...); break;
//     }
//   })
//
// 📝 RETRY BEHAVIOUR:
//   If a handler returns an error, asynq automatically retries the task
//   with exponential backoff. After max retries, it moves to the
//   "archived" (dead-letter) queue where it can be inspected.
// ============================================================
package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
)

// WorkerServer wraps an asynq.Server and provides typed Start/Shutdown methods.
type WorkerServer struct {
	server *asynq.Server // the underlying asynq server listening on Redis
}

// NewWorkerServer creates an asynq server from a Redis URL and concurrency setting.
//
// 📝 CONCURRENCY:
//   concurrency = 5 means up to 5 tasks run simultaneously.
//   Each task runs in its own goroutine — Go handles thousands of goroutines
//   with minimal overhead, unlike Node.js worker threads.
//
// 📝 QUEUES MAP:
//   map[string]int{"default": 1} means only the "default" queue is monitored,
//   with priority 1. You could add: "critical": 10, "default": 5, "low": 1
//   to process critical tasks first.
//
// 📝 ERROR HANDLER:
//   The ErrorHandler is called when a task fails (after retries).
//   Here we just log it. In production you'd alert (Sentry, PagerDuty, etc.).
func NewWorkerServer(redisURL string, concurrency int) (*WorkerServer, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue worker: failed to parse REDIS_URL: %w", err)
	}

	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			"default": 1, // monitor the "default" queue only
		},
		// ErrorHandler is a callback for tasks that permanently fail (all retries exhausted).
		// asynq.ErrorHandlerFunc adapts a plain function to the ErrorHandler interface.
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			// task.Type() returns "apply-to-job" or "save-job"
			fmt.Printf("[Worker] ✗ task %s (type: %s) failed: %v\n", task.Type(), task.Type(), err)
		}),
	})

	return &WorkerServer{server: srv}, nil
}

// Start registers task handlers and begins processing tasks from Redis.
// This is non-blocking: asynq launches its own goroutines internally.
//
// Called from main.go AFTER jobSeekerSvc is constructed (dependency order).
//
// 📝 HOW ServeMux WORKS:
//   asynq.NewServeMux() is analogous to http.NewServeMux() for HTTP.
//   mux.HandleFunc(pattern, fn) registers a handler for a task type.
//   The mux is passed to server.Start(mux) — the server calls mux to route tasks.
func (w *WorkerServer) Start(jobSeekerSvc *services.JobSeekerService) error {
	mux := asynq.NewServeMux()

	// Register: when a task of type "apply-to-job" arrives, call makeApplyHandler(svc)
	mux.HandleFunc(TaskApplyToJob, makeApplyHandler(jobSeekerSvc))

	// Register: when a task of type "save-job" arrives, call makeSaveHandler(svc)
	mux.HandleFunc(TaskSaveJob, makeSaveHandler(jobSeekerSvc))

	// server.Start(mux) begins polling Redis. It spawns goroutines internally.
	// Returns an error only if the connection to Redis fails immediately.
	return w.server.Start(mux)
}

// Shutdown gracefully stops the worker.
// In-flight tasks are allowed to finish; no new tasks are picked up.
// Called via `defer worker.Shutdown()` in main.go.
func (w *WorkerServer) Shutdown() {
	w.server.Shutdown()
}

// ── task handlers ─────────────────────────────────────────────────────────────
// These are factory functions (they RETURN a handler function).
// Using closures lets the handler "capture" the service reference without
// relying on global variables.
//
// 📝 HANDLER SIGNATURE:
//   func(ctx context.Context, t *asynq.Task) error
//   - ctx: carries a deadline (task timeout) + cancellation signal
//   - t:   the task object with Type(), Payload(), and ResultWriter()
//   - return nil on success; return error to trigger a retry

// makeApplyHandler returns the asynq handler for "apply-to-job" tasks.
//
// 📝 STEP-BY-STEP:
//   1. json.Unmarshal(t.Payload(), &p)   → deserialise JSON payload into ApplyToJobPayload
//   2. svc.ApplyToJob(ctx, ...)          → write the application to PostgreSQL
//   3. t.ResultWriter().Write(json)      → store result in Redis for 30s (polling)
func makeApplyHandler(svc *services.JobSeekerService) asynq.HandlerFunc {
	// This anonymous function IS the handler. It's a closure — it "closes over" svc.
	return func(ctx context.Context, t *asynq.Task) error {
		var p ApplyToJobPayload

		// json.Unmarshal decodes the raw []byte payload into the struct.
		// t.Payload() returns the bytes stored when the task was enqueued.
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			// Returning an error triggers a retry.
			// fmt.Errorf with %w wraps the original error (error chain preserved).
			return fmt.Errorf("apply-to-job: failed to decode payload: %w", err)
		}

		fmt.Printf("[Worker] Processing apply-to-job | user=%s job=%s\n", p.UserID, p.JobID)

		// Call the service to actually write the application to the database.
		app, err := svc.ApplyToJob(ctx, p.UserID, p.JobID, p.CoverLetter)
		if err != nil {
			return fmt.Errorf("apply-to-job: %w", err)
		}

		// Serialise the result and write it into the task's result slot in Redis.
		// The frontend can read this via GET /api/queue/job/:taskId → {"result": {...}}
		if b, err := json.Marshal(app); err == nil {
			t.ResultWriter().Write(b) // write result bytes; silently ignored on error
		}

		fmt.Printf("[Worker] ✓ apply-to-job completed | user=%s job=%s\n", p.UserID, p.JobID)
		return nil // nil = success; task moves to "completed" state
	}
}

// makeSaveHandler returns the asynq handler for "save-job" tasks.
// Same pattern as makeApplyHandler — only the payload type and service call differ.
func makeSaveHandler(svc *services.JobSeekerService) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p SaveJobPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("save-job: failed to decode payload: %w", err)
		}

		fmt.Printf("[Worker] Processing save-job | user=%s job=%s\n", p.UserID, p.JobID)

		savedJob, err := svc.SaveJob(ctx, p.UserID, p.JobID)
		if err != nil {
			return fmt.Errorf("save-job: %w", err)
		}

		if b, err := json.Marshal(savedJob); err == nil {
			t.ResultWriter().Write(b)
		}

		fmt.Printf("[Worker] ✓ save-job completed | user=%s job=%s\n", p.UserID, p.JobID)
		return nil
	}
}
