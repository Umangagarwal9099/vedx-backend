package models

import "time"

// EmployeeAttendance is one employee's attendance row for one calendar day —
// self check-in/check-out only, admin only views the result.
type EmployeeAttendance struct {
	ID                string     `json:"id"`
	ShortID           string     `json:"short_id"`
	EmployeeID        string     `json:"employee_id"`
	EmployeeName      string     `json:"employee_name,omitempty"`
	Date              string     `json:"date"`
	Status            string     `json:"status"`
	CheckInAt         *time.Time `json:"check_in_at,omitempty"`
	CheckInLat        *float64   `json:"check_in_lat,omitempty"`
	CheckInLng        *float64   `json:"check_in_lng,omitempty"`
	CheckInSelfieURL  string     `json:"check_in_selfie_url,omitempty"`
	CheckOutAt        *time.Time `json:"check_out_at,omitempty"`
	CheckOutLat       *float64   `json:"check_out_lat,omitempty"`
	CheckOutLng       *float64   `json:"check_out_lng,omitempty"`
	CheckOutSelfieURL string     `json:"check_out_selfie_url,omitempty"`
	Notes             string     `json:"notes,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CheckInInput carries the GPS location + selfie captured at check-in.
type CheckInInput struct {
	Latitude  float64 `json:"latitude"   binding:"min=-90,max=90"`
	Longitude float64 `json:"longitude"  binding:"min=-180,max=180"`
	SelfieURL string  `json:"selfie_url" binding:"required"`
}

// CheckOutInput is identical in shape to CheckInInput — kept separate since
// check-in/out are conceptually distinct actions even though the payload
// matches today.
type CheckOutInput struct {
	Latitude  float64 `json:"latitude"   binding:"min=-90,max=90"`
	Longitude float64 `json:"longitude"  binding:"min=-180,max=180"`
	SelfieURL string  `json:"selfie_url" binding:"required"`
}
