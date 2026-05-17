// Package models contains request/response DTO structs used by handlers and services.
package models

// QueueJobStatusResponse is returned by GET /api/queue/job/:taskId.
// Mirrors the Node.js response shape from queue.route.ts.
type QueueJobStatusResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	AttemptsMade int    `json:"attemptsMade"`
	// FailedReason is populated when Status == "failed"
	FailedReason string `json:"failedReason,omitempty"`
	Result       any    `json:"result,omitempty"`
}
