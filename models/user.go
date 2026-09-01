package models

import "time"

type Role string

const (
	RoleStudent    Role = "student"
	RoleMentor     Role = "mentor"
	RoleEmployee   Role = "employee"
	RoleTeamLead   Role = "team_lead"
	RoleSuperAdmin Role = "super_admin"
	// RolePlatformAdmin, RoleCollegeAdmin and RoleCollegeStaff are
	// college-scoped roles (multi-tenant college management) — deliberately
	// NEVER added to the existing adminOrAbove/staffOrAbove middleware
	// groups, which grant platform-wide power. They get their own dedicated
	// route groups so a college_admin can only ever act within their own
	// college_id, never platform-wide.
	RolePlatformAdmin Role = "platform_admin"
	RoleCollegeAdmin  Role = "college_admin"
	RoleCollegeStaff  Role = "college_staff"
)

type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Phone        string `json:"phone,omitempty"`
	DateOfBirth  string `json:"date_of_birth,omitempty"`
	Role         Role   `json:"role"`
	CollegeID    string `json:"college_id,omitempty"` // empty for super_admin / legacy users predating multi-tenancy
	// Department/DepartmentTeam only apply to role=employee and live on the
	// employees profile table (employees.department already existed, unused
	// until now; department_team is new) — populated via a LEFT JOIN in the
	// listing queries, never written through this struct directly (see
	// UserRepository.SetEmployeeDepartment).
	Department     string     `json:"department,omitempty"`
	DepartmentTeam string     `json:"department_team,omitempty"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// PasswordResetOTP is a single row in password_reset_otps — one issued
// code for the "forgot password" flow.
type PasswordResetOTP struct {
	ID        string
	UserID    string
	OTPHash   string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// LoginOTP is a single row in login_otps — one issued code for the
// mandatory email-OTP second factor checked on every password login.
type LoginOTP struct {
	ID        string
	UserID    string
	OTPHash   string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// UpdateUserInput carries the editable fields for PATCH /users/:id.
// All fields are optional — send only the ones you want to change.
type UpdateUserInput struct {
	FirstName   *string `json:"first_name"    example:"John"`
	LastName    *string `json:"last_name"     example:"Doe"`
	Phone       *string `json:"phone"         example:"+919876543210"`
	DateOfBirth *string `json:"date_of_birth" example:"1998-05-20"`
	IsActive    *bool   `json:"is_active"     example:"false"`
}

// EmployeeNote is one Manager-authored note on an employee's profile — a
// running record, not a one-time review. See repository.EmployeeNoteRepository.
type EmployeeNote struct {
	ShortID        string    `json:"short_id"`
	EmployeeUserID string    `json:"employee_user_id"`
	AuthorUserID   string    `json:"author_user_id"`
	AuthorName     string    `json:"author_name,omitempty"`
	NoteText       string    `json:"note_text"`
	CreatedAt      time.Time `json:"created_at"`
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
