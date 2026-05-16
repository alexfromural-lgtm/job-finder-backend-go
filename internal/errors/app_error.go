// Package errors defines typed application errors used across services and handlers.
package errors

import "fmt"

// AppError is a typed error that carries an HTTP status code alongside a human-readable
// message. Services return *AppError so handlers can render the correct HTTP response.
// This mirrors the Node.js src/errors/AppError.ts class.
type AppError struct {
	Message    string
	StatusCode int
}

// Error implements the built-in error interface.
func (e *AppError) Error() string {
	return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
}

// New creates an *AppError with the given message and HTTP status code.
// Usage (mirrors Node.js): errors.New("Job not found", 404)
func New(message string, statusCode int) *AppError {
	return &AppError{Message: message, StatusCode: statusCode}
}
