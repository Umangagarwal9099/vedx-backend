-- ============================================================================
-- Team hierarchy: which Operations employee reports to which Team Lead.
-- Fully additive — no existing table is touched. Run this once against the
-- database; idempotent (IF NOT EXISTS) so re-running is safe.
--
-- A member belongs to exactly one Team Lead at a time — reassigning someone
-- to a different Team Lead is an upsert on member_id, not a new row.
-- ============================================================================

CREATE TABLE IF NOT EXISTS team_memberships (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id     VARCHAR(16) NOT NULL UNIQUE,
    team_lead_id UUID NOT NULL REFERENCES users(id),
    member_id    UUID NOT NULL UNIQUE REFERENCES users(id),
    assigned_by  UUID NOT NULL REFERENCES users(id),
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_team_memberships_lead ON team_memberships (team_lead_id);

-- ============================================================================
-- End of file
-- ============================================================================
