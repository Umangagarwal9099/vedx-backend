-- Deadline reminder flags for assignments and projects, mirroring the
-- reminder_sent pattern already used on sessions/batches/assessments.
ALTER TABLE assignments ADD COLUMN IF NOT EXISTS deadline_reminder_sent BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deadline_reminder_sent BOOLEAN NOT NULL DEFAULT FALSE;
