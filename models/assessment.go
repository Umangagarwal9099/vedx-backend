package models

import "time"

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
	IsActive                  bool       `json:"is_active"`
	CreatedBy                 string     `json:"created_by"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
	DeletedAt                 *time.Time `json:"deleted_at,omitempty"`
}

type CreateAssessmentInput struct {
	Name                      string   `json:"name"                         binding:"required"                                         example:"Golang Fundamentals Quiz"`
	Description               string   `json:"description"                                                                             example:"Test your knowledge of Go basics."`
	Thumbnail                 string   `json:"thumbnail"                                                                               example:"https://cdn.example.com/thumbnail.jpg"`
	FileURL                   string   `json:"file_url"                                                                                example:"https://cdn.example.com/file.pdf"`
	GeneralInstructions       string   `json:"general_instructions"                                                                    example:"Read all questions carefully before answering."`
	TotalMarks                int      `json:"total_marks"                  binding:"required,min=1"                                   example:"100"`
	PassingPercentage         float64  `json:"passing_percentage"           binding:"required,min=0,max=100"                           example:"60"`
	ResultDeclaration         string   `json:"result_declaration"           binding:"required,oneof=manual automatic"                  example:"automatic"`
	ResultDisplay             string   `json:"result_display"               binding:"required,oneof=marks_and_status status_only"      example:"marks_and_status"`
	AllowAttemptsAfterPassing bool     `json:"allow_attempts_after_passing"                                                            example:"false"`
}

type UpdateAssessmentInput struct {
	Name                      *string   `json:"name"                         example:"Updated Assessment Name"`
	Description               *string   `json:"description"                  example:"Updated description."`
	Thumbnail                 *string   `json:"thumbnail"                    example:"https://cdn.example.com/new-thumb.jpg"`
	FileURL                   *string   `json:"file_url"                     example:"https://cdn.example.com/new-file.pdf"`
	GeneralInstructions       *string   `json:"general_instructions"         example:"Updated instructions."`
	TotalMarks                *int      `json:"total_marks"                  example:"150"`
	PassingPercentage         *float64  `json:"passing_percentage"           example:"70"`
	ResultDeclaration         *string   `json:"result_declaration"           example:"manual"`
	ResultDisplay             *string   `json:"result_display"               example:"status_only"`
	AllowAttemptsAfterPassing *bool     `json:"allow_attempts_after_passing" example:"true"`
	IsActive                  *bool     `json:"is_active"                    example:"false"`
}

type AssessmentFilter struct {
	Name        string `form:"name"`
	Description string `form:"description"`
	IsActive    string `form:"is_active"`
}
