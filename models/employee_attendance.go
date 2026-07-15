package models

import "time"

// EmployeeAttendance is one employee's attendance row for one calendar day —
// self check-in/check-out only, admin only views the result.
type EmployeeAttendance struct {
	ID           string     `json:"id"`
	ShortID      string     `json:"short_id"`
	EmployeeID   string     `json:"employee_id"`
	EmployeeName string     `json:"employee_name,omitempty"`
	Date         string     `json:"date"`
	Status       string     `json:"status"`
	CheckInAt    *time.Time `json:"check_in_at,omitempty"`
	CheckOutAt   *time.Time `json:"check_out_at,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
