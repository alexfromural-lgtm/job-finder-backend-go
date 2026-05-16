// Package queue defines asynq task-type constants and payload structs.
// Mirrors the Node.js Bull queue types in src/queue/.
package queue

const (
	// TaskApplyToJob is the task type for processing a job application.
	// Mirrors Node.js: { type: "apply-to-job", ... }
	TaskApplyToJob = "apply-to-job"

	// TaskSaveJob is the task type for saving a job to a seeker's list.
	// Mirrors Node.js: { type: "save-job", ... }
	TaskSaveJob = "save-job"
)

// ApplyToJobPayload is the JSON payload stored in Redis for apply-to-job tasks.
type ApplyToJobPayload struct {
	UserID      string `json:"userId"`
	JobID       string `json:"jobId"`
	CoverLetter string `json:"coverLetter,omitempty"`
}

// SaveJobPayload is the JSON payload stored in Redis for save-job tasks.
type SaveJobPayload struct {
	UserID string `json:"userId"`
	JobID  string `json:"jobId"`
}
