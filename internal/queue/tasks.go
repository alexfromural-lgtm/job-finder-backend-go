// ============================================================
// FILE: internal/queue/tasks.go
// ROLE: Task-type constants and JSON payload structs for the asynq queue.
//
// 📚 WHAT IS A TASK?
//   A "task" is a unit of background work. It has:
//   - A TYPE (string): identifies WHAT to do ("apply-to-job", "save-job")
//   - A PAYLOAD ([]byte): JSON-encoded data the worker needs to do it
//
// 📝 HOW DATA FLOWS:
//   Handler → enqueue task with payload → Redis → worker picks it up
//           → deserialise payload → call service method → write to DB
//
// 📝 COMPARISON with Node.js (src/queue/):
//   Bull queue jobs had { type: "apply-to-job", userId, jobId, coverLetter }
//   stored as JSON in Redis.
//   Here we keep the same type strings for frontend API compatibility,
//   but define Go structs for type-safe serialisation.
// ============================================================
package queue

// Task type name constants.
// Using constants instead of inline strings prevents typos like "applyToJob" vs "apply-to-job".
//
// 📝 WHY CONSTANTS?
//   const TaskApplyToJob = "apply-to-job"
//   If we mistype "apply-to-jb" in one place, Go gives a compile error.
//   With inline strings we'd get a silent runtime failure (task never processed).
const (
	// TaskApplyToJob identifies the "process a job application" task.
	// The worker matches this string to call makeApplyHandler.
	TaskApplyToJob = "apply-to-job"

	// TaskSaveJob identifies the "save a job to seeker's list" task.
	TaskSaveJob = "save-job"
)

// ApplyToJobPayload is the JSON payload stored in Redis for apply-to-job tasks.
// It contains everything the worker needs to call svc.ApplyToJob().
//
// 📝 JSON TAGS:
//   json:"userId" means the JSON key is "userId" (camelCase like Node.js).
//   json:"coverLetter,omitempty" means the key is omitted if the string is empty.
//
// 📝 SERIALISATION FLOW:
//   1. Handler:  json.Marshal(ApplyToJobPayload{UserID: "u1", JobID: "j2"})
//      → []byte  `{"userId":"u1","jobId":"j2"}`
//   2. Worker:   json.Unmarshal(task.Payload(), &p)
//      → p.UserID == "u1", p.JobID == "j2"
type ApplyToJobPayload struct {
	UserID      string `json:"userId"`
	JobID       string `json:"jobId"`
	CoverLetter string `json:"coverLetter,omitempty"` // optional — empty string → omitted from JSON
}

// SaveJobPayload is the JSON payload for save-job tasks.
// Simpler than ApplyToJobPayload — no cover letter needed.
type SaveJobPayload struct {
	UserID string `json:"userId"`
	JobID  string `json:"jobId"`
}
