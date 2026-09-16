package models

import "time"

// TeamMember is one Operations employee reporting to a Team Lead.
type TeamMember struct {
	UserID     string    `json:"user_id"`
	FirstName  string    `json:"first_name"`
	LastName   string    `json:"last_name"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	AssignedAt time.Time `json:"assigned_at"`
}

// Team is one Team Lead plus everyone currently assigned to them.
type Team struct {
	TeamLeadID   string       `json:"team_lead_id"`
	TeamLeadName string       `json:"team_lead_name"`
	Members      []TeamMember `json:"members"`
}

// AddTeamMemberInput names the Operations employee being added to a team.
type AddTeamMemberInput struct {
	MemberID string `json:"member_id" binding:"required"`
}

// StaffOption is a minimal user picked from a narrow, department-scoped
// list — used to populate the "choose a Team Lead" / "choose a member"
// pickers without exposing the general user directory to a Manager, who
// only ever needs these two specific slices of it.
type StaffOption struct {
	UserID    string `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}
