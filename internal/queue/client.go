// ============================================================
// FILE: internal/queue/client.go
// ROLE: Asynq queue client — the PRODUCER side of the task queue.
//
// 📚 PRODUCER vs CONSUMER
//   This file is the PRODUCER: it pushes tasks into Redis.
//   worker.go is the CONSUMER: it pulls tasks out of Redis and runs them.
//
//   They communicate via Redis: a list of serialised task structs.
//   Redis is essentially a super-fast, persistent message broker here.
//
// 📝 WHY ASYNC TASKS?
//   Writing a job application involves DB inserts + potential email sending.
//   Doing all of that synchronously in the HTTP handler would:
//     a) Block the HTTP response for 100-500ms
//     b) Fail entirely if the DB is momentarily slow
//   Instead, we enqueue the task (< 5ms) and return 202 Accepted immediately.
//   The worker processes it in the background, retrying on failure.
//
// 📝 COMPARISON with Node.js:
//   const job = await dbWriteQueue.add({ type: "apply-to-job", userId, jobId, coverLetter })
//   res.status(202).json({ queueJobId: job.id, status: "queued" })
//   → Client.EnqueueApplyToJob(payload) + return taskID for polling
// ============================================================
package queue

import (
	"encoding/json"
	"fmt"
	"time"

	// asynq — Go's most popular Redis-backed task queue.
	// Equivalent to Bull/BullMQ in Node.js.
	"github.com/hibiken/asynq"
)

// completedTaskRetention is how long asynq keeps a completed task in Redis.
// The frontend polls GET /api/queue/job/:taskId to check if the task finished.
// 30 seconds gives the frontend enough time to observe "completed" status.
//
// After this duration, the task entry is deleted from Redis automatically.
//
// 📝 EXAMPLE POLLING FLOW:
//   POST /api/jobseeker/apply/job123 → 202 {"queueJobId": "abc"}
//   GET /api/queue/job/abc          → {"status": "waiting"}
//   GET /api/queue/job/abc          → {"status": "completed", "result": {...}}
const completedTaskRetention = 30 * time.Second

// Client wraps asynq.Client and exposes typed enqueue helpers.
// Using a wrapper (instead of using asynq.Client directly) means:
//   - Callers can't accidentally enqueue arbitrary task types
//   - The API is self-documenting: EnqueueApplyToJob(payload) is clearer than Enqueue(task, opts)
type Client struct {
	inner *asynq.Client // the underlying asynq client talking to Redis
}

// NewClient creates a Client from a Redis URL.
// asynq.ParseRedisURI parses URLs like:
//   "redis://localhost:6379"
//   "redis://:password@host:6379/db"
//   "rediss://host:6380" (TLS)
//
// 📝 ERROR HANDLING:
//   Returns (nil, error) on failure — callers decide what to do (main.go exits).
//   This is the Go idiom: return (value, error), check error before using value.
func NewClient(redisURL string) (*Client, error) {
	// asynq.ParseRedisURI converts the URL string into an asynq.RedisClientOpt.
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue: failed to parse REDIS_URL %q: %w", redisURL, err)
		// %w wraps the original error — errors.Is() / errors.As() can still find it
	}
	// asynq.NewClient returns a client object (doesn't connect yet — lazy connect on first use)
	return &Client{inner: asynq.NewClient(opt)}, nil
}

// Close releases the Redis connection.
// Call via `defer qClient.Close()` in main.go.
func (c *Client) Close() error {
	return c.inner.Close()
}

// EnqueueApplyToJob serialises the payload to JSON and pushes it into Redis.
// Returns the asynq task ID (a UUID string) for the client to poll.
//
// 📝 STEP-BY-STEP:
//   1. json.Marshal(payload)      → []byte JSON
//   2. asynq.NewTask(type, json) → *asynq.Task struct
//   3. c.inner.Enqueue(task, opts) → pushes to Redis, returns *asynq.TaskInfo
//   4. Return info.ID             → the UUID for polling
//
// 📝 EXAMPLE:
//   taskID, _ := client.EnqueueApplyToJob(ApplyToJobPayload{
//       UserID: "user-123", JobID: "job-456", CoverLetter: "I am great",
//   })
//   // taskID: "550e8400-e29b-41d4-a716-446655440000"
//   // Redis now has: LIST "asynq:{default}:pending" → ["550e8400..."]
func (c *Client) EnqueueApplyToJob(payload ApplyToJobPayload) (string, error) {
	// json.Marshal serialises the struct to JSON bytes.
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// asynq.NewTask(typeName, payload) creates a task object.
	// TaskApplyToJob == "apply-to-job" — the worker uses this string to route the task.
	task := asynq.NewTask(TaskApplyToJob, b)

	// c.inner.Enqueue pushes the task into the Redis "default" queue.
	// asynq.Retention(30s) tells asynq to keep the result in Redis for 30s after completion.
	info, err := c.inner.Enqueue(task, asynq.Retention(completedTaskRetention))
	if err != nil {
		return "", fmt.Errorf("queue: enqueue apply-to-job: %w", err)
	}

	return info.ID, nil // info.ID is the UUID task identifier
}

// EnqueueSaveJob pushes a save-job task into the queue.
// Returns the task ID for polling (same pattern as EnqueueApplyToJob).
//
// 📝 EXAMPLE:
//   taskID, _ := client.EnqueueSaveJob(SaveJobPayload{UserID: "u1", JobID: "j2"})
func (c *Client) EnqueueSaveJob(payload SaveJobPayload) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(TaskSaveJob, b)
	info, err := c.inner.Enqueue(task, asynq.Retention(completedTaskRetention))
	if err != nil {
		return "", fmt.Errorf("queue: enqueue save-job: %w", err)
	}
	return info.ID, nil
}
