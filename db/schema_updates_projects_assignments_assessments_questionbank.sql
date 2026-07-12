-- ============================================================================
-- Schema updates for: Assignments, Projects, Assessments (+ Exam Engine),
-- Question Bank.
--
-- No migrations framework exists in this repo (db/db.go just opens a pool),
-- so this file is meant to be run by hand, once, against your Postgres/
-- Supabase instance. It is written to be safe to re-run:
--   - CREATE TYPE is wrapped so it won't error if the type already exists.
--   - CREATE TABLE uses IF NOT EXISTS.
--   - ALTER TABLE ... ADD COLUMN uses IF NOT EXISTS.
-- It assumes these tables already exist from earlier work: users, batches,
-- batch_students, modules, sessions, coding_questions.
-- ============================================================================


-- ============================================================================
-- 1. ASSIGNMENTS
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE assignment_status AS ENUM ('draft', 'active', 'closed');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE assignment_submission_type AS ENUM ('link', 'text', 'file');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE assignment_submission_status AS ENUM ('submitted', 'late', 'evaluated', 'resubmission_required');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS assignments (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id                 VARCHAR(16) NOT NULL UNIQUE,
    title                    TEXT NOT NULL,
    description              TEXT,
    batch_id                 UUID NOT NULL REFERENCES batches(id),
    module_id                UUID REFERENCES modules(id),
    session_id               UUID REFERENCES sessions(id),
    max_marks                INT NOT NULL,
    deadline                 TIMESTAMPTZ NOT NULL,
    allowed_submission_types TEXT[] NOT NULL DEFAULT '{}',
    allowed_file_formats     TEXT[] DEFAULT '{}',
    max_file_size_mb         INT,
    late_submission_allowed  BOOLEAN NOT NULL DEFAULT FALSE,
    late_penalty_percent     INT,
    status                   assignment_status NOT NULL DEFAULT 'draft',
    created_by               UUID NOT NULL REFERENCES users(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at               TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_assignments_batch_id ON assignments(batch_id);
CREATE INDEX IF NOT EXISTS idx_assignments_deleted_at ON assignments(deleted_at);

CREATE TABLE IF NOT EXISTS assignment_submissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id        VARCHAR(16) NOT NULL UNIQUE,
    assignment_id   UUID NOT NULL REFERENCES assignments(id),
    student_id      UUID NOT NULL REFERENCES users(id),
    submission_type assignment_submission_type NOT NULL,
    content         TEXT,
    file_url        TEXT,
    status          assignment_submission_status NOT NULL,
    marks           INT,
    feedback        TEXT,
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    evaluated_at    TIMESTAMPTZ,
    evaluated_by    UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (assignment_id, student_id)
);

CREATE INDEX IF NOT EXISTS idx_assignment_submissions_assignment_id ON assignment_submissions(assignment_id);


-- ============================================================================
-- 2. PROJECTS (+ milestones, teams, submissions)
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE project_category AS ENUM ('individual', 'group', 'module', 'capstone', 'internship', 'final_course');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE project_status AS ENUM ('draft', 'active', 'closed');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE project_submission_type AS ENUM ('link', 'text', 'file');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE project_submission_status AS ENUM ('submitted', 'late', 'evaluated', 'resubmission_required');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS projects (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id                 VARCHAR(16) NOT NULL UNIQUE,
    title                    TEXT NOT NULL,
    problem_statement        TEXT,
    requirements             TEXT,
    expected_deliverables    TEXT,
    evaluation_criteria      TEXT,
    reference_files          TEXT[] DEFAULT '{}',
    category                 project_category NOT NULL,
    is_team_project          BOOLEAN NOT NULL DEFAULT FALSE,
    batch_id                 UUID NOT NULL REFERENCES batches(id),
    module_id                UUID REFERENCES modules(id),
    max_marks                INT NOT NULL,
    start_date               DATE,
    final_deadline           TIMESTAMPTZ NOT NULL,
    allowed_submission_types TEXT[] NOT NULL DEFAULT '{}',
    status                   project_status NOT NULL DEFAULT 'draft',
    created_by               UUID NOT NULL REFERENCES users(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at               TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_projects_batch_id ON projects(batch_id);
CREATE INDEX IF NOT EXISTS idx_projects_deleted_at ON projects(deleted_at);

CREATE TABLE IF NOT EXISTS project_milestones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id    VARCHAR(16) NOT NULL UNIQUE,
    project_id  UUID NOT NULL REFERENCES projects(id),
    title       TEXT NOT NULL,
    description TEXT,
    due_date    TIMESTAMPTZ,
    order_index INT NOT NULL DEFAULT 0,
    is_final    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_project_milestones_project_id ON project_milestones(project_id);

CREATE TABLE IF NOT EXISTS project_teams (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id   VARCHAR(16) NOT NULL UNIQUE,
    project_id UUID NOT NULL REFERENCES projects(id),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_project_teams_project_id ON project_teams(project_id);

CREATE TABLE IF NOT EXISTS project_team_members (
    team_id   UUID NOT NULL REFERENCES project_teams(id),
    user_id   UUID NOT NULL REFERENCES users(id),
    added_by  UUID REFERENCES users(id),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE IF NOT EXISTS project_submissions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       VARCHAR(16) NOT NULL UNIQUE,
    milestone_id   UUID NOT NULL REFERENCES project_milestones(id),
    student_id     UUID REFERENCES users(id),
    team_id        UUID REFERENCES project_teams(id),
    submission_type project_submission_type NOT NULL,
    content        TEXT,
    file_url       TEXT,
    status         project_submission_status NOT NULL,
    marks          INT,
    feedback       TEXT,
    submitted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    evaluated_at   TIMESTAMPTZ,
    evaluated_by   UUID REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_submission_owner CHECK (
        (student_id IS NOT NULL AND team_id IS NULL) OR
        (student_id IS NULL AND team_id IS NOT NULL)
    )
);

-- Two partial unique indexes stand in for one UNIQUE(milestone_id, student_id/team_id)
-- because exactly one of the two columns is always NULL (see CHECK above), and
-- Postgres treats NULL as distinct in a plain UNIQUE constraint.
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_submissions_student
    ON project_submissions(milestone_id, student_id) WHERE team_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_submissions_team
    ON project_submissions(milestone_id, team_id) WHERE student_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_project_submissions_milestone_id ON project_submissions(milestone_id);


-- ============================================================================
-- 3. QUESTION BANK
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE assessment_question_type AS ENUM
        ('mcq', 'multi_select', 'true_false', 'fill_blank', 'short_answer', 'descriptive', 'coding');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE question_visibility AS ENUM ('private', 'course', 'global');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- NOTE: table name is "assessment_questions" (it's the question bank — the name
-- reflects that questions are attached to assessments, not that they belong to
-- just one).
CREATE TABLE IF NOT EXISTS assessment_questions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id            VARCHAR(16) NOT NULL UNIQUE,
    question_type       assessment_question_type NOT NULL,
    question_text       TEXT,
    options             JSONB NOT NULL DEFAULT '[]',       -- [{ "id": "a", "text": "..." }, ...]
    correct_option_ids  TEXT[] DEFAULT '{}',
    correct_text        TEXT,
    explanation         TEXT,
    marks               INT NOT NULL DEFAULT 1,
    negative_marks      INT NOT NULL DEFAULT 0,
    topic               TEXT,
    difficulty          TEXT,                               -- easy | medium | hard
    coding_question_id  UUID REFERENCES coding_questions(id),
    visibility          question_visibility NOT NULL DEFAULT 'private',
    created_by          UUID NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_assessment_questions_created_by ON assessment_questions(created_by);
CREATE INDEX IF NOT EXISTS idx_assessment_questions_deleted_at ON assessment_questions(deleted_at);


-- ============================================================================
-- 4. ASSESSMENTS — extend the existing table into a real exam engine
-- ============================================================================
-- These assume the "assessments" table already exists from earlier work with
-- columns: id, short_id, name, description, thumbnail, file_url,
-- general_instructions, total_marks, passing_percentage, result_declaration,
-- result_display, allow_attempts_after_passing, is_active, created_by,
-- created_at, updated_at, deleted_at.
-- If that table does NOT exist yet in your database, run the CREATE TABLE
-- further below instead of the ALTER block.

ALTER TABLE assessments ADD COLUMN IF NOT EXISTS batch_id             UUID REFERENCES batches(id);
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS start_at             TIMESTAMPTZ;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS end_at               TIMESTAMPTZ;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS duration_minutes     INT;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS max_attempts         INT NOT NULL DEFAULT 1;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS negative_marking     BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS randomize_questions  BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS randomize_options    BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS auto_submit          BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS show_correct_answers BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS requires_proctoring  BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_assessments_batch_id ON assessments(batch_id);

-- Fallback full CREATE TABLE — only use this if "assessments" does not already
-- exist in your database (skip if the ALTER block above ran without error).
--
-- CREATE TABLE IF NOT EXISTS assessments (
--     id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
--     short_id                    VARCHAR(16) NOT NULL UNIQUE,
--     name                        TEXT NOT NULL,
--     description                 TEXT,
--     thumbnail                   TEXT,
--     file_url                    TEXT,
--     general_instructions        TEXT,
--     total_marks                 INT NOT NULL,
--     passing_percentage          NUMERIC(5,2) NOT NULL,
--     result_declaration          TEXT NOT NULL,   -- manual | automatic
--     result_display              TEXT NOT NULL,   -- marks_and_status | status_only
--     allow_attempts_after_passing BOOLEAN NOT NULL DEFAULT FALSE,
--     batch_id                    UUID REFERENCES batches(id),
--     start_at                    TIMESTAMPTZ,
--     end_at                      TIMESTAMPTZ,
--     duration_minutes            INT,
--     max_attempts                INT NOT NULL DEFAULT 1,
--     negative_marking            BOOLEAN NOT NULL DEFAULT FALSE,
--     randomize_questions         BOOLEAN NOT NULL DEFAULT FALSE,
--     randomize_options           BOOLEAN NOT NULL DEFAULT FALSE,
--     auto_submit                 BOOLEAN NOT NULL DEFAULT TRUE,
--     show_correct_answers        BOOLEAN NOT NULL DEFAULT FALSE,
--     requires_proctoring         BOOLEAN NOT NULL DEFAULT FALSE,
--     is_active                   BOOLEAN NOT NULL DEFAULT TRUE,
--     created_by                  UUID NOT NULL REFERENCES users(id),
--     created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
--     updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
--     deleted_at                  TIMESTAMPTZ
-- );

-- Links a question-bank question to an assessment, with per-assessment
-- ordering and an optional marks override.
CREATE TABLE IF NOT EXISTS assessment_question_links (
    assessment_id  UUID NOT NULL REFERENCES assessments(id),
    question_id    UUID NOT NULL REFERENCES assessment_questions(id),
    order_index    INT NOT NULL DEFAULT 0,
    marks_override INT,
    PRIMARY KEY (assessment_id, question_id)
);

CREATE INDEX IF NOT EXISTS idx_assessment_question_links_assessment_id ON assessment_question_links(assessment_id);


-- ============================================================================
-- 5. EXAM ATTEMPTS (student attempts + answers)
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE exam_attempt_status AS ENUM ('in_progress', 'submitted', 'evaluated');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS exam_attempts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       VARCHAR(16) NOT NULL UNIQUE,
    assessment_id  UUID NOT NULL REFERENCES assessments(id),
    student_id     UUID NOT NULL REFERENCES users(id),
    attempt_number INT NOT NULL,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_at   TIMESTAMPTZ,
    auto_submitted BOOLEAN NOT NULL DEFAULT FALSE,
    status         exam_attempt_status NOT NULL DEFAULT 'in_progress',
    total_score    INT,
    max_score      INT NOT NULL,
    passed         BOOLEAN,
    question_order TEXT[] NOT NULL DEFAULT '{}',   -- ordered list of question short_ids
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (assessment_id, student_id, attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_exam_attempts_assessment_id ON exam_attempts(assessment_id);
CREATE INDEX IF NOT EXISTS idx_exam_attempts_student_id ON exam_attempts(student_id);

CREATE TABLE IF NOT EXISTS exam_answers (
    attempt_id          UUID NOT NULL REFERENCES exam_attempts(id),
    question_id         UUID NOT NULL REFERENCES assessment_questions(id),
    selected_option_ids TEXT[] DEFAULT '{}',
    text_answer         TEXT,
    is_correct          BOOLEAN,
    marks_awarded       INT,
    feedback            TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (attempt_id, question_id)
);

-- ============================================================================
-- End of file
-- ============================================================================
