package models

import "time"

// Assessment is a content entity that can be either static (a downloadable
// file/quiz with manually-declared results — the original behavior) or a real
// timed exam backed by questions from the question bank, attempts, and
// auto-grading, depending on which optional fields are set. Setting
// batch_id scopes it to one batch; leaving it unset makes it globally visible.
type Assessment struct {
	ID                        string     `json:"id"`
	ShortID                   string     `json:"short_id"`
	Name                      string     `json:"name"`
	Description               string     `json:"description,omitempty"`
	Thumbnail                 string     `json:"thumbnail,omitempty"`
	FileURL                   string     `json:"file_url,omitempty"`
	GeneralInstructions       string     `json:"general_instructions,omitempty"`
	TotalMarks                int        `json:"total_marks"`
	PassingPercentage         float64    `json:"passing_percentage"`
	ResultDeclaration         string     `json:"result_declaration"`
	ResultDisplay             string     `json:"result_display"`
	AllowAttemptsAfterPassing bool       `json:"allow_attempts_after_passing"`
	BatchShortID              string     `json:"batch_short_id,omitempty"`
	BatchNumber               string     `json:"batch_number,omitempty"`
	StartAt                   *time.Time `json:"start_at,omitempty"`
	EndAt                     *time.Time `json:"end_at,omitempty"`
	DurationMinutes           *int       `json:"duration_minutes,omitempty"`
	MaxAttempts               int        `json:"max_attempts"`
	NegativeMarking           bool       `json:"negative_marking"`
	RandomizeQuestions        bool       `json:"randomize_questions"`
	RandomizeOptions          bool       `json:"randomize_options"`
	AutoSubmit                bool       `json:"auto_submit"`
	ShowCorrectAnswers        bool       `json:"show_correct_answers"`
	RequiresProctoring        bool       `json:"requires_proctoring"`
	QuestionCount             int        `json:"question_count"`
	IsActive                  bool       `json:"is_active"`
	CancelledAt               *time.Time `json:"cancelled_at,omitempty"`
	CancelledBy               string     `json:"cancelled_by,omitempty"`
	// ResultPublishedAt gates student-visible marks/grade/feedback — set only
	// by the explicit "publish results" action, never by grading itself.
	ResultPublishedAt     *time.Time `json:"result_published_at,omitempty"`
	CloseOnTabSwitch      bool       `json:"close_on_tab_switch"`
	CloseOnWindowBlur     bool       `json:"close_on_window_blur"`
	CloseOnFullscreenExit bool       `json:"close_on_fullscreen_exit"`
	AllowedWarningCount   int        `json:"allowed_warning_count"`
	AutoSubmitOnViolation bool       `json:"auto_submit_on_violation"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
}

// ResultsVisibleWith reports whether this assessment's marks/grade/feedback
// may be shown to students — either the result has been explicitly
// published, or the assessment declares automatic (immediate) results.
// publishedAt/configFound come from AssessmentRepository.GetSecurityConfig,
// a query isolated from the assessment's own base select (see that
// repository for why) — when configFound is false (that migration hasn't
// been applied yet), this falls back to "automatic" semantics, i.e. results
// stay visible exactly as they were before result-publication gating existed.
func (a *Assessment) ResultsVisibleWith(publishedAt *time.Time, configFound bool) bool {
	if a.ResultDeclaration == "automatic" {
		return true
	}
	if !configFound {
		return true
	}
	return publishedAt != nil
}

type CreateAssessmentInput struct {
	Name                      string     `json:"name"                         binding:"required"                                         example:"Golang Fundamentals Quiz"`
	Description               string     `json:"description"                                                                             example:"Test your knowledge of Go basics."`
	Thumbnail                 string     `json:"thumbnail"                                                                               example:"https://cdn.example.com/thumbnail.jpg"`
	FileURL                   string     `json:"file_url"                                                                                example:"https://cdn.example.com/file.pdf"`
	GeneralInstructions       string     `json:"general_instructions"                                                                    example:"Read all questions carefully before answering."`
	TotalMarks                int        `json:"total_marks"                  binding:"required,min=1"                                   example:"100"`
	PassingPercentage         float64    `json:"passing_percentage"           binding:"required,min=0,max=100"                           example:"60"`
	ResultDeclaration         string     `json:"result_declaration"           binding:"required,oneof=manual automatic"                  example:"automatic"`
	ResultDisplay             string     `json:"result_display"               binding:"required,oneof=marks_and_status status_only"      example:"marks_and_status"`
	AllowAttemptsAfterPassing bool       `json:"allow_attempts_after_passing"                                                            example:"false"`
	BatchShortID              string     `json:"batch_short_id"                                                                          example:"use GET /batches to pick a real short_id, or omit for global visibility"`
	StartAt                   *time.Time `json:"start_at"                                                                                example:"2026-07-15T09:00:00Z"`
	EndAt                     *time.Time `json:"end_at"                                                                                  example:"2026-07-15T11:00:00Z"`
	DurationMinutes           *int       `json:"duration_minutes"                                                                        example:"60"`
	MaxAttempts               int        `json:"max_attempts"                                                                            example:"1"`
	NegativeMarking           bool       `json:"negative_marking"                                                                        example:"false"`
	RandomizeQuestions        bool       `json:"randomize_questions"                                                                     example:"true"`
	RandomizeOptions          bool       `json:"randomize_options"                                                                       example:"true"`
	AutoSubmit                bool       `json:"auto_submit"                                                                             example:"true"`
	ShowCorrectAnswers        bool       `json:"show_correct_answers"                                                                    example:"false"`
	RequiresProctoring        bool       `json:"requires_proctoring"                                                                     example:"false"`
	CloseOnTabSwitch          bool       `json:"close_on_tab_switch"                                                                     example:"false"`
	CloseOnWindowBlur         bool       `json:"close_on_window_blur"                                                                    example:"false"`
	CloseOnFullscreenExit     bool       `json:"close_on_fullscreen_exit"                                                                example:"false"`
	AllowedWarningCount       int        `json:"allowed_warning_count"                                                                   example:"1"`
	AutoSubmitOnViolation     bool       `json:"auto_submit_on_violation"                                                                example:"true"`
}

type UpdateAssessmentInput struct {
	Name                      *string    `json:"name"                         example:"Updated Assessment Name"`
	Description               *string    `json:"description"                  example:"Updated description."`
	Thumbnail                 *string    `json:"thumbnail"                    example:"https://cdn.example.com/new-thumb.jpg"`
	FileURL                   *string    `json:"file_url"                     example:"https://cdn.example.com/new-file.pdf"`
	GeneralInstructions       *string    `json:"general_instructions"         example:"Updated instructions."`
	TotalMarks                *int       `json:"total_marks"                  example:"150"`
	PassingPercentage         *float64   `json:"passing_percentage"           example:"70"`
	ResultDeclaration         *string    `json:"result_declaration"           example:"manual"`
	ResultDisplay             *string    `json:"result_display"               example:"status_only"`
	AllowAttemptsAfterPassing *bool      `json:"allow_attempts_after_passing" example:"true"`
	BatchShortID              *string    `json:"batch_short_id"               example:""`
	StartAt                   *time.Time `json:"start_at"                     example:"2026-07-16T09:00:00Z"`
	EndAt                     *time.Time `json:"end_at"                       example:"2026-07-16T11:00:00Z"`
	DurationMinutes           *int       `json:"duration_minutes"             example:"90"`
	MaxAttempts               *int       `json:"max_attempts"                 example:"2"`
	NegativeMarking           *bool      `json:"negative_marking"             example:"true"`
	RandomizeQuestions        *bool      `json:"randomize_questions"          example:"false"`
	RandomizeOptions          *bool      `json:"randomize_options"            example:"false"`
	AutoSubmit                *bool      `json:"auto_submit"                  example:"false"`
	ShowCorrectAnswers        *bool      `json:"show_correct_answers"         example:"true"`
	RequiresProctoring        *bool      `json:"requires_proctoring"          example:"true"`
	IsActive                  *bool      `json:"is_active"                    example:"false"`
	CloseOnTabSwitch          *bool      `json:"close_on_tab_switch"          example:"true"`
	CloseOnWindowBlur         *bool      `json:"close_on_window_blur"         example:"true"`
	CloseOnFullscreenExit     *bool      `json:"close_on_fullscreen_exit"     example:"true"`
	AllowedWarningCount       *int       `json:"allowed_warning_count"        example:"1"`
	AutoSubmitOnViolation     *bool      `json:"auto_submit_on_violation"     example:"true"`
}

type AssessmentFilter struct {
	Name         string `form:"name"`
	Description  string `form:"description"`
	IsActive     string `form:"is_active"`
	BatchShortID string `form:"batch_short_id"`
}
