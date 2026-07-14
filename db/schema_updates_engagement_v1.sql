-- ============================================================================
-- Phase 5 of the LMS-flow audit: engagement layer — certificates, activity
-- streaks, and at-risk detection.
--
-- Streaks and at-risk detection need NO new tables — they're computed
-- on demand from data that already exists (assignment/project submissions,
-- exam attempts, session attendance, and the Phase 3 score/attendance
-- rollups). Only certificates need a new table, since issuing one is a real
-- event that must be durably recorded. Fully additive.
-- ============================================================================

CREATE TABLE IF NOT EXISTS certificates (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id          VARCHAR(16) NOT NULL UNIQUE,
    certificate_number VARCHAR(32) NOT NULL UNIQUE,
    student_id        UUID NOT NULL REFERENCES users(id),
    batch_id          UUID NOT NULL REFERENCES batches(id),
    course_id         UUID NOT NULL REFERENCES courses(id),
    final_score       NUMERIC(6,2),
    final_rank        INT,
    issued_by         UUID NOT NULL REFERENCES users(id),
    issued_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (student_id, batch_id)
);

CREATE INDEX IF NOT EXISTS idx_certificates_student ON certificates(student_id);
CREATE INDEX IF NOT EXISTS idx_certificates_batch ON certificates(batch_id);

-- ============================================================================
-- End of file
-- ============================================================================
