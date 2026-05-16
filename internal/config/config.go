// Package config parses and validates all environment variables at startup.
// The process exits immediately if any required variable is missing or invalid,
// mirroring the behaviour of the Node.js src/config/env.ts Zod schema.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all validated environment variables for the application.
type Config struct {
	// Database
	DatabaseURL string

	// JWT secrets
	AccessTokenSecret  string
	RefreshTokenSecret string

	// Token durations
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration

	// Server
	Port    string
	GoEnv   string

	// Redis / queue
	RedisURL          string
	QueueConcurrency  int

	// CORS
	CORSOrigin string
}

// IsProduction returns true when GO_ENV is "production".
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.GoEnv, "production")
}

// IsDevelopment returns true when GO_ENV is "development" (default).
func (c *Config) IsDevelopment() bool {
	return !c.IsProduction()
}

// Load reads environment variables, validates them, and returns a Config.
// It calls os.Exit(1) with a descriptive error if any required value is
// missing or invalid — fail fast, never silently.
func Load() *Config {
	cfg := &Config{}
	errs := []string{}

	// ── Required string fields ────────────────────────────────────────────────
	required := map[string]*string{
		"DATABASE_URL":         &cfg.DatabaseURL,
		"ACCESS_TOKEN_SECRET":  &cfg.AccessTokenSecret,
		"REFRESH_TOKEN_SECRET": &cfg.RefreshTokenSecret,
		"REDIS_URL":            &cfg.RedisURL,
	}
	for key, dest := range required {
		val := os.Getenv(key)
		if val == "" {
			errs = append(errs, fmt.Sprintf("  missing required env var: %s", key))
			continue
		}
		*dest = val
	}

	// ── Secret length validation (≥ 16 chars) ─────────────────────────────────
	if cfg.AccessTokenSecret != "" && len(cfg.AccessTokenSecret) < 16 {
		errs = append(errs, "  ACCESS_TOKEN_SECRET must be at least 16 characters")
	}
	if cfg.RefreshTokenSecret != "" && len(cfg.RefreshTokenSecret) < 16 {
		errs = append(errs, "  REFRESH_TOKEN_SECRET must be at least 16 characters")
	}

	// ── Token expiry durations ────────────────────────────────────────────────
	accessExpiry := getEnvOrDefault("ACCESS_TOKEN_EXPIRES_IN", "15m")
	d, err := parseDuration(accessExpiry)
	if err != nil {
		errs = append(errs, fmt.Sprintf("  invalid ACCESS_TOKEN_EXPIRES_IN %q: %v", accessExpiry, err))
	} else {
		cfg.AccessTokenExpiry = d
	}

	refreshExpiry := getEnvOrDefault("REFRESH_TOKEN_EXPIRES_IN", "168h") // 7d
	d, err = parseDuration(refreshExpiry)
	if err != nil {
		errs = append(errs, fmt.Sprintf("  invalid REFRESH_TOKEN_EXPIRES_IN %q: %v", refreshExpiry, err))
	} else {
		cfg.RefreshTokenExpiry = d
	}

	// ── Optional fields with defaults ────────────────────────────────────────
	cfg.Port = getEnvOrDefault("PORT", "5002")
	cfg.GoEnv = getEnvOrDefault("GO_ENV", "development")
	cfg.CORSOrigin = getEnvOrDefault("CORS_ORIGIN", "http://localhost:3000")

	concurrencyStr := getEnvOrDefault("QUEUE_CONCURRENCY", "5")
	concurrency, err := strconv.Atoi(concurrencyStr)
	if err != nil || concurrency < 1 {
		errs = append(errs, fmt.Sprintf("  invalid QUEUE_CONCURRENCY %q: must be a positive integer", concurrencyStr))
	} else {
		cfg.QueueConcurrency = concurrency
	}

	// ── Fail fast on validation errors ───────────────────────────────────────
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "[config] Environment validation failed:")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	return cfg
}

// getEnvOrDefault returns the environment variable value or a fallback.
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseDuration understands Go duration strings (e.g. "15m", "168h") as well
// as the shorthand "7d" format used in the Node.js .env.sample.
func parseDuration(s string) (time.Duration, error) {
	// Convert "Nd" → "N*24h" so time.ParseDuration handles it.
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid day value in %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
