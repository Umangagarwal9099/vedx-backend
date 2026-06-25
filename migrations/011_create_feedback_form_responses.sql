-- ============================================================
-- Migration 011 — Feedback Form Responses table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS feedback_form_responses (
  id          UUID        DEFAULT gen_random_uuid() PRIMARY KEY,
  short_id    VARCHAR(20) NOT NULL UNIQUE,
  form_id     UUID        NOT NULL REFERENCES feedback_forms(id) ON DELETE CASCADE,
  submitted_by UUID       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  submitted_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS feedback_form_answers (
  id           UUID        DEFAULT gen_random_uuid() PRIMARY KEY,
  response_id  UUID        NOT NULL REFERENCES feedback_form_responses(id) ON DELETE CASCADE,
  question_id  UUID        NOT NULL REFERENCES feedback_form_questions(id) ON DELETE CASCADE,
  answer_text  TEXT,
  answer_number NUMERIC,
  answer_array JSONB
);

CREATE INDEX IF NOT EXISTS idx_ffr_form        ON feedback_form_responses(form_id);
CREATE INDEX IF NOT EXISTS idx_ffr_user        ON feedback_form_responses(submitted_by);
CREATE INDEX IF NOT EXISTS idx_ffa_response    ON feedback_form_answers(response_id);
CREATE INDEX IF NOT EXISTS idx_ffa_question    ON feedback_form_answers(question_id);
