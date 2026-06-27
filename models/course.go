package models

import "time"

type Course struct {
	ID           string     `json:"id"`
	ShortID      string     `json:"short_id"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	Thumbnail    string     `json:"thumbnail,omitempty"`
	Overview     string     `json:"overview,omitempty"`
	Objectives   []string   `json:"objectives"`
	Requirements []string   `json:"requirements"`
	Instructor   string     `json:"instructor,omitempty"`
	Duration     string     `json:"duration,omitempty"`
	Level        string     `json:"level,omitempty"`
	Category     string     `json:"category,omitempty"`
	IsActive     bool       `json:"is_active"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type CreateCourseInput struct {
	Name         string   `json:"name"         binding:"required"`
	Description  string   `json:"description"`
	Thumbnail    string   `json:"thumbnail"`
	Overview     string   `json:"overview"`
	Objectives   []string `json:"objectives"`
	Requirements []string `json:"requirements"`
	Instructor   string   `json:"instructor"`
	Duration     string   `json:"duration"`
	Level        string   `json:"level"`
	Category     string   `json:"category"`
}

type UpdateCourseInput struct {
	Name         *string  `json:"name"`
	Description  *string  `json:"description"`
	Thumbnail    *string  `json:"thumbnail"`
	Overview     *string  `json:"overview"`
	Objectives   []string `json:"objectives"`
	Requirements []string `json:"requirements"`
	Instructor   *string  `json:"instructor"`
	Duration     *string  `json:"duration"`
	Level        *string  `json:"level"`
	Category     *string  `json:"category"`
	IsActive     *bool    `json:"is_active"`
}
