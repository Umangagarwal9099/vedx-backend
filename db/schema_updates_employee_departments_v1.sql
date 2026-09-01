-- Stage 1: let one employee/mentor/team_lead serve more than one college.
-- users.college_id stays exactly as-is (the employee's default/home college
-- — everything that reads it today keeps working unmodified); this table is
-- the full membership list, including that same default row.
CREATE TABLE IF NOT EXISTS college_employees (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    college_id  UUID NOT NULL REFERENCES colleges(id),
    user_id     UUID NOT NULL REFERENCES users(id),
    is_default  BOOLEAN NOT NULL DEFAULT false,
    assigned_by UUID REFERENCES users(id),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (college_id, user_id)
);

-- Backfill: every existing staff member's current college becomes their
-- default membership row, so nothing regresses the moment this ships.
INSERT INTO college_employees (college_id, user_id, is_default, assigned_at)
SELECT college_id, id, true, created_at
FROM users
WHERE role IN ('mentor', 'employee', 'team_lead')
  AND college_id IS NOT NULL
  AND deleted_at IS NULL
ON CONFLICT (college_id, user_id) DO NOTHING;

-- Stage 2: department sub-role for employees. employees.department already
-- existed (added by an earlier migration, never wired to any Go code until
-- now) — department_team is new, and only ever populated when department='hr'.
ALTER TABLE employees ADD COLUMN IF NOT EXISTS department_team VARCHAR(50);

-- Stage 5: manager notes on an employee's profile — a running record, not a
-- one-time review.
CREATE TABLE IF NOT EXISTS employee_notes (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id         TEXT UNIQUE NOT NULL,
    employee_user_id UUID NOT NULL REFERENCES users(id),
    author_user_id   UUID NOT NULL REFERENCES users(id),
    note_text        TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_employee_notes_employee ON employee_notes(employee_user_id, created_at DESC);
