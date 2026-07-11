package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// sessionBaseSelect joins mentor, batch and feedback-form details alongside their IDs.
const sessionBaseSelect = `
	SELECT s.id, s.short_id, s.name,
	       s.session_date::TEXT,
	       TO_CHAR(s.start_time, 'HH24:MI'),
	       TO_CHAR(s.end_time,   'HH24:MI'),
	       s.mentor_id, CONCAT(m.first_name, ' ', m.last_name),
	       s.mode::TEXT,
	       COALESCE(s.meeting_platform::TEXT, ''),
	       s.send_confirmation_email,
	       s.session_reminder_notifications,
	       COALESCE(s.topics, '{}'),
	       s.generate_shareable_link,
	       COALESCE(s.share_token, ''),
	       s.zoom_meeting_id,
	       COALESCE(s.zoom_join_url, ''),
	       COALESCE(s.zoom_start_url, ''),
	       COALESCE(s.recording_url, ''),
	       COALESCE(ff.short_id, ''),
	       COALESCE(ff.title, ''),
	       s.session_type::TEXT,
	       s.batch_id, b.short_id, b.batch_number,
	       s.is_active, s.created_by::TEXT,
	       s.created_at, s.updated_at
	FROM sessions s
	JOIN  users    m  ON s.mentor_id        = m.id  AND m.deleted_at  IS NULL
	JOIN  batches  b  ON s.batch_id         = b.id  AND b.deleted_at  IS NULL
	LEFT JOIN feedback_forms ff ON s.feedback_form_id = ff.id AND ff.deleted_at IS NULL`

