// Package middleware contains HTTP middleware for the chi router.
package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"

	apperrors "github.com/alexfromural-lgtm/job-finder-backend-go/internal/errors"
)

// errorResponse is the JSON envelope returned to the client on any error.
// Mirrors the Node.js { error: "..." } shape used everywhere in the original API.
type errorResponse struct {
	Error string `json:"error"`
}

// WriteError encodes an error as JSON and writes it to the response.
// Handles *apperrors.AppError, pgx unique-violation errors, JWT errors, and
// generic errors — exactly the same classification as the Node.js errorHandler
// middleware (src/middleware/errorHandler.middleware.ts).
func WriteError(w http.ResponseWriter, err error) {
	code, msg := classifyError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

// classifyError maps an error to (httpStatusCode, userFacingMessage).
func classifyError(err error) (int, string) {
	// ── 1. Structured AppError — status code is authoritative ─────────────────
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr.StatusCode, appErr.Message
	}

	// ── 2. PostgreSQL unique-constraint violation (code 23505) ────────────────
	// Equivalent to Prisma P2002 handling in the Node.js error handler.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return http.StatusConflict, "A record with that value already exists."
	}

	// ── 3. JWT errors ─────────────────────────────────────────────────────────
	if errors.Is(err, jwt.ErrTokenExpired) {
		return http.StatusUnauthorized, "jwt expired"
	}
	if errors.Is(err, jwt.ErrTokenSignatureInvalid) ||
		errors.Is(err, jwt.ErrSignatureInvalid) ||
		errors.Is(err, jwt.ErrTokenMalformed) {
		return http.StatusUnauthorized, err.Error()
	}

	// ── 4. Generic validation errors (go-playground/validator) ───────────────
	msg := err.Error()
	if strings.Contains(msg, "Key:") && strings.Contains(msg, "Error:") {
		return http.StatusBadRequest, fmt.Sprintf("Validation failed: %s", msg)
	}

	// ── 5. Fallback — log internally, return generic 500 ─────────────────────
	log.Printf("[error] unhandled: %v", err)
	return http.StatusInternalServerError, "Internal server error"
}
