-- ============================================================
-- Migration 013 — Certificates table
-- Run in: Supabase Dashboard → SQL Editor
-- ============================================================

CREATE TABLE IF NOT EXISTS certificates (
  id               UUID         DEFAULT gen_random_uuid() PRIMARY KEY,
  certificate_id   VARCHAR(60)  UNIQUE NOT NULL,   -- e.g. VDX-2026-IC-SF9E7Z
  recipient_name   VARCHAR(255) NOT NULL,
  program_name     VARCHAR(255) NOT NULL,
  certificate_type VARCHAR(50)  NOT NULL,          -- internship_completion | course_completion | appreciation | project_completion
  start_date       VARCHAR(50),                    -- "May 2026" (display string)
  end_date         VARCHAR(50),                    -- "June 2026"
  issued_on        DATE         NOT NULL,
  is_valid         BOOLEAN      NOT NULL DEFAULT TRUE,
  issued_by        UUID         REFERENCES users(id) ON DELETE SET NULL,
  created_at       TIMESTAMPTZ  DEFAULT NOW(),
  updated_at       TIMESTAMPTZ  DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_certificates_certificate_id ON certificates (certificate_id);
CREATE INDEX IF NOT EXISTS idx_certificates_is_valid       ON certificates (is_valid);

CREATE OR REPLACE TRIGGER trg_certificates_updated_at
  BEFORE UPDATE ON certificates
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
