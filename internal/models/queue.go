// ============================================================
// FILE: internal/models/queue.go
// ROLE: Response DTO for async queue job status polling.
//
// 📚 HOW THE ASYNCHRONOUS PATTERN WORKS:
//   1. Client sends POST /api/jobseeker/apply/:jobId
//   2. Backend enqueues task to Redis and returns:
//      HTTP 202 Accepted: {"queueJobId": "abc-123", "status": "queued"}
//   3. Client polls GET /api/queue/job/abc-123 every 500ms-1s:
//      Backend returns QueueJobStatusResponse.
//   4. When status becomes "completed", client knows operation succeeded
//      and can display confirmation to user.
// ============================================================
package models

// QueueJobStatusResponse represents the status of an asynchronous background task.
// Returned by GET /api/queue/job/:taskId.
//
// 📝 COMPATIBILITY WITH BULL (Node.js):
//   The React frontend expects Bull-compatible status strings:
//   - "waiting"   : task is waiting in line to be picked up by worker
//   - "active"    : worker is currently executing the task
//   - "completed" : task finished without errors
//   - "failed"    : task encountered an unrecoverable error
//   - "delayed"   : task scheduled for future execution
//
// 📝 EXAMPLE OUTGOING JSON (Completed):
//   {
//     "id": "3b29c924-...",
//     "type": "apply-to-job",
//     "status": "completed",
//     "attemptsMade": 1,
//     "result": { "id": "...", "status": "submitted" }
//   }
//
// 📝 EXAMPLE OUTGOING JSON (Failed):
//   {
//     "id": "3b29c924-...",
//     "type": "apply-to-job",
//     "status": "failed",
//     "attemptsMade": 3,
//     "failedReason": "You have already applied to this job"
//   }
type QueueJobStatusResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	AttemptsMade int    `json:"attemptsMade"`

	// FailedReason: omitted if empty (`omitempty`), populated only on failure.
	FailedReason string `json:"failedReason,omitempty"`

	// Result: optional return payload produced by the background worker.
	// `any` in Go (alias for `interface{}`) allows serializing any JSON structure.
	Result       any    `json:"result,omitempty"`
}
