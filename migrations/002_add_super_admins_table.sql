-- ============================================================
-- Migration 002: Add super_admins profile table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS super_admins (
  id           UUID       DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id      UUID       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  admin_level  SMALLINT   DEFAULT 1 CHECK (admin_level BETWEEN 1 AND 3),
  department   VARCHAR(100),
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE OR REPLACE TRIGGER set_updated_at_super_admins
  BEFORE UPDATE ON super_admins
  FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE INDEX IF NOT EXISTS idx_super_admins_user ON super_admins(user_id);
