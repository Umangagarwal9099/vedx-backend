package models

import "time"

// ExamAttempt is one student's attempt at a timed assessment.
type ExamAttempt struct {
	ID                string     `json:"id"`
	ShortID           string     `json:"short_id"`
	AssessmentShortID string     `json:"assessment_short_id"`
	StudentID         string     `json:"student_id"`
	StudentName       string     `json:"student_name,omitempty"`
	AttemptNumber     int        `json:"attempt_number"`
	StartedAt         time.Time  `json:"started_at"`
	SubmittedAt       *time.Time `json:"submitted_at,omitempty"`
	AutoSubmitted     bool       `json:"auto_submitted"`
	Status            string     `json:"status"` // in_progress | submitted | evaluated
	TotalScore        *int       `json:"total_score,omitempty"`
	MaxScore          int        `json:"max_score"`
	Passed            *bool      `json:"passed,omitempty"`
}

// StudentAnswer is one answer within an attempt.
type StudentAnswer struct {
	QuestionShortID   string   `json:"question_short_id"`
	SelectedOptionIDs []string `json:"selected_option_ids,omitempty"`
	TextAnswer        string   `json:"text_answer,omitempty"`
	IsCorrect         *bool    `json:"is_correct,omitempty"`
	MarksAwarded      *int     `json:"marks_awarded,omitempty"`
	Feedback          string   `json:"feedback,omitempty"`
}

// SubmitAnswerInput autosaves a single answer within an in-progress attempt.
type SubmitAnswerInput struct {
	QuestionShortID   string   `json:"question_short_id"   binding:"required" example:"A3F72C1D"`
	SelectedOptionIDs []string `json:"selected_option_ids"                    example:"[\"a\"]"`
	TextAnswer        string   `json:"text_answer"                            example:"Goroutine"`
}

// GradeAnswerInput records a mentor's manual grade for one answer (descriptive/coding types).
type GradeAnswerInput struct {
	MarksAwarded int    `json:"marks_awarded" binding:"required" example:"8"`
	Feedback     string `json:"feedback"                         example:"Good explanation, minor detail missing."`
}

// AttemptQuestionView is what the student sees while taking (or reviewing) an
// exam — correct answers/explanation are only populated when the attempt is
// finished and the assessment allows revealing them.
type AttemptQuestionView struct {
	ShortID               string           `json:"short_id"`
	QuestionType          string           `json:"question_type"`
	QuestionText          string           `json:"question_text,omitempty"`
	Options               []QuestionOption `json:"options,omitempty"`
	Marks                 int              `json:"marks"`
	CodingQuestionShortID string           `json:"coding_question_short_id,omitempty"`
	CodingQuestionTitle   string           `json:"coding_question_title,omitempty"`
	MyAnswer              *StudentAnswer   `json:"my_answer,omitempty"`
	CorrectOptionIDs      []string         `json:"correct_option_ids,omitempty"`
	CorrectText           string           `json:"correct_text,omitempty"`
	Explanation           string           `json:"explanation,omitempty"`
}

// AttemptDetail is a full attempt with its (possibly sanitized) question list.
type AttemptDetail struct {
	ExamAttempt
	Questions []AttemptQuestionView `json:"questions"`
}
