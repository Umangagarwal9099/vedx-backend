package models

import "time"

// Project is a multi-day/multi-week academic activity scoped to a batch, made
// up of milestones. If IsTeamProject is true, students submit as a team
// (see ProjectTeam); otherwise each student submits individually.
type Project struct {
	ID                     string     `json:"id"`
	ShortID                string     `json:"short_id"`
	Title                  string     `json:"title"`
	ProblemStatement       string     `json:"problem_statement,omitempty"`
	Requirements           string     `json:"requirements,omitempty"`
	ExpectedDeliverables   string     `json:"expected_deliverables,omitempty"`
	EvaluationCriteria     string     `json:"evaluation_criteria,omitempty"`
	ReferenceFiles         []string   `json:"reference_files,omitempty"`
	Category               string     `json:"category"` // individual | group | module | capstone | internship | final_course
	IsTeamProject          bool       `json:"is_team_project"`
	BatchShortID           string     `json:"batch_short_id"`
	BatchNumber            string     `json:"batch_number"`
	ModuleShortID          string     `json:"module_short_id,omitempty"`
	ModuleName             string     `json:"module_name,omitempty"`
	MaxMarks               int        `json:"max_marks"`
	StartDate              string     `json:"start_date,omitempty"`
	FinalDeadline          time.Time  `json:"final_deadline"`
	AllowedSubmissionTypes []string   `json:"allowed_submission_types"`
	Status                 string     `json:"status"` // draft | active | closed
	CreatedBy              string     `json:"created_by"`
	CreatedByName          string     `json:"created_by_name,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	DeletedAt              *time.Time `json:"deleted_at,omitempty"`
}

// ProjectDetail embeds the milestones and (if applicable) teams for a project,
// returned by GET /projects/{short_id} so the caller doesn't need extra round-trips.
type ProjectDetail struct {
	Project
	Milestones []ProjectMilestone `json:"milestones"`
	Teams      []ProjectTeam      `json:"teams,omitempty"`
}

type CreateProjectInput struct {
	Title                  string   `json:"title"                    binding:"required"                                                     example:"E-Commerce Capstone"`
	ProblemStatement       string   `json:"problem_statement"                                                                                 example:"Build a full-stack e-commerce platform with cart, checkout, and admin panel."`
	Requirements           string   `json:"requirements"                                                                                       example:"React frontend, Node.js backend, PostgreSQL database."`
	ExpectedDeliverables   string   `json:"expected_deliverables"                                                                              example:"GitHub repo, deployed app, final report."`
	EvaluationCriteria     string   `json:"evaluation_criteria"                                                                                example:"Functionality 40%, code quality 30%, presentation 30%."`
	ReferenceFiles         []string `json:"reference_files"                                                                                    example:"[\"https://cdn.example.com/brief.pdf\"]"`
	Category               string   `json:"category"                 binding:"required,oneof=individual group module capstone internship final_course" example:"capstone"`
	IsTeamProject          bool     `json:"is_team_project"                                                                                    example:"true"`
	BatchShortID           string   `json:"batch_short_id"           binding:"required"                                                       example:"use GET /batches to pick a real short_id"`
	ModuleShortID          string   `json:"module_short_id"                                                                                    example:"use GET /modules to pick a real short_id"`
	MaxMarks               int      `json:"max_marks"                binding:"required,min=1"                                                 example:"200"`
	StartDate              string   `json:"start_date"                                                                                         example:"2026-07-01"`
	FinalDeadline          time.Time `json:"final_deadline"          binding:"required"                                                       example:"2026-08-15T23:59:00Z"`
	AllowedSubmissionTypes []string `json:"allowed_submission_types" binding:"required,min=1,dive,oneof=link text file"                        example:"[\"link\",\"file\"]"`
	Status                 string   `json:"status"                   binding:"omitempty,oneof=draft active"                                  example:"active"`
}

type UpdateProjectInput struct {
	Title                  *string    `json:"title"                    example:"E-Commerce Capstone — Extended"`
	ProblemStatement       *string    `json:"problem_statement"        example:"Updated brief."`
	Requirements           *string    `json:"requirements"              example:"Updated requirements."`
	ExpectedDeliverables   *string    `json:"expected_deliverables"    example:"Updated deliverables."`
	EvaluationCriteria     *string    `json:"evaluation_criteria"       example:"Updated rubric."`
	ReferenceFiles         []string   `json:"reference_files"          example:"[\"https://cdn.example.com/brief-v2.pdf\"]"`
	Category               *string    `json:"category"                  binding:"omitempty,oneof=individual group module capstone internship final_course" example:"group"`
	IsTeamProject          *bool      `json:"is_team_project"           example:"false"`
	BatchShortID           *string    `json:"batch_short_id"            example:"use GET /batches to pick a real short_id"`
	ModuleShortID          *string    `json:"module_short_id"           example:""`
	MaxMarks               *int       `json:"max_marks"                 example:"250"`
	StartDate              *string    `json:"start_date"                example:"2026-07-05"`
	FinalDeadline          *time.Time `json:"final_deadline"            example:"2026-08-20T23:59:00Z"`
	AllowedSubmissionTypes []string   `json:"allowed_submission_types"  example:"[\"file\"]"`
	Status                 *string    `json:"status"                    binding:"omitempty,oneof=draft active closed" example:"closed"`
}

// ProjectFilter holds query params for GET /projects.
type ProjectFilter struct {
	BatchShortID string `form:"batch_short_id"`
	Status       string `form:"status"`
}

// ── Milestones ───────────────────────────────────────────────────────────────

type ProjectMilestone struct {
	ID          string     `json:"id"`
	ShortID     string     `json:"short_id"`
	ProjectID   string     `json:"project_id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	OrderIndex  int        `json:"order_index"`
	IsFinal     bool       `json:"is_final"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreateMilestoneInput struct {
	Title       string     `json:"title"        binding:"required" example:"Topic Approval"`
	Description string     `json:"description"                     example:"Submit your chosen project idea for approval."`
	DueDate     *time.Time `json:"due_date"                         example:"2026-07-05T23:59:00Z"`
	OrderIndex  int        `json:"order_index"                      example:"1"`
	IsFinal     bool       `json:"is_final"                         example:"false"`
}

type UpdateMilestoneInput struct {
	Title       *string    `json:"title"        example:"Topic Approval (Revised)"`
	Description *string    `json:"description"  example:"Updated instructions."`
	DueDate     *time.Time `json:"due_date"     example:"2026-07-07T23:59:00Z"`
	OrderIndex  *int       `json:"order_index"  example:"2"`
	IsFinal     *bool      `json:"is_final"     example:"true"`
}

// ── Teams ────────────────────────────────────────────────────────────────────

type ProjectTeam struct {
	ID        string              `json:"id"`
	ShortID   string              `json:"short_id"`
	ProjectID string              `json:"project_id"`
	Name      string              `json:"name"`
	Members   []ProjectTeamMember `json:"members"`
	CreatedAt time.Time           `json:"created_at"`
}

type ProjectTeamMember struct {
	UserID    string    `json:"user_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	JoinedAt  time.Time `json:"joined_at"`
}

