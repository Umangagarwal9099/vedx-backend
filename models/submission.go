package models

import "time"

type SubmissionStatus string

const (
	SubmissionAccepted     SubmissionStatus = "accepted"
	SubmissionWrongAnswer  SubmissionStatus = "wrong_answer"
	SubmissionRuntimeError SubmissionStatus = "runtime_error"
	SubmissionCompileError SubmissionStatus = "compile_error"
)

type Submission struct {
	ID               string           `json:"id"`
	UserID           string           `json:"user_id"`
	QuestionShortID  string           `json:"question_short_id"`
	Language         string           `json:"language"`
	Code             string           `json:"code"`
	Status           SubmissionStatus `json:"status"`
	PassedTests      int              `json:"passed_tests"`
	TotalTests       int              `json:"total_tests"`
	CreatedAt        time.Time        `json:"created_at"`
}

// SubmissionView is the enriched shape returned to admin/mentor — includes student and question info.
type SubmissionView struct {
	ID              string           `json:"id"`
	UserID          string           `json:"user_id"`
	StudentName     string           `json:"student_name"`
	StudentEmail    string           `json:"student_email"`
	QuestionShortID string           `json:"question_short_id"`
	QuestionTitle   string           `json:"question_title"`
	Language        string           `json:"language"`
	Code            string           `json:"code"`
	Status          SubmissionStatus `json:"status"`
	PassedTests     int              `json:"passed_tests"`
	TotalTests      int              `json:"total_tests"`
	CreatedAt       time.Time        `json:"created_at"`
}

type CreateSubmissionInput struct {
	QuestionShortID string           `json:"question_short_id" binding:"required"`
	Language        string           `json:"language"          binding:"required"`
	Code            string           `json:"code"              binding:"required"`
	Status          SubmissionStatus `json:"status"            binding:"required,oneof=accepted wrong_answer runtime_error compile_error"`
	PassedTests     int              `json:"passed_tests"`
	TotalTests      int              `json:"total_tests"`
}
