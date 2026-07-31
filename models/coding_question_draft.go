package models

import "time"

// CodingQuestionDraft is a student's latest in-progress code for one
// coding_questions row, in one language — separate from Submission (which
// stays write-once/final-result-only). One row per (user, question,
// language); autosave upserts it repeatedly as the student types.
type CodingQuestionDraft struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	QuestionID string    `json:"question_id"`
	Language   string    `json:"language"`
	Code       string    `json:"code"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SaveDraftInput carries an autosave tick. Code is deliberately allowed to
// be empty — a student clearing their editor is a valid state to persist.
type SaveDraftInput struct {
	Language string `json:"language" binding:"required"`
	Code     string `json:"code"`
}
