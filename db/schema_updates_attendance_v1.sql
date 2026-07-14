-- ============================================================================
-- Phase 2 of the LMS-flow audit: real session attendance. A student's
-- attendance is tracked per (session, student) — a mentor/admin marks it,
-- and rollups (per-batch, per-student) are computed from these rows.
-- Fully additive — no existing table is touched.
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE attendance_status AS ENUM ('present', 'absent', 'late', 'excused');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS session_attendance (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id     VARCHAR(16) NOT NULL UNIQUE,
    session_id   UUID NOT NULL REFERENCES sessions(id),
    batch_id     UUID NOT NULL REFERENCES batches(id),
    student_id   UUID NOT NULL REFERENCES users(id),
    status       attendance_status NOT NULL DEFAULT 'absent',
    marked_by    UUID REFERENCES users(id),
    marked_at    TIMESTAMPTZ,
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, student_id)
);

CREATE INDEX IF NOT EXISTS idx_session_attendance_batch_student
    ON session_attendance (batch_id, student_id);
CREATE INDEX IF NOT EXISTS idx_session_attendance_session
    ON session_attendance (session_id);

-- ============================================================================
-- End of file
-- ============================================================================
