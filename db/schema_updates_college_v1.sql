-- ============================================================================
-- College Feature Configuration — multi-tenancy foundation.
--
-- Design notes (read before extending):
--   - `college_id` is added as NULLABLE on users/batches/courses/leads, and
--     every existing row is backfilled onto one auto-created "Default
--     College" (code DEFAULT) with every feature enabled. This is the
--     additive-migration safety net: no existing insert/query path breaks,
--     since college_id is optional everywhere and NULL is treated as
--     "ungated / full access" by the RequireFeature middleware.
--   - Only users/batches/courses/leads carry college_id directly. Everything
--     that already hangs off a batch (sessions, projects, assignments,
--     assessments, attendance, communities, certificates...) is scoped
--     TRANSITIVELY through the batch's college — it does not need its own
--     college_id column. This keeps the migration's blast radius to 4
--     tables instead of 20+.
--   - super_admin is the one role that is never college-scoped (manages
--     every college) — RequireFeature bypasses the check entirely for that
--     role, regardless of college_id.
--   - enabled_features is a JSONB map of feature-key -> bool rather than a
--     separate table, since the feature list is small (~25 keys) and never
--     needs relational joins/filtering of its own.
-- ============================================================================

CREATE TABLE IF NOT EXISTS colleges (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id                VARCHAR(16) NOT NULL UNIQUE,
    name                    VARCHAR(200) NOT NULL,
    code                    VARCHAR(50) NOT NULL UNIQUE,
    logo_url                TEXT,
    contact_person          VARCHAR(200),
    contact_email           VARCHAR(200),
    contact_phone           VARCHAR(20),
    address                 TEXT,
    subscription_start_date DATE,
    subscription_end_date   DATE,
    max_students            INTEGER,
    max_employees           INTEGER,
    status                  VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'expired')),
    enabled_features        JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_by              UUID REFERENCES users(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

-- One default college so every pre-existing row has somewhere to land.
-- Every feature enabled, since this college represents "everything that
-- already existed before multi-tenancy" — nothing should disappear from
-- current users' sidebars because of this migration.
INSERT INTO colleges (short_id, name, code, status, enabled_features)
VALUES (
    'DEFAULTCOL', 'Default College', 'DEFAULT', 'active',
    '{
        "dashboard": true, "courses": true, "modules": true, "resources": true,
        "recorded_videos": true, "live_sessions": true, "assignments": true,
        "exams": true, "projects": true,
        "coding": true, "coding_practice": true, "coding_assessments": true,
        "coding_question_bank": true, "coding_leaderboard": true,
        "community": true, "messages": true, "notifications": true, "announcements": true,
        "internship": true, "job_assistance": true, "placements": true, "employer_management": true,
        "crm": true, "employee_portal": true, "attendance": true, "leave": true, "reports": true,
        "analytics": true, "student_analytics": true, "mentor_analytics": true,
        "college_analytics": true, "coding_analytics": true
    }'::JSONB
)
ON CONFLICT (code) DO NOTHING;

ALTER TABLE users   ADD COLUMN IF NOT EXISTS college_id UUID REFERENCES colleges(id);
ALTER TABLE batches ADD COLUMN IF NOT EXISTS college_id UUID REFERENCES colleges(id);
ALTER TABLE courses ADD COLUMN IF NOT EXISTS college_id UUID REFERENCES colleges(id);
ALTER TABLE leads   ADD COLUMN IF NOT EXISTS college_id UUID REFERENCES colleges(id);

UPDATE users   SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;
UPDATE batches SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;
UPDATE courses SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;
UPDATE leads   SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_college   ON users(college_id);
CREATE INDEX IF NOT EXISTS idx_batches_college ON batches(college_id);
CREATE INDEX IF NOT EXISTS idx_courses_college ON courses(college_id);
CREATE INDEX IF NOT EXISTS idx_leads_college   ON leads(college_id);
