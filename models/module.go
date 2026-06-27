package models

import "time"

// ── Curriculum (nested response for GET /courses/:id/curriculum) ──────────────

type MaterialSummary struct {
	ShortID         string  `json:"short_id"`
	MaterialName    string  `json:"material_name"`
	MaterialType    string  `json:"material_type"`
	FileURL         *string `json:"file_url,omitempty"`
	EnableDownloads bool    `json:"enable_downloads"`
	IsPrerequisite  bool    `json:"is_prerequisite"`
	IsActive        bool    `json:"is_active"`
}

type SectionSummary struct {
	ShortID          string            `json:"short_id"`
	SectionName      string            `json:"section_name"`
	ShortDescription string            `json:"short_description"`
	IsPrerequisite   bool              `json:"is_prerequisite"`
	IsActive         bool              `json:"is_active"`
	Materials        []MaterialSummary `json:"materials"`
}

type ModuleWithSections struct {
	ShortID          string           `json:"short_id"`
	ModuleName       string           `json:"module_name"`
	ModuleBranch     string           `json:"module_branch"`
	MaxViewDuration  string           `json:"max_view_duration"`
	WatchTimeMinutes *int             `json:"watch_time_minutes,omitempty"`
	IsActive         bool             `json:"is_active"`
	OrderIndex       int              `json:"order_index"`
	Sections         []SectionSummary `json:"sections"`
}

type AssignModuleInput struct {
	ModuleShortID string `json:"module_short_id" binding:"required"`
	OrderIndex    int    `json:"order_index"`
}

