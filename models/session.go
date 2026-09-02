package models

import "time"

type Session struct {
	ID                           string     `json:"id"`
	ShortID                      string     `json:"short_id"`
	Name                         string     `json:"name"`
	SessionDate                  string     `json:"session_date"`
	StartTime                    string     `json:"start_time"`
	EndTime                      string     `json:"end_time"`
	MentorID                     string     `json:"mentor_id"`
	MentorName                   string     `json:"mentor_name"`
	Mode                         string     `json:"mode"`
	MeetingPlatform              string     `json:"meeting_platform,omitempty"`
	SendConfirmationEmail        bool       `json:"send_confirmation_email"`
	SessionReminderNotifications bool       `json:"session_reminder_notifications"`
	Topics                       []string   `json:"topics"`
	GenerateShareableLink        bool       `json:"generate_shareable_link"`
	ShareToken                   string     `json:"share_token,omitempty"`
	ShareLink                    string     `json:"share_link,omitempty"`
	ZoomMeetingID                *int64     `json:"zoom_meeting_id,omitempty"`
	ZoomJoinURL                  string     `json:"zoom_join_url,omitempty"`
	ZoomStartURL                 string     `json:"zoom_start_url,omitempty"` // host token — stripped for students in the controller
	RecordingURL                 string     `json:"recording_url,omitempty"`
	RecordingVisible             bool       `json:"recording_visible"`
	RecordingAvailableFrom       *time.Time `json:"recording_available_from,omitempty"`
	IsDemo                       bool       `json:"is_demo"`
	FeedbackFormShortID          string     `json:"feedback_form_short_id,omitempty"`
	FeedbackFormTitle            string     `json:"feedback_form_title,omitempty"`
	SessionType                  string     `json:"session_type"`
	BatchID                      string     `json:"batch_id"`
	BatchShortID                 string     `json:"batch_short_id"`
	BatchNumber                  string     `json:"batch_number"`
	IsActive                     bool       `json:"is_active"`
	// Status is computed server-side (not stored) from is_active plus
	// session_date/start_time/end_time against the current time, in the
	// app's configured timezone — one of upcoming | live | completed |
	// cancelled. The frontend should render its Join/Watch Recording button
	// off this field instead of recomputing it from local wall-clock time,
	// so all clients agree regardless of the viewer's own timezone.
	Status string `json:"status"`
	// ZoomWarning is set only in the response to a Create/Update call where a
	// Zoom meeting was expected but couldn't be created — never persisted or
	// read back, purely a one-shot signal to the admin/mentor who made the call.
	ZoomWarning string     `json:"zoom_warning,omitempty"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// CreateSessionInput carries the fields required to schedule a new session.
// meeting_platform is required when mode = "online". session_type currently
// only supports "batch" — pick the target via batch_short_id (use GET /batches
// to list available batches).
type CreateSessionInput struct {
	Name                         string   `json:"name"                            binding:"required"                              example:"React Hooks Deep Dive"`
	SessionDate                  string   `json:"session_date"                    binding:"required"                              example:"2025-09-15"`
	StartTime                    string   `json:"start_time"                      binding:"required"                              example:"10:00"`
	EndTime                      string   `json:"end_time"                        binding:"required"                              example:"11:30"`
	MentorID                     string   `json:"mentor_id"                       binding:"required"                              example:"use GET /mentors to pick a real ID"`
	Mode                         string   `json:"mode"                            binding:"required,oneof=online offline"         example:"online"`
	MeetingPlatform              string   `json:"meeting_platform"                binding:"omitempty,oneof=zoom google_meet teams" example:"zoom"`
	SendConfirmationEmail        bool     `json:"send_confirmation_email"                                                          example:"true"`
	SessionReminderNotifications bool     `json:"session_reminder_notifications"                                                   example:"true"`
	Topics                       []string `json:"topics"                                                                           example:"[\"hooks\",\"state management\"]"`
	GenerateShareableLink        bool     `json:"generate_shareable_link"                                                          example:"true"`
	FeedbackFormShortID          string   `json:"feedback_form_short_id"                                                           example:"A3F72C1D"`
	SessionType                  string   `json:"session_type"                    binding:"required,oneof=batch"                   example:"batch"`
	BatchShortID                 string   `json:"batch_short_id"                  binding:"required"                              example:"use GET /batches to pick a real short_id"`
}

// UpdateSessionInput — all fields optional; send only what you want to change.
type UpdateSessionInput struct {
	Name                         *string    `json:"name"                            example:"React Hooks Deep Dive — Part 2"`
	SessionDate                  *string    `json:"session_date"                    example:"2025-09-22"`
	StartTime                    *string    `json:"start_time"                      example:"10:30"`
	EndTime                      *string    `json:"end_time"                        example:"12:00"`
	MentorID                     *string    `json:"mentor_id"                       example:"use GET /mentors to pick a real ID"`
	Mode                         *string    `json:"mode"                            example:"offline"`
	MeetingPlatform              *string    `json:"meeting_platform"                example:"google_meet"`
	SendConfirmationEmail        *bool      `json:"send_confirmation_email"         example:"false"`
	SessionReminderNotifications *bool      `json:"session_reminder_notifications"  example:"false"`
	Topics                       []string   `json:"topics"                          example:"[\"hooks\"]"`
	GenerateShareableLink        *bool      `json:"generate_shareable_link"         example:"false"`
	FeedbackFormShortID          *string    `json:"feedback_form_short_id"          example:"B4G83D2E"`
	BatchShortID                 *string    `json:"batch_short_id"                  example:"use GET /batches to pick a real short_id"`
	IsActive                     *bool      `json:"is_active"                       example:"false"`
	RecordingVisible             *bool      `json:"recording_visible"              example:"false"`
	RecordingAvailableFrom       *time.Time `json:"recording_available_from"       example:"2026-08-01T00:00:00Z"`
}

// ZoomMeetingInfo carries a created/updated Zoom meeting's identifiers from
// the controller (which talks to Zoom) down to the repository (which persists them).
type ZoomMeetingInfo struct {
	ID       int64
	JoinURL  string
	StartURL string
}

// SessionJoinInfo is the public, unauthenticated view of a session resolved
// by share token or short ID. It deliberately omits ZoomStartURL (a host token).
type SessionJoinInfo struct {
	Name            string `json:"name"`
	SessionDate     string `json:"session_date"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	MentorName      string `json:"mentor_name"`
	Mode            string `json:"mode"`
	MeetingPlatform string `json:"meeting_platform,omitempty"`
	ZoomJoinURL     string `json:"zoom_join_url,omitempty"`
	Status          string `json:"status,omitempty"`
	Completed       bool   `json:"completed"`
	Message         string `json:"message,omitempty"`
	JoinURL         string `json:"join_url,omitempty"`
}

