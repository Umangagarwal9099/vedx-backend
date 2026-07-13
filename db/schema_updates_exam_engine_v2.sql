-- ============================================================================
-- Exam engine v2: server-side time authority, auto-submit, cancel, reattempts.
-- All additive — safe to run against a database that already has
-- db/schema_updates_projects_assignments_assessments_questionbank.sql applied.
-- Nothing here renames or drops an existing column.
-- ============================================================================

-- Per-attempt deadline (min of assessment end_at vs. started_at + duration).
-- Computed once when the attempt starts; the auto-submit sweep and the
-- frontend timer both read this instead of recomputing it independently.
ALTER TABLE exam_attempts ADD COLUMN IF NOT EXISTS ends_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_exam_attempts_sweep
    ON exam_attempts (ends_at)
    WHERE status = 'in_progress';

-- Backfill ends_at for attempts that are already open so the sweep job
-- doesn't treat every currently in-progress attempt as instantly expired
-- the moment it goes live.
UPDATE exam_attempts ea
SET ends_at = CASE
    WHEN a.end_at IS NOT NULL AND a.duration_minutes IS NOT NULL
        THEN LEAST(a.end_at, ea.started_at + (a.duration_minutes || ' minutes')::interval)
    WHEN a.end_at IS NOT NULL THEN a.end_at
    WHEN a.duration_minutes IS NOT NULL
        THEN ea.started_at + (a.duration_minutes || ' minutes')::interval
    ELSE NULL
END
FROM assessments a
WHERE ea.assessment_id = a.id
  AND ea.status = 'in_progress'
  AND ea.ends_at IS NULL;

-- Cancelling an exam is an assessment-level action (blocks new attempts and
-- cascades to every in-progress attempt for it) rather than a per-attempt one.
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS cancelled_by UUID REFERENCES users(id);

-- One reminder tier for now ("starting soon") — see scheduler/exam_reminders.go.
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS start_reminder_sent BOOLEAN NOT NULL DEFAULT FALSE;

-- New attempt-status values for the cases the original enum didn't cover.
ALTER TYPE exam_attempt_status ADD VALUE IF NOT EXISTS 'cancelled';

-- Reattempt grants — append-only history, never overwrites a past attempt.
CREATE TABLE IF NOT EXISTS exam_reattempt_grants (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id           VARCHAR(16) NOT NULL UNIQUE,
    assessment_id      UUID NOT NULL REFERENCES assessments(id),
    student_id         UUID NOT NULL REFERENCES users(id),
    granted_by         UUID NOT NULL REFERENCES users(id),
    reason             TEXT,
    new_attempt_number INT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exam_reattempt_grants_lookup
    ON exam_reattempt_grants (assessment_id, student_id);

-- ============================================================================
-- End of file
-- ============================================================================
