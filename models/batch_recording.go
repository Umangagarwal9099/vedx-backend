package models

import "time"

// BatchRecording is a video an admin/mentor uploaded directly to a batch —
// unlike Session.RecordingURL (only ever auto-populated from a Zoom
// webhook), these aren't tied to any live class. They exist purely so staff
// can bulk-attach an existing archive of recordings to a batch without
// creating a session per video.
type BatchRecording struct {
	ID             string    `json:"id"`
	ShortID        string    `json:"short_id"`
	BatchID        string    `json:"batch_id"`
	BatchShortID   string    `json:"batch_short_id"`
	BatchNumber    string    `json:"batch_number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	FileSize       int64     `json:"file_size,omitempty"`
	ContentType    string    `json:"content_type,omitempty"`
	UploadedBy     string    `json:"uploaded_by"`
	UploadedByName string    `json:"uploaded_by_name"`
	OrderIndex     int       `json:"order_index"`
	IsDemo         bool      `json:"is_demo"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
