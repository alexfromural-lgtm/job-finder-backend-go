// Package queue provides the asynq client used to enqueue background tasks.
// Mirrors Node.js: src/queue/queue.ts (dbWriteQueue.add())
package queue

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Client wraps asynq.Client and exposes typed enqueue helpers.
type Client struct {
	inner *asynq.Client
}

// NewClient creates a Client from a Redis URL (e.g. "redis://host:6379").
// Calls os.Exit if the URL is unparseable — mirrors the fail-fast pattern.
func NewClient(redisURL string) (*Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue: failed to parse REDIS_URL %q: %w", redisURL, err)
	}
	return &Client{inner: asynq.NewClient(opt)}, nil
}

// Close releases the underlying Redis connection.
func (c *Client) Close() error {
	return c.inner.Close()
}

// EnqueueApplyToJob enqueues an apply-to-job task and returns the asynq task ID.
// Mirrors Node.js: dbWriteQueue.add({ type: "apply-to-job", userId, jobId, coverLetter })
func (c *Client) EnqueueApplyToJob(payload ApplyToJobPayload) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(TaskApplyToJob, b)
	info, err := c.inner.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("queue: enqueue apply-to-job: %w", err)
	}
	return info.ID, nil
}

// EnqueueSaveJob enqueues a save-job task and returns the asynq task ID.
// Mirrors Node.js: dbWriteQueue.add({ type: "save-job", userId, jobId })
func (c *Client) EnqueueSaveJob(payload SaveJobPayload) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(TaskSaveJob, b)
	info, err := c.inner.Enqueue(task)
	if err != nil {
		return "", fmt.Errorf("queue: enqueue save-job: %w", err)
	}
	return info.ID, nil
}