func scanSession(row pgx.Row) (*models.Session, error) {
	var s models.Session
	err := row.Scan(
		&s.ID, &s.ShortID, &s.Name,
		&s.SessionDate, &s.StartTime, &s.EndTime,
		&s.MentorID, &s.MentorName,
		&s.Mode,
		&s.MeetingPlatform,
		&s.SendConfirmationEmail,
		&s.SessionReminderNotifications,
		&s.Topics,
		&s.GenerateShareableLink,
		&s.ShareToken,
		&s.ZoomMeetingID,
		&s.ZoomJoinURL,
		&s.ZoomStartURL,
		&s.RecordingURL,
		&s.FeedbackFormShortID,
		&s.FeedbackFormTitle,
		&s.SessionType,
		&s.BatchID, &s.BatchShortID, &s.BatchNumber,
		&s.IsActive,
		&s.CreatedBy,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserts a new session, retrying up to 3 times on short_id/share_token collision.
// zoom is optional — pass nil when no Zoom meeting was created (not configured, or
// mode/platform doesn't call for one).
func (r *SessionRepository) Create(ctx context.Context, in models.CreateSessionInput, createdBy string, zoom *models.ZoomMeetingInfo) (*models.Session, error) {
	topics := in.Topics
	if topics == nil {
		topics = []string{}
	}

	var zoomMeetingID interface{}
	var zoomJoinURL, zoomStartURL string
	if zoom != nil {
		zoomMeetingID = zoom.ID
		zoomJoinURL = zoom.JoinURL
		zoomStartURL = zoom.StartURL
	}

	const q = `
		WITH ins AS (
			INSERT INTO sessions (
				short_id, name, session_date, start_time, end_time,
				mentor_id, mode, meeting_platform,
				send_confirmation_email, session_reminder_notifications,
				topics, generate_shareable_link, share_token,
				feedback_form_id, session_type, batch_id, created_by,
				zoom_meeting_id, zoom_join_url, zoom_start_url
			) VALUES (
				$1, $2, $3::DATE, $4::TIME, $5::TIME,
				$6::UUID, $7::session_mode, NULLIF($8,'')::session_meeting_platform,
				$9, $10,
				$11, $12, NULLIF($13,''),
				(SELECT id FROM feedback_forms WHERE short_id = NULLIF($14,'') AND deleted_at IS NULL),
				$15::session_type,
				(SELECT id FROM batches WHERE short_id = $16 AND deleted_at IS NULL),
				$17,
				$18, NULLIF($19,''), NULLIF($20,'')
			)
			RETURNING *
		)
		SELECT ins.id, ins.short_id, ins.name,
		       ins.session_date::TEXT,
		       TO_CHAR(ins.start_time, 'HH24:MI'),
		       TO_CHAR(ins.end_time,   'HH24:MI'),
		       ins.mentor_id, CONCAT(m.first_name, ' ', m.last_name),
		       ins.mode::TEXT,
		       COALESCE(ins.meeting_platform::TEXT, ''),
		       ins.send_confirmation_email,
		       ins.session_reminder_notifications,
		       COALESCE(ins.topics, '{}'),
		       ins.generate_shareable_link,
		       COALESCE(ins.share_token, ''),
		       ins.zoom_meeting_id,
		       COALESCE(ins.zoom_join_url, ''),
		       COALESCE(ins.zoom_start_url, ''),
		       COALESCE(ins.recording_url, ''),
		       COALESCE(ff.short_id, ''),
		       COALESCE(ff.title, ''),
		       ins.session_type::TEXT,
		       ins.batch_id, b.short_id, b.batch_number,
		       ins.is_active, ins.created_by::TEXT,
		       ins.created_at, ins.updated_at
		FROM ins
		JOIN  users   m ON ins.mentor_id = m.id
		JOIN  batches b ON ins.batch_id  = b.id
		LEFT JOIN feedback_forms ff ON ins.feedback_form_id = ff.id`

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		shareToken := ""
		if in.GenerateShareableLink {
			shareToken = util.GenerateShortID() + util.GenerateShortID()
		}

		s, err := scanSession(r.pool.QueryRow(ctx, q,
			shortID, in.Name, in.SessionDate, in.StartTime, in.EndTime,
			in.MentorID, in.Mode, in.MeetingPlatform,
			in.SendConfirmationEmail, in.SessionReminderNotifications,
			topics, in.GenerateShareableLink, shareToken,
			in.FeedbackFormShortID, in.SessionType, in.BatchShortID, createdBy,
			zoomMeetingID, zoomJoinURL, zoomStartURL,
		))
		if err == nil {
			return s, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted sessions ordered newest first.
func (r *SessionRepository) FindAll(ctx context.Context) ([]models.Session, error) {
	q := sessionBaseSelect + ` WHERE s.deleted_at IS NULL ORDER BY s.session_date DESC, s.start_time DESC`
	return r.scanSessions(ctx, q)
}

// FindByShortID returns a single non-deleted session.
func (r *SessionRepository) FindByShortID(ctx context.Context, shortID string) (*models.Session, error) {
	q := sessionBaseSelect + ` WHERE s.short_id = $1 AND s.deleted_at IS NULL LIMIT 1`
	s, err := scanSession(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

// FindByShareToken returns a single non-deleted session by its public share token.
// Used by the unauthenticated join-resolution endpoint.
func (r *SessionRepository) FindByShareToken(ctx context.Context, token string) (*models.Session, error) {
	q := sessionBaseSelect + ` WHERE s.share_token = $1 AND s.deleted_at IS NULL LIMIT 1`
	s, err := scanSession(r.pool.QueryRow(ctx, q, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

// FindDueForReminder returns active sessions whose scheduled start time falls within
// [windowStart, windowEnd] and which haven't had a reminder notification sent yet.
// windowStart/windowEnd are naive "YYYY-MM-DD HH:MM:SS" wall-clock timestamps in the
// app's configured timezone (config.AppConfig.Timezone) — matching how session_date/
// start_time are stored (no timezone), so they must NOT be pre-converted to UTC.
func (r *SessionRepository) FindDueForReminder(ctx context.Context, windowStart, windowEnd string) ([]models.Session, error) {
	q := sessionBaseSelect + `
		WHERE s.deleted_at IS NULL
		  AND s.is_active
		  AND s.session_reminder_notifications
		  AND s.reminder_sent_at IS NULL
		  AND (s.session_date + s.start_time) BETWEEN $1::TIMESTAMP AND $2::TIMESTAMP`
	return r.scanSessions(ctx, q, windowStart, windowEnd)
}

// MarkReminderSent records that the start-time reminder notification has been sent,
// so the reminder worker doesn't send it again on the next poll.
func (r *SessionRepository) MarkReminderSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET reminder_sent_at = NOW() WHERE id = $1::UUID`, id)
	return err
}

// Filter returns non-deleted sessions matching the provided filter (batch, mentor, date).
func (r *SessionRepository) Filter(ctx context.Context, f models.SessionFilter) ([]models.Session, error) {
	conditions := []string{"s.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1

	if f.BatchShortID != "" {
		conditions = append(conditions, fmt.Sprintf("b.short_id = $%d", i))
		args = append(args, f.BatchShortID)
		i++
	}
	if f.MentorID != "" {
		conditions = append(conditions, fmt.Sprintf("s.mentor_id = $%d::UUID", i))
		args = append(args, f.MentorID)
		i++
	}
	if f.Date != "" {
		conditions = append(conditions, fmt.Sprintf("s.session_date = $%d::DATE", i))
		args = append(args, f.Date)
		i++
	}
	if f.IsActive == "true" {
		conditions = append(conditions, "s.is_active = TRUE")
	} else if f.IsActive == "false" {
		conditions = append(conditions, "s.is_active = FALSE")
	}

	q := sessionBaseSelect + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY s.session_date DESC, s.start_time DESC"
	return r.scanSessions(ctx, q, args...)
}

// Update applies a partial update — only non-nil fields are changed.
func (r *SessionRepository) Update(ctx context.Context, shortID string, in models.UpdateSessionInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		add("name = $%d", *in.Name)
	}
	if in.SessionDate != nil {
		add("session_date = $%d::DATE", *in.SessionDate)
	}
	if in.StartTime != nil {
		add("start_time = $%d::TIME", *in.StartTime)
	}
	if in.EndTime != nil {
		add("end_time = $%d::TIME", *in.EndTime)
	}
	if in.MentorID != nil {
		add("mentor_id = $%d::UUID", *in.MentorID)
	}
	if in.Mode != nil {
		add("mode = $%d::session_mode", *in.Mode)
	}
	if in.MeetingPlatform != nil {
		add("meeting_platform = NULLIF($%d,'')::session_meeting_platform", *in.MeetingPlatform)
	}
	if in.SendConfirmationEmail != nil {
		add("send_confirmation_email = $%d", *in.SendConfirmationEmail)
	}
	if in.SessionReminderNotifications != nil {
		add("session_reminder_notifications = $%d", *in.SessionReminderNotifications)
	}
	if in.Topics != nil {
		add("topics = $%d", in.Topics)
	}
	if in.FeedbackFormShortID != nil {
		add("feedback_form_id = (SELECT id FROM feedback_forms WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.FeedbackFormShortID)
	}
	if in.BatchShortID != nil {
		add("batch_id = (SELECT id FROM batches WHERE short_id = $%d AND deleted_at IS NULL)", *in.BatchShortID)
	}
	if in.IsActive != nil {
		add("is_active = $%d", *in.IsActive)
	}
	if in.GenerateShareableLink != nil {
		if *in.GenerateShareableLink {
			add("generate_shareable_link = TRUE, share_token = COALESCE(share_token, $%d)", util.GenerateShortID()+util.GenerateShortID())
		} else {
			setClauses = append(setClauses, "generate_shareable_link = FALSE", "share_token = NULL")
		}
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE sessions SET %s WHERE short_id = $1 AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
	)
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateRecordingURL stores the Zoom recording link on the session matching
// zoomMeetingID, called from the recording.completed webhook. A no-op (not
// an error) if no session has that meeting ID — e.g. a Zoom meeting created
// outside this app, or the session was later deleted.
func (r *SessionRepository) UpdateRecordingURL(ctx context.Context, zoomMeetingID int64, recordingURL string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sessions SET recording_url = $1, updated_at = NOW() WHERE zoom_meeting_id = $2`,
		recordingURL, zoomMeetingID,
	)
	return err
}

// Delete soft-deletes a session.
func (r *SessionRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE sessions SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *SessionRepository) scanSessions(ctx context.Context, q string, args ...interface{}) ([]models.Session, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		if err := rows.Scan(
			&s.ID, &s.ShortID, &s.Name,
			&s.SessionDate, &s.StartTime, &s.EndTime,
			&s.MentorID, &s.MentorName,
			&s.Mode,
			&s.MeetingPlatform,
			&s.SendConfirmationEmail,
			&s.SessionReminderNotifications,
			&s.Topics,
			&s.GenerateShareableLink,
			&s.ShareToken,
			&s.ZoomMeetingID,
			&s.ZoomJoinURL,
			&s.ZoomStartURL,
			&s.RecordingURL,
			&s.FeedbackFormShortID,
			&s.FeedbackFormTitle,
			&s.SessionType,
			&s.BatchID, &s.BatchShortID, &s.BatchNumber,
			&s.IsActive,
			&s.CreatedBy,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