// RecordingListItem is one recording, as returned by the batch-recordings
// endpoint. Source is "session" for a recording auto-attached to a live
// Zoom session, or "upload" for a video an admin/mentor uploaded directly
// via POST /batches/{short_id}/recordings/upload. SessionShortID is only
// set for the former; RecordingShortID (the batch_recordings row) only for
// the latter — use RecordingShortID with DELETE
// /batches/{short_id}/recordings/{recording_short_id} to remove an upload.
type RecordingListItem struct {
	SessionShortID   string `json:"session_short_id"`
	RecordingShortID string `json:"recording_short_id,omitempty"`
	Source           string `json:"source"`
	Name             string `json:"name"`
	SessionDate      string `json:"session_date"`
	RecordingURL     string `json:"recording_url"`
	IsDemo           bool   `json:"is_demo"`
}

// BatchRecordingsResponse is the student-facing view of a batch's session
// recordings. When FeesPaid is false, Recordings is always empty and Message
// explains why — the frontend shouldn't need to infer that itself.
type BatchRecordingsResponse struct {
	FeesPaid   bool                `json:"fees_paid"`
	Message    string              `json:"message,omitempty"`
	Recordings []RecordingListItem `json:"recordings"`
}

// SessionFilter holds query params for GET /sessions.
type SessionFilter struct {
	BatchShortID string `form:"batch_short_id"`
	MentorID     string `form:"mentor_id"`
	Date         string `form:"date"`
	IsActive     string `form:"is_active"` // "true" | "false" | ""
}
