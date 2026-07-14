package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/repository"
)

// RunDeadlineReminders polls every minute for assignments/projects whose
// deadline falls within the next 24 hours and haven't been reminded about
// yet, and notifies every enrolled student once — the same
// reminder-sent-flag pattern used for sessions/batches/exams.
func RunDeadlineReminders(
	ctx context.Context,
	assignmentRepo *repository.AssignmentRepository,
	projectRepo *repository.ProjectRepository,
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
			processDueAssignmentReminders(ctx, assignmentRepo, batchRepo, notificationRepo)
			processDueProjectReminders(ctx, projectRepo, batchRepo, notificationRepo)
		}
	}
}

func processDueAssignmentReminders(
	ctx context.Context,
	assignmentRepo *repository.AssignmentRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
) {
	due, err := assignmentRepo.FindDueForDeadlineReminder(ctx)
	if err != nil {
		log.Printf("scheduler: find due assignment reminders: %v", err)
		return
	}

	for _, a := range due {
		title := "Due soon: " + a.Title
		message := fmt.Sprintf("Your assignment %q for batch %s is due %s.", a.Title, a.BatchNumber, a.Deadline.Format("Jan 2, 2006 3:04 PM"))

		students, err := batchRepo.GetStudents(ctx, a.BatchShortID)
		if err != nil {
			log.Printf("scheduler: fetch batch students for assignment %s: %v", a.ShortID, err)
		}
		recipients := make([]string, 0, len(students))
		for _, s := range students {
			recipients = append(recipients, s.UserID)
		}
		if err := notificationRepo.NotifyUsers(ctx, title, message, "assignment_due_soon", "assignment", a.ShortID, a.CreatedBy, recipients); err != nil {
			log.Printf("scheduler: notify assignment reminder for %s: %v", a.ShortID, err)
		}

		if err := assignmentRepo.MarkDeadlineReminderSent(ctx, a.ID); err != nil {
			log.Printf("scheduler: mark deadline reminder sent for assignment %s: %v", a.ShortID, err)
		}
	}
}

func processDueProjectReminders(
	ctx context.Context,
	projectRepo *repository.ProjectRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
) {
	due, err := projectRepo.FindDueForDeadlineReminder(ctx)
	if err != nil {
		log.Printf("scheduler: find due project reminders: %v", err)
		return
	}

	for _, p := range due {
		title := "Due soon: " + p.Title
		message := fmt.Sprintf("Your project %q for batch %s is due %s.", p.Title, p.BatchNumber, p.FinalDeadline.Format("Jan 2, 2006 3:04 PM"))

		students, err := batchRepo.GetStudents(ctx, p.BatchShortID)
		if err != nil {
			log.Printf("scheduler: fetch batch students for project %s: %v", p.ShortID, err)
		}
		recipients := make([]string, 0, len(students))
		for _, s := range students {
			recipients = append(recipients, s.UserID)
		}
		if err := notificationRepo.NotifyUsers(ctx, title, message, "project_due_soon", "project", p.ShortID, p.CreatedBy, recipients); err != nil {
			log.Printf("scheduler: notify project reminder for %s: %v", p.ShortID, err)
		}

		if err := projectRepo.MarkDeadlineReminderSent(ctx, p.ID); err != nil {
			log.Printf("scheduler: mark deadline reminder sent for project %s: %v", p.ShortID, err)
		}
	}
}
