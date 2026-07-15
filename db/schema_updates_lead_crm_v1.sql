-- ============================================================================
-- Lead CRM: sales leads for course-enrollment prospects, employee attendance,
-- leave requests, and monthly targets. Fully additive — no existing table is
-- touched. Run this once against the database; every statement is
-- idempotent (IF NOT EXISTS / duplicate_object-safe) so re-running is safe.
-- ============================================================================

-- ── Leads ─────────────────────────────────────────────────────────────────

DO $$ BEGIN
    CREATE TYPE lead_status AS ENUM (
        'new', 'contacted', 'interested', 'follow_up', 'not_reachable',
        'converted', 'not_interested', 'lost'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE lead_priority AS ENUM ('low', 'medium', 'high', 'urgent');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE lead_source AS ENUM ('excel_import', 'manual', 'website', 'referral', 'other');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS leads (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id          VARCHAR(16) NOT NULL UNIQUE,
    name              TEXT NOT NULL,
    phone             TEXT NOT NULL,
    email             TEXT,
    city              TEXT,
    course_interest   TEXT NOT NULL,
    course_id         UUID REFERENCES courses(id),
    source            lead_source NOT NULL DEFAULT 'manual',
    status            lead_status NOT NULL DEFAULT 'new',
    priority          lead_priority NOT NULL DEFAULT 'medium',
    assigned_to       UUID REFERENCES users(id),
    assigned_at       TIMESTAMPTZ,
    assigned_by       UUID REFERENCES users(id),
    next_follow_up_at TIMESTAMPTZ,
    last_contacted_at TIMESTAMPTZ,
    converted_at      TIMESTAMPTZ,
    notes             TEXT,
    created_by        UUID NOT NULL REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_leads_assigned_to   ON leads (assigned_to) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_status         ON leads (status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_priority        ON leads (priority) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_next_follow_up  ON leads (next_follow_up_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_course_id       ON leads (course_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_city            ON leads (city) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_created_at      ON leads (created_at DESC);

-- ── Call / activity log ────────────────────────────────────────────────────

DO $$ BEGIN
    CREATE TYPE call_outcome AS ENUM (
        'connected', 'no_answer', 'busy', 'switched_off', 'invalid_number', 'callback_requested'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS lead_call_logs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id         VARCHAR(16) NOT NULL UNIQUE,
    lead_id          UUID NOT NULL REFERENCES leads(id),
    employee_id      UUID NOT NULL REFERENCES users(id),
    outcome          call_outcome NOT NULL,
    status_after     lead_status NOT NULL,
    notes            TEXT,
    follow_up_set_at TIMESTAMPTZ,
    called_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_call_logs_lead     ON lead_call_logs (lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_call_logs_employee ON lead_call_logs (employee_id, called_at DESC);

-- ── Assignment history ─────────────────────────────────────────────────────

DO $$ BEGIN
    CREATE TYPE lead_assignment_action AS ENUM ('assign', 'reassign', 'unassign', 'auto_assign');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS lead_assignment_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id         VARCHAR(16) NOT NULL UNIQUE,
    lead_id          UUID NOT NULL REFERENCES leads(id),
    action           lead_assignment_action NOT NULL,
    from_employee_id UUID REFERENCES users(id),
    to_employee_id   UUID REFERENCES users(id),
    priority_set     lead_priority,
    follow_up_set_at TIMESTAMPTZ,
    performed_by     UUID NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_assignment_history_lead ON lead_assignment_history (lead_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lead_assignment_history_to   ON lead_assignment_history (to_employee_id);

-- ── Employee attendance (self check-in/out) ────────────────────────────────

DO $$ BEGIN
    CREATE TYPE employee_attendance_status AS ENUM ('present', 'absent', 'half_day', 'on_leave', 'holiday');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS employee_attendance (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id     VARCHAR(16) NOT NULL UNIQUE,
    employee_id  UUID NOT NULL REFERENCES users(id),
    date         DATE NOT NULL,
    status       employee_attendance_status NOT NULL DEFAULT 'present',
    check_in_at  TIMESTAMPTZ,
    check_out_at TIMESTAMPTZ,
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (employee_id, date)
);

CREATE INDEX IF NOT EXISTS idx_employee_attendance_employee_date ON employee_attendance (employee_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_employee_attendance_date          ON employee_attendance (date);

-- ── Leave requests + balance ───────────────────────────────────────────────

DO $$ BEGIN
    CREATE TYPE leave_type AS ENUM ('full_day', 'half_day');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE leave_status AS ENUM ('pending', 'approved', 'rejected');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS employee_leave_requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id      VARCHAR(16) NOT NULL UNIQUE,
    employee_id   UUID NOT NULL REFERENCES users(id),
    from_date     DATE NOT NULL,
    to_date       DATE NOT NULL,
    leave_type    leave_type NOT NULL DEFAULT 'full_day',
    reason        TEXT NOT NULL,
    status        leave_status NOT NULL DEFAULT 'pending',
    admin_note    TEXT,
    reviewed_by   UUID REFERENCES users(id),
    reviewed_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_employee_leave_requests_employee ON employee_leave_requests (employee_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_employee_leave_requests_status   ON employee_leave_requests (status);

CREATE TABLE IF NOT EXISTS employee_leave_balances (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES users(id),
    year        INT NOT NULL,
    total_days  NUMERIC(5,1) NOT NULL DEFAULT 0,
    used_days   NUMERIC(5,1) NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (employee_id, year)
);

-- ── Monthly targets ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS employee_monthly_targets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id            VARCHAR(16) NOT NULL UNIQUE,
    employee_id         UUID NOT NULL REFERENCES users(id),
    year                INT NOT NULL,
    month               INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    target_conversions  INT NOT NULL DEFAULT 0,
    set_by              UUID NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (employee_id, year, month)
);
-- "achieved" is computed on read, not stored:
-- COUNT(leads WHERE assigned_to = employee_id AND status = 'converted'
--             AND converted_at BETWEEN <month start> AND <month end>)

-- ============================================================================
-- End of file
-- ============================================================================
