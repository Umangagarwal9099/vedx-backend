package models

import "time"

type Banner struct {
	ID        string     `json:"id"`
	ShortID   string     `json:"short_id"`
	Name      string     `json:"name"`
	Branches  []string   `json:"branches"`
	Thumbnail string     `json:"thumbnail,omitempty"`
	CTAURL    string     `json:"cta_url,omitempty"`
	Category  string     `json:"category,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type CreateBannerInput struct {
	Name      string   `json:"name"      binding:"required" example:"Summer Sale"`
	Branches  []string `json:"branches"  binding:"required" example:"[\"engineering\",\"design\"]"`
	Thumbnail string   `json:"thumbnail"                   example:"https://cdn.example.com/banner.jpg"`
	CTAURL    string   `json:"cta_url"                     example:"https://example.com/sale"`
	Category  string   `json:"category"                    example:"promotion"`
	IsActive  bool     `json:"is_active"                   example:"true"`
}

// UpdateBannerInput — all fields optional; send only what you want to change.
type UpdateBannerInput struct {
	Name      *string  `json:"name"      example:"Updated Banner Name"`
	Branches  []string `json:"branches"  example:"[\"design\",\"marketing\"]"`
	Thumbnail *string  `json:"thumbnail" example:"https://cdn.example.com/new-banner.jpg"`
	CTAURL    *string  `json:"cta_url"   example:"https://example.com/new-sale"`
	Category  *string  `json:"category"  example:"event"`
	IsActive  *bool    `json:"is_active" example:"false"`
}

// BannerFilter holds query params for GET /banners.
type BannerFilter struct {
	Name     string `form:"name"`
	Category string `form:"category"`
}
