-- ============================================================
-- Migration 010 — Code Submissions table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

DO $$ BEGIN
  CREATE TYPE submission_status AS ENUM ('accepted', 'wrong_answer', 'runtime_error', 'compile_error');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS submissions (
  id                UUID                DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id           UUID                NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question_short_id VARCHAR(20)         NOT NULL REFERENCES coding_questions(short_id) ON DELETE CASCADE,
  language          VARCHAR(20)         NOT NULL,
  code              TEXT                NOT NULL,
  status            submission_status   NOT NULL,
  passed_tests      SMALLINT            NOT NULL DEFAULT 0,
  total_tests       SMALLINT            NOT NULL DEFAULT 0,
  created_at        TIMESTAMPTZ         DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_submissions_user        ON submissions(user_id);
CREATE INDEX IF NOT EXISTS idx_submissions_question    ON submissions(question_short_id);
CREATE INDEX IF NOT EXISTS idx_submissions_user_q      ON submissions(user_id, question_short_id);
CREATE INDEX IF NOT EXISTS idx_submissions_status      ON submissions(status);