type CreateTeamInput struct {
	Name      string   `json:"name"       binding:"required"       example:"Team Phoenix"`
	StudentIDs []string `json:"student_ids"                        example:"[\"11111111-1111-1111-1111-111111111111\"]"`
}

type AddTeamMembersInput struct {
	StudentIDs []string `json:"student_ids" binding:"required,min=1" example:"[\"11111111-1111-1111-1111-111111111111\"]"`
}

// ── Submissions ──────────────────────────────────────────────────────────────

// ProjectSubmission is one student's (or one team's) submission for a milestone.
type ProjectSubmission struct {
	ID              string     `json:"id"`
	ShortID         string     `json:"short_id"`
	MilestoneShortID string    `json:"milestone_short_id"`
	// ProjectTitle/MilestoneTitle/BatchShortID/BatchNumber are only populated
	// by the cross-project workspace listing (FindAllSubmissionsForMentor).
	ProjectTitle    string     `json:"project_title,omitempty"`
	MilestoneTitle  string     `json:"milestone_title,omitempty"`
	BatchShortID    string     `json:"batch_short_id,omitempty"`
	BatchNumber     string     `json:"batch_number,omitempty"`
	StudentID       string     `json:"student_id,omitempty"`
	StudentName     string     `json:"student_name,omitempty"`
	TeamShortID     string     `json:"team_short_id,omitempty"`
	TeamName        string     `json:"team_name,omitempty"`
	SubmissionType  string     `json:"submission_type"` // link | text | file
	Content         string     `json:"content,omitempty"`
	FileURL         string     `json:"file_url,omitempty"`
	Status          string     `json:"status"` // submitted | late | evaluated | resubmission_required
	Marks           *int       `json:"marks,omitempty"`
	Feedback        string     `json:"feedback,omitempty"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	EvaluatedAt     *time.Time `json:"evaluated_at,omitempty"`
	EvaluatedBy     string     `json:"evaluated_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type CreateProjectSubmissionInput struct {
	SubmissionType string `json:"submission_type" binding:"required,oneof=link text file" example:"link"`
	Content        string `json:"content"                                                 example:"https://github.com/team/repo"`
	FileURL        string `json:"file_url"                                                example:"https://cdn.example.com/report.pdf"`
}

type GradeProjectSubmissionInput struct {
	Marks    *int   `json:"marks"    binding:"required" example:"90"`
	Feedback string `json:"feedback"                    example:"Great execution, minor UI polish needed."`
	Status   string `json:"status"   binding:"omitempty,oneof=evaluated resubmission_required" example:"evaluated"`
}
