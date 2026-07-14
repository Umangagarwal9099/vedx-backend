-- ============================================================================
-- Sample questions covering every question_type the exam engine supports:
-- mcq, multi_select, true_false, fill_blank, short_answer, descriptive, coding.
--
-- Requires db/schema_updates_projects_assignments_assessments_questionbank.sql
-- to have already been run (creates assessment_questions + its enums).
--
-- created_by is resolved to your first super_admin at insert time — there's no
-- hardcoded user ID to worry about. Same idea for the coding question: it
-- links to whatever coding question happens to exist first in your
-- coding_questions table (adjust the WHERE title = '...' below if you want a
-- specific one instead).
-- ============================================================================

-- 1. MCQ — single correct answer
INSERT INTO assessment_questions (
    short_id, question_type, question_text, options, correct_option_ids,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'mcq', 'Which keyword is used to declare a constant in Go?',
    '[{"id":"a","text":"var"},{"id":"b","text":"const"},{"id":"c","text":"let"},{"id":"d","text":"define"}]'::jsonb,
    ARRAY['b'],
    5, 1, 'Golang Basics', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 2. MCQ — another example
INSERT INTO assessment_questions (
    short_id, question_type, question_text, options, correct_option_ids,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'mcq', 'What is the time complexity of binary search on a sorted array of n elements?',
    '[{"id":"a","text":"O(n)"},{"id":"b","text":"O(n log n)"},{"id":"c","text":"O(log n)"},{"id":"d","text":"O(1)"}]'::jsonb,
    ARRAY['c'],
    5, 1, 'Data Structures & Algorithms', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 3. Multi-select — more than one correct option
INSERT INTO assessment_questions (
    short_id, question_type, question_text, options, correct_option_ids,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'multi_select', 'Which of the following are valid Go built-in data types? (select all that apply)',
    '[{"id":"a","text":"int"},{"id":"b","text":"string"},{"id":"c","text":"float64"},{"id":"d","text":"char"}]'::jsonb,
    ARRAY['a','b','c'],
    10, 2, 'Golang Basics', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 4. Multi-select — another example
INSERT INTO assessment_questions (
    short_id, question_type, question_text, options, correct_option_ids,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'multi_select', 'Which of these are NoSQL databases? (select all that apply)',
    '[{"id":"a","text":"MongoDB"},{"id":"b","text":"PostgreSQL"},{"id":"c","text":"Redis"},{"id":"d","text":"MySQL"}]'::jsonb,
    ARRAY['a','c'],
    10, 0, 'Databases', 'easy', 'course',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 5. True / False
INSERT INTO assessment_questions (
    short_id, question_type, question_text, options, correct_option_ids,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'true_false', 'Go supports multiple inheritance through classes.',
    '[{"id":"true","text":"True"},{"id":"false","text":"False"}]'::jsonb,
    ARRAY['false'],
    5, 0, 'Golang Basics', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 6. Fill in the blank
INSERT INTO assessment_questions (
    short_id, question_type, question_text, correct_text,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'fill_blank', 'The Go keyword used to launch a goroutine is _____.',
    'go',
    5, 0, 'Golang Concurrency', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 7. Short answer (manually graded — correct_text is a reference answer for the grader)
INSERT INTO assessment_questions (
    short_id, question_type, question_text, correct_text,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'short_answer', 'In one sentence, what problem does a mutex solve?',
    'It prevents multiple goroutines from accessing shared data at the same time, avoiding race conditions.',
    10, 0, 'Golang Concurrency', 'medium', 'course',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 8. Descriptive / essay (manually graded)
INSERT INTO assessment_questions (
    short_id, question_type, question_text, correct_text,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'descriptive', 'Explain the difference between a slice and an array in Go, and when you would use each.',
    'Reference answer: arrays have a fixed length that is part of their type; slices are a flexible, growable view over an underlying array and are used far more often in idiomatic Go.',
    15, 0, 'Golang Basics', 'medium', 'course',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- 9. Coding — links to an existing row in the separate coding_questions table
-- instead of duplicating a problem statement. Adjust the WHERE clause to pick
-- a specific problem if you have more than one and want a particular one.
INSERT INTO assessment_questions (
    short_id, question_type, coding_question_id,
    marks, negative_marks, topic, difficulty, visibility, created_by
)
SELECT
    substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'coding',
    (SELECT id FROM coding_questions WHERE deleted_at IS NULL ORDER BY created_at LIMIT 1),
    20, 0, 'Coding', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1)
WHERE EXISTS (SELECT 1 FROM coding_questions WHERE deleted_at IS NULL);
-- ^ the WHERE EXISTS guard skips this insert entirely (rather than inserting a
-- broken NULL link) if you don't have any coding questions yet — create one
-- in Question Bank → Coding first, then re-run just this block.

-- ============================================================================
-- Sanity check — should show 9 rows (or 8 if you had no coding question yet),
-- one per question_type except mcq/multi_select which have 2 each.
-- ============================================================================
-- SELECT question_type, question_text, marks, visibility FROM assessment_questions ORDER BY created_at DESC LIMIT 10;
