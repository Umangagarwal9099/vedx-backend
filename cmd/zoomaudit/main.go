package main

// zoomaudit is a READ-ONLY cross-check of every LMS session against what
// Zoom actually holds. It never downloads, uploads or writes to the
// database. For each session it reports whether the class was held on its
// Zoom meeting, whether Zoom has (or had, in trash) a cloud recording for
// it, and whether the recording the LMS points at really exists in R2.
// It also lists Zoom recordings that match no session at all.
//
//	go run ./cmd/zoomaudit -from 2026-07-01 -out /path/to/audit
//
// writes audit.json (full detail) and audit.csv (one row per session).

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	appconfig "github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/db"
	"github.com/umangagarwal/vedx-backend/service"
)

type instanceSummary struct {
	UUID        string   `json:"uuid"`
	MeetingID   int64    `json:"meeting_id"`
	Topic       string   `json:"topic"`
	HostEmail   string   `json:"host_email"`
	HostID      string   `json:"host_id"`
	StartUTC    string   `json:"start_utc"`
	StartLocal  string   `json:"start_local"`
	DurationMin int      `json:"duration_min"`
	TotalSizeMB float64  `json:"total_size_mb"`
	Files       []string `json:"files"` // "TYPE/recording_type/sizeMB/status"
	HasVideo    bool     `json:"has_video"`
	InTrash     bool     `json:"in_trash"`
}

type sessionRow struct {
	ShortID       string            `json:"short_id"`
	Name          string            `json:"name"`
	Date          string            `json:"date"`
	Start         string            `json:"start"`
	End           string            `json:"end"`
	Batch         string            `json:"batch"`
	Course        string            `json:"course"`
	Mentor        string            `json:"mentor"`
	ZoomMeetingID int64             `json:"zoom_meeting_id,omitempty"`
	RecordingURL  string            `json:"recording_url,omitempty"`
	R2Exists      *bool             `json:"r2_exists,omitempty"`
	R2SizeMB      float64           `json:"r2_size_mb,omitempty"`
	ZoomHost      string            `json:"zoom_host,omitempty"`
	AutoRecording string            `json:"zoom_auto_recording,omitempty"`
	ZoomMeetingOK string            `json:"zoom_meeting_lookup,omitempty"`
	HeldInstances []string          `json:"held_instances,omitempty"`
	Recordings    []instanceSummary `json:"zoom_recordings,omitempty"`
	Status        string            `json:"status"`
	Notes         []string          `json:"notes,omitempty"`
}

