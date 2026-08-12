package models

// DashboardStats is the admin home-screen summary — every field is computed
// from real tables (batches, enrollments, sessions, attendance, certificates).
// There's deliberately no "views/ratings/time-spent/DAU" here: nothing in this
// backend tracks page views, session duration, or ratings, so those numbers
// would have to be fabricated.
type DashboardStats struct {
	ActiveBatches         int                    `json:"active_batches"`
	ActiveStudents        int                    `json:"active_students"`
	CertificatesIssued30d int                    `json:"certificates_issued_30d"`
	AvgAttendanceRate7d   float64                `json:"avg_attendance_rate_7d"`
	SessionsToday         []DashboardSession     `json:"sessions_today"`
	EnrollmentTrend30d    []EnrollmentTrendPoint `json:"enrollment_trend_30d"`
}

// DashboardSession is one row in the "Today's Sessions" list — attendance
// counts are whatever's been marked so far (usually 0 for a session that
// hasn't started yet).
type DashboardSession struct {
	ShortID      string `json:"short_id"`
	Name         string `json:"name"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	BatchNumber  string `json:"batch_number"`
	MentorName   string `json:"mentor_name"`
	PresentCount int    `json:"present_count"`
	TotalCount   int    `json:"total_count"`
	// ZoomJoinURL/ZoomStartURL back the dashboard's "Join" button — never
	// stripped here the way student-facing endpoints strip ZoomStartURL,
	// since this endpoint is college-staff-only (see collegeReadOrAbove on
	// GET /dashboard/stats). Omitted (both empty) for offline sessions or
	// ones whose Zoom meeting hasn't been created.
	ZoomJoinURL  string `json:"zoom_join_url,omitempty"`
	ZoomStartURL string `json:"zoom_start_url,omitempty"`
}

// EnrollmentTrendPoint is one day's new-enrollment count.
type EnrollmentTrendPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
