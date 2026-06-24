package models

import "time"

// ── Feedback Form ─────────────────────────────────────────────────────────────

type FeedbackForm struct {
	ID          string     `json:"id"`
	ShortID     string     `json:"short_id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	FormType    string     `json:"form_type"`
	Template    string     `json:"template"`
	IsActive    bool       `json:"is_active"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// FeedbackFormWithQuestions is returned by GET /feedback-forms/:short_id.
type FeedbackFormWithQuestions struct {
	FeedbackForm
	Questions []FeedbackFormQuestion `json:"questions"`
}

// ValidTemplatesFor maps each form_type to its allowed templates.
var ValidTemplatesFor = map[string]map[string]bool{
	"session_feedback": {"blank_form": true, "trainer_performance": true, "course_content": true, "overall_satisfaction": true},
	"link_to_course":   {"blank_form": true, "content_rating": true, "csat": true, "course_rating": true},
	"general_survey":   {"blank_form": true},
}

// TemplateTitles maps a template value to its human-readable title.
var TemplateTitles = map[string]string{
	"blank_form":           "Blank Form",
	"trainer_performance":  "Trainer Performance",
	"course_content":       "Course Content",
	"overall_satisfaction": "Overall Satisfaction",
	"content_rating":       "Content Rating",
	"csat":                 "CSAT",
	"course_rating":        "Course Rating",
}

type CreateFeedbackFormInput struct {
	FormType string `json:"form_type" binding:"required,oneof=session_feedback link_to_course general_survey"                                                                    example:"session_feedback"`
	Template string `json:"template"  binding:"required,oneof=blank_form trainer_performance course_content overall_satisfaction content_rating csat course_rating"              example:"trainer_performance"`
}

// UpdateFeedbackFormInput — all fields optional; send only what you want to change.
// Set is_active to false to disable the form, true to re-enable it.
type UpdateFeedbackFormInput struct {
	Title       *string `json:"title"       example:"Custom Form Title"`
	Description *string `json:"description" example:"Optional description"`
	IsActive    *bool   `json:"is_active"   example:"false"`
}

// ── Feedback Form Question ────────────────────────────────────────────────────

type FeedbackFormQuestion struct {
	ID           string    `json:"id"`
	ShortID      string    `json:"short_id"`
	FormID       string    `json:"form_id"`
	QuestionType string    `json:"question_type"`
	QuestionText string    `json:"question_text"`
	IsRequired   bool      `json:"is_required"`
	OrderIndex   int       `json:"order_index"`
	ScaleMin     *int      `json:"scale_min,omitempty"`
	ScaleMax     *int      `json:"scale_max,omitempty"`
	StartLabel   *string   `json:"start_label,omitempty"`
	EndLabel     *string   `json:"end_label,omitempty"`
	Options      []string  `json:"options,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateFeedbackFormQuestionInput struct {
	QuestionType string   `json:"question_type" binding:"required,oneof=session_rating trainer_rating single_choice multiple_choice star_rating linear_scale date number short_answer long_answer" example:"star_rating"`
	QuestionText string   `json:"question_text" binding:"required"                                                                                                                                   example:"Rate the trainer's performance"`
	IsRequired   bool     `json:"is_required"                                                                                                                                                        example:"true"`
	OrderIndex   int      `json:"order_index"                                                                                                                                                        example:"1"`
	ScaleMin     *int     `json:"scale_min"                                                                                                                                                          example:"1"`
	ScaleMax     *int     `json:"scale_max"                                                                                                                                                          example:"5"`
	StartLabel   *string  `json:"start_label"                                                                                                                                                        example:"Poor"`
	EndLabel     *string  `json:"end_label"                                                                                                                                                          example:"Excellent"`
	Options      []string `json:"options"                                                                                                                                                            example:"Option A,Option B,Option C"`
}

// UpdateFeedbackFormQuestionInput — all fields optional; send only what you want to change.
// To clear options, send "options": [].
type UpdateFeedbackFormQuestionInput struct {
	QuestionText *string  `json:"question_text" example:"Updated question text"`
	IsRequired   *bool    `json:"is_required"   example:"false"`
	OrderIndex   *int     `json:"order_index"   example:"2"`
	ScaleMin     *int     `json:"scale_min"     example:"1"`
	ScaleMax     *int     `json:"scale_max"     example:"10"`
	StartLabel   *string  `json:"start_label"   example:"Low"`
	EndLabel     *string  `json:"end_label"     example:"High"`
	Options      []string `json:"options"       example:"A,B,C"`
}
