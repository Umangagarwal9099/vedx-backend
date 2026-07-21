package models

import "time"

// ExamAttempt is one student's attempt at a timed assessment.
type ExamAttempt struct {
	ID                string `json:"id"`
	ShortID           string `json:"short_id"`
	AssessmentShortID string `json:"assessment_short_id"`
	// AssessmentName/BatchShortID/BatchNumber are only populated by the
	// cross-assessment workspace listing (FindAllAttemptsForMentor).
	AssessmentName string     `json:"assessment_name,omitempty"`
	BatchShortID   string     `json:"batch_short_id,omitempty"`
	BatchNumber    string     `json:"batch_number,omitempty"`
	StudentID      string     `json:"student_id"`
	StudentName    string     `json:"student_name,omitempty"`
	AttemptNumber  int        `json:"attempt_number"`
	StartedAt      time.Time  `json:"started_at"`
	EndsAt         *time.Time `json:"ends_at,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`
	AutoSubmitted  bool       `json:"auto_submitted"`
	Status         string     `json:"status"` // in_progress | submitted | evaluated | cancelled
	// SubmissionType records how the attempt ended: manual | timer_expired |
	// violation | admin_closed | exam_window_closed. Empty while in_progress.
	SubmissionType string `json:"submission_type,omitempty"`
	TotalScore     *int   `json:"total_score,omitempty"`
	MaxScore       int    `json:"max_score"`
	Passed         *bool  `json:"passed,omitempty"`
	// ViolationCount is computed at read time from exam_attempt_violations —
	// not stored redundantly on the attempt row.
	ViolationCount int `json:"violation_count,omitempty"`
}

// ExamAccessStatus is the single source of truth the student frontend must
// render the Start/Resume/Closed/Missed/Submitted state from, instead of
// separately recomputing exam-window/attempt-status business logic. See the
// "fix the current exam issue" requirement — the UI renders off this, full stop.
type ExamAccessStatus struct {
	CanStart       bool       `json:"can_start"`
	CanResume      bool       `json:"can_resume"`
	DisplayStatus  string     `json:"display_status"`
	Reason         string     `json:"reason,omitempty"`
	AttemptStatus  string     `json:"attempt_status,omitempty"`
	AttemptShortID string     `json:"attempt_short_id,omitempty"`
	StartsAt       *time.Time `json:"starts_at,omitempty"`
	EndsAt         *time.Time `json:"ends_at,omitempty"`
	AttemptNumber  int        `json:"attempt_number,omitempty"`
	MaxAttempts    int        `json:"max_attempts,omitempty"`
	ResultsVisible bool       `json:"results_visible"`
}

// ExamViolation is one recorded proctoring-integrity event during an attempt.
type ExamViolation struct {
	ShortID        string    `json:"short_id"`
	AttemptShortID string    `json:"attempt_short_id"`
	StudentID      string    `json:"student_id"`
	ViolationType  string    `json:"violation_type"`
	ViolationTime  time.Time `json:"violation_time"`
	WarningNumber  int       `json:"warning_number"`
	BrowserInfo    string    `json:"browser_info,omitempty"`
	ActionTaken    string    `json:"action_taken"` // warned | auto_submitted
}

// RecordViolationInput reports one proctoring-integrity event from the
// exam-taking page.
type RecordViolationInput struct {
	ViolationType string `json:"violation_type" binding:"required,oneof=tab_switched window_blurred fullscreen_exited page_refreshed browser_back_attempt multiple_tab_attempt multiple_device_attempt network_disconnected exam_window_closed" example:"tab_switched"`
	BrowserInfo   string `json:"browser_info"                                                                                                                                                                                        example:"Mozilla/5.0 ..."`
}

// RecordViolationResponse tells the frontend what happened as a result of
// this violation — whether it was just a warning or triggered an auto-submit.
type RecordViolationResponse struct {
	WarningNumber int    `json:"warning_number"`
	ActionTaken   string `json:"action_taken"` // warned | auto_submitted
	AttemptStatus string `json:"attempt_status"`
}

// ReattemptGrant records an admin/mentor granting a student an extra attempt
// beyond the assessment's normal max_attempts. Append-only — never overwrites
// a past attempt's data.
type ReattemptGrant struct {
	ShortID           string    `json:"short_id"`
	AssessmentShortID string    `json:"assessment_short_id"`
	StudentID         string    `json:"student_id"`
	GrantedBy         string    `json:"granted_by"`
	Reason            string    `json:"reason,omitempty"`
	NewAttemptNumber  int       `json:"new_attempt_number"`
	CreatedAt         time.Time `json:"created_at"`
}

// GrantReattemptInput is the request body for granting a student an extra attempt.
type GrantReattemptInput struct {
	StudentID string `json:"student_id" binding:"required" example:"use GET /assessments/{id}/attempts to find a student_id"`
	Reason    string `json:"reason"                         example:"Missed the window due to a technical issue."`
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
