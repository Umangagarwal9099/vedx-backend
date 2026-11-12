package main

// syncrecordings runs the same durable recording sync the server runs, from
// an operator's machine — for backfills and for draining the queue when the
// server can't (e.g. before a deploy).
//
//	# 1. enqueue every Zoom recording since July that the LMS hasn't synced
//	go run ./cmd/syncrecordings -reconcile -from 2026-07-01
//
//	# 2a. archive originals to R2 only (no ffmpeg needed — protects against Zoom retention)
//	go run ./cmd/syncrecordings -process -archive-only
//
//	# 2b. full processing (needs ffmpeg + WATERMARK_FONT_PATH)
//	FFMPEG_PATH=/path/ffmpeg WATERMARK_FONT_PATH=/path/font.ttf go run ./cmd/syncrecordings -process
//
//	# status
//	go run ./cmd/syncrecordings -status
//
// Safe to run repeatedly and alongside the server: jobs are leased, every
// write is idempotent.

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"
	_ "time/tzdata" // distroless image: don't depend on system zoneinfo

	"github.com/joho/godotenv"
	appconfig "github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/db"
	"github.com/umangagarwal/vedx-backend/recordingsync"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

func main() {
	reconcile := flag.Bool("reconcile", false, "list Zoom recordings and enqueue any not yet known")
	from := flag.String("from", "", "reconcile from date YYYY-MM-DD (default: lookback window)")
	process := flag.Bool("process", false, "process due jobs")
	archiveOnly := flag.Bool("archive-only", false, "with -process: archive originals to R2, defer watermark/attach")
	maxJobs := flag.Int("max-jobs", 0, "with -process: stop after N jobs (0 = all due)")
	status := flag.Bool("status", false, "print queue status")
	since := flag.String("since", "", "with -process: only recordings on/after YYYY-MM-DD")
	budget := flag.Duration("budget", 0, "with -process: stop claiming new jobs after this long (e.g. 90m); 0 = until queue empty")
	flag.Parse()

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
	ctx := context.Background()

	jobs := repository.NewZoomRecordingJobRepository(pool)
	svc := recordingsync.New(service.NewZoomService(cfg.Zoom), service.NewStorageService(cfg.Storage), jobs,
		repository.NewBatchRecordingRepository(pool), cfg.App.Timezone)

	if *reconcile {
		to := time.Now().AddDate(0, 0, 1)
		start := to.AddDate(0, 0, -cfg.RecordingSync.LookbackDays)
		if *from != "" {
			if start, err = time.Parse("2006-01-02", *from); err != nil {
				log.Fatalf("bad -from: %v", err)
			}
		}
		sum, err := svc.Reconcile(ctx, start, to, "backfill")
		if err != nil {
			log.Fatalf("reconcile: %v", err)
		}
		fmt.Printf("reconcile: discovered=%d enqueued=%d requeued=%d errors=%v\n", sum.Discovered, sum.Enqueued, sum.Requeued, sum.Errors)
	}
	if *process {
		if !*archiveOnly {
			if err := service.WatermarkAvailable(); err != nil {
				log.Printf("NOTE: watermarking unavailable here (%v) — jobs will be archived and deferred to a worker that has ffmpeg", err)
			}
		}
		opt := recordingsync.Options{MaxJobs: *maxJobs, ArchiveOnly: *archiveOnly, Budget: *budget}
		if *since != "" {
			t, err := time.ParseInLocation("2006-01-02", *since, time.FixedZone("IST", 5*3600+1800))
			if err != nil {
				log.Fatalf("bad -since: %v", err)
			}
			opt.Since = &t
		}
		sum := svc.ProcessDue(ctx, opt)
		fmt.Printf("process: processed=%d outcomes=%v errors=%v\n", sum.Processed, sum.Outcomes, sum.Errors)
	}
	if *process || *reconcile {
		if counts, err := jobs.CountByStatus(ctx); err == nil {
			fmt.Printf("queue after run: %v\n", counts)
		}
	}
	if *status || (!*reconcile && !*process) {
		counts, err := jobs.CountByStatus(ctx)
		if err != nil {
			log.Fatalf("status: %v", err)
		}
		fmt.Printf("queue: %v\n", counts)
		list, err := jobs.List(ctx, "", 500)
		if err != nil {
			log.Fatalf("list: %v", err)
		}
		for _, j := range list {
			start := ""
			if j.RecordingStart != nil {
				start = j.RecordingStart.In(time.FixedZone("IST", 5*3600+1800)).Format("2006-01-02 15:04")
			}
			note := j.SkipReason
			if note == "" {
				note = j.LastError
			}
			fmt.Printf("%-10s %-7s %-8s %s %3dm meeting=%d archived=%t att=%d %s\n",
				j.Status, j.Role, j.SessionShortID, start, j.DurationMin, j.ZoomMeetingID, j.RawKey != "", j.Attempts, note)
		}
	}
}
