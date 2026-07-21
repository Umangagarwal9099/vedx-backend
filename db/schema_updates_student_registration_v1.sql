-- ============================================================================
-- Rich learner-detail page support: extended student registration/demographic
-- profile, a student-level status lifecycle with history, staff-authored
-- learner notes, and student login/device activity tracking.
-- Fully additive — no existing table is touched except adding a `status`
-- column to `students`.
-- ============================================================================

-- ── Student status lifecycle ────────────────────────────────────────────────

DO $$ BEGIN
    CREATE TYPE student_status AS ENUM ('registered', 'enrolled', 'completed', 'on_leave', 'archived');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER TABLE students ADD COLUMN IF NOT EXISTS status student_status NOT NULL DEFAULT 'registered';

CREATE TABLE IF NOT EXISTS student_status_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id         VARCHAR(16) NOT NULL UNIQUE,
    student_user_id  UUID NOT NULL REFERENCES users(id),
    status           student_status NOT NULL,
    notes            TEXT,
    changed_by       UUID NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_student_status_history_student ON student_status_history (student_user_id, created_at DESC);

-- ── Extended registration/demographic profile (one row per student) ────────

CREATE TABLE IF NOT EXISTS student_registration_details (
    user_id             UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    gender              TEXT,
    alternate_contact   TEXT,
    student_source      TEXT,
    religion            TEXT,
    standard            TEXT,
    occupation          TEXT,
    timezone            TEXT,
    parent_name         TEXT,
    parent_contact      TEXT,
    parent_email        TEXT,
    area                TEXT,
    school_college_name TEXT,
    residential_address TEXT,
    permanent_address   TEXT,
    city                TEXT,
    state               TEXT,
    pincode             TEXT,
    opt_whatsapp        BOOLEAN NOT NULL DEFAULT TRUE,
    opt_email           BOOLEAN NOT NULL DEFAULT TRUE,
    opt_sms             BOOLEAN NOT NULL DEFAULT TRUE,
    opt_push            BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Learner notes (staff-authored, freeform) ────────────────────────────────

CREATE TABLE IF NOT EXISTS student_notes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id        VARCHAR(16) NOT NULL UNIQUE,
    student_user_id UUID NOT NULL REFERENCES users(id),
    note            TEXT NOT NULL,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_student_notes_student ON student_notes (student_user_id, created_at DESC);

-- ── Login / device activity tracking ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS login_activity (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    device_id        TEXT NOT NULL,
    device_type      TEXT NOT NULL DEFAULT 'unknown',
    os_name          TEXT NOT NULL DEFAULT '',
    browser_name     TEXT NOT NULL DEFAULT '',
    browser_version  TEXT NOT NULL DEFAULT '',
    login_count      INT NOT NULL DEFAULT 1,
    last_login_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    removed_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_login_activity_user ON login_activity (user_id, last_login_at DESC);

-- ============================================================================
-- End of file
-- ============================================================================
