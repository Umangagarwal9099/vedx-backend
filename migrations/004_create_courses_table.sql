-- ============================================================
-- Migration 004: Create courses table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS courses (
  id          UUID         DEFAULT gen_random_uuid() PRIMARY KEY,
  short_id    VARCHAR(20)  UNIQUE NOT NULL,
  name        VARCHAR(255) NOT NULL,
  description TEXT,
  thumbnail   TEXT,
  is_active   BOOLEAN      DEFAULT TRUE,
  created_by  UUID         NOT NULL REFERENCES users(id),
  created_at  TIMESTAMPTZ  DEFAULT NOW(),
  updated_at  TIMESTAMPTZ  DEFAULT NOW(),
  deleted_at  TIMESTAMPTZ
);

CREATE OR REPLACE TRIGGER set_updated_at_courses
  BEFORE UPDATE ON courses
  FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE INDEX IF NOT EXISTS idx_courses_short_id ON courses(short_id);
CREATE INDEX IF NOT EXISTS idx_courses_active   ON courses(id) WHERE deleted_at IS NULL;
