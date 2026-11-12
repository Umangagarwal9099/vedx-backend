package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

// ZoomRecordingJobRepository persists the durable recording-sync queue.
// Every write is keyed so it is safe to repeat: upserts by Zoom instance
// UUID, session writes are compare-and-set, batch recordings dedupe by URL.
type ZoomRecordingJobRepository struct {
	pool *pgxpool.Pool
}

func NewZoomRecordingJobRepository(pool *pgxpool.Pool) *ZoomRecordingJobRepository {
	return &ZoomRecordingJobRepository{pool: pool}
}

var recordingJobSelect = recordingJobSelectFrom("zoom_recording_jobs")

func scanRecordingJob(row pgx.Row) (*models.ZoomRecordingJob, error) {
	var j models.ZoomRecordingJob
	var files []byte
	err := row.Scan(&j.ID, &j.ZoomMeetingUUID, &j.ZoomMeetingID, &j.Topic, &j.HostID, &j.HostEmail,
		&j.RecordingStart, &j.DurationMin, &j.TotalSize, &files, &j.Source, &j.Status,
		&j.Role, &j.SkipReason, &j.SessionID, &j.SessionShortID, &j.SessionName,
		&j.BatchRecordingID, &j.PrimaryFileID, &j.RawKey, &j.RecordingURL,
		&j.Attempts, &j.MaxAttempts, &j.NextAttemptAt, &j.LastError,
		&j.CompletedAt, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, err
	}
	j.Files = json.RawMessage(files)
	return &j, nil
}

// UpsertInput describes a recorded instance as reported by Zoom.
type RecordingJobUpsert struct {
	UUID           string
	MeetingID      int64
	Topic          string
	HostID         string
	HostEmail      string
	RecordingStart *time.Time
	DurationMin    int
	TotalSize      int64
	Files          interface{}
	Source         string
}

// Upsert records a Zoom instance. A new UUID becomes a pending job; a known
// one only has its metadata refreshed (Zoom re-delivers webhooks, and
// reconciliation sees every instance on every pass) — its status is never
// reset here. Returns whether a new row was created.
func (r *ZoomRecordingJobRepository) Upsert(ctx context.Context, in RecordingJobUpsert) (bool, error) {
	files, err := json.Marshal(in.Files)
	if err != nil {
		return false, fmt.Errorf("encode recording files: %w", err)
	}
	var inserted bool
	err = r.pool.QueryRow(ctx, `
		INSERT INTO zoom_recording_jobs
			(zoom_meeting_uuid, zoom_meeting_id, topic, host_id, host_email, recording_start,
			 duration_min, total_size, files, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)
		ON CONFLICT (zoom_meeting_uuid) DO UPDATE SET
			topic        = EXCLUDED.topic,
			host_id      = COALESCE(NULLIF(EXCLUDED.host_id,''), zoom_recording_jobs.host_id),
			host_email   = COALESCE(NULLIF(EXCLUDED.host_email,''), zoom_recording_jobs.host_email),
			duration_min = GREATEST(EXCLUDED.duration_min, zoom_recording_jobs.duration_min),
			total_size   = GREATEST(EXCLUDED.total_size, zoom_recording_jobs.total_size),
			files        = CASE WHEN jsonb_array_length(EXCLUDED.files) > 0 THEN EXCLUDED.files ELSE zoom_recording_jobs.files END,
			updated_at   = NOW()
		RETURNING (xmax = 0)`,
		in.UUID, in.MeetingID, in.Topic, in.HostID, in.HostEmail, in.RecordingStart,
		in.DurationMin, in.TotalSize, string(files), in.Source,
	).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("upsert recording job: %w", err)
	}
	return inserted, nil
}

// leaseDuration bounds how long one attempt may hold a job. A job whose
// lease expires (process OOM-killed, instance shut down) is picked up again,
// and since attempts are counted at claim time a job that keeps crashing
// its worker still runs out of attempts instead of looping forever.
const leaseDuration = 2 * time.Hour

