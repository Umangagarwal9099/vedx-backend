package models

import "time"

// StudentRegistrationDetails holds the extended demographic/registration
// fields shown on the admin's learner-detail Profile tab — distinct from
// ProfileDetails (bio/skills/social, shared by every role) and from the core
// `users` row (name/email/phone).
type StudentRegistrationDetails struct {
	UserID             string    `json:"user_id"`
	Status             string    `json:"status,omitempty"`        // empty for non-student roles
	EnrollmentNo       string    `json:"enrollment_no,omitempty"` // the student's admission/roll number, from students.enrollment_no
	Gender             string    `json:"gender"`
	AlternateContact   string    `json:"alternate_contact"`
	StudentSource      string    `json:"student_source"`
	Religion           string    `json:"religion"`
	Standard           string    `json:"standard"`
	Occupation         string    `json:"occupation"`
	Timezone           string    `json:"timezone"`
	ParentName         string    `json:"parent_name"`
	ParentContact      string    `json:"parent_contact"`
	ParentEmail        string    `json:"parent_email"`
	Area               string    `json:"area"`
	SchoolCollegeName  string    `json:"school_college_name"`
	ResidentialAddress string    `json:"residential_address"`
	PermanentAddress   string    `json:"permanent_address"`
	City               string    `json:"city"`
	State              string    `json:"state"`
	Pincode            string    `json:"pincode"`
	OptWhatsapp        bool      `json:"opt_whatsapp"`
	OptEmail           bool      `json:"opt_email"`
	OptSMS             bool      `json:"opt_sms"`
	OptPush            bool      `json:"opt_push"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UpdateStudentRegistrationInput carries the editable fields — all optional,
// send only what changed.
type UpdateStudentRegistrationInput struct {
	EnrollmentNo       *string `json:"enrollment_no"`
	Gender             *string `json:"gender"`
	AlternateContact   *string `json:"alternate_contact"`
	StudentSource      *string `json:"student_source"`
	Religion           *string `json:"religion"`
	Standard           *string `json:"standard"`
	Occupation         *string `json:"occupation"`
	Timezone           *string `json:"timezone"`
	ParentName         *string `json:"parent_name"`
	ParentContact      *string `json:"parent_contact"`
	ParentEmail        *string `json:"parent_email"`
	Area               *string `json:"area"`
	SchoolCollegeName  *string `json:"school_college_name"`
	ResidentialAddress *string `json:"residential_address"`
	PermanentAddress   *string `json:"permanent_address"`
	City               *string `json:"city"`
	State              *string `json:"state"`
	Pincode            *string `json:"pincode"`
	OptWhatsapp        *bool   `json:"opt_whatsapp"`
	OptEmail           *bool   `json:"opt_email"`
	OptSMS             *bool   `json:"opt_sms"`
	OptPush            *bool   `json:"opt_push"`
}

// StudentStatusHistoryEntry is one append-only status-change event.
type StudentStatusHistoryEntry struct {
	ID            string    `json:"id"`
	ShortID       string    `json:"short_id"`
	StudentUserID string    `json:"student_user_id"`
	Status        string    `json:"status"`
	Notes         string    `json:"notes,omitempty"`
	ChangedBy     string    `json:"changed_by"`
	ChangedByName string    `json:"changed_by_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateStudentStatusInput is the admin's status-change request.
type UpdateStudentStatusInput struct {
	Status string `json:"status" binding:"required,oneof=registered enrolled completed on_leave archived" example:"enrolled"`
	Notes  string `json:"notes" example:"Fees cleared, moved to active batch"`
}
