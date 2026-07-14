package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

// RunBatchReminders polls hourly for batches starting the next day and sends a
// one-time reminder (in-app + email) to enrolled students and the batch
// manager. Hourly polling is enough since batch start dates only have day
// granularity, unlike session reminders which need minute precision. Blocks
// until ctx is cancelled.
func RunBatchReminders(
	ctx context.Context,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
	userRepo *repository.UserRepository,
	emailSvc *service.EmailService,
	timezone string,
) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("scheduler: invalid timezone %q, falling back to UTC: %v", timezone, err)
		loc = time.UTC
	}

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processBatchReminders(ctx, batchRepo, notificationRepo, userRepo, emailSvc, loc)
		}
	}
}

func processBatchReminders(
	ctx context.Context,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
	userRepo *repository.UserRepository,
	emailSvc *service.EmailService,
	loc *time.Location,
) {
	tomorrow := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")

	due, err := batchRepo.FindDueForStartReminder(ctx, tomorrow)
	if err != nil {
		log.Printf("scheduler: find due batch start reminders: %v", err)
		return
	}

	for _, batch := range due {
		title := "Batch starting tomorrow: " + batch.BatchNumber
		message := fmt.Sprintf("Batch %q starts tomorrow (%s).", batch.BatchNumber, batch.StartDate)

		students, err := batchRepo.GetStudents(ctx, batch.ShortID)
		if err != nil {
			log.Printf("scheduler: fetch batch students for batch %s: %v", batch.ShortID, err)
		}
		recipients := make([]string, 0, len(students)+1)
		for _, s := range students {
			recipients = append(recipients, s.UserID)
		}
		recipients = append(recipients, batch.BatchManagerID)

		if err := notificationRepo.NotifyUsers(ctx,
			title, message, "batch", "batch", batch.ShortID, batch.CreatedBy, recipients,
		); err != nil {
			log.Printf("scheduler: notify batch start reminder for batch %s: %v", batch.ShortID, err)
		}

		if emailSvc.Configured() {
			subject, html := service.BatchStartReminderEmail(batch.BatchNumber, batch.StartDate)

			for _, s := range students {
				if s.Email == "" {
					continue
				}
				emailSvc.SendAsync(s.Email, subject, html)
			}
			if manager, err := userRepo.FindByID(ctx, batch.BatchManagerID); err != nil {
				log.Printf("scheduler: fetch batch manager for reminder email: %v", err)
			} else if manager != nil && manager.Email != "" {
				emailSvc.SendAsync(manager.Email, subject, html)
			}
		}

		if err := batchRepo.MarkStartReminderSent(ctx, batch.ID); err != nil {
			log.Printf("scheduler: mark start reminder sent for batch %s: %v", batch.ShortID, err)
		}
	}
}
