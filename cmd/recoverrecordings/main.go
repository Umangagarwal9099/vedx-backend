package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	appconfig "github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/db"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

// Batch recovery for the window where the R2 subscription had been removed
// from the Cloudflare account (discovered 2026-10-08): every
// recording.completed webhook since then got as far as downloading and
// watermarking the file, then failed on the R2 upload and gave up without
// retrying or touching the original on Zoom. This re-walks every session in
// that window that's still missing a recording_url, pulls the matching
// instance straight from Zoom's cloud, and runs it through the exact same
// watermark+upload pipeline the webhook uses.
//
// recoveryWindowStart is deliberately 2026-09-01: the last session that DID
// save successfully. Earlier gaps are unrelated to this outage (recordings
// interspersed with successful ones) and are left alone here.
const recoveryWindowStart = "2026-09-01"

type target struct {
	shortID    string
	meetingID  int64
	sessionDay string // YYYY-MM-DD, for matching the right instance when Zoom returns more than one
}

func main() {
	_ = godotenv.Load()
	cfg, err := appconfig.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := db.NewPool(cfg.Database)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	zoomSvc := service.NewZoomService(cfg.Zoom)
	storageSvc := service.NewStorageService(cfg.Storage)
	sessionRepo := repository.NewSessionRepository(pool)

	rows, err := pool.Query(context.Background(), `
		SELECT short_id, zoom_meeting_id, session_date
		FROM sessions
		WHERE zoom_meeting_id IS NOT NULL
		  AND (recording_url IS NULL OR recording_url = '')
		  AND session_date >= $1
		  AND session_date <= CURRENT_DATE
		  AND deleted_at IS NULL
		ORDER BY session_date`, recoveryWindowStart)
	if err != nil {
		log.Fatalf("query affected sessions: %v", err)
	}
	var targets []target
	for rows.Next() {
		var t target
		var day time.Time
		if err := rows.Scan(&t.shortID, &t.meetingID, &day); err != nil {
			log.Fatalf("scan session row: %v", err)
		}
		t.sessionDay = day.Format("2006-01-02")
		targets = append(targets, t)
	}
	rows.Close()

	fmt.Printf("Found %d session(s) missing a recording since %s\n\n", len(targets), recoveryWindowStart)
	if len(targets) == 0 {
		return
	}

	from, _ := time.Parse("2006-01-02", recoveryWindowStart)
	to := time.Now().AddDate(0, 0, 1)
	instances, err := zoomSvc.ListUserRecordings(from.AddDate(0, 0, -1), to)
	if err != nil {
		log.Fatalf("list zoom recordings: %v", err)
	}
	fmt.Printf("Zoom reports %d recorded instance(s) in that window\n\n", len(instances))

	byMeetingAndDay := make(map[string]*service.ZoomRecordingInstance)
	for i := range instances {
		inst := &instances[i]
		startTime, err := time.Parse(time.RFC3339, inst.StartTime)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("%d|%s", inst.ID, startTime.Format("2006-01-02"))
		byMeetingAndDay[key] = inst
	}

	var recovered, notFound, failed int
	for _, t := range targets {
		fmt.Printf("=== %s (meeting %d, %s) ===\n", t.shortID, t.meetingID, t.sessionDay)

		// A class starting late at night can be recorded under the next
		// calendar day in Zoom's UTC start_time, so check both.
		var inst *service.ZoomRecordingInstance
		if m, ok := byMeetingAndDay[fmt.Sprintf("%d|%s", t.meetingID, t.sessionDay)]; ok {
			inst = m
		} else if d, err := time.Parse("2006-01-02", t.sessionDay); err == nil {
			if m, ok := byMeetingAndDay[fmt.Sprintf("%d|%s", t.meetingID, d.AddDate(0, 0, 1).Format("2006-01-02"))]; ok {
				inst = m
			}
		}
		if inst == nil {
			fmt.Println("  no matching Zoom recording found (likely already deleted by Zoom's retention policy)")
			notFound++
			continue
		}

		var file *service.ZoomRecordingFile
		for i := range inst.RecordingFiles {
			if inst.RecordingFiles[i].RecordingType == "shared_screen_with_speaker_view" {
				file = &inst.RecordingFiles[i]
				break
			}
		}
		if file == nil {
			for i := range inst.RecordingFiles {
				if inst.RecordingFiles[i].FileType == "MP4" {
					file = &inst.RecordingFiles[i]
					break
				}
			}
		}
		if file == nil {
			fmt.Println("  found the Zoom instance but it has no MP4 file")
			failed++
			continue
		}
		fmt.Printf("  found file id=%s size=%d bytes\n", file.ID, file.FileSize)

		token, err := zoomSvc.AccessToken()
		if err != nil {
			fmt.Printf("  FAILED getting zoom access token: %v\n", err)
			failed++
			continue
		}

		body, contentType, err := zoomSvc.DownloadRecording(file.DownloadURL, token)
		if err != nil {
			fmt.Printf("  FAILED downloading from zoom: %v\n", err)
			failed++
			continue
		}

		loc, err := time.LoadLocation(cfg.App.Timezone)
		if err != nil {
			loc = time.UTC
		}
		timestampLabel := ""
		if startTime, err := time.Parse(time.RFC3339, inst.StartTime); err == nil {
			timestampLabel = startTime.In(loc).Format("2 Jan 2006, 3:04 PM")
		}

		watermarked, err := service.WatermarkRecording(context.Background(), body, service.DefaultWatermarkText, timestampLabel)
		body.Close()
		if err != nil {
			fmt.Printf("  FAILED watermarking: %v\n", err)
			failed++
			continue
		}

		key := fmt.Sprintf("%d/%s-recovered.mp4", t.meetingID, file.ID)
		url, err := storageSvc.UploadRecording(context.Background(), watermarked, key, contentType)
		watermarked.Close()
		if err != nil {
			fmt.Printf("  FAILED uploading to r2: %v\n", err)
			failed++
			continue
		}
		fmt.Printf("  uploaded: %s\n", url)

		wrote, err := sessionRepo.UpdateRecordingURL(context.Background(), t.meetingID, url)
		if err != nil {
			fmt.Printf("  FAILED saving recording_url: %v\n", err)
			failed++
			continue
		}
		if !wrote {
			fmt.Println("  session already had a recording_url by the time we finished (race) — uploaded file is orphaned in R2, left as-is")
			continue
		}
		fmt.Println("  recovered")
		recovered++
	}

	fmt.Printf("\nDone. recovered=%d not_found=%d failed=%d total=%d\n", recovered, notFound, failed, len(targets))
}
