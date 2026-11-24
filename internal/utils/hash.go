// ============================================================
// FILE: internal/utils/hash.go
// ROLE: Password hashing and comparison using bcrypt.
//
// 📚 WHAT IS BCRYPT?
//   bcrypt is a password-hashing algorithm designed to be SLOW on purpose.
//   The "cost" parameter (10 here) doubles the work for each increment:
//     cost=10 → ~100ms per hash
//     cost=11 → ~200ms per hash
//     cost=12 → ~400ms per hash
//
//   This makes brute-force attacks impractical even if the hash database leaks.
//
// 📝 NEVER store plain-text passwords. Always store the bcrypt hash.
//
// 📝 COMPARISON with Node.js:
//   const hash = await bcrypt.hash(password, 10)         → HashPassword
//   const match = await bcrypt.compare(plain, hash)      → ComparePasswords
//
// 📝 EXAMPLE:
//   hash, _ := utils.HashPassword("secret123")
//   // hash looks like: "$2a$10$..."  (60 chars, includes salt)
//
//   err := utils.ComparePasswords("secret123", hash)
//   // err == nil → passwords match
//   // err == bcrypt.ErrMismatchedHashAndPassword → wrong password
// ============================================================
package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword hashes a plain-text password using bcrypt with cost 10.
// The hash embeds a random salt automatically, so two hashes of the
// same password will be different strings — both will verify correctly.
//
// 📝 SALT EXAMPLE:
//   HashPassword("abc") → "$2a$10$XyZrandom...hashedpart"
//   HashPassword("abc") → "$2a$10$AbCdifferent...hashedpart"
//   Both verify against "abc" even though they look different.
//
// Returns (hash string, error). Error is non-nil only if the system
// runs out of entropy (extremely rare) or cost is out of range.
func HashPassword(password string) (string, error) {
	// []byte(password) converts the string to a byte slice — bcrypt works on bytes.
	// bcrypt.DefaultCost == 10, which gives ~100ms on modern hardware.
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	// string(bytes) converts the []byte result back to a Go string.
	return string(bytes), err
}

// ComparePasswords checks whether a plain-text password matches a bcrypt hash.
// It runs the same bcrypt computation on `plain` and compares the result
// to `hashed` in constant time (to prevent timing attacks).
//
// 📝 TIMING ATTACK:
//   A naive string comparison (a == b) exits early on the first mismatch.
//   An attacker can measure the response time to guess characters one by one.
//   bcrypt.CompareHashAndPassword always takes the same amount of time regardless
//   of where the mismatch is — constant-time comparison.
//
// Returns:
//   nil                                  → passwords match (login succeeds)
//   bcrypt.ErrMismatchedHashAndPassword  → wrong password (login fails)
//
// 📝 USAGE IN AUTH SERVICE:
//   if err := utils.ComparePasswords(inputPassword, user.Password); err != nil {
//       return apperrors.New("Invalid credentials", 401)
//   }
func ComparePasswords(plain, hashed string) error {
	// Note the argument order: (hash, plain) — the hash comes first in bcrypt's API.
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}
