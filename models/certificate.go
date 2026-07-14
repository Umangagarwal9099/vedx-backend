package models

import "time"

// Certificate is a durable record that a student completed a batch. Marks/rank
// are snapshotted at issuance time so a later re-grading doesn't rewrite history.
type Certificate struct {
	ID                string     `json:"id"`
	ShortID           string     `json:"short_id"`
	CertificateNumber string     `json:"certificate_number"`
	StudentID         string     `json:"student_id"`
	StudentName       string     `json:"student_name,omitempty"`
	BatchID           string     `json:"batch_id"`
	BatchShortID      string     `json:"batch_short_id,omitempty"`
	BatchNumber       string     `json:"batch_number,omitempty"`
	CourseID          string     `json:"course_id"`
	CourseName        string     `json:"course_name,omitempty"`
	FinalScore        *float64   `json:"final_score,omitempty"`
	FinalRank         *int       `json:"final_rank,omitempty"`
	IssuedBy          string     `json:"issued_by,omitempty"`
	IssuedAt          time.Time  `json:"issued_at"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// CertificateVerification is the public (unauthenticated) view returned by
// the verify-by-number endpoint — deliberately minimal, no internal IDs.
type CertificateVerification struct {
	Valid             bool      `json:"valid"`
	CertificateNumber string    `json:"certificate_number,omitempty"`
	StudentName       string    `json:"student_name,omitempty"`
	CourseName        string    `json:"course_name,omitempty"`
	BatchNumber       string    `json:"batch_number,omitempty"`
	FinalScore        *float64  `json:"final_score,omitempty"`
	FinalRank         *int      `json:"final_rank,omitempty"`
	IssuedAt          time.Time `json:"issued_at,omitempty"`
}
