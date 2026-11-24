// ============================================================
// FILE: internal/config/config.go
// ROLE: Environment configuration loader and validator.
//
// 📚 WHAT IS THIS FILE?
//   This file reads environment variables (from the OS or a .env file)
//   and builds a strongly-typed Config struct that the rest of the app uses.
//
//   Why not just call os.Getenv() everywhere?
//   → Single source of truth: config is validated ONCE at startup.
//   → Type safety: durations are time.Duration, not raw strings.
//   → Fail-fast: missing secrets crash immediately with a clear message.
//
// 📝 EXAMPLE USAGE:
//   cfg := config.Load()
//   fmt.Println(cfg.Port)             // "5002"
//   fmt.Println(cfg.AccessTokenExpiry) // 15m0s (time.Duration)
// ============================================================
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the application.
// Using a struct (instead of global variables) makes it easy to pass to
// functions and to substitute test-only values in unit tests.
//
// 📝 EXAMPLE:
//   type Config struct {
//       Port string        // "5002"
//       GoEnv string       // "development" | "production"
//       DatabaseURL string // "postgres://user:pass@host/db"
//   }
type Config struct {
	// Server
	Port  string // the TCP port to listen on, e.g. "5002"
	GoEnv string // "development" or "production"

	// Database
	DatabaseURL string // full PostgreSQL connection string

	// Redis (used by the asynq task queue)
	RedisURL string

	// JWT secrets — MUST be long random strings, never committed to git
	AccessTokenSecret  string
	RefreshTokenSecret string

	// JWT expiry durations — parsed from strings like "15m", "7d", "168h"
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration

	// CORS — which frontend origin is allowed to call the API
	CORSOrigin string

	// Queue worker concurrency — how many background tasks run in parallel
	QueueConcurrency int
}

// IsProduction returns true when GO_ENV=production.
// Used to gate HSTS headers and Secure cookie flags.
//
// 📝 EXAMPLE:
//   if cfg.IsProduction() {
//       r.Use(middleware.SecureHeadersProduction) // adds HSTS
//   }
func (c *Config) IsProduction() bool {
	return c.GoEnv == "production"
}

