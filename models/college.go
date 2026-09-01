package models

import "time"

// FeatureKey is one togglable module in a college's configuration. Grouped
// here exactly as presented on the Super Admin's Feature Configuration UI —
// keep this list and the frontend's checkbox list in sync.
type FeatureKey = string

const (
	FeatureDashboard          FeatureKey = "dashboard"
	FeatureCourses            FeatureKey = "courses"
	FeatureModules            FeatureKey = "modules"
	FeatureResources          FeatureKey = "resources"
	FeatureRecordedVideos     FeatureKey = "recorded_videos"
	FeatureLiveSessions       FeatureKey = "live_sessions"
	FeatureAssignments        FeatureKey = "assignments"
	FeatureExams              FeatureKey = "exams"
	FeatureProjects           FeatureKey = "projects"
	FeatureCoding             FeatureKey = "coding"
	FeatureCodingPractice     FeatureKey = "coding_practice"
	FeatureCodingAssessments  FeatureKey = "coding_assessments"
	FeatureCodingQuestionBank FeatureKey = "coding_question_bank"
	FeatureCodingLeaderboard  FeatureKey = "coding_leaderboard"
	FeatureCommunity          FeatureKey = "community"
	FeatureMessages           FeatureKey = "messages"
	FeatureNotifications      FeatureKey = "notifications"
	FeatureAnnouncements      FeatureKey = "announcements"
	FeatureInternship         FeatureKey = "internship"
	FeatureJobAssistance      FeatureKey = "job_assistance"
	FeaturePlacements         FeatureKey = "placements"
	FeatureEmployerManagement FeatureKey = "employer_management"
	FeatureCRM                FeatureKey = "crm"
	FeatureEmployeePortal     FeatureKey = "employee_portal"
	FeatureAttendance         FeatureKey = "attendance"
	FeatureLeave              FeatureKey = "leave"
	FeatureReports            FeatureKey = "reports"
	FeatureAnalytics          FeatureKey = "analytics"
	FeatureStudentAnalytics   FeatureKey = "student_analytics"
	FeatureMentorAnalytics    FeatureKey = "mentor_analytics"
	FeatureCollegeAnalytics   FeatureKey = "college_analytics"
	FeatureCodingAnalytics    FeatureKey = "coding_analytics"
)

// AllFeatureKeys is every valid feature key, for validating toggle requests.
var AllFeatureKeys = []FeatureKey{
	FeatureDashboard, FeatureCourses, FeatureModules, FeatureResources, FeatureRecordedVideos,
	FeatureLiveSessions, FeatureAssignments, FeatureExams, FeatureProjects,
	FeatureCoding, FeatureCodingPractice, FeatureCodingAssessments, FeatureCodingQuestionBank, FeatureCodingLeaderboard,
	FeatureCommunity, FeatureMessages, FeatureNotifications, FeatureAnnouncements,
	FeatureInternship, FeatureJobAssistance, FeaturePlacements, FeatureEmployerManagement,
	FeatureCRM, FeatureEmployeePortal, FeatureAttendance, FeatureLeave, FeatureReports,
	FeatureAnalytics, FeatureStudentAnalytics, FeatureMentorAnalytics, FeatureCollegeAnalytics, FeatureCodingAnalytics,
}

type College struct {
	ID                    string          `json:"id"`
	ShortID               string          `json:"short_id"`
	Name                  string          `json:"name"`
	Code                  string          `json:"code"`
	LogoURL               string          `json:"logo_url,omitempty"`
	ContactPerson         string          `json:"contact_person,omitempty"`
	ContactEmail          string          `json:"contact_email,omitempty"`
	ContactPhone          string          `json:"contact_phone,omitempty"`
	Address               string          `json:"address,omitempty"`
	SubscriptionStartDate string          `json:"subscription_start_date,omitempty"`
	SubscriptionEndDate   string          `json:"subscription_end_date,omitempty"`
	MaxStudents           *int            `json:"max_students,omitempty"`
	MaxEmployees          *int            `json:"max_employees,omitempty"`
	Status                string          `json:"status"`
	EnabledFeatures       map[string]bool `json:"enabled_features"`
	StudentCount          int             `json:"student_count"`
	EmployeeCount         int             `json:"employee_count"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// CollegeMembership is one college a mentor/employee/team_lead currently
// serves — see repository.CollegeEmployeeRepository. IsDefault marks the
// one membership that also matches the user's users.college_id column.
type CollegeMembership struct {
	CollegeID      string    `json:"college_id"`
	CollegeShortID string    `json:"college_short_id"`
	CollegeName    string    `json:"college_name"`
	IsDefault      bool      `json:"is_default"`
	AssignedAt     time.Time `json:"assigned_at"`
}

// CreateCollegeInput carries the fields for creating a college. Feature
// toggles are optional — omit to start with everything disabled.
type CreateCollegeInput struct {
	Name                  string          `json:"name"                     binding:"required"          example:"Springfield Institute of Technology"`
	Code                  string          `json:"code"                     binding:"required"          example:"SIT"`
	LogoURL               string          `json:"logo_url"                                              example:"https://cdn.example.com/sit-logo.png"`
	ContactPerson         string          `json:"contact_person"                                        example:"Jane Doe"`
	ContactEmail          string          `json:"contact_email"            binding:"omitempty,email"    example:"admin@sit.edu"`
	ContactPhone          string          `json:"contact_phone"                                         example:"+919876543210"`
	Address               string          `json:"address"                                               example:"123 Main St, Springfield"`
	SubscriptionStartDate string          `json:"subscription_start_date"                               example:"2026-01-01"`
	SubscriptionEndDate   string          `json:"subscription_end_date"                                 example:"2026-12-31"`
	MaxStudents           *int            `json:"max_students"                                          example:"500"`
	MaxEmployees          *int            `json:"max_employees"                                         example:"20"`
	EnabledFeatures       map[string]bool `json:"enabled_features"                                       example:"{\"coding\":true,\"courses\":false}"`
}

// UpdateCollegeInput — all fields optional; send only what you want to change.
type UpdateCollegeInput struct {
	Name                  *string `json:"name"                     example:"Springfield Institute of Technology"`
	LogoURL               *string `json:"logo_url"                 example:"https://cdn.example.com/sit-logo.png"`
	ContactPerson         *string `json:"contact_person"           example:"Jane Doe"`
	ContactEmail          *string `json:"contact_email"            binding:"omitempty,email" example:"admin@sit.edu"`
	ContactPhone          *string `json:"contact_phone"            example:"+919876543210"`
	Address               *string `json:"address"                  example:"123 Main St, Springfield"`
	SubscriptionStartDate *string `json:"subscription_start_date"  example:"2026-01-01"`
	SubscriptionEndDate   *string `json:"subscription_end_date"    example:"2026-12-31"`
	MaxStudents           *int    `json:"max_students"              example:"500"`
	MaxEmployees          *int    `json:"max_employees"             example:"20"`
	Status                *string `json:"status"                    binding:"omitempty,oneof=active suspended expired" example:"active"`
}

// UpdateFeaturesInput replaces (merges into) a college's enabled feature map.
type UpdateFeaturesInput struct {
	Features map[string]bool `json:"features" binding:"required" example:"{\"coding\":true,\"courses\":false}"`
}
