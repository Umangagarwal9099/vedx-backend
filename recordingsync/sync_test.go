package recordingsync

import (
	"testing"
	"time"

	"github.com/umangagarwal/vedx-backend/repository"
)

func ist(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func at(t *testing.T, s string) time.Time {
	v, err := time.ParseInLocation("2006-01-02 15:04", s, ist(t))
	if err != nil {
		t.Fatal(err)
	}
	return v.UTC()
}

func TestPickSession(t *testing.T) {
	loc := ist(t)
	cand := func(id, date, st, en string) repository.RecordingSessionCandidate {
		return repository.RecordingSessionCandidate{ShortID: id, SessionDate: date, StartTime: st, EndTime: en}
	}
	cases := []struct {
		name  string
		cands []repository.RecordingSessionCandidate
		start string
		want  string
	}{
		// Real cases from the Nov 2026 audit.
		{"on time", []repository.RecordingSessionCandidate{cand("5F8C8888", "2026-10-15", "18:00", "19:00")}, "2026-10-15 18:25", "5F8C8888"},
		{"started early", []repository.RecordingSessionCandidate{cand("CC5353C8", "2026-08-13", "19:01", "20:00")}, "2026-08-13 18:54", "CC5353C8"},
		{"AM/PM typo, same day", []repository.RecordingSessionCandidate{cand("0B742BE5", "2026-08-27", "08:00", "09:00")}, "2026-08-27 19:56", "0B742BE5"},
		{"host tested room the evening before", []repository.RecordingSessionCandidate{cand("1C484254", "2026-08-23", "20:00", "21:00")}, "2026-08-22 20:04", ""},
		{"rejoin next afternoon", []repository.RecordingSessionCandidate{cand("9DE27721", "2026-08-06", "18:00", "19:00")}, "2026-08-07 15:19", ""},
		{"late class crossing midnight", []repository.RecordingSessionCandidate{cand("X", "2026-08-23", "23:30", "23:59")}, "2026-08-24 00:20", "X"},
		{"closest of two sessions on one meeting", []repository.RecordingSessionCandidate{
			cand("A", "2026-10-10", "14:00", "15:00"), cand("B", "2026-10-10", "15:25", "16:26")}, "2026-10-10 15:26", "B"},
	}
	for _, tc := range cases {
		got := pickSession(tc.cands, at(t, tc.start), loc)
		gotID := ""
		if got != nil {
			gotID = got.ShortID
		}
		if gotID != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, gotID, tc.want)
		}
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 40 * time.Minute, 80 * time.Minute, 160 * time.Minute, 320 * time.Minute, 6 * time.Hour, 6 * time.Hour}
	for i, w := range want {
		if got := backoff(i + 1); got != w {
			t.Errorf("backoff(%d) = %s want %s", i+1, got, w)
		}
	}
}
