// Package middleware contains HTTP middleware for the chi router.
package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
)

// validate is the singleton validator instance; reuse for performance.
var validate = validator.New()

// DecodeAndValidate decodes a JSON request body into T and validates it using
// struct-tag rules from go-playground/validator.
//
// Mirrors the Node.js validate middleware (src/middleware/validate.middleware.ts)
// which used Zod schema validation on req.body.
//
// Usage in a handler:
//
//	body, err := middleware.DecodeAndValidate[LoginRequest](r)
//	if err != nil { WriteError(w, apperrors.New(err.Error(), 400)); return }
func DecodeAndValidate[T any](r *http.Request) (T, error) {
	var body T
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return body, err
	}
	if err := validate.Struct(body); err != nil {
		return body, err
	}
	return body, nil
}
