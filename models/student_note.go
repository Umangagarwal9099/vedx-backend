package models

import "time"

// StudentNote is one staff-authored freeform note about a student.
type StudentNote struct {
	ID            string    `json:"id"`
	ShortID       string    `json:"short_id"`
	StudentUserID string    `json:"student_user_id"`
	Note          string    `json:"note"`
	CreatedBy     string    `json:"created_by"`
	CreatedByName string    `json:"created_by_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateStudentNoteInput carries a new note's text.
type CreateStudentNoteInput struct {
	Note string `json:"note" binding:"required" example:"Called about pending assignment, will submit by Friday."`
}
