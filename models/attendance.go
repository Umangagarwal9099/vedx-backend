package models

import "time"

const (
	AttendancePresent = "present"
	AttendanceAbsent  = "absent"
	AttendanceLate    = "late"
	AttendanceExcused = "excused"
)

// SessionAttendance is one student's marked attendance for one session.
type SessionAttendance struct {
	ID             string     `json:"id"`
	ShortID        string     `json:"short_id"`
	SessionID      string     `json:"session_id"`
	SessionShortID string     `json:"session_short_id,omitempty"`
	SessionName    string     `json:"session_name,omitempty"`
	SessionDate    string     `json:"session_date,omitempty"`
	BatchID        string     `json:"batch_id"`
	BatchShortID   string     `json:"batch_short_id,omitempty"`
	BatchNumber    string     `json:"batch_number,omitempty"`
	StudentID      string     `json:"student_id"`
	StudentName    string     `json:"student_name,omitempty"`
	Status         string     `json:"status"`
	MarkedBy       string     `json:"marked_by,omitempty"`
	MarkedAt       *time.Time `json:"marked_at,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AttendanceRecordInput is one student's status within a bulk-mark request.
type AttendanceRecordInput struct {
	StudentID string `json:"student_id" binding:"required"`
	Status    string `json:"status"     binding:"required,oneof=present absent late excused"`
	Notes     string `json:"notes"`
}

// BulkMarkAttendanceInput carries every student's attendance for one session
// in a single request — this is what the "take attendance" screen submits.
type BulkMarkAttendanceInput struct {
	Records []AttendanceRecordInput `json:"records" binding:"required,dive"`
}

// MarkAttendanceInput updates a single student's attendance for one session.
type MarkAttendanceInput struct {
	Status string `json:"status" binding:"required,oneof=present absent late excused"`
	Notes  string `json:"notes"`
}

// StudentAttendanceSummary is a rollup of one student's attendance — either
// within a single batch, or (when BatchShortID is empty) across all of them.
type StudentAttendanceSummary struct {
	StudentID      string  `json:"student_id"`
	StudentName    string  `json:"student_name,omitempty"`
	BatchID        string  `json:"batch_id,omitempty"`
	BatchShortID   string  `json:"batch_short_id,omitempty"`
	BatchNumber    string  `json:"batch_number,omitempty"`
	TotalSessions  int     `json:"total_sessions"`
	Present        int     `json:"present"`
	Absent         int     `json:"absent"`
	Late           int     `json:"late"`
	Excused        int     `json:"excused"`
	AttendancePct  float64 `json:"attendance_percentage"`
}

// SessionAttendanceReport is one session's attendance rollup — the row shape
// behind the cross-batch "attendance reports" admin view.
type SessionAttendanceReport struct {
	SessionShortID string  `json:"session_short_id"`
	SessionName    string  `json:"session_name"`
	SessionDate    string  `json:"session_date"`
	BatchShortID   string  `json:"batch_short_id"`
	BatchNumber    string  `json:"batch_number"`
	Total          int     `json:"total"`
	Present        int     `json:"present"`
	Absent         int     `json:"absent"`
	Late           int     `json:"late"`
	Excused        int     `json:"excused"`
	AttendancePct  float64 `json:"attendance_percentage"`
}
