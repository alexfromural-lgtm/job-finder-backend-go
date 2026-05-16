// Package db provides a pgx/v5 connection pool shared across the application.
package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the application-wide PostgreSQL connection pool.
// Initialised once by Open() and passed to handlers/services via dependency injection.
var Pool *pgxpool.Pool

// Open creates the pgx/v5 connection pool and verifies connectivity with a Ping.
// Calls os.Exit(1) if the database is unreachable, so the container restarts cleanly.
func Open(databaseURL string) *pgxpool.Pool {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db] Failed to parse DATABASE_URL: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db] Failed to create connection pool: %v\n", err)
		os.Exit(1)
	}

	if err := pool.Ping(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "[db] Database ping failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[db] PostgreSQL connection pool established.")
	Pool = pool
	return pool
}

// Close drains the pool on shutdown. Call via defer in main.
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
		fmt.Println("[db] Connection pool closed.")
	}
}
