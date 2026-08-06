package models

import "time"

// MonthlyTarget is one employee's conversion target for a calendar month.
// "Achieved" is always computed on read from leads.status='converted', never
// stored, so it can't drift out of sync with the leads table.
type MonthlyTarget struct {
	ID                string    `json:"id"`
	ShortID           string    `json:"short_id"`
	EmployeeID        string    `json:"employee_id"`
	EmployeeName      string    `json:"employee_name,omitempty"`
	Year              int       `json:"year"`
	Month             int       `json:"month"`
	TargetConversions int       `json:"target_conversions"`
	Achieved          int       `json:"achieved"`
	DailyTarget       float64   `json:"daily_target"`
	AchievedToday     int       `json:"achieved_today"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// SetMonthlyTargetInput sets/updates an employee's target for a month.
type SetMonthlyTargetInput struct {
	Year              int `json:"year"               binding:"required" example:"2026"`
	Month             int `json:"month"              binding:"required,min=1,max=12" example:"7"`
	TargetConversions int `json:"target_conversions" binding:"required,min=0" example:"10"`
}
