package controller

import (
	"testing"
	"time"

	"github.com/umangagarwal/vedx-backend/models"
)

func TestRecordingReleasedToStudents(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	cases := []struct {
		name    string
		visible bool
		from    *time.Time
		want    bool
	}{
		{"visible, no delay", true, nil, true},
		{"visible, delay passed", true, &past, true},
		{"visible, delay pending", true, &future, false},
		{"hidden", false, nil, false},
		{"hidden, delay passed", false, &past, false},
	}
	for _, tc := range cases {
		s := &models.Session{RecordingVisible: tc.visible, RecordingAvailableFrom: tc.from}
		if got := recordingReleasedToStudents(s); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
