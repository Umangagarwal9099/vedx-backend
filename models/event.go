package models

import "time"

type Event struct {
	ID                 string     `json:"id"`
	ShortID            string     `json:"short_id"`
	Name               string     `json:"name"`
	EventDate          string     `json:"event_date"`
	StartTime          string     `json:"start_time"`
	EndTime            string     `json:"end_time"`
	ImageURL           string     `json:"image_url,omitempty"`
	Description        string     `json:"description,omitempty"`
	Status             string     `json:"status"`
	Mode               string     `json:"mode"`
	GuestAccess        bool       `json:"guest_access"`
	EventManagerID     string     `json:"event_manager_id,omitempty"`
	EventManagerName   string     `json:"event_manager_name,omitempty"`
	Categories         []string   `json:"categories"`
	IsActive           bool       `json:"is_active"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

type CreateEventInput struct {
	Name         string   `json:"name"          binding:"required"                           example:"Go Workshop 2025"`
	EventDate    string   `json:"event_date"    binding:"required"                           example:"2025-09-15"`
	StartTime    string   `json:"start_time"    binding:"required"                           example:"10:00"`
	EndTime      string   `json:"end_time"      binding:"required"                           example:"13:00"`
	ImageURL     string   `json:"image_url"                                                  example:"https://cdn.example.com/event.jpg"`
	Description  string   `json:"description"                                                example:"<p><b>Join us</b> for a hands-on Go workshop.</p>"`
	Status       string   `json:"status"        binding:"required,oneof=published unpublished" example:"unpublished"`
	Mode         string   `json:"mode"          binding:"required,oneof=virtual in_person"   example:"virtual"`
	GuestAccess  bool     `json:"guest_access"                                               example:"false"`
	EventManager string   `json:"event_manager"                                              example:"uuid-of-manager"`
	Categories   []string `json:"categories"                                                 example:"['workshop','go','backend']"`
}

// UpdateEventInput — all fields optional; send only what you want to change.
type UpdateEventInput struct {
	Name         *string  `json:"name"          example:"Go Workshop 2025 — Updated"`
	EventDate    *string  `json:"event_date"    example:"2025-09-20"`
	StartTime    *string  `json:"start_time"    example:"09:00"`
	EndTime      *string  `json:"end_time"      example:"12:00"`
	ImageURL     *string  `json:"image_url"     example:"https://cdn.example.com/new.jpg"`
	Description  *string  `json:"description"   example:"<p>Updated description</p>"`
	Status       *string  `json:"status"        example:"published"`
	Mode         *string  `json:"mode"          example:"in_person"`
	GuestAccess  *bool    `json:"guest_access"  example:"true"`
	EventManager *string  `json:"event_manager" example:"uuid-of-manager"`
	Categories   []string `json:"categories"    example:"['conference','go']"`
	IsActive     *bool    `json:"is_active"     example:"false"`
}

// EventFilter holds query params for GET /events search.
type EventFilter struct {
	Name   string `form:"name"`
	Status string `form:"status"`
}
