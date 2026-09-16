-- ============================================================================
-- Employee ID: a human-readable code for staff accounts, the same idea as
-- students already have via enrollment_no. Additive only.
-- ============================================================================

ALTER TABLE employees ADD COLUMN IF NOT EXISTS employee_code TEXT UNIQUE;

-- Backfill existing employees sequentially in signup order, EMP-0001 style.
WITH numbered AS (
    SELECT user_id, ROW_NUMBER() OVER (ORDER BY created_at) AS n
    FROM employees
    WHERE employee_code IS NULL
)
UPDATE employees e
SET employee_code = 'EMP-' || LPAD(numbered.n::TEXT, 4, '0')
FROM numbered
WHERE e.user_id = numbered.user_id;

-- ============================================================================
-- End of file
-- ============================================================================
