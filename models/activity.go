package models

// StudentStreak is a student's activity-streak computation — "active" means
// they submitted an assignment/project, attempted an exam, or attended a
// session on that calendar day.
type StudentStreak struct {
	StudentID       string  `json:"student_id"`
	StudentName     string  `json:"student_name,omitempty"`
	CurrentStreak   int     `json:"current_streak"`
	LongestStreak   int     `json:"longest_streak"`
	TotalActiveDays int     `json:"total_active_days"`
	LastActiveDate  *string `json:"last_active_date,omitempty"`
}

// AtRiskStudent is one student flagged by GET /batches/{short_id}/at-risk,
// with the specific reasons they were flagged.
type AtRiskStudent struct {
	StudentID     string   `json:"student_id"`
	StudentName   string   `json:"student_name,omitempty"`
	AttendancePct float64  `json:"attendance_percentage"`
	FinalScore    float64  `json:"final_score"`
	CurrentStreak int      `json:"current_streak"`
	DaysInactive  *int     `json:"days_inactive,omitempty"`
	Reasons       []string `json:"reasons"`
}
