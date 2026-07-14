-- Extends the coding-question bank with the same Subject/Subtopic taxonomy
-- tier used by assessment_questions (the existing free-tagging `topics`
-- array is left as-is for backward compatibility).
ALTER TABLE coding_questions ADD COLUMN IF NOT EXISTS subject VARCHAR(100);
ALTER TABLE coding_questions ADD COLUMN IF NOT EXISTS subtopic VARCHAR(150);

CREATE INDEX IF NOT EXISTS idx_coding_questions_subject ON coding_questions(subject);