// ClaimNext atomically leases the next due job: pending and due, or
// processing with an expired lease. Longest recordings first, so a session's
// main class recording is normally attached before its short extra
// segments. FOR UPDATE SKIP LOCKED keeps concurrent workers (several Cloud
// Run instances, the CLI) from ever processing the same job twice.
// since, when non-nil, restricts the claim to recordings that started on or
// after it (used to prioritise a backfill, e.g. October first).
func (r *ZoomRecordingJobRepository) ClaimNext(ctx context.Context, since *time.Time) (*models.ZoomRecordingJob, error) {
	q := `
		WITH next AS (
			SELECT id FROM zoom_recording_jobs
			WHERE ((status = 'pending' AND next_attempt_at <= NOW())
			   OR (status = 'processing' AND locked_until < NOW()))
			  AND ($2::TIMESTAMPTZ IS NULL OR recording_start >= $2::TIMESTAMPTZ)
			ORDER BY duration_min DESC, next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE zoom_recording_jobs j SET
				last_error   = CASE WHEN j.status = 'processing'
				                    THEN 'previous attempt never finished (worker killed or timed out — check memory/CPU limits)'
				                    ELSE j.last_error END,
				status       = 'processing',
				attempts     = j.attempts + 1,
				locked_until = NOW() + $1::INTERVAL,
				updated_at   = NOW()
			FROM next WHERE j.id = next.id
			RETURNING j.*
		)` + recordingJobSelectFrom("claimed")
	job, err := scanRecordingJob(r.pool.QueryRow(ctx, q, fmt.Sprintf("%d seconds", int(leaseDuration.Seconds())), since))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

// recordingJobSelectFrom is recordingJobSelect reading from a CTE instead of the table.
func recordingJobSelectFrom(cte string) string {
	return `
	SELECT j.id, j.zoom_meeting_uuid, j.zoom_meeting_id, j.topic, j.host_id, j.host_email,
	       j.recording_start, j.duration_min, j.total_size, j.files, j.source, j.status,
	       COALESCE(j.role,''), COALESCE(j.skip_reason,''), COALESCE(j.session_id::TEXT,''),
	       COALESCE(s.short_id,''), COALESCE(s.name,''),
	       COALESCE(j.batch_recording_id::TEXT,''), COALESCE(j.primary_file_id,''),
	       COALESCE(j.raw_key,''), COALESCE(j.recording_url,''),
	       j.attempts, j.max_attempts, j.next_attempt_at, COALESCE(j.last_error,''),
	       j.completed_at, j.created_at, j.updated_at
	FROM ` + cte + ` j
	LEFT JOIN sessions s ON s.id = j.session_id`
}

// SetArchived records the R2 key of the untouched original and which Zoom
// file it came from, so a later attempt can skip re-downloading from Zoom.
func (r *ZoomRecordingJobRepository) SetArchived(ctx context.Context, id, fileID, rawKey string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE zoom_recording_jobs SET primary_file_id = $2, raw_key = $3, updated_at = NOW() WHERE id = $1`,
		id, fileID, rawKey)
	return err
}

// SetPrimaryFile records which Zoom file is this job's video.
func (r *ZoomRecordingJobRepository) SetPrimaryFile(ctx context.Context, id, fileID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE zoom_recording_jobs SET primary_file_id = $2, updated_at = NOW() WHERE id = $1`, id, fileID)
	return err
}

// Complete marks a job done with its final role and attachment.
func (r *ZoomRecordingJobRepository) Complete(ctx context.Context, id, sessionID, role, recordingURL, batchRecordingID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE zoom_recording_jobs SET
			status = 'completed', role = NULLIF($3,''), session_id = NULLIF($2,'')::UUID,
			recording_url = NULLIF($4,''), batch_recording_id = NULLIF($5,'')::UUID,
			last_error = NULL, skip_reason = NULL, locked_until = NULL,
			completed_at = NOW(), updated_at = NOW()
		WHERE id = $1`, id, sessionID, role, recordingURL, batchRecordingID)
	return err
}

// Finish marks a job with a terminal non-success outcome (skipped,
// unmatched, gone) and why.
func (r *ZoomRecordingJobRepository) Finish(ctx context.Context, id, status, sessionID, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE zoom_recording_jobs SET
			status = $2, session_id = NULLIF($3,'')::UUID, skip_reason = $4,
			locked_until = NULL, updated_at = NOW()
		WHERE id = $1`, id, status, sessionID, reason)
	return err
}

// Retry puts a job back in the queue after a failed attempt with
// exponential backoff (5m, 10m, 20m, ... capped at 6h), or marks it failed
// once max_attempts is reached.
func (r *ZoomRecordingJobRepository) Retry(ctx context.Context, id string, cause error, delay time.Duration) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx, `
		UPDATE zoom_recording_jobs SET
			status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
			next_attempt_at = NOW() + $3::INTERVAL,
			last_error = $2, locked_until = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING status`, id, truncate(cause.Error(), 4000), fmt.Sprintf("%d seconds", int(delay.Seconds())),
	).Scan(&status)
	return status, err
}

