-- ============================================================
-- Migration 003: Add date_of_birth and deleted_at to users
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

ALTER TABLE users ADD COLUMN IF NOT EXISTS date_of_birth DATE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at    TIMESTAMPTZ;

-- Partial index speeds up all active-user lookups
CREATE INDEX IF NOT EXISTS idx_users_active ON users(id) WHERE deleted_at IS NULL;
