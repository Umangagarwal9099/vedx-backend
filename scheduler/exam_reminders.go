package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// RunExamStartReminders polls every minute for assessments starting within the
// next 15 minutes and notifies enrolled students once (or a student-wide
// broadcast for global assessments), using the same reminder-sent-flag pattern
// as session/batch reminders. Blocks until ctx is cancelled.
func RunExamStartReminders(
	ctx context.Context,
	assessmentRepo *repository.AssessmentRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processDueExamReminders(ctx, assessmentRepo, batchRepo, notificationRepo)
		}
	}
}

func processDueExamReminders(
	ctx context.Context,
	assessmentRepo *repository.AssessmentRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
) {
	due, err := assessmentRepo.FindDueForStartReminder(ctx)
	if err != nil {
		log.Printf("scheduler: find due exam reminders: %v", err)
		return
	}

	for _, a := range due {
		title := "Starting soon: " + a.Name
		message := fmt.Sprintf("%q starts at %s.", a.Name, a.StartAt.Format("Jan 2, 2006 3:04 PM"))

		if a.BatchShortID != "" {
			students, err := batchRepo.GetStudents(ctx, a.BatchShortID)
			if err != nil {
				log.Printf("scheduler: fetch batch students for assessment %s: %v", a.ShortID, err)
			}
			recipients := make([]string, 0, len(students))
			for _, s := range students {
				recipients = append(recipients, s.UserID)
			}
			if err := notificationRepo.NotifyUsers(ctx, title, message, "assessment_starting_soon", "assessment", a.ShortID, a.CreatedBy, recipients); err != nil {
				log.Printf("scheduler: notify exam reminder (students) for %s: %v", a.ShortID, err)
			}
		} else if err := notificationRepo.NotifyRoles(ctx, title, message, "assessment_starting_soon", "assessment", a.ShortID, a.CreatedBy, []string{string(models.RoleStudent)}); err != nil {
			log.Printf("scheduler: notify exam reminder (broadcast) for %s: %v", a.ShortID, err)
		}

		if err := assessmentRepo.MarkStartReminderSent(ctx, a.ID); err != nil {
			log.Printf("scheduler: mark start reminder sent for %s: %v", a.ShortID, err)
		}
	}
}
