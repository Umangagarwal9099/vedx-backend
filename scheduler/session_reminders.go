// Package scheduler runs small in-process background jobs (no external cron
// dependency), mirroring the ticker pattern already used for the DB keepalive
// in cmd/server/main.go.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// RunSessionReminders polls every minute for sessions whose scheduled start time
// has just arrived (session_reminder_notifications=true, not yet reminded) and
// fires a "starting now" notification to the mentor, enrolled students, team_lead,
// and super_admin — the same recipient set used at session creation. Blocks until
// ctx is cancelled.
func RunSessionReminders(
	ctx context.Context,
	sessionRepo *repository.SessionRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
	timezone string,
) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("scheduler: invalid timezone %q, falling back to UTC: %v", timezone, err)
		loc = time.UTC
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processDueReminders(ctx, sessionRepo, batchRepo, notificationRepo, loc)
		}
	}
}

func processDueReminders(
	ctx context.Context,
	sessionRepo *repository.SessionRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
	loc *time.Location,
) {
	const layout = "2006-01-02 15:04:05"
	now := time.Now().In(loc)
	windowStart := now.Add(-time.Minute).Format(layout)
	windowEnd := now.Format(layout)

	due, err := sessionRepo.FindDueForReminder(ctx, windowStart, windowEnd)
	if err != nil {
		log.Printf("scheduler: find due session reminders: %v", err)
		return
	}

	for _, session := range due {
		title := "Session starting now: " + session.Name
		message := fmt.Sprintf("Your session %q for batch %s is starting now.", session.Name, session.BatchNumber)
		// Prefer the direct Zoom join link so clicking it joins the meeting immediately.
		if session.ZoomJoinURL != "" {
			message = fmt.Sprintf("%s Join: %s", message, session.ZoomJoinURL)
		} else if session.ShareToken != "" {
			message = fmt.Sprintf("%s Join: /sessions/join/%s", message, session.ShareToken)
		}

		students, err := batchRepo.GetStudents(ctx, session.BatchShortID)
		if err != nil {
			log.Printf("scheduler: fetch batch students for session %s: %v", session.ShortID, err)
		}
		recipients := make([]string, 0, len(students)+1)
		for _, s := range students {
			recipients = append(recipients, s.UserID)
		}
		recipients = append(recipients, session.MentorID)

		if err := notificationRepo.NotifyUsers(ctx,
			title, message, "session", "session", session.ShortID, session.CreatedBy, recipients,
		); err != nil {
			log.Printf("scheduler: notify reminder (students/mentor) for session %s: %v", session.ShortID, err)
		}

		if err := notificationRepo.NotifyRoles(ctx,
			title, message, "session", "session", session.ShortID, session.CreatedBy,
			[]string{string(models.RoleTeamLead), string(models.RoleSuperAdmin)},
		); err != nil {
			log.Printf("scheduler: notify reminder (team_lead/super_admin) for session %s: %v", session.ShortID, err)
		}

		if err := sessionRepo.MarkReminderSent(ctx, session.ID); err != nil {
			log.Printf("scheduler: mark reminder sent for session %s: %v", session.ShortID, err)
		}
	}
}
