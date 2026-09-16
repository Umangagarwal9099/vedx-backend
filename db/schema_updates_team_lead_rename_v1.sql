-- ============================================================================
-- Renames the `team_lead` role to `admin`. Same permission set, clearer name
-- — see the app code for the corresponding change (RoleTeamLead → RoleAdmin
-- across ~36 call sites). Run this once, before deploying the renamed code.
--
-- `team_lead` and `business_development` also become valid values of
-- employees.department (a separate, unrelated concept — an ordinary
-- employee tagged "team_lead" who gets the Assign Leads capability in the
-- Employer Portal). No enum/CHECK constraint exists on department today, so
-- nothing to alter for that half — this file only touches the role.
-- ============================================================================

-- Run these as two separate statements/transactions if `role` is a native
-- Postgres ENUM type (Postgres does not allow using a brand-new enum value
-- in the same transaction that added it):
--
--   ALTER TYPE role ADD VALUE IF NOT EXISTS 'admin';
--   -- then, in a second statement/transaction:
--   UPDATE users SET role = 'admin' WHERE role = 'team_lead';
--
-- If `role` is a plain TEXT/VARCHAR column instead, this one statement is
-- sufficient on its own:
UPDATE users SET role = 'admin' WHERE role = 'team_lead';

-- ============================================================================
-- End of file
-- ============================================================================