func main() {
	fromFlag := flag.String("from", "2026-07-01", "earliest date to audit (YYYY-MM-DD)")
	outDir := flag.String("out", ".", "directory to write audit.json and audit.csv")
	flag.Parse()

	_ = godotenv.Load()
	cfg, err := appconfig.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	loc, err := time.LoadLocation(cfg.App.Timezone)
	if err != nil {
		loc = time.UTC
	}
	pool, err := db.NewPool(cfg.Database)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	zoomSvc := service.NewZoomService(cfg.Zoom)
	storageSvc := service.NewStorageService(cfg.Storage)

	from, err := time.Parse("2006-01-02", *fromFlag)
	if err != nil {
		log.Fatalf("bad -from: %v", err)
	}
	to := time.Now().In(loc).AddDate(0, 0, 1)

	// ---- Zoom side -------------------------------------------------------
	users, err := zoomSvc.ListUsers()
	if err != nil {
		// The S2S app may lack user:read:list_users:admin — fall back to the
		// app owner ("me"), which is where every LMS-created meeting lives.
		log.Printf("WARN cannot list account users (%v) — auditing app owner \"me\" only", err)
		users = []service.ZoomUser{{ID: "me", Email: "me (S2S app owner)"}}
	}
	log.Printf("zoom account has %d user(s)", len(users))
	var all []instanceSummary
	userEmail := map[string]string{}
	for _, u := range users {
		userEmail[u.ID] = u.Email
		for _, trash := range []bool{false, true} {
			recs, err := zoomSvc.ListRecordings(u.ID, from, to, trash)
			if err != nil {
				log.Printf("WARN list recordings for %s (trash=%t): %v", u.Email, trash, err)
				continue
			}
			log.Printf("  user %s (type %d, %s): %d recorded instance(s) trash=%t", u.Email, u.Type, u.Status, len(recs), trash)
			for i := range recs {
				all = append(all, summarize(&recs[i], loc, trash))
			}
		}
	}
	byMeeting := map[int64][]instanceSummary{}
	for _, inst := range all {
		byMeeting[inst.MeetingID] = append(byMeeting[inst.MeetingID], inst)
	}

	// ---- LMS side --------------------------------------------------------
	rows, err := pool.Query(ctx, `
		SELECT s.short_id, s.name, s.session_date::TEXT,
		       TO_CHAR(s.start_time,'HH24:MI'), TO_CHAR(s.end_time,'HH24:MI'),
		       b.batch_number, COALESCE(c.name, ''), CONCAT(m.first_name,' ',m.last_name),
		       COALESCE(s.zoom_meeting_id, 0), COALESCE(s.recording_url, '')
		FROM sessions s
		JOIN batches b ON b.id = s.batch_id
		LEFT JOIN courses c ON c.id = b.course_id
		JOIN users m ON m.id = s.mentor_id
		WHERE s.deleted_at IS NULL AND s.session_date >= $1::DATE
		ORDER BY s.session_date, s.start_time`, *fromFlag)
	if err != nil {
		log.Fatalf("query sessions: %v", err)
	}
	var sessions []*sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.ShortID, &s.Name, &s.Date, &s.Start, &s.End, &s.Batch, &s.Course, &s.Mentor, &s.ZoomMeetingID, &s.RecordingURL); err != nil {
			log.Fatalf("scan: %v", err)
		}
		sessions = append(sessions, &s)
	}
	rows.Close()

	matched := map[string]bool{}
	meetingSessions := map[int64][]string{}
	now := time.Now().In(loc)
	for _, s := range sessions {
		end, _ := time.ParseInLocation("2006-01-02 15:04", s.Date+" "+s.End, loc)
		if s.ZoomMeetingID != 0 {
			meetingSessions[s.ZoomMeetingID] = append(meetingSessions[s.ZoomMeetingID], s.ShortID)
			s.Recordings = byMeeting[s.ZoomMeetingID]
			for _, r := range s.Recordings {
				matched[r.UUID] = true
			}
			if m, err := zoomSvc.GetMeeting(s.ZoomMeetingID); err != nil {
				if service.IsZoomNotFound(err) {
					s.ZoomMeetingOK = "meeting deleted/not found in Zoom"
				} else {
					s.ZoomMeetingOK = "lookup error: " + err.Error()
				}
			} else {
				s.ZoomMeetingOK = "ok"
				s.ZoomHost = m.HostEmail
				if s.ZoomHost == "" {
					s.ZoomHost = userEmail[m.HostID]
				}
				s.AutoRecording = m.Settings.AutoRecording
			}
			if inst, err := zoomSvc.ListPastInstances(s.ZoomMeetingID); err == nil {
				for _, pi := range inst {
					t, _ := time.Parse(time.RFC3339, pi.StartTime)
					s.HeldInstances = append(s.HeldInstances, t.In(loc).Format("2006-01-02 15:04"))
				}
				sort.Strings(s.HeldInstances)
			}
		}
		if s.RecordingURL != "" {
			ok, size, err := storageSvc.RecordingExists(ctx, s.RecordingURL)
			if err != nil {
				s.Notes = append(s.Notes, "R2 check error: "+err.Error())
			} else {
				s.R2Exists = &ok
				s.R2SizeMB = float64(size) / 1e6
			}
		}
		s.Status = classify(s, end.Before(now))
	}

	// Duplicate / mapping checks.
	for id, shortIDs := range meetingSessions {
		if len(shortIDs) > 1 {
			for _, s := range sessions {
				if s.ZoomMeetingID == id {
					s.Notes = append(s.Notes, "zoom meeting shared with sessions "+strings.Join(shortIDs, ","))
				}
			}
		}
	}
	type dupKey struct{ batch, date, name string }
	dups := map[dupKey][]string{}
	for _, s := range sessions {
		k := dupKey{s.Batch, s.Date, strings.TrimSpace(s.Name)}
		dups[k] = append(dups[k], s.ShortID)
	}
	for _, s := range sessions {
		if ids := dups[dupKey{s.Batch, s.Date, strings.TrimSpace(s.Name)}]; len(ids) > 1 {
			s.Notes = append(s.Notes, "possible duplicate session (same batch/date/name): "+strings.Join(ids, ","))
		}
		if len(s.Recordings) > 1 {
			s.Notes = append(s.Notes, fmt.Sprintf("%d recorded instances on this meeting ID (host rejoin or split recording)", len(s.Recordings)))
		}
	}

	var orphans []instanceSummary
	for _, inst := range all {
		if !matched[inst.UUID] {
			orphans = append(orphans, inst)
		}
	}

	// ---- Output ----------------------------------------------------------
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	report := map[string]interface{}{
		"generated_at":           time.Now().In(loc).Format(time.RFC3339),
		"from":                   *fromFlag,
		"zoom_users":             users,
		"sessions":               sessions,
		"unmatched_zoom_records": orphans,
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filepath.Join(*outDir, "audit.json"), b, 0o644); err != nil {
		log.Fatalf("write json: %v", err)
	}

	f, err := os.Create(filepath.Join(*outDir, "audit.csv"))
	if err != nil {
		log.Fatalf("create csv: %v", err)
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"short_id", "date", "start", "end", "batch", "mentor", "name", "zoom_meeting_id", "zoom_host", "auto_recording", "held_instances", "zoom_recordings", "lms_recording", "r2_exists", "status", "notes"})
	for _, s := range sessions {
		var recs []string
		for _, r := range s.Recordings {
			recs = append(recs, fmt.Sprintf("%s %dmin %.0fMB video=%t trash=%t", r.StartLocal, r.DurationMin, r.TotalSizeMB, r.HasVideo, r.InTrash))
		}
		r2 := ""
		if s.R2Exists != nil {
			r2 = strconv.FormatBool(*s.R2Exists)
		}
		mid := ""
		if s.ZoomMeetingID != 0 {
			mid = strconv.FormatInt(s.ZoomMeetingID, 10)
		}
		_ = w.Write([]string{s.ShortID, s.Date, s.Start, s.End, s.Batch, s.Mentor, s.Name, mid, s.ZoomHost, s.AutoRecording,
			strings.Join(s.HeldInstances, "; "), strings.Join(recs, "; "), s.RecordingURL, r2, s.Status, strings.Join(s.Notes, "; ")})
	}
	w.Flush()
	f.Close()

	counts := map[string]int{}
	for _, s := range sessions {
		counts[s.Status]++
	}
	fmt.Println("\n=== session status counts ===")
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-34s %d\n", k, counts[k])
	}
	fmt.Printf("\nzoom recorded instances: %d, unmatched to any session: %d\n", len(all), len(orphans))
	fmt.Printf("wrote %s and %s\n", filepath.Join(*outDir, "audit.json"), filepath.Join(*outDir, "audit.csv"))
}

