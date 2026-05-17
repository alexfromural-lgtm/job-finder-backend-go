// Package handlers contains HTTP handlers for the chi router.
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

const defaultQueueName = "default"

// QueueHandler exposes the task status polling endpoint.
type QueueHandler struct {
	inspector *asynq.Inspector
}

// NewQueueHandler constructs a QueueHandler from a Redis URL.
func NewQueueHandler(redisURL string) (*QueueHandler, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	return &QueueHandler{inspector: asynq.NewInspector(opt)}, nil
}

// Routes mounts the queue status endpoint. Public — no auth required.
// Mirrors queue.route.ts: router.get("/job/:jobId", ...)
func (h *QueueHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/job/{taskId}", h.GetJobStatus)
	return r
}

// GetJobStatus godoc: GET /api/queue/job/:taskId
// Polls asynq for the current state of a queued task.
// Returns status, type, retry count, and failedReason (if failed).
// Mirrors the Node.js queue route response shape exactly for frontend compatibility.
func (h *QueueHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskId")

	info, err := h.inspector.GetTaskInfo(defaultQueueName, taskID)
	if err != nil {
		// Task not found in the default queue
		middleware.WriteError(w, apperrors.New("Job not found", 404))
		return
	}

	resp := models.QueueJobStatusResponse{
		ID:           info.ID,
		Type:         info.Type,
		Status:       asynqStateToStatus(info.State),
		AttemptsMade: info.Retried,
	}

	// Include failedReason when the task has errored.
	// Mirrors Node.js: if (state === "failed") { response.failedReason = job.failedReason }
	if resp.Status == "failed" && info.LastErr != "" {
		resp.FailedReason = info.LastErr
	}

	if len(info.Result) > 0 {
		var res any
		if err := json.Unmarshal(info.Result, &res); err == nil {
			resp.Result = res
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// asynqStateToStatus maps asynq task states to the Bull-compatible status strings
// the React frontend expects.
// Mapping:
//
//	active    → "active"
//	pending   → "waiting"
//	scheduled → "delayed"
//	retry     → "waiting"
//	archived  → "failed"   (archived = dead-letter in asynq)
//	completed → "completed"
func asynqStateToStatus(state asynq.TaskState) string {
	switch state {
	case asynq.TaskStateActive:
		return "active"
	case asynq.TaskStatePending:
		return "waiting"
	case asynq.TaskStateScheduled:
		return "delayed"
	case asynq.TaskStateRetry:
		return "waiting"
	case asynq.TaskStateArchived:
		return "failed"
	case asynq.TaskStateCompleted:
		return "completed"
	default:
		return "unknown"
	}
}
