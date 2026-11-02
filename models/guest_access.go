package models

import "time"

// GuestBatchAccess is a batch a student can view the live sessions and
// recordings of without being enrolled there — no assignments, progress,
// certificate, or fee requirement of its own. Returned to the student so
// the frontend can list it clearly separate from their real enrollments.
type GuestBatchAccess struct {
	BatchShortID string    `json:"batch_short_id"`
	BatchNumber  string    `json:"batch_number"`
	CourseName   string    `json:"course_name"`
	StartDate    string    `json:"start_date,omitempty"`
	EndDate      string    `json:"end_date,omitempty"`
	GrantedAt    time.Time `json:"granted_at"`
}

// GuestAccessStudent is one student currently holding guest access to a
// batch — the admin-facing view of who's on a batch's guest list.
type GuestAccessStudent struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	GrantedAt time.Time `json:"granted_at"`
}

// GrantGuestAccessInput is the payload for POST /batches/{short_id}/guest-access.
type GrantGuestAccessInput struct {
	StudentIDs []string `json:"student_ids" binding:"required,min=1"`
}
