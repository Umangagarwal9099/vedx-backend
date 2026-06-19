package models

import "time"

type CodingExample struct {
	Input       string `json:"input"`
	Output      string `json:"output"`
	Explanation string `json:"explanation,omitempty"`
}

type CodingTestCase struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	IsHidden       bool   `json:"is_hidden"`
}

// StarterCode maps language name to starter code string, e.g. {"python": "...", "java": "..."}
type StarterCode map[string]string

type CodingQuestion struct {
	ID          string           `json:"id"`
	ShortID     string           `json:"short_id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Difficulty  string           `json:"difficulty"`
	Topics      []string         `json:"topics"`
	Languages   []string         `json:"languages"`
	Constraints []string         `json:"constraints"`
	Examples    []CodingExample  `json:"examples"`
	StarterCode StarterCode      `json:"starter_code"`
	TestCases   []CodingTestCase `json:"test_cases"`
	IsActive    bool             `json:"is_active"`
	CreatedBy   string           `json:"created_by"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	DeletedAt   *time.Time       `json:"deleted_at,omitempty"`
}

type CreateCodingQuestionInput struct {
	Title       string           `json:"title"        binding:"required"`
	Description string           `json:"description"  binding:"required"`
	Difficulty  string           `json:"difficulty"   binding:"required"`
	Topics      []string         `json:"topics"`
	Languages   []string         `json:"languages"    binding:"required"`
	Constraints []string         `json:"constraints"`
	Examples    []CodingExample  `json:"examples"`
	StarterCode StarterCode      `json:"starter_code" binding:"required"`
	TestCases   []CodingTestCase `json:"test_cases"   binding:"required"`
}

type UpdateCodingQuestionInput struct {
	Title       *string          `json:"title"`
	Description *string          `json:"description"`
	Difficulty  *string          `json:"difficulty"`
	Topics      []string         `json:"topics"`
	Languages   []string         `json:"languages"`
	Constraints []string         `json:"constraints"`
	Examples    []CodingExample  `json:"examples"`
	StarterCode *StarterCode     `json:"starter_code"`
	TestCases   []CodingTestCase `json:"test_cases"`
	IsActive    *bool            `json:"is_active"`
}
