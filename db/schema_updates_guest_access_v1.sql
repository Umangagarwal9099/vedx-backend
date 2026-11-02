-- Guest access: lets a student who is already enrolled in one batch also
-- view another batch's live sessions and recordings, without a second full
-- enrollment — no duplicate assignments, progress, certificate, or fee
-- requirement. Deliberately a separate table from batch_students/
-- student_enrollments, which stay reserved for real enrollment.
CREATE TABLE IF NOT EXISTS batch_guest_access (
    batch_id   UUID NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (batch_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_batch_guest_access_user ON batch_guest_access(user_id);
