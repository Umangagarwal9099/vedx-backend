-- Question bank taxonomy: subject (e.g. "Python") and subtopic (e.g. "Variables")
-- around the existing free-text `topic` column (e.g. "Python Basics"), giving a
-- 3-tier Subject -> Topic -> Subtopic hierarchy for browsing/filtering the bank.
ALTER TABLE assessment_questions ADD COLUMN IF NOT EXISTS subject VARCHAR(100);
ALTER TABLE assessment_questions ADD COLUMN IF NOT EXISTS subtopic VARCHAR(150);

CREATE INDEX IF NOT EXISTS idx_assessment_questions_subject ON assessment_questions(subject);
