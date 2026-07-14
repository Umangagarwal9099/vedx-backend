package models

import "time"

// Enrollment statuses — see StudentEnrollment.Status.
const (
	EnrollmentInvited     = "invited"
	EnrollmentEnrolled    = "enrolled"
	EnrollmentActive      = "active"
	EnrollmentOnHold      = "on_hold"
	EnrollmentCompleted   = "completed"
	EnrollmentDropped     = "dropped"
	EnrollmentTransferred = "transferred"
	EnrollmentRemoved     = "removed"
)

// StudentEnrollment is one student's enrollment in one batch of one course.
// A student can hold several of these at once — one per course they're
// taking — but should only ever have one *active* enrollment per course at a
// time. That exclusivity is enforced in application code
// (EnrollmentRepository.HasActiveEnrollmentInCourse), not by a DB constraint,
// since "active" spans several status values (invited/enrolled/active/on_hold).
type StudentEnrollment struct {
	ID                   string     `json:"id"`
	ShortID              string     `json:"short_id"`
	StudentID            string     `json:"student_id"`
	StudentName          string     `json:"student_name,omitempty"`
	CourseID             string     `json:"course_id"`
	CourseShortID        string     `json:"course_short_id,omitempty"`
	CourseName           string     `json:"course_name,omitempty"`
	BatchID              string     `json:"batch_id"`
	BatchShortID         string     `json:"batch_short_id,omitempty"`
	BatchNumber          string     `json:"batch_number,omitempty"`
	EnrollmentDate       time.Time  `json:"enrollment_date"`
	EnrollmentType       string     `json:"enrollment_type"` // direct | invited | transferred
	Status               string     `json:"status"`
	AccessStartDate      *time.Time `json:"access_start_date,omitempty"`
	AccessEndDate        *time.Time `json:"access_end_date,omitempty"`
	IsLateEnrollment     bool       `json:"is_late_enrollment"`
	CompletionPercentage *float64   `json:"completion_percentage,omitempty"`
	FinalScore           *float64   `json:"final_score,omitempty"`
	FinalRank            *int       `json:"final_rank,omitempty"`
	CreatedBy            string     `json:"created_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// UpdateEnrollmentStatusInput changes a student's enrollment status within a
// batch (e.g. on_hold, completed, dropped). This does not touch batch
// membership itself — use DELETE /batches/{id}/students/{user_id} to actually
// remove someone from the roster.
type UpdateEnrollmentStatusInput struct {
	Status string `json:"status" binding:"required,oneof=invited enrolled active on_hold completed dropped transferred removed" example:"on_hold"`
}

// TransferStudentInput moves a student from their current batch to a
// different batch of the SAME course (schedule conflicts, batch merges,
// etc). The old enrollment is marked "transferred" and kept for history; a
// new "active" enrollment is created for the destination batch.
type TransferStudentInput struct {
	ToBatchShortID string `json:"to_batch_short_id" binding:"required" example:"use GET /batches to pick a real short_id"`
}
