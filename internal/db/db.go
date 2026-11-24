// ============================================================
// FILE: internal/db/db.go
// ROLE: PostgreSQL connection pool — opened once, shared everywhere.
//
// 📚 WHAT IS THIS FILE?
//   This file wraps the pgx/v5 connection pool.
//   A "connection pool" is a cache of open database connections.
//
//   WHY A POOL?
//   Opening a TCP connection + TLS handshake + PostgreSQL auth
//   takes ~10–50ms. If we did that per-request (like simple scripts do)
//   a server under load would spend most of its time just connecting.
//   A pool pre-opens N connections and lends them out to handlers,
//   returning them when the query finishes.
//
// 📝 ANALOGY:
//   Think of it like a taxi rank: instead of building a new car
//   for every passenger (expensive), taxis wait at the rank (pool)
//   and are assigned to the next passenger immediately.
//
// 📝 USAGE IN HANDLERS/SERVICES:
//   pool.QueryRow(ctx, "SELECT ...", arg).Scan(&dest)
//   pool.Exec(ctx, "INSERT INTO ...", arg1, arg2)
// ============================================================
package db

import (
	"context"
	"fmt"
	"os"

	// pgxpool — pgx/v5's built-in connection pool.
	// pgx is the most popular PostgreSQL driver for Go.
	// It speaks the PostgreSQL wire protocol natively (faster than database/sql).
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the application-wide PostgreSQL connection pool.
// It is a package-level variable so it can be accessed without passing it
// explicitly everywhere. In this project we prefer passing pool as a parameter
// (dependency injection) — Pool is kept here mainly for legacy/seed scripts.
//
// 📝 KEY CONCEPT — package-level variables:
//   var Pool *pgxpool.Pool declares a variable at the package scope.
//   It is nil until Open() assigns it.
var Pool *pgxpool.Pool

// Open creates the pgx/v5 connection pool, pings the database, and returns it.
// It calls os.Exit(1) on any error so the container restarts cleanly.
//
// 📝 STEP-BY-STEP:
//   1. pgxpool.ParseConfig(url) → parses the URL into a structured config
//      "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
//   2. pgxpool.NewWithConfig(ctx, cfg) → opens the pool (may not connect yet)
//   3. pool.Ping(ctx) → actually tests the connection
//
// 📝 EXAMPLE:
//   pool := db.Open("postgres://postgres:secret@localhost/jobfinder")
//   // pool is ready; queries can now be made via pool.QueryRow(...)
func Open(databaseURL string) *pgxpool.Pool {
	// pgxpool.ParseConfig parses the connection string into a *pgxpool.Config.
	// This lets us inspect / override settings (max connections, etc.)
	// before the pool is created.
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// os.Stderr is the standard error output stream.
		// fmt.Fprintf formats and writes directly to it (like console.error in JS).
		fmt.Fprintf(os.Stderr, "[db] Failed to parse DATABASE_URL: %v\n", err)
		os.Exit(1) // exit code 1 signals failure to Docker / Kubernetes
	}

	// pgxpool.NewWithConfig creates the pool but doesn't necessarily open connections yet.
	// context.Background() is a never-cancelling context — appropriate for startup.
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[db] Failed to create connection pool: %v\n", err)
		os.Exit(1)
	}

	// pool.Ping sends a simple round-trip to PostgreSQL to verify connectivity.
	// If the database is down, we exit here with a clear message.
	//
	// 📝 WHY PING AT STARTUP?
	//   Without a ping, the server would start "successfully" but every
	//   database call would fail at runtime with confusing errors.
	//   Failing early gives a clear signal: "check your DATABASE_URL / DB status".
	if err := pool.Ping(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "[db] Database ping failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[db] PostgreSQL connection pool established.")
	Pool = pool // also store in the package-level variable
	return pool
}

// Close drains and closes the pool. Call via `defer db.Close(pool)` in main().
//
// 📝 WHY CLOSE?
//   Each connection in the pool is a live TCP socket to PostgreSQL.
//   Closing cleanly lets PostgreSQL reclaim those connection slots and
//   avoids "too many connections" errors on the next server start.
//
// 📝 EXAMPLE:
//   pool := db.Open(cfg.DatabaseURL)
//   defer db.Close(pool)   ← runs when main() returns
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
		fmt.Println("[db] Connection pool closed.")
	}
}
