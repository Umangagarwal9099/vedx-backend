package models

// DemoRecordingSelection identifies one recording to mark (or keep marked)
// as a demo video — source "session" pairs with a Session.ShortID, "upload"
// with a BatchRecording.ShortID, same convention as RecordingListItem.
type DemoRecordingSelection struct {
	Source  string `json:"source"   binding:"required,oneof=session upload" example:"session"`
	ShortID string `json:"short_id" binding:"required"                      example:"A3F72C1D"`
}

// SetDemoSelectionInput replaces the full set of demo recordings for one
// batch — any recording in that batch not listed here is unmarked.
type SetDemoSelectionInput struct {
	Items []DemoRecordingSelection `json:"items"`
}

// DemoBatchSummary is one batch that has at least one demo recording, as
// listed on GET /demo-classes.
type DemoBatchSummary struct {
	BatchShortID   string `json:"batch_short_id"`
	BatchNumber    string `json:"batch_number"`
	CourseName     string `json:"course_name"`
	CourseShortID  string `json:"course_short_id"`
	DemoVideoCount int    `json:"demo_video_count"`
}

// DemoClassesBatchResponse is a batch's demo recordings, as returned by
// GET /demo-classes/{short_id} — open to any authenticated user regardless
// of enrollment or fee status.
type DemoClassesBatchResponse struct {
	BatchShortID string              `json:"batch_short_id"`
	BatchNumber  string              `json:"batch_number"`
	CourseName   string              `json:"course_name"`
	Recordings   []RecordingListItem `json:"recordings"`
}
