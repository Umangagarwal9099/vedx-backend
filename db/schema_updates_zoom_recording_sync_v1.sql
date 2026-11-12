-- Durable Zoom recording sync ledger (Nov 2026).
--
-- Before this, a recording.completed webhook spawned a goroutine that
-- downloaded, watermarked and uploaded the file, and any failure (R2
-- outage, ffmpeg killed when Cloud Run throttled CPU after the 200 response,
-- instance shut down mid-encode, expired download_token) was only logged —
-- the recording was then lost to the LMS for good. 43 sessions between
-- 2026-08-10 and 2026-10-15 ended up with no recording this way.
--
-- One row per recorded Zoom meeting INSTANCE (keyed by Zoom's instance UUID,
-- so webhook re-deliveries and reconciliation passes are idempotent). Rows
-- are created by the webhook (source=webhook) and by the periodic
-- reconciliation that lists recordings straight from Zoom (source=reconcile)
-- so a lost webhook can never lose a recording. A worker claims due rows with
-- a lease, retries failures with backoff, and records every outcome.
--
-- Additive only: no existing table/column is changed. Safe to re-run.

CREATE TABLE IF NOT EXISTS zoom_recording_jobs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zoom_meeting_uuid   TEXT        NOT NULL UNIQUE,
    zoom_meeting_id     BIGINT      NOT NULL,
    topic               TEXT        NOT NULL DEFAULT '',
    host_id             TEXT        NOT NULL DEFAULT '',
    host_email          TEXT        NOT NULL DEFAULT '',
    recording_start     TIMESTAMPTZ,
    duration_min        INT         NOT NULL DEFAULT 0,
    total_size          BIGINT      NOT NULL DEFAULT 0,
    -- every file Zoom produced for the instance (video views, audio,
    -- transcript, chat, timeline) — metadata only, for audit/recovery
    files               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    source              TEXT        NOT NULL DEFAULT 'webhook',   -- webhook | reconcile | backfill

    -- pending | processing | completed | skipped | unmatched | failed | gone
    status              TEXT        NOT NULL DEFAULT 'pending',
    role                TEXT,                                     -- primary | part
    skip_reason         TEXT,
    session_id          UUID REFERENCES sessions(id),
    batch_recording_id  UUID REFERENCES batch_recordings(id),
    primary_file_id     TEXT,
    raw_key             TEXT,     -- R2 key of the untouched original (archived before re-encoding)
    recording_url       TEXT,     -- watermarked copy, as attached to the session / batch

    attempts            INT         NOT NULL DEFAULT 0,
    max_attempts        INT         NOT NULL DEFAULT 8,
    next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until        TIMESTAMPTZ,
    last_error          TEXT,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT zoom_recording_jobs_status_chk CHECK (status IN
        ('pending','processing','completed','skipped','unmatched','failed','gone')),
    CONSTRAINT zoom_recording_jobs_role_chk CHECK (role IS NULL OR role IN ('primary','part'))
);

CREATE INDEX IF NOT EXISTS idx_zoom_recording_jobs_due
    ON zoom_recording_jobs (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_zoom_recording_jobs_meeting
    ON zoom_recording_jobs (zoom_meeting_id);
CREATE INDEX IF NOT EXISTS idx_zoom_recording_jobs_session
    ON zoom_recording_jobs (session_id);
-- one batch_recordings row per job, never two
CREATE UNIQUE INDEX IF NOT EXISTS uq_zoom_recording_jobs_batch_recording
    ON zoom_recording_jobs (batch_recording_id) WHERE batch_recording_id IS NOT NULL;

-- Webhook / reconciliation look sessions up by Zoom meeting ID; there was no
-- index on it.
CREATE INDEX IF NOT EXISTS idx_sessions_zoom_meeting_id
    ON sessions (zoom_meeting_id) WHERE deleted_at IS NULL;
