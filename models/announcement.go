package models

import "time"

type Announcement struct {
	ID         string     `json:"id"`
	ShortID    string     `json:"short_id"`
	Name       string     `json:"name"`
	Description string    `json:"description,omitempty"`
	ImageURL   string     `json:"image_url,omitempty"`
	Urgency    string     `json:"urgency"`
	Visibility string     `json:"visibility"`
	IsActive   bool       `json:"is_active"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type CreateAnnouncementInput struct {
	Name        string `json:"name"        binding:"required"                                              example:"System Maintenance on June 10"`
	Description string `json:"description"                                                                  example:"<p><b>Important:</b> The platform will be down for 2 hours.</p>"`
	ImageURL    string `json:"image_url"                                                                    example:"https://cdn.example.com/banner.jpg"`
	Urgency     string `json:"urgency"     binding:"required,oneof=low medium high"                         example:"high"`
	Visibility  string `json:"visibility"  binding:"required,oneof=existing_only existing_and_new"          example:"existing_only"`
}

// UpdateAnnouncementInput — all fields optional; send only what you want to change.
type UpdateAnnouncementInput struct {
	Name        *string `json:"name"        example:"Updated Announcement Title"`
	Description *string `json:"description" example:"<p>Updated description</p>"`
	ImageURL    *string `json:"image_url"   example:"https://cdn.example.com/new-banner.jpg"`
	Urgency     *string `json:"urgency"     example:"medium"`
	Visibility  *string `json:"visibility"  example:"existing_and_new"`
	IsActive    *bool   `json:"is_active"   example:"false"`
}

// AnnouncementFilter holds query params for GET /announcements search.
type AnnouncementFilter struct {
	Name     string `form:"name"`
	Urgency  string `form:"urgency"`
	IsActive string `form:"is_active"` // "true" | "false" | ""
}
