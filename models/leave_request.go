package models

import "time"

// LeaveRequest is one employee's leave application.
type LeaveRequest struct {
	ID           string     `json:"id"`
	ShortID      string     `json:"short_id"`
	EmployeeID   string     `json:"employee_id"`
	EmployeeName string     `json:"employee_name,omitempty"`
	FromDate     string     `json:"from_date"`
	ToDate       string     `json:"to_date"`
	LeaveType    string     `json:"leave_type"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	AdminNote    string     `json:"admin_note,omitempty"`
	ReviewedBy   string     `json:"reviewed_by,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateLeaveRequestInput carries the fields for a new leave application.
type CreateLeaveRequestInput struct {
	FromDate  string `json:"from_date"  binding:"required" example:"2026-08-01"`
	ToDate    string `json:"to_date"    binding:"required" example:"2026-08-02"`
	LeaveType string `json:"leave_type" binding:"omitempty,oneof=full_day half_day" example:"full_day"`
	Reason    string `json:"reason"     binding:"required" example:"Family function"`
}

// ReviewLeaveRequestInput is the admin's approve/reject decision.
type ReviewLeaveRequestInput struct {
	Status    string `json:"status"     binding:"required,oneof=approved rejected" example:"approved"`
	AdminNote string `json:"admin_note" example:"Approved, enjoy!"`
}

// LeaveBalance is one employee's leave balance for a calendar year.
type LeaveBalance struct {
	EmployeeID string  `json:"employee_id"`
	Year       int     `json:"year"`
	TotalDays  float64 `json:"total_days"`
	UsedDays   float64 `json:"used_days"`
	Remaining  float64 `json:"remaining_days"`
}
