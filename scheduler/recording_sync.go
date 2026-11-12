package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/recordingsync"
)

// RunRecordingSync is the in-process half of the Zoom recording sync: it
// drains due jobs every processEvery and reconciles against Zoom every
// reconcileEvery (first pass two minutes after boot).
//
// On Cloud Run with request-based CPU allocation a background goroutine like
// this one is throttled whenever no request is in flight, so it is a
// best-effort accelerator only. The guaranteed path is Cloud Scheduler
// calling POST /internal/recordings/sync, which does the same work inside a
// request. Both share the job table's leases, so they never double-process.
func RunRecordingSync(ctx context.Context, svc *recordingsync.Service, processEvery, reconcileEvery time.Duration, lookbackDays int) {
	log.Printf("recording sync worker started (process every %s, reconcile every %s, lookback %d days)", processEvery, reconcileEvery, lookbackDays)
	reconcileAt := time.Now().Add(2 * time.Minute)
	ticker := time.NewTicker(processEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if time.Now().After(reconcileAt) {
			to := time.Now().AddDate(0, 0, 1)
			if _, err := svc.Reconcile(ctx, to.AddDate(0, 0, -lookbackDays), to, "reconcile"); err != nil {
				log.Printf("recording sync: reconcile failed: %v", err)
			}
			reconcileAt = time.Now().Add(reconcileEvery)
		}
		sum := svc.ProcessDue(ctx, recordingsync.Options{MaxJobs: 10, Budget: 50 * time.Minute})
		if sum.Processed > 0 || len(sum.Errors) > 0 {
			log.Printf("recording sync: processed=%d outcomes=%v errors=%v", sum.Processed, sum.Outcomes, sum.Errors)
		}
	}
}
