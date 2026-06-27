package models

import "time"

type Blog struct {
	ID                   string     `json:"id"`
	ShortID              string     `json:"short_id"`
	Title                string     `json:"title"`
	Content              string     `json:"content"`
	Excerpt              string     `json:"excerpt,omitempty"`
	Author               string     `json:"author"`
	Status               string     `json:"status"`
	PublishAt            *time.Time `json:"publish_at,omitempty"`
	IsFeatured           bool       `json:"is_featured"`
	ShowInRecentUpdates  bool       `json:"show_in_recent_updates"`
	FeaturedImage        string     `json:"featured_image,omitempty"`
	IsActive             bool       `json:"is_active"`
	CreatedBy            string     `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
}

type CreateBlogInput struct {
	Title               string     `json:"title"                  binding:"required"                                  example:"Introduction to Go"`
	Content             string     `json:"content"                binding:"required"                                  example:"<p>Go is a statically typed language...</p>"`
	Excerpt             string     `json:"excerpt"                                                                     example:"A brief introduction to the Go programming language."`
	Author              string     `json:"author"                 binding:"required"                                  example:"John Doe"`
	Status              string     `json:"status"                 binding:"required,oneof=published draft scheduled"  example:"draft"`
	PublishAt           *time.Time `json:"publish_at"                                                                  example:"2025-09-15T10:00:00Z"`
	IsFeatured          bool       `json:"is_featured"                                                                 example:"false"`
	ShowInRecentUpdates bool       `json:"show_in_recent_updates"                                                      example:"false"`
	FeaturedImage       string     `json:"featured_image"                                                              example:"https://cdn.example.com/blog.jpg"`
}

// UpdateBlogInput — all fields optional; send only what you want to change.
type UpdateBlogInput struct {
	Title               *string    `json:"title"                  example:"Updated Blog Title"`
	Content             *string    `json:"content"                example:"<p>Updated content...</p>"`
	Excerpt             *string    `json:"excerpt"                example:"Updated excerpt"`
	Author              *string    `json:"author"                 example:"Jane Doe"`
	Status              *string    `json:"status"                 example:"published"`
	PublishAt           *time.Time `json:"publish_at"             example:"2025-10-01T09:00:00Z"`
	IsFeatured          *bool      `json:"is_featured"            example:"true"`
	ShowInRecentUpdates *bool      `json:"show_in_recent_updates" example:"true"`
	FeaturedImage       *string    `json:"featured_image"         example:"https://cdn.example.com/new-blog.jpg"`
	IsActive            *bool      `json:"is_active"              example:"false"`
}

// BlogFilter holds query params for GET /blogs.
type BlogFilter struct {
	Title  string `form:"title"`
	Status string `form:"status"`
	Date   string `form:"date"` // YYYY-MM-DD — filters by DATE(created_at)
}
