-- ============================================================
-- Migration 015 — Add overview, objectives, requirements to courses
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

ALTER TABLE courses
  ADD COLUMN IF NOT EXISTS overview     TEXT,
  ADD COLUMN IF NOT EXISTS objectives   TEXT[] DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS requirements TEXT[] DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS instructor   VARCHAR(255),
  ADD COLUMN IF NOT EXISTS duration     VARCHAR(100),
  ADD COLUMN IF NOT EXISTS level        VARCHAR(50) DEFAULT 'intermediate',
  ADD COLUMN IF NOT EXISTS category     VARCHAR(100);
