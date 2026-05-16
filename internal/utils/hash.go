// Package utils provides shared helper functions for the application.
package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword hashes a plain-text password using bcrypt with a cost of 10.
// Mirrors Node.js: bcrypt.hash(password, 10)
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	return string(bytes), err
}

// ComparePasswords compares a plain-text password against a bcrypt hash.
// Returns nil on match, bcrypt.ErrMismatchedHashAndPassword on mismatch.
// Mirrors Node.js: bcrypt.compare(plain, hashed)
func ComparePasswords(plain, hashed string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}
