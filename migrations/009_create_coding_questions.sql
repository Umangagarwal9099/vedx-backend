-- ============================================================
-- Migration 009 — Coding Questions table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS coding_questions (
  id           UUID          DEFAULT gen_random_uuid() PRIMARY KEY,
  short_id     VARCHAR(20)   UNIQUE NOT NULL,
  title        VARCHAR(255)  NOT NULL,
  description  TEXT          NOT NULL,
  difficulty   VARCHAR(10)   NOT NULL CHECK (difficulty IN ('Easy', 'Medium', 'Hard')),
  topics       TEXT[]        NOT NULL DEFAULT '{}',
  languages    TEXT[]        NOT NULL DEFAULT '{}',
  constraints  TEXT[]        NOT NULL DEFAULT '{}',
  examples     JSONB         NOT NULL DEFAULT '[]',
  starter_code JSONB         NOT NULL DEFAULT '{}',
  test_cases   JSONB         NOT NULL DEFAULT '[]',
  is_active    BOOLEAN       NOT NULL DEFAULT TRUE,
  created_by   UUID          NOT NULL REFERENCES users(id),
  created_at   TIMESTAMPTZ   DEFAULT NOW(),
  updated_at   TIMESTAMPTZ   DEFAULT NOW(),
  deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_coding_questions_short_id   ON coding_questions (short_id);
CREATE INDEX IF NOT EXISTS idx_coding_questions_difficulty ON coding_questions (difficulty);
CREATE INDEX IF NOT EXISTS idx_coding_questions_is_active  ON coding_questions (is_active);
CREATE INDEX IF NOT EXISTS idx_coding_questions_deleted_at ON coding_questions (deleted_at);

-- Auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
  CREATE TRIGGER trg_coding_questions_updated_at
    BEFORE UPDATE ON coding_questions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;
