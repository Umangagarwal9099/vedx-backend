-- ============================================================================
-- Phase 4 of the LMS-flow audit: module release scheduling (drip content per
-- batch) and per-session recording visibility. Fully additive.
-- ============================================================================

-- A batch can schedule when each of its course's modules becomes visible to
-- its students. No row for a module means "released from day one" (today's
-- behavior — courses/modules are unaffected unless a batch explicitly
-- schedules something).
CREATE TABLE IF NOT EXISTS batch_module_schedule (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id     UUID NOT NULL REFERENCES batches(id),
    module_id    UUID NOT NULL REFERENCES modules(id),
    release_date DATE NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (batch_id, module_id)
);

CREATE INDEX IF NOT EXISTS idx_batch_module_schedule_batch ON batch_module_schedule(batch_id);

-- Lets a mentor hide a specific recording from students (still processing,
-- embargoed until grading closes, etc) independent of the existing fees_paid
-- gate, and/or delay its release to a specific timestamp. Staff always see
-- every recording regardless of these two fields.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS recording_visible        BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS recording_available_from TIMESTAMPTZ;

-- ============================================================================
-- End of file
-- ============================================================================
