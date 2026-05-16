// cmd/seed/main.go — Standalone database seeder.
// Mirrors prisma/seed.ts: same demo users, recruiter, job, application, and saved job.
// Automatically runs migrations before seeding so it works on a fresh database.
//
// Usage:
//
//	DATABASE_URL=postgresql://... go run ./cmd/seed
//	# or inside Docker:
//	docker compose run --rm backend go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const migrationFile = "internal/db/migrations/001_initial.sql"

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "[seed] ERROR: DATABASE_URL env var is required")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seed] ERROR: failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	fmt.Println("🌱 Starting seed...")

	// ── Run migrations (idempotent — reads 001_initial.sql from disk) ─────────
	fmt.Printf("⚙️  Applying migrations from %s...\n", migrationFile)
	sqlBytes, err := os.ReadFile(migrationFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seed] ERROR: could not read migration file: %v\n", err)
		os.Exit(1)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		fmt.Fprintf(os.Stderr, "[seed] ERROR: migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Migrations applied")

	// ── Clean existing data in FK-safe order ──────────────────────────────────
	tables := []string{
		"saved_jobs", "applications", "jobs",
		"job_seeker_profiles", "recruiter_profiles", "users",
	}
	for _, t := range tables {
		if _, err := pool.Exec(ctx, "DELETE FROM "+t); err != nil {
			fmt.Fprintf(os.Stderr, "[seed] WARN: could not truncate %s: %v\n", t, err)
		}
	}

	// ── Admin user ────────────────────────────────────────────────────────────
	adminHash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	var adminID string
	err = pool.QueryRow(ctx,
		`INSERT INTO users (name, email, password, roles)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		"Admin User", "admin@example1.com", string(adminHash), []string{"ADMIN"},
	).Scan(&adminID)
	must(err, "create admin user")
	fmt.Println("✅ Created admin user: admin@example1.com")

	// ── Recruiter user + profile + job ────────────────────────────────────────
	recruiterHash, _ := bcrypt.GenerateFromPassword([]byte("recruiter123"), bcrypt.DefaultCost)
	var recruiterUserID string
	err = pool.QueryRow(ctx,
		`INSERT INTO users (name, email, password, roles)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		"Recruiter Jane", "recruiter@example.com", string(recruiterHash), []string{"RECRUITER"},
	).Scan(&recruiterUserID)
	must(err, "create recruiter user")

	var recruiterProfileID string
	err = pool.QueryRow(ctx,
		`INSERT INTO recruiter_profiles (user_id, company_name, company_website, description, industry)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		recruiterUserID, "Tech Corp", "https://techcorp.com",
		"A fast-growing tech company", "Software",
	).Scan(&recruiterProfileID)
	must(err, "create recruiter profile")
	fmt.Println("✅ Created recruiter: recruiter@example.com | Company: Tech Corp")

	var jobID string
	err = pool.QueryRow(ctx,
		`INSERT INTO jobs (recruiter_id, title, description, requirements, location, salary_range, category)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		recruiterProfileID,
		"Senior FullStack Developer",
		"React developer needed",
		"8+ years of experience in React and Node.js",
		"Remote",
		"$130,000 - $180,000",
		"Software",
	).Scan(&jobID)
	must(err, "create job")
	fmt.Println("✅ Created job: Senior FullStack Developer")

	// ── Job seeker user + profile + application + saved job ───────────────────
	seekerHash, _ := bcrypt.GenerateFromPassword([]byte("seeker123"), bcrypt.DefaultCost)
	var seekerUserID string
	err = pool.QueryRow(ctx,
		`INSERT INTO users (name, email, password, roles)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		"Job Seeker John", "seeker@example.com", string(seekerHash), []string{"JOB_SEEKER"},
	).Scan(&seekerUserID)
	must(err, "create seeker user")

	var seekerProfileID string
	err = pool.QueryRow(ctx,
		`INSERT INTO job_seeker_profiles (user_id, bio, location, skills, education, experience, resume_url)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		seekerUserID,
		"Passionate about frontend development",
		"Sydney",
		[]string{"React", "TypeScript", "HTML", "CSS"},
		"BSc in Computer Science",
		"2 years at Webify",
		"https://example.com/resume/john.pdf",
	).Scan(&seekerProfileID)
	must(err, "create seeker profile")
	fmt.Println("✅ Created job seeker: seeker@example.com")

	_, err = pool.Exec(ctx,
		`INSERT INTO applications (job_id, job_seeker_id, cover_letter)
		 VALUES ($1, $2, $3)`,
		jobID, seekerProfileID, "I'm very interested in this opportunity!",
	)
	must(err, "create application")
	fmt.Println("✅ Created application")

	_, err = pool.Exec(ctx,
		`INSERT INTO saved_jobs (job_id, job_seeker_id) VALUES ($1, $2)`,
		jobID, seekerProfileID,
	)
	must(err, "create saved job")
	fmt.Println("✅ Created saved job")

	fmt.Println("✅ Seeding complete!")
}

func must(err error, op string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "[seed] ERROR during %s: %v\n", op, err)
		os.Exit(1)
	}
}
