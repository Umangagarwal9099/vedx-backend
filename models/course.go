package models

import "time"

type Course struct {
	ID           string     `json:"id"`
	ShortID      string     `json:"short_id"`
	CollegeID    string     `json:"college_id,omitempty"`
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
	// CollegeShortID is only honored for super_admin callers; every other
	// caller is forced onto their own college_id server-side. Omit for the
	// Internal EdTech Platform.
	CollegeShortID string `json:"college_short_id"`
	// CourseScope: "organization" (default, owned by exactly the one college
	// above) or "global" (a reusable master course assignable to many
	// colleges via POST /courses/{short_id}/assign).
	CourseScope  string   `json:"course_scope" binding:"omitempty,oneof=global organization" example:"organization"`
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
