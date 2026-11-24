// ============================================================
// FILE: internal/middleware/validate.go
// ROLE: Generic JSON body decoder + struct-tag validator.
//
// 📚 WHAT IS THIS FILE?
//   Every handler that accepts a JSON body needs to:
//   1. Read and decode the JSON (json.Decode)
//   2. Validate the fields (required, min length, email format, etc.)
//
//   DecodeAndValidate[T]() does both in one call using Go generics.
//
// 📝 COMPARISON with Node.js (src/middleware/validate.middleware.ts):
//   Node.js used Zod schema validation:
//     const body = loginSchema.parse(req.body)
//   Go equivalent:
//     body, err := middleware.DecodeAndValidate[LoginRequest](r)
//
//   Both reject the request with 400 if validation fails.
//
// 📝 VALIDATION TAGS (on struct fields):
//   validate:"required"       → field must not be zero value ("", 0, nil)
//   validate:"required,email" → required AND must be a valid email format
//   validate:"required,min=6" → required AND string length >= 6
//   validate:"omitempty,url"  → if present, must be a valid URL; "" is ok
//   validate:"oneof=a b c"    → value must be one of "a", "b", or "c"
//
// 📝 EXAMPLE:
//   type LoginRequest struct {
//       Email    string `json:"email"    validate:"required,email"`
//       Password string `json:"password" validate:"required,min=6"`
//   }
//
//   body, err := middleware.DecodeAndValidate[LoginRequest](r)
//   // body.Email, body.Password are now validated
// ============================================================
package middleware

import (
	"encoding/json"
	"net/http"

	// go-playground/validator — struct validation using field tags.
	// Equivalent to Zod, Joi, or class-validator in the Node.js ecosystem.
	"github.com/go-playground/validator/v10"
)

// validate is the singleton validator instance.
// Creating a validator is expensive (reflection-heavy); reusing the same
// instance across all requests is a standard Go optimisation.
//
// 📝 WHY PACKAGE-LEVEL?
//   var validate = validator.New() runs ONCE when the package is first imported.
//   All calls to DecodeAndValidate share the same instance.
var validate = validator.New()

// DecodeAndValidate reads the JSON request body into a value of type T
// and then validates it using the `validate:"..."` struct tags.
//
// 📝 GO GENERICS SYNTAX:
//   func DecodeAndValidate[T any](r *http.Request) (T, error)
//   The [T any] part means: T can be ANY type. The caller specifies T:
//     middleware.DecodeAndValidate[LoginRequest](r)
//   Go infers that T = LoginRequest and creates a LoginRequest value internally.
//
// 📝 STEP-BY-STEP:
//   1. var body T          → creates a zero-value LoginRequest{}
//   2. json.Decode(&body)  → fills it from the JSON body
//   3. validate.Struct(body) → checks all validate:"..." tags
//
// 📝 ERROR CASES:
//   - Invalid JSON (syntax error or wrong types) → decoding error
//   - Missing required field → validation error (Key: ... Error: ...)
//   - Email format wrong → validation error
//
// 📝 USAGE IN HANDLERS:
//   body, err := middleware.DecodeAndValidate[models.LoginRequest](r)
//   if err != nil {
//       middleware.WriteError(w, err)   // → 400 Bad Request
//       return
//   }
//   // body.Email and body.Password are now valid
func DecodeAndValidate[T any](r *http.Request) (T, error) {
	var body T // declares a zero-value of type T (e.g. LoginRequest{})

	// json.NewDecoder(r.Body) creates a streaming JSON decoder from the request body.
	// .Decode(&body) fills `body` from the JSON — it reads the body ONCE
	// (the body is a stream, not a string; you can't re-read it).
	//
	// 📝 &body passes a pointer so Decode can modify the original value.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return body, err // e.g. "invalid character '}' looking for beginning of value"
	}

	// validate.Struct(body) checks every field that has a `validate:"..."` tag.
	// It returns a ValidationErrors slice if any field fails.
	//
	// 📝 EXAMPLE VALIDATION ERROR MESSAGE:
	//   "Key: 'LoginRequest.Email' Error: Field validation for 'Email' failed on the 'email' tag"
	if err := validate.Struct(body); err != nil {
		return body, err
	}

	// Both decoding and validation succeeded — return the populated struct.
	return body, nil
}
