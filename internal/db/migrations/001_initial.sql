-- ============================================================
-- Migration 001: Initial schema
-- Mirrors prisma/schema.prisma exactly.
-- ============================================================

-- ── Enums ─────────────────────────────────────────────────────
CREATE TYPE role AS ENUM ('JOB_SEEKER', 'RECRUITER', 'ADMIN');

CREATE TYPE application_status AS ENUM (
  'submitted',
  'shortlisted',
  'rejected',
  'under_review'
);

CREATE TYPE notification_type AS ENUM (
  'application_update',
  'system'
);

CREATE TYPE report_status AS ENUM (
  'open',
  'reviewed',
  'dismissed'
);

-- ── Users ──────────────────────────────────────────────────────
CREATE TABLE users (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  email       TEXT NOT NULL UNIQUE,
  password    TEXT NOT NULL,
  roles       role[] NOT NULL DEFAULT '{}',
  is_active   BOOLEAN NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Job Seeker Profiles ────────────────────────────────────────
CREATE TABLE job_seeker_profiles (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  bio         TEXT,
  location    TEXT,
  skills      TEXT[] NOT NULL DEFAULT '{}',
  education   TEXT,
  experience  TEXT,
  resume_url  TEXT
);

-- ── Recruiter Profiles ─────────────────────────────────────────
CREATE TABLE recruiter_profiles (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  company_name     TEXT NOT NULL,
  company_website  TEXT,
  description      TEXT,
  industry         TEXT
);

-- ── Jobs ───────────────────────────────────────────────────────
CREATE TABLE jobs (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  recruiter_id  UUID NOT NULL REFERENCES recruiter_profiles(id) ON DELETE CASCADE,
  title         TEXT NOT NULL,
  description   TEXT NOT NULL,
  requirements  TEXT NOT NULL,
  location      TEXT NOT NULL,
  salary_range  TEXT,
  category      TEXT,
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Performance indexes (mirrors Prisma @@index directives)
CREATE INDEX idx_jobs_is_active ON jobs(is_active);
CREATE INDEX idx_jobs_is_active_category ON jobs(is_active, category);
CREATE INDEX idx_jobs_recruiter_created ON jobs(recruiter_id, created_at DESC);
CREATE INDEX idx_jobs_is_active_created ON jobs(is_active, created_at DESC);

-- ── Applications ───────────────────────────────────────────────
CREATE TABLE applications (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id         UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  job_seeker_id  UUID NOT NULL REFERENCES job_seeker_profiles(id) ON DELETE CASCADE,
  cover_letter   TEXT,
  status         application_status NOT NULL DEFAULT 'submitted',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Saved Jobs ─────────────────────────────────────────────────
CREATE TABLE saved_jobs (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id         UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  job_seeker_id  UUID NOT NULL REFERENCES job_seeker_profiles(id) ON DELETE CASCADE,
  saved_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uq_saved_jobs UNIQUE (job_id, job_seeker_id)
);

-- ── Notifications ──────────────────────────────────────────────
CREATE TABLE notifications (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type        notification_type NOT NULL,
  is_read     BOOLEAN NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Reports ────────────────────────────────────────────────────
CREATE TABLE reports (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  reporter_id      UUID NOT NULL REFERENCES users(id),
  reported_user_id UUID REFERENCES users(id),
  reported_job_id  UUID REFERENCES jobs(id) ON DELETE SET NULL,
  reason           TEXT NOT NULL,
  status           report_status NOT NULL DEFAULT 'open',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── updated_at trigger (auto-update on row mutation) ──────────
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
  BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_jobs_updated_at
  BEFORE UPDATE ON jobs
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_applications_updated_at
  BEFORE UPDATE ON applications
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
