-- users.email had a plain, table-wide UNIQUE(email) constraint since the very
-- first migration, but EmailExists() (repository/user_repo.go) has always
-- excluded soft-deleted rows from its pre-check — the two never matched.
-- Reproduced live: soft-delete a user, then try to create a new account with
-- that same email — the app-level check passes, then the INSERT crashes with
-- an unhandled 23505 ("duplicate key value violates unique constraint
-- users_email_key"), surfaced to the admin as a raw 500 with the Postgres
-- error text leaked into the response body.
--
-- This makes the DB match the app's actual intent: a deleted account's email
-- becomes reusable, and only currently-active accounts need to stay unique.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
CREATE UNIQUE INDEX IF NOT EXISTS users_email_key ON users (email) WHERE deleted_at IS NULL;
