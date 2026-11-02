package main

import (
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	appconfig "github.com/umangagarwal/vedx-backend/config"
	"github.com/umangagarwal/vedx-backend/service"
)

// Read-only diagnostic: for the two sessions the batch recovery couldn't
// find, check whether Zoom genuinely has nothing for that meeting ID
// anywhere in a wide window (deleted/never recorded) vs. the recovery
// tool's day-matching missing an instance that's really there under a
// different date.
func main() {
	_ = godotenv.Load()
	cfg, err := appconfig.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	zoomSvc := service.NewZoomService(cfg.Zoom)

	targets := map[int64]string{
		83864239421: "4D9679E8 (2026-09-01)",
		86563235262: "79D0A90A (2026-09-22)",
	}

	from, _ := time.Parse("2006-01-02", "2026-08-01")
	to := time.Now().AddDate(0, 0, 1)
	instances, err := zoomSvc.ListUserRecordings(from, to)
	if err != nil {
		log.Fatalf("list zoom recordings: %v", err)
	}
	fmt.Printf("Zoom reports %d recorded instance(s) total from 2026-08-01 to today\n\n", len(instances))

	found := map[int64]bool{}
	for _, inst := range instances {
		if label, ok := targets[inst.ID]; ok {
			found[inst.ID] = true
			fmt.Printf("FOUND for %s: uuid=%s topic=%q start_time=%s files=%d\n", label, inst.UUID, inst.Topic, inst.StartTime, len(inst.RecordingFiles))
		}
	}
	for id, label := range targets {
		if !found[id] {
			fmt.Printf("NOT FOUND anywhere for %s (meeting %d) in this window\n", label, id)
		}
	}
}
