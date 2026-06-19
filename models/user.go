package models

import "time"

type Role string

const (
	RoleStudent    Role = "student"
	RoleMentor     Role = "mentor"
	RoleEmployee   Role = "employee"
	RoleTeamLead   Role = "team_lead"
	RoleSuperAdmin Role = "super_admin"
)

type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	FirstName    string     `json:"first_name"`
	LastName     string     `json:"last_name"`
	Phone        string     `json:"phone,omitempty"`
	DateOfBirth  string     `json:"date_of_birth,omitempty"`
	Role         Role       `json:"role"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// UpdateUserInput carries the editable fields for PATCH /users/:id.
// All fields are optional — send only the ones you want to change.
type UpdateUserInput struct {
	FirstName   *string `json:"first_name"    example:"John"`
	LastName    *string `json:"last_name"     example:"Doe"`
	Phone       *string `json:"phone"         example:"+919876543210"`
	DateOfBirth *string `json:"date_of_birth" example:"1998-05-20"`
}

// ── Role-specific profile structs (populated via separate APIs) ──────────────

type StudentProfile struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	EnrollmentNo string    `json:"enrollment_no"`
	Course       string    `json:"course"`
	YearOfStudy  int       `json:"year_of_study"`
	Batch        string    `json:"batch"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MentorProfile struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	Expertise         string    `json:"expertise"`
	Bio               string    `json:"bio"`
	YearsOfExperience int       `json:"years_of_experience"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type EmployeeProfile struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	EmployeeCode string    `json:"employee_code"`
	Department   string    `json:"department"`
	Designation  string    `json:"designation"`
	JoiningDate  string    `json:"joining_date"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TeamLeadProfile struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	TeamName   string    `json:"team_name"`
	Department string    `json:"department"`
	TeamSize   int       `json:"team_size"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SuperAdminProfile struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	AdminLevel int       `json:"admin_level"`
	Department string    `json:"department"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
