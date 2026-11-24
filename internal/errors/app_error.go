// ============================================================
// FILE: internal/errors/app_error.go
// ROLE: Typed application error carrying an HTTP status code.
//
// 📚 WHAT IS THIS FILE?
//   Go's built-in `error` interface has only one method:
//     Error() string
//   That's minimal — it doesn't tell us WHICH HTTP status code to send.
//
//   This file defines AppError, a custom error type that also carries
//   an HTTP status code. Services return *AppError so the error handler
//   middleware can render the correct HTTP response.
//
// 📝 COMPARISON with Node.js:
//   Node.js approach (src/errors/AppError.ts):
//     class AppError extends Error {
//       constructor(message, statusCode) { ... }
//     }
//   Go approach:
//     type AppError struct { Message string; StatusCode int }
//     func (e *AppError) Error() string { ... }  ← implements error interface
//
// 📝 USAGE EXAMPLE:
//   // In a service:
//   return apperrors.New("Job not found", 404)
//
//   // In error handler middleware:
//   var appErr *apperrors.AppError
//   if errors.As(err, &appErr) {
//       w.WriteHeader(appErr.StatusCode)  // e.g. 404
//       json.Encode(appErr.Message)       // "Job not found"
//   }
// ============================================================
package errors

import "fmt"

// AppError is a typed error that carries an HTTP status code alongside
// a human-readable message.
//
// 📝 KEY CONCEPT — custom error types in Go:
//   Any type that has an Error() string method implements the error interface.
//   So *AppError can be passed anywhere an error is expected.
type AppError struct {
	Message    string // shown to the client (e.g. "User not found")
	StatusCode int    // HTTP status code (e.g. 404, 401, 409, 500)
}

// Error implements the built-in error interface.
// This method is what gets printed in log output.
//
// 📝 EXAMPLE:
//   err := apperrors.New("Not found", 404)
//   fmt.Println(err) // prints: [404] Not found
func (e *AppError) Error() string {
	return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
}

// New creates an *AppError with the given message and HTTP status code.
//
// 📝 WHY A CONSTRUCTOR FUNCTION?
//   We could write `&AppError{Message: "...", StatusCode: 404}` everywhere,
//   but New("...", 404) is shorter and matches the Node.js pattern.
//
// 📝 COMMON STATUS CODES USED IN THIS APP:
//   400 Bad Request          — invalid input
//   401 Unauthorized         — not logged in / bad token
//   403 Forbidden            — logged in but not allowed (wrong role)
//   404 Not Found            — resource doesn't exist
//   409 Conflict             — duplicate record (email already registered)
//   500 Internal Server Error — unexpected server-side failure
//
// 📝 EXAMPLE:
//   return apperrors.New("Email already registered", 409)
//   return apperrors.New("Invalid token", 401)
func New(message string, statusCode int) *AppError {
	// & takes the address of the struct literal, returning a pointer *AppError.
	// Using a pointer means the caller gets the same value (not a copy),
	// and errors.As() can match it correctly.
	return &AppError{Message: message, StatusCode: statusCode}
}
