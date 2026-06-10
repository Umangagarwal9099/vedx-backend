-- ============================================================
-- Migration 008 — Announcements table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

DO $$ BEGIN
  CREATE TYPE announcement_urgency AS ENUM ('low', 'medium', 'high');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE announcement_visibility AS ENUM ('existing_only', 'existing_and_new');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS announcements (
  id          UUID                      DEFAULT gen_random_uuid() PRIMARY KEY,
  short_id    VARCHAR(8)                UNIQUE NOT NULL,
  name        VARCHAR(255)              NOT NULL,
  description TEXT,
  image_url   TEXT,
  urgency     announcement_urgency      NOT NULL DEFAULT 'low',
  visibility  announcement_visibility   NOT NULL DEFAULT 'existing_only',
  is_active   BOOLEAN                   NOT NULL DEFAULT TRUE,
  created_by  UUID                      NOT NULL REFERENCES users(id),
  created_at  TIMESTAMPTZ               DEFAULT NOW(),
  updated_at  TIMESTAMPTZ               DEFAULT NOW(),
  deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_announcements_short_id  ON announcements (short_id);
CREATE INDEX IF NOT EXISTS idx_announcements_urgency   ON announcements (urgency);
CREATE INDEX IF NOT EXISTS idx_announcements_is_active ON announcements (is_active);
CREATE INDEX IF NOT EXISTS idx_announcements_deleted   ON announcements (deleted_at);
