package models

import (
	"encoding/json"
	"time"
)

// Zoom recording job statuses — see db/schema_updates_zoom_recording_sync_v1.sql.
const (
	RecordingJobPending    = "pending"
	RecordingJobProcessing = "processing"
	RecordingJobCompleted  = "completed"
	RecordingJobSkipped    = "skipped"   // no video file / too-short extra segment
	RecordingJobUnmatched  = "unmatched" // no LMS session for this meeting ID near the recording time
	RecordingJobFailed     = "failed"    // retries exhausted; reconciliation re-queues daily while Zoom still has it
	RecordingJobGone       = "gone"      // Zoom no longer has the recording (deleted / retention)
)

// ZoomRecordingJob is one recorded Zoom meeting instance moving through the
// sync pipeline (archive original → watermark → attach to session).
type ZoomRecordingJob struct {
	ID               string          `json:"id"`
	ZoomMeetingUUID  string          `json:"zoom_meeting_uuid"`
	ZoomMeetingID    int64           `json:"zoom_meeting_id"`
	Topic            string          `json:"topic"`
	HostID           string          `json:"host_id"`
	HostEmail        string          `json:"host_email"`
	RecordingStart   *time.Time      `json:"recording_start,omitempty"`
	DurationMin      int             `json:"duration_min"`
	TotalSize        int64           `json:"total_size"`
	Files            json.RawMessage `json:"files"`
	Source           string          `json:"source"`
	Status           string          `json:"status"`
	Role             string          `json:"role,omitempty"`
	SkipReason       string          `json:"skip_reason,omitempty"`
	SessionID        string          `json:"session_id,omitempty"`
	SessionShortID   string          `json:"session_short_id,omitempty"`
	SessionName      string          `json:"session_name,omitempty"`
	BatchRecordingID string          `json:"batch_recording_id,omitempty"`
	PrimaryFileID    string          `json:"primary_file_id,omitempty"`
	RawKey           string          `json:"raw_key,omitempty"`
	RecordingURL     string          `json:"recording_url,omitempty"`
	Attempts         int             `json:"attempts"`
	MaxAttempts      int             `json:"max_attempts"`
	NextAttemptAt    time.Time       `json:"next_attempt_at"`
	LastError        string          `json:"last_error,omitempty"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// RecordingSyncSummary is returned by a sync/reconcile run.
type RecordingSyncSummary struct {
	Discovered int            `json:"discovered"` // Zoom instances seen by reconciliation
	Enqueued   int            `json:"enqueued"`   // new jobs created
	Requeued   int            `json:"requeued"`   // failed/unmatched jobs put back in the queue
	Processed  int            `json:"processed"`
	Outcomes   map[string]int `json:"outcomes"`
	Errors     []string       `json:"errors,omitempty"`
}
