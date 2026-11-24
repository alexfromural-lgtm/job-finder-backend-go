// ============================================================
// FILE: internal/middleware/error_handler.go
// ROLE: Centralised error-to-HTTP-response translation.
//
// 📚 WHAT IS THIS FILE?
//   Every service function can return different error types:
//     - *AppError (our typed errors with status codes)
//     - *pgconn.PgError (PostgreSQL errors, e.g. unique constraint violation)
//     - jwt errors (expired, invalid signature, malformed)
//     - validator errors (struct validation failures)
//     - generic errors (unexpected, become 500)
//
//   WriteError() is the single function that maps ANY of these to the
//   correct JSON error response. Handlers call it like this:
//
//   📝 HANDLER PATTERN:
//     result, err := service.DoSomething(ctx, ...)
//     if err != nil {
//         middleware.WriteError(w, err)
//         return
//     }
//
//   📝 COMPARISON with Node.js:
//     Express has errorHandler middleware: app.use((err, req, res, next) => {...})
//     In Go we call WriteError explicitly (there's no equivalent to Express's
//     "pass to next with error" pattern). The classification logic is the same.
// ============================================================
package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	// pgconn provides PostgreSQL-specific error types.
	// PgError.Code == "23505" means unique constraint violation.
	"github.com/jackc/pgx/v5/pgconn"

	// golang-jwt error sentinels — used to identify specific JWT failure modes.
	"github.com/golang-jwt/jwt/v5"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
)

// errorResponse is the JSON shape sent to the client on errors.
// All errors — regardless of type — are normalised to this shape.
//
// 📝 WIRE FORMAT:
//   {"error": "User not found"}
//   {"error": "jwt expired"}
//   {"error": "Internal server error"}
type errorResponse struct {
	Error string `json:"error"` // human-readable message
}

// WriteError maps err to the correct HTTP status + message and writes JSON.
// This is the single place where ALL error classification happens.
//
// 📝 EXAMPLE:
//   WriteError(w, apperrors.New("Not found", 404))
//   // → HTTP 404  {"error":"Not found"}
//
//   WriteError(w, pgUniqueErr)
//   // → HTTP 409  {"error":"A record with that value already exists."}
//
//   WriteError(w, jwt.ErrTokenExpired)
//   // → HTTP 401  {"error":"jwt expired"}
//
//   WriteError(w, unexpectedErr)
//   // → HTTP 500  {"error":"Internal server error"}
//   //             + server-side log of the real error
func WriteError(w http.ResponseWriter, err error) {
	code, msg := classifyError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code) // must be called BEFORE Encode, or it defaults to 200
	// json.NewEncoder(w).Encode(...) writes JSON directly to the response body.
	// The _ discards the encoding error — if the response is broken at this point
	// there's nothing sensible to do.
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

// classifyError maps an error to (httpStatusCode, userFacingMessage).
// It checks error types in priority order: specific → generic.
//
// 📝 KEY GO CONCEPT — errors.As vs errors.Is:
//   errors.As(err, &target)  → true if err (or any wrapped cause) is of target's type
//                              useful for struct types like *AppError, *PgError
//   errors.Is(err, sentinel) → true if err (or any wrapped cause) == sentinel value
//                              useful for specific error variables like jwt.ErrTokenExpired
func classifyError(err error) (int, string) {
	// ── 1. AppError — our typed errors; status code is authoritative ──────────
	// errors.As unwraps the error chain looking for a *apperrors.AppError.
	// If found, it fills appErr and returns true.
	//
	// 📝 EXAMPLE:
	//   apperrors.New("Job not found", 404) → code=404, msg="Job not found"
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr.StatusCode, appErr.Message
	}

	// ── 2. PostgreSQL unique-constraint violation (error code "23505") ─────────
	// pgconn.PgError is the structured error from the PostgreSQL wire protocol.
	// Code "23505" = unique_violation — e.g. duplicate email on signup.
	//
	// 📝 EQUIVALENT to Prisma's P2002 handling in Node.js errorHandler middleware.
	//
	// 📝 EXAMPLE:
	//   INSERT INTO users(email) VALUES ('existing@email.com')
	//   → PgError{Code: "23505"} → HTTP 409 Conflict
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return http.StatusConflict, "A record with that value already exists."
	}

	// ── 3. JWT errors — specific error values from the jwt library ────────────
	// errors.Is checks if the error chain contains the specific sentinel value.
	//
	// 📝 EXAMPLE:
	//   expired accessToken → jwt.ErrTokenExpired → HTTP 401 "jwt expired"
	if errors.Is(err, jwt.ErrTokenExpired) {
		return http.StatusUnauthorized, "jwt expired"
	}
	// Tampered or wrong-secret token
	if errors.Is(err, jwt.ErrTokenSignatureInvalid) ||
		errors.Is(err, jwt.ErrSignatureInvalid) ||
		errors.Is(err, jwt.ErrTokenMalformed) {
		return http.StatusUnauthorized, err.Error()
	}

	// ── 4. Validation errors from go-playground/validator ────────────────────
	// The validator library produces errors with messages like:
	//   "Key: 'LoginRequest.Email' Error: ... 'email' tag"
	// We detect this heuristically and return 400.
	msg := err.Error()
	if strings.Contains(msg, "Key:") && strings.Contains(msg, "Error:") {
		return http.StatusBadRequest, fmt.Sprintf("Validation failed: %s", msg)
	}

	// ── 5. Fallback — unexpected errors become 500 ────────────────────────────
	// Log the full error server-side (for debugging) but return a generic
	// message to the client (security: don't leak internals).
	//
	// 📝 WHY GENERIC MESSAGE?
	//   If we returned err.Error() directly, internal details like SQL queries,
	//   file paths, or stack traces would be visible to the client. That's an
	//   information-disclosure vulnerability.
	log.Printf("[error] unhandled: %v", err)
	return http.StatusInternalServerError, "Internal server error"
}
