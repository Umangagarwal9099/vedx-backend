-- Fix: project_milestones was created without a deleted_at column, but
-- ProjectRepository.GetMilestones/UpdateMilestone/DeleteMilestone have always
-- queried/written pm.deleted_at (mirroring the soft-delete pattern used by
-- the sibling project_teams table, which DOES have the column). This broke
-- every project detail fetch — GET /projects/{short_id} — for every caller,
-- since the query fails at the SQL level regardless of how many milestones
-- exist. Purely additive; safe to run any time.

ALTER TABLE project_milestones ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