// Defer puts a job back in the queue WITHOUT consuming an attempt — used
// when this worker simply can't do the next step (e.g. no ffmpeg on the
// machine running an archive-only pass).
func (r *ZoomRecordingJobRepository) Defer(ctx context.Context, id, reason string, delay time.Duration) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE zoom_recording_jobs SET
			status = 'pending', attempts = GREATEST(attempts - 1, 0),
			next_attempt_at = NOW() + $3::INTERVAL, last_error = $2,
			locked_until = NULL, updated_at = NOW()
		WHERE id = $1`, id, reason, fmt.Sprintf("%d seconds", int(delay.Seconds())))
	return err
}

// RequeueStale gives failed jobs a fresh set of attempts once a day, and
// re-checks unmatched ones (a session may since have been created/linked),
// so a transient outage never permanently loses a recording that Zoom still
// holds. Returns how many rows were re-queued.
func (r *ZoomRecordingJobRepository) RequeueStale(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE zoom_recording_jobs SET status = 'pending', attempts = 0, next_attempt_at = NOW(), updated_at = NOW()
		WHERE status IN ('failed','unmatched') AND updated_at < NOW() - INTERVAL '24 hours'`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// FindByMeetingID returns every job for one Zoom meeting ID.
func (r *ZoomRecordingJobRepository) FindByMeetingID(ctx context.Context, meetingID int64) ([]models.ZoomRecordingJob, error) {
	return r.list(ctx, recordingJobSelect+` WHERE j.zoom_meeting_id = $1 ORDER BY j.recording_start`, meetingID)
}

// List returns jobs, optionally filtered by status, newest recording first.
func (r *ZoomRecordingJobRepository) List(ctx context.Context, status string, limit int) ([]models.ZoomRecordingJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if status != "" {
		return r.list(ctx, recordingJobSelect+` WHERE j.status = $1 ORDER BY j.recording_start DESC NULLS LAST LIMIT $2`, status, limit)
	}
	return r.list(ctx, recordingJobSelect+` ORDER BY j.recording_start DESC NULLS LAST LIMIT $1`, limit)
}

// CountByStatus summarises the queue.
func (r *ZoomRecordingJobRepository) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `SELECT status, COUNT(*) FROM zoom_recording_jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

func (r *ZoomRecordingJobRepository) list(ctx context.Context, q string, args ...interface{}) ([]models.ZoomRecordingJob, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ZoomRecordingJob
	for rows.Next() {
		j, err := scanRecordingJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// RecordingSessionCandidate is a session a recording could belong to.
type RecordingSessionCandidate struct {
	ID           string
	ShortID      string
	Name         string
	SessionDate  string // YYYY-MM-DD
	StartTime    string // HH:MM
	EndTime      string // HH:MM
	MentorID     string
	BatchShortID string
	RecordingURL string
}

// SessionsForMeeting returns the live sessions linked to a Zoom meeting ID.
func (r *ZoomRecordingJobRepository) SessionsForMeeting(ctx context.Context, meetingID int64) ([]RecordingSessionCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.short_id, s.name, s.session_date::TEXT,
		       TO_CHAR(s.start_time,'HH24:MI'), TO_CHAR(s.end_time,'HH24:MI'),
		       s.mentor_id::TEXT, b.short_id, COALESCE(s.recording_url,'')
		FROM sessions s
		JOIN batches b ON b.id = s.batch_id AND b.deleted_at IS NULL
		WHERE s.zoom_meeting_id = $1 AND s.deleted_at IS NULL`, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordingSessionCandidate
	for rows.Next() {
		var c RecordingSessionCandidate
		if err := rows.Scan(&c.ID, &c.ShortID, &c.Name, &c.SessionDate, &c.StartTime, &c.EndTime, &c.MentorID, &c.BatchShortID, &c.RecordingURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SwapSessionRecording sets a session's recording_url only if it still
// holds expected (compare-and-set), so two workers — or a worker and an
// admin edit — can never silently overwrite each other. Returns whether the
// write happened.
func (r *ZoomRecordingJobRepository) SwapSessionRecording(ctx context.Context, sessionID, expected, newURL string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE sessions SET recording_url = $3, updated_at = NOW()
		WHERE id = $1::UUID AND deleted_at IS NULL AND COALESCE(recording_url,'') = $2`,
		sessionID, expected, newURL)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// FindBatchRecordingByURL returns the id of a live batch recording already
// pointing at url (dedupe for extra-segment uploads), or "".
func (r *ZoomRecordingJobRepository) FindBatchRecordingByURL(ctx context.Context, url string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id::TEXT FROM batch_recordings WHERE url = $1 AND deleted_at IS NULL LIMIT 1`, url).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// DemoteToPart re-labels a previously-primary job as an extra segment after
// a longer recording of the same session replaced it.
func (r *ZoomRecordingJobRepository) DemoteToPart(ctx context.Context, id, batchRecordingID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE zoom_recording_jobs SET role = 'part', batch_recording_id = NULLIF($2,'')::UUID, updated_at = NOW() WHERE id = $1`,
		id, batchRecordingID)
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
