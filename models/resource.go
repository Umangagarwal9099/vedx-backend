package models

import "time"

// Resource is a learning material uploaded by a mentor or admin. Setting
// batch_id makes it visible only to students enrolled in that batch; leaving
// it unset (global) makes it visible to every student.
type Resource struct {
	ID             string     `json:"id"`
	ShortID        string     `json:"short_id"`
	Title          string     `json:"title"`
	Description    string     `json:"description,omitempty"`
	URL            string     `json:"url"`
	ResourceType   string     `json:"resource_type"` // pdf | document | video | link | image | code | other
	BatchShortID   string     `json:"batch_short_id,omitempty"`
	BatchNumber    string     `json:"batch_number,omitempty"`
	ModuleShortID  string     `json:"module_short_id,omitempty"`
	ModuleName     string     `json:"module_name,omitempty"`
	SessionShortID string     `json:"session_short_id,omitempty"`
	SessionName    string     `json:"session_name,omitempty"`
	IsDownloadable bool       `json:"is_downloadable"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedByName  string     `json:"created_by_name,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// CreateResourceInput carries the fields required to publish a new resource.
// Leave batch_short_id empty to make the resource globally visible to every
// student; set it to scope visibility to one batch. module_short_id and
// session_short_id are optional further ties.
type CreateResourceInput struct {
	Title          string     `json:"title"           binding:"required"                                              example:"Python Cheatsheet — Variables & Data Types"`
	Description    string     `json:"description"                                                                      example:"Quick reference for week 1."`
	URL            string     `json:"url"             binding:"required"                                              example:"https://cdn.example.com/cheatsheet.pdf"`
	ResourceType   string     `json:"resource_type"   binding:"required,oneof=pdf document video link image code other" example:"pdf"`
	BatchShortID   string     `json:"batch_short_id"                                                                   example:"use GET /batches to pick a real short_id, or omit for global visibility"`
	ModuleShortID  string     `json:"module_short_id"                                                                  example:"use GET /modules to pick a real short_id"`
	SessionShortID string     `json:"session_short_id"                                                                 example:"use GET /sessions to pick a real short_id"`
	IsDownloadable bool       `json:"is_downloadable"                                                                  example:"true"`
	ExpiresAt      *time.Time `json:"expires_at"                                                                       example:"2026-12-31T23:59:00Z"`
}

// UpdateResourceInput — all fields optional; send only what you want to change.
// To clear batch/module/session scoping (make global again), send an empty string.
type UpdateResourceInput struct {
	Title          *string    `json:"title"           example:"Updated title"`
	Description    *string    `json:"description"      example:"Updated description"`
	URL            *string    `json:"url"              example:"https://cdn.example.com/new-file.pdf"`
	ResourceType   *string    `json:"resource_type"    binding:"omitempty,oneof=pdf document video link image code other" example:"video"`
	BatchShortID   *string    `json:"batch_short_id"   example:""`
	ModuleShortID  *string    `json:"module_short_id"  example:""`
	SessionShortID *string    `json:"session_short_id" example:""`
	IsDownloadable *bool      `json:"is_downloadable"  example:"false"`
	ExpiresAt      *time.Time `json:"expires_at"       example:"2027-01-01T00:00:00Z"`
}

// ResourceFilter holds query params for GET /resources.
type ResourceFilter struct {
	BatchShortID string `form:"batch_short_id"`
	ResourceType string `form:"resource_type"`
}
