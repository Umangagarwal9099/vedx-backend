package models

import "time"

// Assignment is a submission-based academic activity scoped to a batch,
// optionally tied to a module and/or the session it was given out in.
type Assignment struct {
	ID                     string     `json:"id"`
	ShortID                string     `json:"short_id"`
	Title                  string     `json:"title"`
	Description            string     `json:"description,omitempty"`
	BatchID                string     `json:"batch_id"`
	BatchShortID           string     `json:"batch_short_id"`
	BatchNumber            string     `json:"batch_number"`
	ModuleShortID          string     `json:"module_short_id,omitempty"`
	ModuleName             string     `json:"module_name,omitempty"`
	SessionShortID         string     `json:"session_short_id,omitempty"`
	SessionName            string     `json:"session_name,omitempty"`
	MaxMarks               int        `json:"max_marks"`
	Deadline               time.Time  `json:"deadline"`
	AllowedSubmissionTypes []string   `json:"allowed_submission_types"`
	AllowedFileFormats     []string   `json:"allowed_file_formats,omitempty"`
	MaxFileSizeMB          int        `json:"max_file_size_mb,omitempty"`
	LateSubmissionAllowed  bool       `json:"late_submission_allowed"`
	LatePenaltyPercent     int        `json:"late_penalty_percent,omitempty"`
	Status                 string     `json:"status"` // draft | active | closed
	CreatedBy              string     `json:"created_by"`
	CreatedByName          string     `json:"created_by_name,omitempty"`
	SubmissionCount        int        `json:"submission_count"`
	PendingReviewCount     int        `json:"pending_review_count"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	DeletedAt              *time.Time `json:"deleted_at,omitempty"`
}

// CreateAssignmentInput carries the fields required to create a new assignment.
// Pick batch_short_id from GET /batches. module_short_id and session_short_id are
// optional ties to the module/session the assignment was given out in. Set status
// to "draft" to save without notifying students, or "active" to publish immediately.
type CreateAssignmentInput struct {
	Title                  string    `json:"title"                    binding:"required"                                  example:"Java Collections Assignment"`
	Description            string    `json:"description"                                                                   example:"Implement a LinkedList from scratch and submit your source file."`
	BatchShortID           string    `json:"batch_short_id"           binding:"required"                                  example:"use GET /batches to pick a real short_id"`
	ModuleShortID          string    `json:"module_short_id"                                                               example:"use GET /modules to pick a real short_id"`
	SessionShortID         string    `json:"session_short_id"                                                              example:"use GET /sessions to pick a real short_id"`
	MaxMarks               int       `json:"max_marks"                binding:"required,min=1"                            example:"100"`
	Deadline               time.Time `json:"deadline"                 binding:"required"                                  example:"2026-07-15T23:59:00Z"`
	AllowedSubmissionTypes []string  `json:"allowed_submission_types" binding:"required,min=1,dive,oneof=link text file"   example:"[\"file\",\"link\"]"`
	AllowedFileFormats     []string  `json:"allowed_file_formats"                                                          example:"[\".pdf\",\".zip\"]"`
	MaxFileSizeMB          int       `json:"max_file_size_mb"                                                              example:"25"`
	LateSubmissionAllowed  bool      `json:"late_submission_allowed"                                                       example:"true"`
	LatePenaltyPercent     int       `json:"late_penalty_percent"                                                          example:"10"`
	Status                 string    `json:"status"                   binding:"omitempty,oneof=draft active"              example:"active"`
}

// UpdateAssignmentInput — all fields optional; send only what you want to change.
type UpdateAssignmentInput struct {
	Title                  *string    `json:"title"                    example:"Java Collections Assignment — Part 2"`
	Description            *string    `json:"description"               example:"Updated instructions."`
	BatchShortID           *string    `json:"batch_short_id"            example:"use GET /batches to pick a real short_id"`
	ModuleShortID          *string    `json:"module_short_id"           example:"use GET /modules to pick a real short_id"`
	SessionShortID         *string    `json:"session_short_id"          example:"use GET /sessions to pick a real short_id"`
	MaxMarks               *int       `json:"max_marks"                 example:"150"`
	Deadline               *time.Time `json:"deadline"                  example:"2026-07-20T23:59:00Z"`
	AllowedSubmissionTypes []string   `json:"allowed_submission_types"  example:"[\"file\"]"`
	AllowedFileFormats     []string   `json:"allowed_file_formats"      example:"[\".pdf\"]"`
	MaxFileSizeMB          *int       `json:"max_file_size_mb"          example:"50"`
	LateSubmissionAllowed  *bool      `json:"late_submission_allowed"   example:"false"`
	LatePenaltyPercent     *int       `json:"late_penalty_percent"      example:"0"`
	Status                 *string    `json:"status"                   binding:"omitempty,oneof=draft active closed"      example:"closed"`
}

// AssignmentFilter holds query params for GET /assignments.
type AssignmentFilter struct {
	BatchShortID string `form:"batch_short_id"`
	Status       string `form:"status"`
}

// AssignmentSubmission is a single student's submission for an assignment.
type AssignmentSubmission struct {
	ID                string     `json:"id"`
	ShortID           string     `json:"short_id"`
	AssignmentShortID string     `json:"assignment_short_id"`
	StudentID         string     `json:"student_id"`
	StudentName       string     `json:"student_name,omitempty"`
	StudentEmail      string     `json:"student_email,omitempty"`
	SubmissionType    string     `json:"submission_type"` // link | text | file
	Content           string     `json:"content,omitempty"`
	FileURL           string     `json:"file_url,omitempty"`
	Status            string     `json:"status"` // submitted | late | evaluated | resubmission_required
	Marks             *int       `json:"marks,omitempty"`
	Feedback          string     `json:"feedback,omitempty"`
	SubmittedAt       time.Time  `json:"submitted_at"`
	EvaluatedAt       *time.Time `json:"evaluated_at,omitempty"`
	EvaluatedBy       string     `json:"evaluated_by,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CreateAssignmentSubmissionInput carries a student's submission for an assignment.
// content holds the link URL or text answer depending on submission_type;
// file_url is set after uploading via POST /upload/assignment-file.
type CreateAssignmentSubmissionInput struct {
	SubmissionType string `json:"submission_type" binding:"required,oneof=link text file" example:"file"`
	Content        string `json:"content"                                                 example:"https://github.com/student/repo"`
	FileURL        string `json:"file_url"                                                example:"https://cdn.example.com/submission.pdf"`
}

// GradeSubmissionInput records a mentor's evaluation of a submission.
type GradeSubmissionInput struct {
	Marks    *int   `json:"marks"    binding:"required" example:"85"`
	Feedback string `json:"feedback"                    example:"Good structure, but missing edge-case handling."`
	Status   string `json:"status"   binding:"omitempty,oneof=evaluated resubmission_required" example:"evaluated"`
}
