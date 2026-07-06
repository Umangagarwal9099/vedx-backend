package models

import "time"

// Notification is the shared content of a notification, sent to one or more recipients.
type Notification struct {
	ID         string    `json:"id"`
	ShortID    string    `json:"short_id"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	Type       string    `json:"type"`
	RefType    string    `json:"ref_type,omitempty"`
	RefShortID string    `json:"ref_short_id,omitempty"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NotificationView is a single notification as seen in a recipient's inbox.
type NotificationView struct {
	ShortID    string     `json:"short_id"`
	Title      string     `json:"title"`
	Message    string     `json:"message"`
	Type       string     `json:"type"`
	RefType    string     `json:"ref_type,omitempty"`
	RefShortID string     `json:"ref_short_id,omitempty"`
	IsRead     bool       `json:"is_read"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateNotificationInput manually broadcasts a notification to one or more roles.
// Use role "all" to target every active user.
type CreateNotificationInput struct {
	Title   string   `json:"title"   binding:"required"        example:"Platform maintenance tonight"`
	Message string   `json:"message" binding:"required"        example:"We'll be down for maintenance from 11 PM to 1 AM."`
	Type    string   `json:"type"                              example:"general"`
	Roles   []string `json:"roles"   binding:"required,min=1"  example:"[\"student\",\"mentor\"]"`
}

// UpdateNotificationInput — all fields optional; edits the shared notification content.
type UpdateNotificationInput struct {
	Title   *string `json:"title"   example:"Updated title"`
	Message *string `json:"message" example:"Updated message"`
}

// MarkReadInput toggles the read state of a notification for the current user.
// Omit the body (or is_read) to mark it read.
type MarkReadInput struct {
	IsRead *bool `json:"is_read" example:"true"`
}
