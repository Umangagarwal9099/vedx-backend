-- Commit 987fc58 ("added demo video functionality") started selecting an
-- is_demo flag from both sessions and batch_recordings (sessionBaseSelect in
-- repository/session_repo.go, batchRecordingBaseSelect in
-- repository/batch_recording_repo.go), and SetDemoFlags on each repo writes
-- it. The column was never added to the schema, so every query through those
-- base selects failed with `column s.is_demo does not exist` — surfaced as a
-- 500 "could not fetch sessions" on GET /batches/{short_id}/recordings.
--
-- Default FALSE so all existing rows are simply "not a demo"; the admin
-- "Select Demo Classes" flow flips individual rows on afterwards.
ALTER TABLE sessions         ADD COLUMN IF NOT EXISTS is_demo BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE batch_recordings ADD COLUMN IF NOT EXISTS is_demo BOOLEAN NOT NULL DEFAULT FALSE;
