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

// LeaderboardEntry is one student's row on the coding-practice leaderboard —
// ranked by distinct problems solved (accepted submissions), not by academic
// score. Ties (equal SolvedCount + AcceptedSubmissions) share the same rank,
// standard competition-ranking style (1, 2, 2, 4).
type LeaderboardEntry struct {
	Rank                int        `json:"rank"`
	StudentID           string     `json:"student_id"`
	StudentName         string     `json:"student_name"`
	// CollegeID is only meaningful to super_admin (unscoped view spans every
	// college) — used client-side to render/filter by college, the same way
	// the Learners list does. Always empty for a college-scoped caller since
	// every row is already their own college.
	CollegeID           string     `json:"college_id,omitempty"`
	SolvedCount         int        `json:"solved_count"`
	AcceptedSubmissions int        `json:"accepted_submissions"`
	TotalSubmissions    int        `json:"total_submissions"`
	LastSolvedAt        *time.Time `json:"last_solved_at,omitempty"`
}
