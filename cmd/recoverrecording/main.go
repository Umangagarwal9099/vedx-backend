package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	appconfig "github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/db"
	"github.com/umangagarwal/vedx-backend/service"
)

// One-off recovery for session short_id 9DE27721 (meeting 87486189942):
// the real Aug 6 class recording never made it into our system — only a
// tiny Aug 7 host-rejoin recording did, since the webhook path had no guard
// against a second recording.completed event for the same meeting ID (now
// fixed in repository/session_repo.go). This pulls the Aug 6 instance
// directly from Zoom's cloud, runs it through the exact same
// watermark+upload pipeline the webhook uses, and attaches it to the
// session in place of the Aug 7 file.
const (
	sessionShortID  = "9DE27721"
	targetMeetingID = 87486189942
	targetDateISO   = "2026-08-06"
)

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

	day, err := time.Parse("2006-01-02", targetDateISO)
	if err != nil {
		log.Fatalf("parse target date: %v", err)
	}
	instances, err := zoomSvc.ListUserRecordings(day.AddDate(0, 0, -1), day.AddDate(0, 0, 1))
	if err != nil {
		log.Fatalf("list zoom recordings: %v", err)
	}

	fmt.Printf("Found %d recorded instance(s) in range:\n", len(instances))
	var target *service.ZoomRecordingInstance
	for i := range instances {
		inst := &instances[i]
		fmt.Printf("  id=%d uuid=%s topic=%q start_time=%s files=%d\n", inst.ID, inst.UUID, inst.Topic, inst.StartTime, len(inst.RecordingFiles))
		if inst.ID != targetMeetingID {
			continue
		}
		// Zoom does not guarantee chronological order here — observed
		// returning the Aug 7 rejoin before the Aug 6 instance in practice —
		// so match explicitly by date instead of taking the first hit.
		startTime, err := time.Parse(time.RFC3339, inst.StartTime)
		if err != nil {
			continue
		}
		if startTime.Format("2006-01-02") == targetDateISO {
			target = inst
		}
	}
	if target == nil {
		log.Fatalf("no recorded instance found for meeting %d on %s (range checked: %s..%s)", targetMeetingID, targetDateISO, day.AddDate(0, 0, -1).Format("2006-01-02"), day.AddDate(0, 0, 1).Format("2006-01-02"))
	}
	fmt.Printf("\nSelected instance: uuid=%s start_time=%s\n", target.UUID, target.StartTime)

	var file *service.ZoomRecordingFile
	for i := range target.RecordingFiles {
		if target.RecordingFiles[i].RecordingType == "shared_screen_with_speaker_view" {
			file = &target.RecordingFiles[i]
			break
		}
	}
	if file == nil {
		for i := range target.RecordingFiles {
			if target.RecordingFiles[i].FileType == "MP4" {
				file = &target.RecordingFiles[i]
				break
			}
		}
	}
	if file == nil {
		log.Fatalf("no MP4 file found in selected instance")
	}
	fmt.Printf("Selected file: id=%s type=%s recording_type=%s size=%d bytes\n", file.ID, file.FileType, file.RecordingType, file.FileSize)

	token, err := zoomSvc.AccessToken()
	if err != nil {
		log.Fatalf("get zoom access token: %v", err)
	}

	body, contentType, err := zoomSvc.DownloadRecording(file.DownloadURL, token)
	if err != nil {
		log.Fatalf("download recording: %v", err)
	}
	defer body.Close()

	loc, err := time.LoadLocation(cfg.App.Timezone)
	if err != nil {
		loc = time.UTC
	}
	startTimeParsed, err := time.Parse(time.RFC3339, target.StartTime)
	timestampLabel := ""
	if err == nil {
		timestampLabel = startTimeParsed.In(loc).Format("2 Jan 2006, 3:04 PM")
	}

	watermarked, err := service.WatermarkRecording(context.Background(), body, service.DefaultWatermarkText, timestampLabel)
	if err != nil {
		log.Fatalf("watermark recording: %v", err)
	}
	defer watermarked.Close()

	key := fmt.Sprintf("%d/%s-recovered.mp4", targetMeetingID, file.ID)
	url, err := storageSvc.UploadRecording(context.Background(), watermarked, key, contentType)
	if err != nil {
		log.Fatalf("upload recording: %v", err)
	}
	fmt.Printf("\nUploaded to: %s\n", url)

	tag, err := pool.Exec(context.Background(),
		`UPDATE sessions SET recording_url = $1, updated_at = NOW() WHERE short_id = $2`,
		url, sessionShortID,
	)
	if err != nil {
		log.Fatalf("update session recording_url: %v", err)
	}
	fmt.Printf("Rows updated: %d\n", tag.RowsAffected())

	var verifyURL string
	if err := pool.QueryRow(context.Background(), `SELECT recording_url FROM sessions WHERE short_id = $1`, sessionShortID).Scan(&verifyURL); err != nil {
		log.Fatalf("verify: %v", err)
	}
	fmt.Printf("Verified session %s recording_url = %s\n", sessionShortID, verifyURL)
}
