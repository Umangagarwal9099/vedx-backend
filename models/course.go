package models

import "time"

type Course struct {
	ID          string     `json:"id"`
	ShortID     string     `json:"short_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Thumbnail   string     `json:"thumbnail,omitempty"`
	IsActive    bool       `json:"is_active"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type CreateCourseInput struct {
	Name        string `json:"name"        binding:"required" example:"Go for Beginners"`
	Description string `json:"description"                    example:"Learn Go from scratch"`
	Thumbnail   string `json:"thumbnail"                      example:"https://cdn.example.com/thumb.jpg"`
}

// UpdateCourseInput — all fields optional; send only what you want to change.
type UpdateCourseInput struct {
	Name        *string `json:"name"        example:"Go Advanced"`
	Description *string `json:"description" example:"Deep dive into Go internals"`
	Thumbnail   *string `json:"thumbnail"   example:"https://cdn.example.com/new-thumb.jpg"`
	IsActive    *bool   `json:"is_active"   example:"false"`
}
