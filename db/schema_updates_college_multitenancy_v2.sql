-- ============================================================================
-- Multi-tenant college management, student segregation, feature-access and
-- authorization system — built incrementally, stage by stage. Each stage
-- appends to this same file rather than creating a new one, since they're
-- all part of one feature and applied together.
--
-- Design notes (read before extending):
--   - Every statement here is additive and safe to re-run (IF NOT EXISTS /
--     ON CONFLICT DO NOTHING / idempotent UPDATE ... WHERE). Nothing drops
--     or renames an existing column, and no existing row's behavior changes
--     as a result of running this file.
--   - "organization_type" formalizes a distinction that already exists in
--     practice: the auto-created Default College (code='DEFAULT',
--     short_id='DEFAULTCOL') represents the EdTech company's own direct
--     students/batches, not an external tenant. Every OTHER college row
--     created via POST /colleges represents a real external organization.
--   - This migration does not change any existing college's features,
--     status, or student/batch/course/lead ownership — it only adds new,
--     optional structure for stages that come after Stage 1.
-- ============================================================================

-- ── Stage 1: organization foundation ────────────────────────────────────────

ALTER TABLE colleges ADD COLUMN IF NOT EXISTS organization_type VARCHAR(20) NOT NULL DEFAULT 'college';

DO $$ BEGIN
    ALTER TABLE colleges ADD CONSTRAINT colleges_organization_type_check
        CHECK (organization_type IN ('internal', 'college', 'training_partner', 'corporate'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- The Default College already exists as of schema_updates_college_v1.sql —
-- this just labels it correctly. Idempotent: a no-op on every re-run once
-- applied once.
UPDATE colleges SET organization_type = 'internal' WHERE code = 'DEFAULT' AND organization_type <> 'internal';

-- users.role is a Postgres ENUM (user_role), not a plain TEXT/CHECK column —
-- confirmed by inspecting the live schema. New role values must be added to
-- this type before any Go code can insert/update a user with that role.
-- IMPORTANT: a value added by ALTER TYPE ... ADD VALUE cannot be used in the
-- SAME transaction that adds it (a hard Postgres restriction, not specific to
-- this migration) — this is fine here since this whole file is applied as
-- its own migration, separately from any application code that will later
-- insert rows using these roles.
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'platform_admin';
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'college_admin';
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'college_staff';

-- ── Stage 2: relational feature management ──────────────────────────────────
-- Replaces colleges.enabled_features (a JSONB blob with no audit trail) as
-- the source of truth for feature-gating decisions, while continuing to
-- dual-write that JSONB column so every existing read path (CollegesPage's
-- list view, GetMyFeatures) keeps working unchanged. HasFeature reads
-- college_features first and only falls back to the JSONB blob when a
-- college has zero rows there yet (not backfilled / migration pending).

CREATE TABLE IF NOT EXISTS platform_features (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(60) NOT NULL UNIQUE,
    name        VARCHAR(120) NOT NULL,
    description TEXT,
    category    VARCHAR(40) NOT NULL,
    route_key   VARCHAR(60),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS college_features (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    college_id    UUID NOT NULL REFERENCES colleges(id),
    feature_id    UUID NOT NULL REFERENCES platform_features(id),
    is_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    configuration JSONB,
    enabled_by    UUID REFERENCES users(id),
    enabled_at    TIMESTAMPTZ,
    disabled_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (college_id, feature_id)
);

CREATE INDEX IF NOT EXISTS idx_college_features_college ON college_features(college_id);

-- Seed platform_features from the existing ~30 keys in models.AllFeatureKeys
-- (models/college.go), grouped exactly as the Feature Configuration UI
-- presents them (spec section 15).
INSERT INTO platform_features (code, name, category) VALUES
    ('dashboard',            'Dashboard',             'learning'),
    ('courses',               'Courses',               'learning'),
    ('modules',               'Modules',               'learning'),
    ('resources',             'Resources',             'learning'),
    ('recorded_videos',       'Recorded Videos',       'learning'),
    ('live_sessions',         'Live Sessions',         'learning'),
    ('assignments',           'Assignments',           'learning'),
    ('exams',                 'Assessments/Exams',     'learning'),
    ('projects',              'Projects',              'learning'),
    ('coding',                'Coding Practice',       'coding'),
    ('coding_practice',       'Coding Practice Sets',  'coding'),
    ('coding_assessments',    'Coding Assessments',    'coding'),
    ('coding_question_bank',  'Coding Question Bank',  'coding'),
    ('coding_leaderboard',    'Coding Leaderboard',    'coding'),
    ('community',             'Community',             'engagement'),
    ('messages',              'Messages',              'engagement'),
    ('notifications',         'Notifications',         'engagement'),
    ('announcements',         'Announcements',         'engagement'),
    ('internship',            'Internship',            'career'),
    ('job_assistance',        'Job Assistance',        'career'),
    ('placements',            'Placements',            'career'),
    ('employer_management',   'Employer Management',  'career'),
    ('crm',                   'CRM',                   'operations'),
    ('employee_portal',       'Employee Portal',       'operations'),
    ('attendance',            'Attendance',            'operations'),
    ('leave',                 'Leave Management',      'operations'),
    ('reports',               'Reports',               'operations'),
    ('analytics',             'Analytics',             'analytics'),
    ('student_analytics',     'Student Analytics',    'analytics'),
    ('mentor_analytics',      'Mentor Analytics',     'analytics'),
    ('college_analytics',     'College Analytics',    'analytics'),
    ('coding_analytics',      'Coding Analytics',     'analytics')
ON CONFLICT (code) DO NOTHING;

-- Backfill: for every college, for every key currently `true` in its
-- enabled_features JSONB, create the matching college_features row —
-- best-effort provenance (enabled_by/enabled_at borrowed from the college's
-- own creation record, since the real historical actor isn't known).
INSERT INTO college_features (college_id, feature_id, is_enabled, enabled_by, enabled_at)
SELECT c.id, pf.id, TRUE, c.created_by, c.created_at
FROM colleges c
CROSS JOIN LATERAL jsonb_each(c.enabled_features) AS ef(key, value)
JOIN platform_features pf ON pf.code = ef.key
WHERE (ef.value)::text = 'true'
ON CONFLICT (college_id, feature_id) DO NOTHING;

-- ── Stage 3: organization ownership wiring ──────────────────────────────────
-- Every insert path for users/batches/courses/leads now sets college_id in
-- application code going forward (Stage 1 fixed users; this stage fixes
-- batches/courses/leads) — this re-run of the original migration's backfill
-- closes the gap for anything created between that migration landing and
-- these code paths being fixed. Confirmed via a live read-only check before
-- writing this: zero non-deleted rows in any of these four tables currently
-- have a NULL college_id, so this is a no-op safety net, not a real fix.
UPDATE users   SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;
UPDATE batches SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;
UPDATE courses SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;
UPDATE leads   SET college_id = (SELECT id FROM colleges WHERE code = 'DEFAULT') WHERE college_id IS NULL;

-- ── Stage 4: College Admin onboarding, student provisioning, registration
--    numbers, college transfer ────────────────────────────────────────────
-- Per-college sequential registration numbers: each college (Default
-- included) gets its own independent counter, assigned once at creation via
-- an atomic UPDATE...RETURNING (never a separate SELECT-then-UPDATE, which
-- would race under concurrent registrations). students.college_id is a
-- deliberate denormalized copy of users.college_id (kept in sync at
-- creation and on transfer) purely so UNIQUE(college_id, registration_no)
-- can be a real constraint — a plain UNIQUE can't span two tables.
ALTER TABLE colleges ADD COLUMN IF NOT EXISTS next_registration_no INTEGER NOT NULL DEFAULT 1;
ALTER TABLE students ADD COLUMN IF NOT EXISTS registration_no INTEGER;
ALTER TABLE students ADD COLUMN IF NOT EXISTS college_id UUID REFERENCES colleges(id);

DO $$ BEGIN
    ALTER TABLE students ADD CONSTRAINT students_college_registration_no_unique UNIQUE (college_id, registration_no);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- No backfill for existing students' registration_no (spec explicitly
-- allows deferring this) — only new registrations from here forward get a
-- number. Backfill college_id on students to match their user row, though,
-- since that's needed immediately for the unique constraint to mean
-- anything and for future college-scoped student queries.
UPDATE students s SET college_id = u.college_id
FROM users u
WHERE s.user_id = u.id AND s.college_id IS NULL AND u.college_id IS NOT NULL;

-- Full audit trail for Super Admin-initiated college transfers — every
-- transfer writes a row here, whether or not it was force-confirmed over an
-- active-enrollment objection.
CREATE TABLE IF NOT EXISTS user_college_history (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id       VARCHAR(16) NOT NULL UNIQUE,
    user_id        UUID NOT NULL REFERENCES users(id),
    old_college_id UUID REFERENCES colleges(id),
    new_college_id UUID NOT NULL REFERENCES colleges(id),
    moved_by       UUID NOT NULL REFERENCES users(id),
    reason         TEXT,
    metadata       JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_college_history_user ON user_college_history(user_id);

-- ── Stage 5: course/batch/coding ownership rules ────────────────────────────
-- course_scope distinguishes a reusable "global master course" (assignable
-- to many colleges via college_courses, e.g. authored by Super Admin/
-- Platform Admin) from an "organization" course owned by exactly one
-- college — existing courses default to 'organization' since every course
-- created so far belongs to exactly the one college that created it
-- (typically Default). Nothing changes for them; this only matters for
-- NEW courses explicitly marked global going forward.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS course_scope VARCHAR(20) NOT NULL DEFAULT 'organization';

DO $$ BEGIN
    ALTER TABLE courses ADD CONSTRAINT courses_course_scope_check CHECK (course_scope IN ('global', 'organization'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Maps a global course to the colleges allowed to use it, with an optional
-- access window. A college-scoped batch may only be created for a course
-- that is either 'organization'-owned by that same college, or 'global' AND
-- has an active (enabled, within-window) row here for that college.
CREATE TABLE IF NOT EXISTS college_courses (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    college_id        UUID NOT NULL REFERENCES colleges(id),
    course_id         UUID NOT NULL REFERENCES courses(id),
    is_enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    access_start_date DATE,
    access_end_date   DATE,
    assigned_by       UUID REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (college_id, course_id)
);

CREATE INDEX IF NOT EXISTS idx_college_courses_college ON college_courses(college_id);

-- Per-college coding-content restrictions — a college only sees the
-- languages/topics/question-bank question-sets explicitly listed here (an
-- empty set for a college means "no restriction configured yet," handled in
-- application code as "show everything," not "show nothing," so existing
-- colleges aren't silently locked out of coding content they already used).
CREATE TABLE IF NOT EXISTS college_languages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    college_id UUID NOT NULL REFERENCES colleges(id),
    language   VARCHAR(60) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (college_id, language)
);

CREATE TABLE IF NOT EXISTS college_topics (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    college_id UUID NOT NULL REFERENCES colleges(id),
    topic      VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (college_id, topic)
);

CREATE TABLE IF NOT EXISTS college_question_sets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    college_id          UUID NOT NULL REFERENCES colleges(id),
    coding_question_id  UUID NOT NULL REFERENCES coding_questions(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (college_id, coding_question_id)
);

CREATE INDEX IF NOT EXISTS idx_college_languages_college ON college_languages(college_id);
CREATE INDEX IF NOT EXISTS idx_college_topics_college ON college_topics(college_id);
CREATE INDEX IF NOT EXISTS idx_college_question_sets_college ON college_question_sets(college_id);

-- ── Stage 6: subscriptions ───────────────────────────────────────────────────
-- A college may have zero or more subscription rows over its lifetime
-- (renewals create a new row rather than mutating the old one, so history is
-- never lost). "Currently active" is resolved as the most recent row whose
-- status is 'active' or 'renewed' and whose date range covers today — a
-- college with NO rows here has no subscription configured at all, which is
-- treated as "unrestricted" (fail open), not "expired," so this feature can
-- be adopted gradually without locking out every existing college the
-- moment the migration lands.
DO $$ BEGIN
    CREATE TYPE college_subscription_status AS ENUM ('upcoming', 'active', 'expired', 'suspended', 'cancelled', 'renewed');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS college_subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id   VARCHAR(16) NOT NULL UNIQUE,
    college_id UUID NOT NULL REFERENCES colleges(id),
    plan_id    VARCHAR(60),
    start_date DATE NOT NULL,
    end_date   DATE NOT NULL,
    status     college_subscription_status NOT NULL DEFAULT 'active',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    renewed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_college_subscriptions_college ON college_subscriptions(college_id);
