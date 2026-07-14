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
	Status                string     `json:"status"` // draft | upcoming | active | completed | cancelled | archived
	MaxStudents           *int       `json:"max_students,omitempty"`
	StudentCount          int        `json:"student_count"`
	ScoreWeightAssignments int       `json:"score_weight_assignments"`
	ScoreWeightExams       int       `json:"score_weight_exams"`
	ScoreWeightProjects    int       `json:"score_weight_projects"`
	CreatedBy             string     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
}

// CreateBatchInput — student_ids is optional; students can also be enrolled
// later via POST /batches/{short_id}/students.
type CreateBatchInput struct {
	BatchNumber         string   `json:"batch_number"          binding:"required" example:"BATCH-2024-001"`
	CourseShortID       string   `json:"course_short_id"       binding:"required" example:"A3F72C1D"`
	BatchManagerID      string   `json:"batch_manager_id"      binding:"required" example:"use GET /mentors to pick a real ID"`
	AdditionalManagerID string   `json:"additional_manager_id"                   example:"use GET /mentors to pick a real ID"`
	Module              string   `json:"module"                                  example:"Module 1"`
	StartDate           string   `json:"start_date"            binding:"required" example:"2024-01-15"`
	EndDate             string   `json:"end_date"              binding:"required" example:"2024-06-15"`
	Status              string   `json:"status"                                  binding:"omitempty,oneof=draft upcoming active completed cancelled archived" example:"draft"`
	MaxStudents         *int     `json:"max_students"                            example:"30"`
	StudentIDs          []string `json:"student_ids"                             example:"[\"11111111-1111-1111-1111-111111111111\"]"`
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
	Status              *string `json:"status"                binding:"omitempty,oneof=draft upcoming active completed cancelled archived" example:"completed"`
	MaxStudents         *int    `json:"max_students"          example:"30"`
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
	Status        string `form:"status"`    // draft | upcoming | active | completed | cancelled | archived
}

// BatchStudent represents a single student's enrollment in a batch.
// FeesPaid gates access to session recordings — live classes remain open to
// every enrolled student regardless of this flag.
type BatchStudent struct {
	UserID    string    `json:"user_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	FeesPaid  bool      `json:"fees_paid"`
	JoinedAt  time.Time `json:"joined_at"`
}

// AddBatchStudentsInput carries one or more student user IDs to enroll in a batch.
type AddBatchStudentsInput struct {
	StudentIDs []string `json:"student_ids" binding:"required,min=1" example:"[\"11111111-1111-1111-1111-111111111111\"]"`
}

// UpdateFeesPaidInput sets a single student's fee-payment status for a batch,
// used to grant or revoke access to that batch's session recordings.
type UpdateFeesPaidInput struct {
	FeesPaid *bool `json:"fees_paid" binding:"required" example:"true"`
}

// UpdateScoreWeightsInput sets how much each category contributes to a
// batch's final score / leaderboard ranking. The three weights must sum to
// 100 — validated by the controller, since that check spans all three fields.
type UpdateScoreWeightsInput struct {
	AssignmentsWeight int `json:"assignments_weight" binding:"min=0,max=100" example:"40"`
	ExamsWeight       int `json:"exams_weight"       binding:"min=0,max=100" example:"40"`
	ProjectsWeight    int `json:"projects_weight"    binding:"min=0,max=100" example:"20"`
}
