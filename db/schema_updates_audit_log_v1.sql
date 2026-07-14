-- ============================================================================
-- Audit log: an append-only record of who created/updated/deleted/graded/
-- issued what, across the academic subsystems built this project (batches,
-- sessions, attendance, assignments, projects, assessments, exam attempts,
-- resources, certificates, module schedules, score weights, enrollment).
-- Fully additive — one new table, nothing else touched.
-- ============================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id        VARCHAR(16) NOT NULL UNIQUE,
    actor_id        UUID NOT NULL REFERENCES users(id),
    action          TEXT NOT NULL,        -- create | update | delete | grade | issue | revoke | transfer | schedule | unschedule | cancel | grant_reattempt | enroll | remove
    entity_type     TEXT NOT NULL,        -- batch | session | attendance | assignment | assignment_submission | project | project_submission | assessment | exam_attempt | exam_answer | resource | certificate | module_schedule | score_weights | enrollment | fees
    entity_id       UUID,
    entity_short_id TEXT,
    entity_label    TEXT,                 -- human-readable snapshot (e.g. assignment title) so the row stays meaningful if the entity is later renamed
    batch_id        UUID REFERENCES batches(id),
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_batch_id ON audit_logs(batch_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

-- ============================================================================
-- End of file
-- ============================================================================
