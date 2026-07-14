package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/controller"
)

// RunExamAttemptSweep polls every minute for exam attempts still "in_progress"
// whose computed deadline (exam_attempts.ends_at) has passed, and force-submits
// them exactly as a manual submit would — this is what makes an exam's
// duration/auto_submit setting actually end the exam server-side, instead of
// relying on the student's browser calling submit before closing the tab.
// Blocks until ctx is cancelled.
func RunExamAttemptSweep(ctx context.Context, examAttemptCtrl *controller.ExamAttemptController) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := examAttemptCtrl.AutoSubmitExpired(ctx)
			if err != nil {
				log.Printf("scheduler: exam attempt sweep: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("scheduler: auto-submitted %d expired exam attempt(s)", n)
			}
		}
	}
}
