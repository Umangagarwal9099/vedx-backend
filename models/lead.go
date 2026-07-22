package models

import "time"

// Lead is a sales lead for course-enrollment — a prospect who has not yet
// enrolled, distinct from a Student. Admin imports/creates leads and assigns
// them to an employee (or team_lead), who works them through calls/follow-ups
// to conversion.
type Lead struct {
	ID              string     `json:"id"`
	ShortID         string     `json:"short_id"`
	CollegeID       string     `json:"college_id,omitempty"`
	Name            string     `json:"name"`
	Phone           string     `json:"phone"`
	Email           string     `json:"email,omitempty"`
	City            string     `json:"city,omitempty"`
	CourseInterest  string     `json:"course_interest"`
	CourseShortID   string     `json:"course_short_id,omitempty"`
	Source          string     `json:"source"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	AssignedTo      string     `json:"assigned_to,omitempty"`
	AssignedToName  string     `json:"assigned_to_name,omitempty"`
	AssignedAt      *time.Time `json:"assigned_at,omitempty"`
	AssignedBy      string     `json:"assigned_by,omitempty"`
	NextFollowUpAt  *time.Time `json:"next_follow_up_at,omitempty"`
	LastContactedAt *time.Time `json:"last_contacted_at,omitempty"`
	ConvertedAt     *time.Time `json:"converted_at,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	CreatedBy       string     `json:"created_by"`
	CreatedByName   string     `json:"created_by_name,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateLeadInput carries the fields for a single manually-created lead.
type CreateLeadInput struct {
	// CollegeShortID is only honored for super_admin callers; every other
	// caller is forced onto their own college_id server-side. Omit for the
	// Internal EdTech Platform's own direct-website leads.
	CollegeShortID string `json:"college_short_id" example:"use GET /colleges to pick a real short_id, or omit for the Internal EdTech Platform"`
	Name           string `json:"name"            binding:"required" example:"Priya Sharma"`
	Phone          string `json:"phone"           binding:"required" example:"+919876543210"`
	Email          string `json:"email"           example:"priya@example.com"`
	City           string `json:"city"            example:"Bengaluru"`
	CourseInterest string `json:"course_interest" binding:"required" example:"Full Stack Development"`
	Source         string `json:"source"          binding:"omitempty,oneof=excel_import manual website referral other" example:"manual"`
	Priority       string `json:"priority"        binding:"omitempty,oneof=low medium high urgent" example:"medium"`
	Notes          string `json:"notes"`
}

// UpdateLeadInput is a partial update. An employee may only send status/
// priority/next_follow_up_at/notes on their own lead — the controller
// enforces that restriction, not this struct.
type UpdateLeadInput struct {
	Name           *string    `json:"name"`
	Phone          *string    `json:"phone"`
	Email          *string    `json:"email"`
	City           *string    `json:"city"`
	CourseInterest *string    `json:"course_interest"`
	Status         *string    `json:"status"   binding:"omitempty,oneof=new contacted interested follow_up not_reachable converted not_interested lost"`
	Priority       *string    `json:"priority" binding:"omitempty,oneof=low medium high urgent"`
	NextFollowUpAt *time.Time `json:"next_follow_up_at"`
	Notes          *string    `json:"notes"`
}

// LeadFilter holds query params shared by GET /leads for both the admin
// (unscoped) and employee (own-leads) views.
type LeadFilter struct {
	Status      string `form:"status"`
	Priority    string `form:"priority"`
	Course      string `form:"course"`
	City        string `form:"city"`
	EmployeeID  string `form:"employee_id"`
	Today       bool   `form:"today"`
	FollowUpDue bool   `form:"follow_up_due"`
	Overdue     bool   `form:"overdue"`
	DateFrom    string `form:"date_from"`
	DateTo      string `form:"date_to"`
}

// LeadCallLog is one call/interaction record against a lead.
type LeadCallLog struct {
	ID            string     `json:"id"`
	ShortID       string     `json:"short_id"`
	LeadShortID   string     `json:"lead_short_id"`
	EmployeeID    string     `json:"employee_id"`
	EmployeeName  string     `json:"employee_name,omitempty"`
	Outcome       string     `json:"outcome"`
	StatusAfter   string     `json:"status_after"`
	Notes         string     `json:"notes,omitempty"`
	FollowUpSetAt *time.Time `json:"follow_up_set_at,omitempty"`
	CalledAt      time.Time  `json:"called_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// CreateCallLogInput records a call outcome + optional new next-follow-up
// date; this also drives the lead's own status/last_contacted_at update.
type CreateCallLogInput struct {
	Outcome        string     `json:"outcome" binding:"required,oneof=connected no_answer busy switched_off invalid_number callback_requested" example:"connected"`
	Status         string     `json:"status"  binding:"required,oneof=new contacted interested follow_up not_reachable converted not_interested lost" example:"interested"`
	Notes          string     `json:"notes"   example:"Asked for a callback next week"`
	NextFollowUpAt *time.Time `json:"next_follow_up_at"`
}

// LeadAssignmentHistoryEntry is one append-only assign/reassign/unassign event.
type LeadAssignmentHistoryEntry struct {
	ID               string     `json:"id"`
	ShortID          string     `json:"short_id"`
	LeadShortID      string     `json:"lead_short_id"`
	Action           string     `json:"action"`
	FromEmployeeID   string     `json:"from_employee_id,omitempty"`
	FromEmployeeName string     `json:"from_employee_name,omitempty"`
	ToEmployeeID     string     `json:"to_employee_id,omitempty"`
	ToEmployeeName   string     `json:"to_employee_name,omitempty"`
	PrioritySet      string     `json:"priority_set,omitempty"`
	FollowUpSetAt    *time.Time `json:"follow_up_set_at,omitempty"`
	PerformedBy      string     `json:"performed_by"`
	CreatedAt        time.Time  `json:"created_at"`
}

// LeadImportRow is one parsed row from an uploaded Excel/CSV file, before
// insertion — kept separate from CreateLeadInput so per-row errors can be
// reported without failing the whole batch.
type LeadImportRow struct {
	RowNumber      int    `json:"row_number"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	City           string `json:"city"`
	CourseInterest string `json:"course_interest"`
}

// LeadImportRowError reports why one row of an import was skipped.
type LeadImportRowError struct {
	RowNumber int    `json:"row_number"`
	Reason    string `json:"reason"`
}

// LeadImportResult summarizes a bulk import.
type LeadImportResult struct {
	Imported int                  `json:"imported"`
	Skipped  []LeadImportRowError `json:"skipped"`
}

// AssignLeadsInput assigns one or many leads to an employee at once (single
// assign is just the len==1 case of the same call).
type AssignLeadsInput struct {
	LeadShortIDs   []string   `json:"lead_short_ids" binding:"required,min=1"`
	EmployeeID     string     `json:"employee_id"    binding:"required"`
	Priority       string     `json:"priority"       binding:"omitempty,oneof=low medium high urgent"`
	NextFollowUpAt *time.Time `json:"next_follow_up_at"`
}

// ReassignLeadsInput moves already-assigned leads to a different employee.
type ReassignLeadsInput struct {
	LeadShortIDs   []string   `json:"lead_short_ids" binding:"required,min=1"`
	EmployeeID     string     `json:"employee_id"    binding:"required"`
	Priority       string     `json:"priority"       binding:"omitempty,oneof=low medium high urgent"`
	NextFollowUpAt *time.Time `json:"next_follow_up_at"`
}

// UnassignLeadsInput clears assignment on the given leads.
type UnassignLeadsInput struct {
	LeadShortIDs []string `json:"lead_short_ids" binding:"required,min=1"`
}

// AutoAssignInput triggers least-loaded round-robin assignment, optionally
// scoped to a course/city, across all active employees.
type AutoAssignInput struct {
	LeadShortIDs []string `json:"lead_short_ids" binding:"required,min=1"`
}

// LeadDashboard powers the employee's (or team_lead's own-quota) daily
// dashboard cards — everything derivable from leads/lead_call_logs alone.
// Leave balance / attendance status / monthly target are separate calls
// against their own endpoints (Phase 3).
type LeadDashboard struct {
	LeadsAssignedToday  int `json:"leads_assigned_today"`
	TotalActiveLeads    int `json:"total_active_leads"`
	CallsToMakeToday    int `json:"calls_to_make_today"`
	CallsCompletedToday int `json:"calls_completed_today"`
	PendingCalls        int `json:"pending_calls"`
	FollowUpsDueToday   int `json:"follow_ups_due_today"`
	OverdueFollowUps    int `json:"overdue_follow_ups"`
	NewLeads            int `json:"new_leads"`
	InterestedLeads     int `json:"interested_leads"`
	ConvertedLeads      int `json:"converted_leads"`
	NotReachableLeads   int `json:"not_reachable_leads"`
}

// EmployeeLeadSummary is one row of the admin's team-monitoring dashboard.
type EmployeeLeadSummary struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	ActiveLeads  int    `json:"active_leads"`
	CallsToday   int    `json:"calls_today"`
	Conversions  int    `json:"conversions"`
}
