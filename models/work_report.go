package models

import "time"

// WorkReport is one employee's end-of-day activity log for a single date.
// Upserted on (employee_id, report_date) — re-submitting the same day
// updates the existing row rather than creating a second one.
type WorkReport struct {
	ID             string    `json:"id"`
	ShortID        string    `json:"short_id"`
	EmployeeID     string    `json:"employee_id"`
	EmployeeName   string    `json:"employee_name,omitempty"`
	ReportDate     string    `json:"report_date"`
	CallsMade      int       `json:"calls_made"`
	LeadsContacted int       `json:"leads_contacted"`
	FollowUpsDone  int       `json:"follow_ups_done"`
	Admissions     int       `json:"admissions"`
	Summary        string    `json:"summary,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SubmitWorkReportInput carries today's report. report_date is never
// client-supplied — the server always stamps CURRENT_DATE.
type SubmitWorkReportInput struct {
	CallsMade      int    `json:"calls_made"      binding:"min=0" example:"12"`
	LeadsContacted int    `json:"leads_contacted" binding:"min=0" example:"8"`
	FollowUpsDone  int    `json:"follow_ups_done" binding:"min=0" example:"5"`
	Admissions     int    `json:"admissions"      binding:"min=0" example:"1"`
	Summary        string `json:"summary" example:"Followed up with 5 warm leads, 1 admission closed."`
}

// WorkReportSuggestions are today's real activity counts, offered as
// starting values the employee can confirm or edit before submitting.
type WorkReportSuggestions struct {
	CallsMade      int `json:"calls_made"`
	LeadsContacted int `json:"leads_contacted"`
	FollowUpsDone  int `json:"follow_ups_done"`
	Admissions     int `json:"admissions"`
}
