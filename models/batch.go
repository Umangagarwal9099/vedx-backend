package models

import "time"

type Batch struct {
	ID                    string     `json:"id"`
	ShortID               string     `json:"short_id"`
	BatchNumber           string     `json:"batch_number"`
	CourseID              string     `json:"course_id"`
	CourseName            string     `json:"course_name"`
	CourseShortID         string     `json:"course_short_id"`
	BatchManagerID        string     `json:"batch_manager_id"`
	BatchManagerName      string     `json:"batch_manager_name"`
	AdditionalManagerID   string     `json:"additional_manager_id"`
	AdditionalManagerName string     `json:"additional_manager_name"`
	Module                string     `json:"module"`
	StartDate             string     `json:"start_date"`
	EndDate               string     `json:"end_date"`
	IsActive              bool       `json:"is_active"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
}

type CreateBatchInput struct {
	BatchNumber         string `json:"batch_number"          binding:"required" example:"BATCH-2024-001"`
	CourseShortID       string `json:"course_short_id"       binding:"required" example:"A3F72C1D"`
	BatchManagerID      string `json:"batch_manager_id"      binding:"required" example:"use GET /mentors to pick a real ID"`
	AdditionalManagerID string `json:"additional_manager_id"                   example:"use GET /mentors to pick a real ID"`
	Module              string `json:"module"                                  example:"Module 1"`
	StartDate           string `json:"start_date"            binding:"required" example:"2024-01-15"`
	EndDate             string `json:"end_date"              binding:"required" example:"2024-06-15"`
}

// UpdateBatchInput — all fields optional; send only what you want to change.
// Set additional_manager_id to "" to clear it.
type UpdateBatchInput struct {
	BatchNumber         *string `json:"batch_number"          example:"BATCH-2024-002"`
	CourseShortID       *string `json:"course_short_id"       example:"B4G83D2E"`
	BatchManagerID      *string `json:"batch_manager_id"      example:"use GET /mentors to pick a real ID"`
	AdditionalManagerID *string `json:"additional_manager_id" example:""`
	Module              *string `json:"module"                example:"Module 2"`
	StartDate           *string `json:"start_date"            example:"2024-02-01"`
	EndDate             *string `json:"end_date"              example:"2024-07-01"`
	IsActive            *bool   `json:"is_active"             example:"false"`
}

// BatchFilter holds query params for GET /batches/filter.
type BatchFilter struct {
	BatchNumber   string `form:"batch_number"`
	CourseShortID string `form:"course_short_id"`
	ManagerID     string `form:"batch_manager_id"`
	Module        string `form:"module"`
	StartDate     string `form:"start_date"`
	EndDate       string `form:"end_date"`
	IsActive      string `form:"is_active"` // "true" | "false" | ""
}
