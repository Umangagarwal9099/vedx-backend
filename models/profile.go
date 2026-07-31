package models

import "time"

// ProfileDetails holds the personal bio/social fields shown on a user's
// profile page — distinct from the core `users` row (name/email/phone) and
// from the role-specific academic/HR profile structs in user.go.
type ProfileDetails struct {
	UserID      string    `json:"user_id"`
	Phone       string    `json:"phone,omitempty"`
	Bio         string    `json:"bio"`
	Location    string    `json:"location"`
	Education   string    `json:"education"`
	Skills      []string  `json:"skills"`
	LinkedInURL string    `json:"linkedin_url"`
	GithubURL   string    `json:"github_url"`
	ResumeURL   string    `json:"resume_url"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpdateProfileDetailsInput carries the editable fields — all optional, send
// only what changed. Phone writes through to the `users` table itself.
type UpdateProfileDetailsInput struct {
	Phone       *string   `json:"phone"`
	Bio         *string   `json:"bio"`
	Location    *string   `json:"location"`
	Education   *string   `json:"education"`
	Skills      *[]string `json:"skills"`
	LinkedInURL *string   `json:"linkedin_url"`
	GithubURL   *string   `json:"github_url"`
	ResumeURL   *string   `json:"resume_url"`
}
