// Package queue provides the asynq worker server that processes background tasks.
// Mirrors src/queue/worker.ts — same task types, same service dispatch logic.
package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/services"
)

// WorkerServer wraps an asynq.Server and its task dispatcher.
type WorkerServer struct {
	server *asynq.Server
}

// NewWorkerServer creates an asynq server from a Redis URL.
// Concurrency mirrors Node.js: parseInt(process.env.QUEUE_CONCURRENCY || "5", 10)
func NewWorkerServer(redisURL string, concurrency int) (*WorkerServer, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue worker: failed to parse REDIS_URL: %w", err)
	}

	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			"default": 1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			fmt.Printf("[Worker] ✗ task %s (type: %s) failed: %v\n", task.Type(), task.Type(), err)
		}),
	})

	return &WorkerServer{server: srv}, nil
}

// Start registers task handlers and begins processing.
// Runs in the background (non-blocking after the goroutine is launched by main.go).
// Mirrors Node.js: dbWriteQueue.process(CONCURRENCY, async (job) => { switch job.data.type ... })
func (w *WorkerServer) Start(jobSeekerSvc *services.JobSeekerService) error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskApplyToJob, makeApplyHandler(jobSeekerSvc))
	mux.HandleFunc(TaskSaveJob, makeSaveHandler(jobSeekerSvc))
	return w.server.Start(mux)
}

// Shutdown gracefully stops the worker, allowing in-flight tasks to complete.
func (w *WorkerServer) Shutdown() {
	w.server.Shutdown()
}

// ── task handlers ─────────────────────────────────────────────────────────────

// makeApplyHandler returns the asynq handler for apply-to-job tasks.
// Mirrors the "apply-to-job" case in worker.ts.
func makeApplyHandler(svc *services.JobSeekerService) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var p ApplyToJobPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("apply-to-job: failed to decode payload: %w", err)
		}

		fmt.Printf("[Worker] Processing apply-to-job | user=%s job=%s\n", p.UserID, p.JobID)

		app, err := svc.ApplyToJob(ctx, p.UserID, p.JobID, p.CoverLetter)
		if err != nil {
			return fmt.Errorf("apply-to-job: %w", err)
		}

		if b, err := json.Marshal(app); err == nil {
			t.ResultWriter().Write(b)
		}

		fmt.Printf("[Worker] ✓ apply-to-job completed | user=%s job=%s\n", p.UserID, p.JobID)
		return nil
	}
}

// makeSaveHandler returns the asynq handler for save-job tasks.
// Mirrors the "save-job" case in worker.ts.
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
