package models

import "time"

// QuestionOption is one selectable option for mcq/multi_select/true_false questions.
type QuestionOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Question is a single question-bank entry usable across assessments. For
// question_type "coding", coding_question_short_id links to an existing entry
// in the separate coding-practice question bank rather than duplicating it —
// grading for coding answers within an exam is manual (the student's code is
// captured as text, reviewed by a mentor), not auto-executed.
type Question struct {
	ID                    string     `json:"id"`
	ShortID               string     `json:"short_id"`
	QuestionType          string     `json:"question_type"` // mcq | multi_select | true_false | fill_blank | short_answer | descriptive | coding
	QuestionText          string     `json:"question_text,omitempty"`
	Options               []QuestionOption `json:"options,omitempty"`
	CorrectOptionIDs      []string   `json:"correct_option_ids,omitempty"`
	CorrectText           string     `json:"correct_text,omitempty"`
	Explanation           string     `json:"explanation,omitempty"`
	Marks                 int        `json:"marks"`
	NegativeMarks         int        `json:"negative_marks"`
	Subject               string     `json:"subject,omitempty"`
	Topic                 string     `json:"topic,omitempty"`
	Subtopic              string     `json:"subtopic,omitempty"`
	Difficulty            string     `json:"difficulty,omitempty"` // easy | medium | hard
	CodingQuestionShortID string     `json:"coding_question_short_id,omitempty"`
	CodingQuestionTitle   string     `json:"coding_question_title,omitempty"`
	Visibility            string     `json:"visibility"` // private | course | global
	CreatedBy             string     `json:"created_by"`
	CreatedByName         string     `json:"created_by_name,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
}

// CreateQuestionInput carries the fields for a new question-bank entry.
// For question_type "coding", set coding_question_short_id and leave the MCQ
// fields empty. Visibility defaults to "private" (only the creator sees it in
// the bank) — set "course" or "global" to share it more broadly.
type CreateQuestionInput struct {
	QuestionType          string           `json:"question_type"            binding:"required,oneof=mcq multi_select true_false fill_blank short_answer descriptive coding" example:"mcq"`
	QuestionText          string           `json:"question_text"                                                                                                          example:"Which of these is a Go concurrency primitive?"`
	Options               []QuestionOption `json:"options"                                                                                                                example:"[{\"id\":\"a\",\"text\":\"Goroutine\"},{\"id\":\"b\",\"text\":\"Thread\"}]"`
	CorrectOptionIDs      []string         `json:"correct_option_ids"                                                                                                     example:"[\"a\"]"`
	CorrectText           string           `json:"correct_text"                                                                                                           example:"Goroutine"`
	Explanation           string           `json:"explanation"                                                                                                            example:"Goroutines are Go's lightweight concurrency primitive."`
	Marks                 int              `json:"marks"                    binding:"required,min=1"                                                                     example:"5"`
	NegativeMarks         int              `json:"negative_marks"                                                                                                         example:"1"`
	Subject               string           `json:"subject"                                                                                                                example:"Python"`
	Topic                 string           `json:"topic"                                                                                                                  example:"Concurrency"`
	Subtopic              string           `json:"subtopic"                                                                                                               example:"Generators"`
	Difficulty            string           `json:"difficulty"               binding:"omitempty,oneof=easy medium hard"                                                   example:"medium"`
	CodingQuestionShortID string           `json:"coding_question_short_id"                                                                                               example:"use GET /coding-questions to pick a real short_id"`
	Visibility            string           `json:"visibility"               binding:"omitempty,oneof=private course global"                                               example:"course"`
}

type UpdateQuestionInput struct {
	QuestionText          *string          `json:"question_text"            example:"Updated question text"`
	Options               []QuestionOption `json:"options"                  example:"[{\"id\":\"a\",\"text\":\"Updated option\"}]"`
	CorrectOptionIDs      []string         `json:"correct_option_ids"       example:"[\"a\",\"b\"]"`
	CorrectText           *string          `json:"correct_text"             example:"Updated answer"`
	Explanation           *string          `json:"explanation"              example:"Updated explanation"`
	Marks                 *int             `json:"marks"                    example:"10"`
	NegativeMarks         *int             `json:"negative_marks"           example:"2"`
	Subject               *string          `json:"subject"                  example:"Python"`
	Topic                 *string          `json:"topic"                    example:"Updated topic"`
	Subtopic              *string          `json:"subtopic"                 example:"Generators"`
	Difficulty            *string          `json:"difficulty"               binding:"omitempty,oneof=easy medium hard" example:"hard"`
	CodingQuestionShortID *string          `json:"coding_question_short_id" example:""`
	Visibility            *string          `json:"visibility"               binding:"omitempty,oneof=private course global" example:"global"`
}

// QuestionFilter holds query params for GET /questions.
type QuestionFilter struct {
	QuestionType string `form:"question_type"`
	Subject      string `form:"subject"`
	Topic        string `form:"topic"`
	Subtopic     string `form:"subtopic"`
	Difficulty   string `form:"difficulty"`
}

// AssessmentQuestion is a question attached to an assessment, in display order.
// marks_override lets the same bank question carry different weight per assessment.
type AssessmentQuestion struct {
	Question
	OrderIndex    int  `json:"order_index"`
	MarksOverride *int `json:"marks_override,omitempty"`
}

// AttachQuestionInput attaches an existing question-bank entry to an assessment.
type AttachQuestionInput struct {
	QuestionShortID string `json:"question_short_id" binding:"required" example:"use GET /questions to pick a real short_id"`
	OrderIndex      int    `json:"order_index"                          example:"1"`
	MarksOverride   *int   `json:"marks_override"                       example:"10"`
}

type UpdateAttachedQuestionInput struct {
	OrderIndex    *int `json:"order_index"    example:"2"`
	MarksOverride *int `json:"marks_override" example:"15"`
}
