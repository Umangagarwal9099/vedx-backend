-- ============================================================
-- Migration 006: Restore manager columns as UUID FKs
-- If you already ran the previous version of this migration
-- (which added name columns), run this to fix it.
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

-- Drop name columns if they were added
ALTER TABLE batches
  DROP COLUMN IF EXISTS batch_manager_name,
  DROP COLUMN IF EXISTS additional_manager_name;

-- Add back proper FK columns
ALTER TABLE batches
  ADD COLUMN IF NOT EXISTS batch_manager_id      UUID NOT NULL REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS additional_manager_id UUID REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_batches_manager ON batches(batch_manager_id);
