package models

import "time"

// LoginActivityDevice is one device a user has logged in from.
type LoginActivityDevice struct {
	ID             string     `json:"id"`
	DeviceID       string     `json:"device_id"`
	DeviceType     string     `json:"device_type"`
	OSName         string     `json:"os_name"`
	BrowserName    string     `json:"browser_name"`
	BrowserVersion string     `json:"browser_version"`
	LoginCount     int        `json:"login_count"`
	LastLoginAt    time.Time  `json:"last_login_at"`
	RemovedAt      *time.Time `json:"removed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// LoginActivitySummary is the aggregate + device list shown on the admin's
// learner-detail Activity tab.
type LoginActivitySummary struct {
	TotalLogins           int                   `json:"total_logins"`
	RegisteredDeviceCount int                   `json:"registered_device_count"`
	RemovedDeviceCount    int                   `json:"removed_device_count"`
	Devices               []LoginActivityDevice `json:"devices"`
}
