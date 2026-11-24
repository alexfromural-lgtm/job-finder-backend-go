// ============================================================
// FILE: internal/handlers/queue.go
// ROLE: HTTP handler for polling queued task status.
//
// 📚 WHAT IS THIS FILE?
//   After the client enqueues an async task (apply-to-job, save-job)
//   and receives a 202 Accepted with a queueJobId, the frontend polls
//   GET /api/queue/job/:taskId to check whether the task has finished.
//
// 📝 THE POLLING FLOW:
//   1. POST /api/jobseeker/apply/job123    → 202 {"queueJobId":"abc"}
//   2. GET  /api/queue/job/abc             → {"status":"waiting"}
//   3. GET  /api/queue/job/abc             → {"status":"completed","result":{...}}
//   (or)
//   3. GET  /api/queue/job/abc             → {"status":"failed","failedReason":"..."}
//
// 📝 ASYNQ INSPECTOR:
//   asynq.Inspector queries Redis directly to get task metadata.
//   It's a read-only view into the queue — no processing happens here.
//
// 📝 COMPARISON with Node.js:
//   queue.route.ts: router.get("/job/:jobId", async (req, res) => {
//       const job = await dbWriteQueue.getJob(req.params.jobId)
//       res.json({ id: job.id, type: job.name, status: await job.getState(), ... })
//   })
// ============================================================
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/middleware"
	"github.com/alexfromural-lgtm/job-finder-backend-go/internal/models"
)

// defaultQueueName is the asynq queue name used throughout the application.
// Must match the queue name used when enqueueing tasks (in queue/client.go).
const defaultQueueName = "default"

// QueueHandler exposes the task status polling endpoint.
// It holds an asynq.Inspector which queries Redis for task info.
type QueueHandler struct {
	inspector *asynq.Inspector // read-only view into asynq's Redis data
}

// NewQueueHandler constructs a QueueHandler from a Redis URL.
// The Inspector connects to the same Redis instance as the client/worker.
//
// 📝 WHY A SEPARATE CONSTRUCTOR?
//   QueueHandler doesn't need a *services.SomeService — it talks to asynq directly.
//   Its constructor only needs the Redis URL to build the Inspector.
func NewQueueHandler(redisURL string) (*QueueHandler, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	// asynq.NewInspector creates a read-only client for inspecting queues.
	return &QueueHandler{inspector: asynq.NewInspector(opt)}, nil
}

// Routes mounts the queue status endpoint on a chi sub-router.
// No auth required — the task ID is unguessable (UUID), so it acts as a capability token.
//
// 📝 ROUTE:  GET /api/queue/job/{taskId}
//   {taskId} is a chi URL parameter — extracted in the handler via chi.URLParam()
func (h *QueueHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/job/{taskId}", h.GetJobStatus)
	return r
}

// GetJobStatus godoc: GET /api/queue/job/:taskId
// Queries asynq for the current state of the task and returns a status response.
//
// 📝 RESPONSE FIELDS:
//   id:           the task UUID (same as the queueJobId from the enqueue response)
//   type:         "apply-to-job" or "save-job"
//   status:       "waiting" | "active" | "delayed" | "completed" | "failed"
//   attemptsMade: number of times the task has been tried (0 on first attempt)
//   failedReason: error message (only present when status == "failed")
//   result:       the return value of the worker handler (only when completed)
//
// 📝 WHY MAP STATUS VALUES?
//   asynq uses its own enum (TaskStateActive, TaskStatePending, etc.)
//   The frontend was built against Bull queue's status strings ("waiting", "active", etc.)
//   asynqStateToStatus() translates between the two.
func (h *QueueHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	// chi.URLParam extracts the named URL path segment.
	// /api/queue/job/abc123 → taskID = "abc123"
	taskID := chi.URLParam(r, "taskId")

	// GetTaskInfo looks up the task by ID in the specified queue.
	// Returns an error if the task doesn't exist or has been garbage collected.
	info, err := h.inspector.GetTaskInfo(defaultQueueName, taskID)
	if err != nil {
		middleware.WriteError(w, apperrors.New("Job not found", 404))
		return
	}

	// Build the response from the asynq TaskInfo struct.
	resp := models.QueueJobStatusResponse{
		ID:           info.ID,
		Type:         info.Type,
		Status:       asynqStateToStatus(info.State),
		AttemptsMade: info.Retried, // number of retry attempts made so far
	}

	// Only include failedReason when the task has actually failed.
	// info.LastErr holds the last error message from the handler.
	if resp.Status == "failed" && info.LastErr != "" {
		resp.FailedReason = info.LastErr
	}

	// Deserialise and attach the result if present (task completed successfully).
	// info.Result is the bytes written by t.ResultWriter().Write(b) in the worker.
	if len(info.Result) > 0 {
		var res any // `any` is an alias for `interface{}` — accepts any JSON structure
		if err := json.Unmarshal(info.Result, &res); err == nil {
			resp.Result = res
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// asynqStateToStatus maps asynq's typed task states to Bull-compatible status strings.
//
// 📝 ASYNQ vs BULL STATE NAMES:
//   asynq.TaskStatePending  = task is in queue waiting to be picked up → "waiting"
//   asynq.TaskStateActive   = worker is currently processing it        → "active"
//   asynq.TaskStateScheduled= task is delayed (not used here)          → "delayed"
//   asynq.TaskStateRetry    = task failed, waiting to retry            → "waiting"
//   asynq.TaskStateArchived = task exhausted all retries (dead-letter) → "failed"
//   asynq.TaskStateCompleted= task finished successfully               → "completed"
//
// 📝 WHY A SWITCH?
//   Go's switch is the idiomatic way to handle multiple discrete cases.
//   It's equivalent to a chain of if/else if, but cleaner and exhaustive.
func asynqStateToStatus(state asynq.TaskState) string {
	switch state {
	case asynq.TaskStateActive:
		return "active"
	case asynq.TaskStatePending:
		return "waiting"
	case asynq.TaskStateScheduled:
		return "delayed"
	case asynq.TaskStateRetry:
		return "waiting" // retry = still pending from the frontend's perspective
	case asynq.TaskStateArchived:
		return "failed" // archived = dead-letter = permanently failed
	case asynq.TaskStateCompleted:
		return "completed"
	default:
		return "unknown"
	}
}