func summarize(r *service.ZoomRecordingInstance, loc *time.Location, trash bool) instanceSummary {
	st := r.StartTimeParsed()
	s := instanceSummary{
		UUID: r.UUID, MeetingID: r.ID, Topic: r.Topic, HostEmail: r.HostEmail, HostID: r.HostID,
		StartUTC: r.StartTime, StartLocal: st.In(loc).Format("2006-01-02 15:04"),
		DurationMin: r.Duration, TotalSizeMB: float64(r.TotalSize) / 1e6, InTrash: trash,
		HasVideo: r.PrimaryVideoFile() != nil,
	}
	for _, f := range r.RecordingFiles {
		s.Files = append(s.Files, fmt.Sprintf("%s/%s/%.0fMB/%s", f.FileType, f.RecordingType, float64(f.FileSize)/1e6, f.Status))
	}
	return s
}

func classify(s *sessionRow, past bool) string {
	hasLive, hasTrash := false, false
	for _, r := range s.Recordings {
		if r.HasVideo && !r.InTrash {
			hasLive = true
		}
		if r.HasVideo && r.InTrash {
			hasTrash = true
		}
	}
	switch {
	case !past:
		return "upcoming"
	case s.RecordingURL != "" && s.R2Exists != nil && !*s.R2Exists:
		if hasLive {
			return "METADATA_ONLY_RECOVERABLE"
		}
		return "METADATA_ONLY_FILE_MISSING"
	case s.RecordingURL != "":
		return "ok"
	case s.ZoomMeetingID == 0:
		return "NO_ZOOM_MEETING"
	case hasLive:
		return "MISSING_RECOVERABLE"
	case hasTrash:
		return "MISSING_IN_ZOOM_TRASH"
	case len(s.HeldInstances) > 0:
		return "HELD_BUT_NO_ZOOM_RECORDING"
	default:
		return "NOT_HELD_ON_THIS_MEETING"
	}
}
