-- ============================================================
-- Migration 014 — Course ↔ Module junction table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS course_modules (
  id          UUID DEFAULT gen_random_uuid() PRIMARY KEY,
  course_id   UUID NOT NULL REFERENCES courses(id)  ON DELETE CASCADE,
  module_id   UUID NOT NULL REFERENCES modules(id)  ON DELETE CASCADE,
  order_index INT  NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE (course_id, module_id)
);

CREATE INDEX IF NOT EXISTS idx_course_modules_course  ON course_modules (course_id);
CREATE INDEX IF NOT EXISTS idx_course_modules_module  ON course_modules (module_id);
