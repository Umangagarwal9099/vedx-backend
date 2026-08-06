package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/repository"
)

// RunLeadFollowUpReminders polls every minute for assigned, still-open leads
// whose next_follow_up_at has arrived and haven't been reminded about yet,
// and notifies the assigned employee once — the same reminder-sent-flag
// pattern used for assignments/projects/sessions/batches/exams.
func RunLeadFollowUpReminders(
	ctx context.Context,
	leadRepo *repository.LeadRepository,
	notificationRepo *repository.NotificationRepository,
) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processDueFollowUpReminders(ctx, leadRepo, notificationRepo)
		}
	}
}

func processDueFollowUpReminders(
	ctx context.Context,
	leadRepo *repository.LeadRepository,
	notificationRepo *repository.NotificationRepository,
) {
	due, err := leadRepo.FindDueForFollowUpReminder(ctx)
	if err != nil {
		log.Printf("scheduler: find due lead follow-up reminders: %v", err)
		return
	}

	for _, l := range due {
		title := "Follow-up due: " + l.Name
		message := fmt.Sprintf("Your follow-up with %q was due %s.", l.Name, l.NextFollowUpAt.Format("Jan 2, 3:04 PM"))

		if err := notificationRepo.NotifyUsers(ctx, title, message, "lead_follow_up_due", "lead", l.ShortID, l.CreatedBy, []string{l.AssignedTo}); err != nil {
			log.Printf("scheduler: notify follow-up reminder for lead %s: %v", l.ShortID, err)
		}

		if err := leadRepo.MarkFollowUpReminderSent(ctx, l.ID); err != nil {
			log.Printf("scheduler: mark follow-up reminder sent for lead %s: %v", l.ShortID, err)
		}
	}
}
