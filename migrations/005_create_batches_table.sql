-- ============================================================
-- Migration 005: Create batches table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS batches (
  id                    UUID         DEFAULT gen_random_uuid() PRIMARY KEY,
  short_id              VARCHAR(20)  UNIQUE NOT NULL,
  batch_number          VARCHAR(100) UNIQUE NOT NULL,
  course_id             UUID         NOT NULL REFERENCES courses(id),
  batch_manager_id      UUID         NOT NULL REFERENCES users(id),
  additional_manager_id UUID         REFERENCES users(id),
  module                VARCHAR(255),
  start_date            DATE         NOT NULL,
  end_date              DATE         NOT NULL,
  is_active             BOOLEAN      DEFAULT TRUE,
  created_by            UUID         NOT NULL REFERENCES users(id),
  created_at            TIMESTAMPTZ  DEFAULT NOW(),
  updated_at            TIMESTAMPTZ  DEFAULT NOW(),
  deleted_at            TIMESTAMPTZ
);

CREATE OR REPLACE TRIGGER set_updated_at_batches
  BEFORE UPDATE ON batches
  FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE INDEX IF NOT EXISTS idx_batches_short_id   ON batches(short_id);
CREATE INDEX IF NOT EXISTS idx_batches_course      ON batches(course_id);
CREATE INDEX IF NOT EXISTS idx_batches_manager     ON batches(batch_manager_id);
CREATE INDEX IF NOT EXISTS idx_batches_active      ON batches(id) WHERE deleted_at IS NULL;
