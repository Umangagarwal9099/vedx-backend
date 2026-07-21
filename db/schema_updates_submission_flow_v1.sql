-- ============================================================================
-- Student submission flow hardening: exam/assignment/project result
-- publication gating, and exam violation tracking. Fully additive.
--
-- Design notes:
--   - result_published_at is nullable on exam_attempts / assignment_submissions
--     / project_submissions. Grading (FinalizeAttempt / GradeSubmission) never
--     sets it — a separate bulk "publish results" action does, per assessment
--     / assignment / milestone. Until published, the student-facing API
--     redacts marks/grade/feedback and reports "evaluation_pending" instead.
--   - Exam violation tracking (tab-switch, fullscreen-exit, etc.) is new:
--     exam_attempt_violations, plus a small set of configurable thresholds on
--     assessments (defaulting to the spec's "recommended strict mode": first
--     violation warns, second auto-submits).
-- ============================================================================

ALTER TABLE exam_attempts         ADD COLUMN IF NOT EXISTS result_published_at TIMESTAMPTZ;
ALTER TABLE assignment_submissions ADD COLUMN IF NOT EXISTS result_published_at TIMESTAMPTZ;
ALTER TABLE project_submissions    ADD COLUMN IF NOT EXISTS result_published_at TIMESTAMPTZ;

ALTER TABLE assessments ADD COLUMN IF NOT EXISTS result_published_at TIMESTAMPTZ;

-- Exam security / violation configuration — additive columns on assessments,
-- all with safe defaults matching the spec's recommended strict mode.
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS close_on_tab_switch       BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS close_on_window_blur      BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS close_on_fullscreen_exit  BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS allowed_warning_count     INTEGER NOT NULL DEFAULT 1;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS auto_submit_on_violation  BOOLEAN NOT NULL DEFAULT TRUE;

DO $$ BEGIN
    CREATE TYPE exam_violation_type AS ENUM (
        'tab_switched', 'window_blurred', 'fullscreen_exited', 'page_refreshed',
        'browser_back_attempt', 'multiple_tab_attempt', 'multiple_device_attempt',
        'network_disconnected', 'exam_window_closed'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS exam_attempt_violations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       VARCHAR(16) NOT NULL UNIQUE,
    attempt_id     UUID NOT NULL REFERENCES exam_attempts(id),
    student_id     UUID NOT NULL REFERENCES users(id),
    violation_type exam_violation_type NOT NULL,
    violation_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    warning_number INTEGER NOT NULL,
    browser_info   TEXT,
    action_taken   VARCHAR(20) NOT NULL DEFAULT 'warned' CHECK (action_taken IN ('warned', 'auto_submitted')),
    metadata       JSONB NOT NULL DEFAULT '{}'::JSONB
);

CREATE INDEX IF NOT EXISTS idx_exam_attempt_violations_attempt ON exam_attempt_violations(attempt_id);

-- submission_type on exam_attempts distinguishes how an attempt ended —
-- needed to tell "auto_submitted (timer)" apart from "auto_submitted
-- (violation)" apart from "manual", per the spec's submission_type list.
ALTER TABLE exam_attempts ADD COLUMN IF NOT EXISTS submission_type VARCHAR(20);