type Module struct {
	ID               string     `json:"id"`
	ShortID          string     `json:"short_id"`
	ModuleName       string     `json:"module_name"`
	ModuleBranch     string     `json:"module_branch"`
	MaxViewDuration  string     `json:"max_view_duration"` // "unlimited" | "restricted"
	WatchTimeMinutes *int       `json:"watch_time_minutes,omitempty"`
	IsActive         bool       `json:"is_active"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type CreateModuleInput struct {
	ModuleName       string `json:"module_name"       binding:"required"                           example:"Introduction to Go"`
	ModuleBranch     string `json:"module_branch"     binding:"required"                           example:"Computer Science"`
	MaxViewDuration  string `json:"max_view_duration" binding:"required,oneof=unlimited restricted" example:"restricted"`
	WatchTimeMinutes *int   `json:"watch_time_minutes"                                              example:"120"`
}

// UpdateModuleInput — all fields optional; send only what you want to change.
type UpdateModuleInput struct {
	ModuleName       *string `json:"module_name"        example:"Advanced Go"`
	ModuleBranch     *string `json:"module_branch"      example:"Software Engineering"`
	MaxViewDuration  *string `json:"max_view_duration"  example:"unlimited"`
	WatchTimeMinutes *int    `json:"watch_time_minutes" example:"180"`
	IsActive         *bool   `json:"is_active"          example:"false"`
}

// ModuleFilter holds query params for GET /modules/filter.
type ModuleFilter struct {
	ModuleName      string `form:"module_name"`
	ModuleBranch    string `form:"module_branch"`
	MaxViewDuration string `form:"max_view_duration"` // "unlimited" | "restricted"
	IsActive        string `form:"is_active"`         // "true" | "false" | ""
}

// ── Sections ──────────────────────────────────────────────────────────────────

type ModuleSection struct {
	ID               string     `json:"id"`
	ShortID          string     `json:"short_id"`
	ModuleID         string     `json:"module_id"`
	SectionName      string     `json:"section_name"`
	ShortDescription string     `json:"short_description"`
	IsPrerequisite   bool       `json:"is_prerequisite"`
	IsActive         bool       `json:"is_active"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type CreateModuleSectionInput struct {
	SectionName      string `json:"section_name"      binding:"required" example:"Introduction"`
	ShortDescription string `json:"short_description"                    example:"Overview of the section"`
	IsPrerequisite   bool   `json:"is_prerequisite"                      example:"false"`
}

// UpdateModuleSectionInput — all fields optional; send only what you want to change.
type UpdateModuleSectionInput struct {
	SectionName      *string `json:"section_name"      example:"Advanced Topics"`
	ShortDescription *string `json:"short_description" example:"Deep dive into the topic"`
	IsPrerequisite   *bool   `json:"is_prerequisite"   example:"true"`
	IsActive         *bool   `json:"is_active"         example:"false"`
}

// ── Section Materials ──────────────────────────────────────────────────────────

// MaterialType enumerates all supported material kinds.
type MaterialType string

const (
	MaterialTypeLink       MaterialType = "link"
	MaterialTypeImage      MaterialType = "image"
	MaterialTypeVideo      MaterialType = "video"
	MaterialTypePDF        MaterialType = "pdf"
	MaterialTypeAudio      MaterialType = "audio"
	MaterialTypeDoc        MaterialType = "doc"
	MaterialTypeSheet      MaterialType = "sheet"
	MaterialTypeFile       MaterialType = "file"
	MaterialTypeZip        MaterialType = "zip"
	MaterialTypeSlide      MaterialType = "slide"
	MaterialTypeAssignment MaterialType = "assignment"
	MaterialTypeForm       MaterialType = "form"
	MaterialTypeExercise   MaterialType = "exercise"
)

type SectionMaterial struct {
	ID              string       `json:"id"`
	ShortID         string       `json:"short_id"`
	SectionID       string       `json:"section_id"`
	MaterialName    string       `json:"material_name"`
	MaterialType    MaterialType `json:"material_type"`
	FileURL         *string      `json:"file_url,omitempty"`
	MaxViews        string       `json:"max_views"`
	MaxViewsCount   *int         `json:"max_views_count,omitempty"`
	IsPrerequisite  bool         `json:"is_prerequisite"`
	EnableDownloads bool         `json:"enable_downloads"`
	AllowAccessOn   string       `json:"allow_access_on"`
	IsActive        bool         `json:"is_active"`
	CreatedBy       string       `json:"created_by"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	DeletedAt       *time.Time   `json:"deleted_at,omitempty"`
}

type CreateMaterialInput struct {
	MaterialName    string       `json:"material_name"    binding:"required"                                                                                                  example:"Intro Video"`
	MaterialType    MaterialType `json:"material_type"    binding:"required,oneof=link image video pdf audio doc sheet file zip slide assignment form exercise"               example:"video"`
	FileURL         *string      `json:"file_url"                                                                                                                             example:"https://example.com/video.mp4"`
	MaxViews        string       `json:"max_views"        binding:"required,oneof=unlimited limited"                                                                          example:"unlimited"`
	MaxViewsCount   *int         `json:"max_views_count"                                                                                                                      example:"5"`
	IsPrerequisite  bool         `json:"is_prerequisite"                                                                                                                      example:"false"`
	EnableDownloads bool         `json:"enable_downloads"                                                                                                                     example:"true"`
	AllowAccessOn   string       `json:"allow_access_on"  binding:"required,oneof=both app"                                                                                   example:"both"`
}

// UpdateMaterialInput — all fields optional; send only what you want to change.
type UpdateMaterialInput struct {
	MaterialName    *string       `json:"material_name"    example:"Updated Intro Video"`
	MaterialType    *MaterialType `json:"material_type"    example:"pdf"`
	FileURL         *string       `json:"file_url"         example:"https://example.com/updated.pdf"`
	MaxViews        *string       `json:"max_views"        example:"limited"`
	MaxViewsCount   *int          `json:"max_views_count"  example:"10"`
	IsPrerequisite  *bool         `json:"is_prerequisite"  example:"true"`
	EnableDownloads *bool         `json:"enable_downloads" example:"false"`
	AllowAccessOn   *string       `json:"allow_access_on"  example:"app"`
	IsActive        *bool         `json:"is_active"        example:"false"`
}
