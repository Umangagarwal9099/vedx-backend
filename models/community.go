package models

import "time"

type Community struct {
	ID            string     `json:"id"`
	ShortID       string     `json:"short_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	BatchID       string     `json:"batch_id"`
	BatchShortID  string     `json:"batch_short_id"`
	BatchNumber   string     `json:"batch_number"`
	IsActive      bool       `json:"is_active"`
	CreatedBy     string     `json:"created_by"`
	CreatedByName string     `json:"created_by_name"`
	MemberCount   int        `json:"member_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
}

type CreateCommunityInput struct {
	Name         string `json:"name"           binding:"required" example:"Batch 2024 Community"`
	Description  string `json:"description"                       example:"Community space for Batch 2024 students and mentors"`
	BatchShortID string `json:"batch_short_id" binding:"required" example:"A3F72C1D"`
}

// UpdateCommunityInput — all fields optional; send only what you want to change.
type UpdateCommunityInput struct {
	Name         *string `json:"name"           example:"Batch 2024 Community"`
	Description  *string `json:"description"    example:"Updated description"`
	BatchShortID *string `json:"batch_short_id" example:"B4G83D2E"`
	IsActive     *bool   `json:"is_active"      example:"false"`
}

// CommunityMember represents a single user's membership in a community.
type CommunityMember struct {
	UserID    string    `json:"user_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

// AddCommunityMembersInput carries one or more user IDs to add to a community.
type AddCommunityMembersInput struct {
	UserIDs []string `json:"user_ids" binding:"required,min=1" example:"[\"11111111-1111-1111-1111-111111111111\"]"`
}
