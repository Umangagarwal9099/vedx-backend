package models

// CategoryScore is one scoring category's (assignments/exams/projects) earned
// vs. max marks for a student, plus the derived percentage.
type CategoryScore struct {
	Earned     float64 `json:"earned_marks"`
	Max        float64 `json:"max_marks"`
	Percentage float64 `json:"percentage"`
}

// StudentScoreBreakdown is one student's full score computation within a
// batch — the three category scores, the weighted final score, and their
// rank among the batch's other students.
type StudentScoreBreakdown struct {
	StudentID   string        `json:"student_id"`
	StudentName string        `json:"student_name,omitempty"`
	BatchID     string        `json:"batch_id,omitempty"`
	BatchShortID string       `json:"batch_short_id,omitempty"`
	BatchNumber string        `json:"batch_number,omitempty"`
	Assignments CategoryScore `json:"assignments"`
	Exams       CategoryScore `json:"exams"`
	Projects    CategoryScore `json:"projects"`
	FinalScore  float64       `json:"final_score"`
	Rank        int           `json:"rank,omitempty"`
}
