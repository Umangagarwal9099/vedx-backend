-- ============================================================================
-- Phase 1 of the LMS-flow audit: real StudentEnrollment (course + batch +
-- status + dates), a proper batch lifecycle status, and batch capacity.
-- All additive — nothing here renames or drops an existing column, and
-- existing batch_students rows get a matching enrollment record backfilled
-- so no history is lost.
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE batch_status AS ENUM ('draft', 'upcoming', 'active', 'completed', 'cancelled', 'archived');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE enrollment_status AS ENUM ('invited', 'enrolled', 'active', 'on_hold', 'completed', 'dropped', 'transferred', 'removed');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER TABLE batches ADD COLUMN IF NOT EXISTS status batch_status NOT NULL DEFAULT 'draft';
ALTER TABLE batches ADD COLUMN IF NOT EXISTS max_students INT;

-- Backfill a sensible status for batches that already exist, from their
-- current is_active flag + dates, so nothing that's live today looks wrong.
-- Only touches rows still sitting at the new column's default.
UPDATE batches
SET status = (CASE
    WHEN NOT is_active THEN 'archived'
    WHEN start_date > CURRENT_DATE THEN 'upcoming'
    WHEN end_date < CURRENT_DATE THEN 'completed'
    ELSE 'active'
END)::batch_status
WHERE status = 'draft';

-- ============================================================================
-- Student enrollments — one row per (student, batch). A student can hold
-- several of these at once (one per course they're taking), but application
-- code enforces at most one *active* enrollment per course at a time
-- (see EnrollmentRepository.HasActiveEnrollmentInCourse).
-- ============================================================================
CREATE TABLE IF NOT EXISTS student_enrollments (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id              VARCHAR(16) NOT NULL UNIQUE,
    student_id            UUID NOT NULL REFERENCES users(id),
    course_id             UUID NOT NULL REFERENCES courses(id),
    batch_id              UUID NOT NULL REFERENCES batches(id),
    enrollment_date       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    enrollment_type       TEXT NOT NULL DEFAULT 'direct', -- direct | invited | transferred
    status                enrollment_status NOT NULL DEFAULT 'active',
    access_start_date     TIMESTAMPTZ,
    access_end_date       TIMESTAMPTZ,
    is_late_enrollment    BOOLEAN NOT NULL DEFAULT FALSE,
    completion_percentage NUMERIC(5,2),
    final_score           NUMERIC(6,2),
    final_rank            INT,
    created_by            UUID REFERENCES users(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (student_id, batch_id)
);

CREATE INDEX IF NOT EXISTS idx_student_enrollments_student_course
    ON student_enrollments (student_id, course_id, status);
CREATE INDEX IF NOT EXISTS idx_student_enrollments_batch
    ON student_enrollments (batch_id);

-- Backfill: give every existing batch_students membership a matching
-- enrollment record, so reporting/analytics built on student_enrollments
-- isn't missing everyone who joined before this migration ran.
INSERT INTO student_enrollments (
    short_id, student_id, course_id, batch_id, enrollment_date, enrollment_type, status,
    access_start_date, access_end_date, is_late_enrollment, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text || bs.user_id::text || bs.batch_id::text), 1, 8),
    bs.user_id, b.course_id, b.id, bs.joined_at, 'direct', 'active',
    bs.joined_at, b.end_date::TIMESTAMPTZ,
    (bs.joined_at::DATE > b.start_date),
    bs.added_by
FROM batch_students bs
JOIN batches b ON b.id = bs.batch_id
WHERE NOT EXISTS (
    SELECT 1 FROM student_enrollments se WHERE se.student_id = bs.user_id AND se.batch_id = bs.batch_id
);

-- ============================================================================
-- End of file
-- ============================================================================
