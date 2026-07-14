package models

// BatchAnalyticsRow is one batch's rollup for the admin's Batch & Progress
// analytics page — real numbers computed from enrollments/attendance/
// certificates, not placeholders.
type BatchAnalyticsRow struct {
	BatchShortID          string  `json:"batch_short_id"`
	BatchNumber           string  `json:"batch_number"`
	CourseName            string  `json:"course_name"`
	StudentCount          int     `json:"student_count"`
	AvgCompletionPercent  float64 `json:"avg_completion_percentage"`
	AvgAttendanceRate     float64 `json:"avg_attendance_rate"`
	// AtRiskCount is a lightweight proxy (students under 60% attendance) —
	// not the full 3-factor at-risk algorithm behind GET /batches/:id/at-risk,
	// which is per-batch only. Good enough for a summary card across batches.
	AtRiskCount        int `json:"at_risk_count"`
	CertificatesIssued int `json:"certificates_issued"`
}

// BatchAttendanceTrendPoint is one week's average attendance rate for a batch.
type BatchAttendanceTrendPoint struct {
	WeekStart  string  `json:"week_start"`
	Attendance float64 `json:"attendance_rate"`
}
