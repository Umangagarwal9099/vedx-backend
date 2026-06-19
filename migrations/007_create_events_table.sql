-- ============================================================
-- Migration 007 — Events table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

DO $$ BEGIN
  CREATE TYPE event_status AS ENUM ('published', 'unpublished');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE event_mode AS ENUM ('virtual', 'in_person');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS events (
  id              UUID          DEFAULT gen_random_uuid() PRIMARY KEY,
  short_id        VARCHAR(8)    UNIQUE NOT NULL,
  name            VARCHAR(255)  NOT NULL,
  event_date      DATE          NOT NULL,
  start_time      TIME          NOT NULL,
  end_time        TIME          NOT NULL,
  image_url       TEXT,
  description     TEXT,
  status          event_status  NOT NULL DEFAULT 'unpublished',
  mode            event_mode    NOT NULL DEFAULT 'virtual',
  guest_access    BOOLEAN       NOT NULL DEFAULT FALSE,
  event_manager   UUID          REFERENCES users(id) ON DELETE SET NULL,
  categories      TEXT[]        NOT NULL DEFAULT '{}',
  is_active       BOOLEAN       NOT NULL DEFAULT TRUE,
  created_by      UUID          NOT NULL REFERENCES users(id),
  created_at      TIMESTAMPTZ   DEFAULT NOW(),
  updated_at      TIMESTAMPTZ   DEFAULT NOW(),
  deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_events_short_id   ON events (short_id);
CREATE INDEX IF NOT EXISTS idx_events_status     ON events (status);
CREATE INDEX IF NOT EXISTS idx_events_name       ON events USING gin (to_tsvector('english', name));
CREATE INDEX IF NOT EXISTS idx_events_deleted_at ON events (deleted_at);