// Load reads environment variables, validates them, and returns a *Config.
// It calls os.Exit(1) on validation failure so the binary crashes
// loudly rather than starting with broken configuration.
//
// 📝 HOW IT WORKS:
//   1. godotenv.Load() tries to read .env (silently ignored if missing in prod)
//   2. Required fields are checked; missing ones accumulate in `errs`
//   3. Optional fields get sensible defaults via getEnvOrDefault()
//   4. If errs is non-empty, all errors are printed at once and the program exits
func Load() *Config {
	cfg := &Config{}
	var errs []string // collect ALL errors before failing so the user sees them all at once

	// ── Required string fields ────────────────────────────────────────────────
	// os.Getenv returns "" if the variable is not set.
	// We treat "" as "missing" and add a human-readable error message.

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		errs = append(errs, "  DATABASE_URL is required")
	}

	cfg.RedisURL = os.Getenv("REDIS_URL")
	if cfg.RedisURL == "" {
		errs = append(errs, "  REDIS_URL is required")
	}

	cfg.AccessTokenSecret = os.Getenv("ACCESS_TOKEN_SECRET")
	if cfg.AccessTokenSecret == "" {
		errs = append(errs, "  ACCESS_TOKEN_SECRET is required")
	}

	cfg.RefreshTokenSecret = os.Getenv("REFRESH_TOKEN_SECRET")
	if cfg.RefreshTokenSecret == "" {
		errs = append(errs, "  REFRESH_TOKEN_SECRET is required")
	}

	// ── Minimum length validation for secrets ─────────────────────────────────
	// Short secrets are weak. 16 chars (128 bits) is the minimum for HMAC-SHA256.
	//
	// 📝 EXAMPLE: ACCESS_TOKEN_SECRET="abc" → error (too short)
	//             ACCESS_TOKEN_SECRET="my-super-secret-key" → ok (19 chars)
	if cfg.AccessTokenSecret != "" && len(cfg.AccessTokenSecret) < 16 {
		errs = append(errs, "  ACCESS_TOKEN_SECRET must be at least 16 characters")
	}
	if cfg.RefreshTokenSecret != "" && len(cfg.RefreshTokenSecret) < 16 {
		errs = append(errs, "  REFRESH_TOKEN_SECRET must be at least 16 characters")
	}

	// ── Token expiry durations ────────────────────────────────────────────────
	// We parse duration strings like "15m", "2h", "7d" into time.Duration values.
	// time.Duration is an int64 (nanoseconds), so comparisons and arithmetic work:
	//   15 * time.Minute == time.Duration(15 * 60 * 1_000_000_000)
	//
	// 📝 EXAMPLE:
	//   ACCESS_TOKEN_EXPIRES_IN=15m  → cfg.AccessTokenExpiry = 15 * time.Minute
	//   REFRESH_TOKEN_EXPIRES_IN=7d  → cfg.RefreshTokenExpiry = 168 * time.Hour

	accessExpiry := getEnvOrDefault("ACCESS_TOKEN_EXPIRES_IN", "15m")
	d, err := parseDuration(accessExpiry)
	if err != nil {
		errs = append(errs, fmt.Sprintf("  invalid ACCESS_TOKEN_EXPIRES_IN %q: %v", accessExpiry, err))
	} else {
		cfg.AccessTokenExpiry = d
	}

	refreshExpiry := getEnvOrDefault("REFRESH_TOKEN_EXPIRES_IN", "168h") // 7 days
	d, err = parseDuration(refreshExpiry)
	if err != nil {
		errs = append(errs, fmt.Sprintf("  invalid REFRESH_TOKEN_EXPIRES_IN %q: %v", refreshExpiry, err))
	} else {
		cfg.RefreshTokenExpiry = d
	}

	// ── Optional fields with defaults ─────────────────────────────────────────
	// getEnvOrDefault(key, fallback) returns the env var value, or fallback if unset.
	cfg.Port       = getEnvOrDefault("PORT", "5002")
	cfg.GoEnv      = getEnvOrDefault("GO_ENV", "development")
	cfg.CORSOrigin = getEnvOrDefault("CORS_ORIGIN", "http://localhost:3000")

	concurrencyStr := getEnvOrDefault("QUEUE_CONCURRENCY", "5")
	concurrency, err := strconv.Atoi(concurrencyStr) // converts string "5" → int 5
	if err != nil || concurrency < 1 {
		errs = append(errs, fmt.Sprintf("  invalid QUEUE_CONCURRENCY %q: must be a positive integer", concurrencyStr))
	} else {
		cfg.QueueConcurrency = concurrency
	}

	// ── Fail fast ─────────────────────────────────────────────────────────────
	// If any required env vars are missing or invalid, print ALL errors and exit.
	// Printing all at once is friendlier than exiting on the first error.
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "[config] Environment validation failed:")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e) // e.g. "  DATABASE_URL is required"
		}
		os.Exit(1)
	}

	return cfg
}

// getEnvOrDefault returns the environment variable value, or fallback if unset/empty.
//
// 📝 EXAMPLE:
//   os.Setenv("PORT", "8080")
//   getEnvOrDefault("PORT", "5002") → "8080"
//   getEnvOrDefault("MISSING", "5002") → "5002"
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseDuration extends time.ParseDuration to also understand "Nd" (N days).
// Go's standard library does not have a "d" unit, so we convert it manually.
//
// 📝 SUPPORTED FORMATS:
//   "15m"  → 15 minutes
//   "2h"   → 2 hours
//   "168h" → 168 hours (7 days)
//   "7d"   → 7 × 24h = 168h (our custom extension)
//
// 📝 EXAMPLE:
//   parseDuration("7d") → 168h0m0s, nil
//   parseDuration("abc") → 0, error
func parseDuration(s string) (time.Duration, error) {
	// strings.HasSuffix checks if s ends with "d"
	if strings.HasSuffix(s, "d") {
		// strings.TrimSuffix("7d", "d") → "7"
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid day value in %q", s)
		}
		// time.Duration(days) converts the int to time.Duration nanoseconds
		// then we multiply: 7 * 24 * time.Hour = 604800000000000 ns
		return time.Duration(days) * 24 * time.Hour, nil
	}
	// Fall through to Go's built-in parser for "15m", "2h", "30s", etc.
	return time.ParseDuration(s)
}
