package models

import "time"

// FAQ is a single Help & Support question/answer entry, admin-managed.
// Only published FAQs are visible to students; staff/admin see all.
type FAQ struct {
	ID          string    `json:"id"`
	ShortID     string    `json:"short_id"`
	Question    string    `json:"question"`
	Answer      string    `json:"answer"`
	Category    string    `json:"category"`
	OrderIndex  int       `json:"order_index"`
	IsPublished bool      `json:"is_published"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateFAQInput struct {
	Question    string `json:"question"    binding:"required"`
	Answer      string `json:"answer"      binding:"required"`
	Category    string `json:"category"`
	OrderIndex  int    `json:"order_index"`
	IsPublished *bool  `json:"is_published"`
}

// UpdateFAQInput carries the editable fields — all optional, send only what changed.
type UpdateFAQInput struct {
	Question    *string `json:"question"`
	Answer      *string `json:"answer"`
	Category    *string `json:"category"`
	OrderIndex  *int    `json:"order_index"`
	IsPublished *bool   `json:"is_published"`
}

type SupportTicketStatus string

const (
	SupportTicketOpen       SupportTicketStatus = "open"
	SupportTicketInProgress SupportTicketStatus = "in_progress"
	SupportTicketResolved   SupportTicketStatus = "resolved"
)

// SupportTicket is a "Contact Support" submission. Name/email/phone are
// exactly what the student typed on the form (not read live off their
// profile), so a ticket stays an accurate record of what was submitted even
// if the profile changes later.
type SupportTicket struct {
	ID          string              `json:"id"`
	ShortID     string              `json:"short_id"`
	UserID      string              `json:"user_id"`
	StudentName string              `json:"student_name,omitempty"` // joined in for admin list views
	Name        string              `json:"name"`
	Email       string              `json:"email"`
	Phone       string              `json:"phone"`
	Subject     string              `json:"subject"`
	Message     string              `json:"message"`
	Status      SupportTicketStatus `json:"status"`
	ResolvedAt  *time.Time          `json:"resolved_at,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type CreateSupportTicketInput struct {
	Name    string `json:"name"    binding:"required"`
	Email   string `json:"email"   binding:"required,email"`
	Phone   string `json:"phone"   binding:"required"`
	Subject string `json:"subject" binding:"required"`
	Message string `json:"message" binding:"required"`
}

type UpdateSupportTicketStatusInput struct {
	Status SupportTicketStatus `json:"status" binding:"required,oneof=open in_progress resolved"`
}
