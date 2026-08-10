// Package scheduler runs small in-process background jobs (no external cron
// dependency), mirroring the ticker pattern already used for the DB keepalive
// in cmd/server/main.go.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
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
	userRepo *repository.UserRepository,
	emailSvc *service.EmailService,
	timezone, studentPortalURL, adminPortalURL string,
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
			processDueReminders(ctx, sessionRepo, batchRepo, notificationRepo, userRepo, emailSvc, loc, studentPortalURL, adminPortalURL)
		}
	}
}

func processDueReminders(
	ctx context.Context,
	sessionRepo *repository.SessionRepository,
	batchRepo *repository.BatchRepository,
	notificationRepo *repository.NotificationRepository,
	userRepo *repository.UserRepository,
	emailSvc *service.EmailService,
	loc *time.Location,
	studentPortalURL, adminPortalURL string,
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

		if emailSvc.Configured() {
			studentLink := sessionJoinLink(studentPortalURL, session)
			mentorLink := sessionJoinLink(adminPortalURL, session)
			studentSubject, studentHTML := service.SessionReminderEmail(session.Name, session.BatchNumber, studentLink)
			mentorSubject, mentorHTML := service.SessionReminderEmail(session.Name, session.BatchNumber, mentorLink)

			for _, s := range students {
				if s.Email == "" {
					continue
				}
				emailSvc.SendAsync(s.Email, studentSubject, studentHTML)
			}
			if mentor, err := userRepo.FindByID(ctx, session.MentorID); err != nil {
				log.Printf("scheduler: fetch mentor for reminder email: %v", err)
			} else if mentor != nil && mentor.Email != "" {
				emailSvc.SendAsync(mentor.Email, mentorSubject, mentorHTML)
			}
		}

		if err := sessionRepo.MarkReminderSent(ctx, session.ID); err != nil {
			log.Printf("scheduler: mark reminder sent for session %s: %v", session.ShortID, err)
		}
	}
}

// sessionJoinLink mirrors SessionController.studentJoinLink/mentorJoinLink:
// when portalURL is configured it points at the login-gated join-gateway
// page in that app, otherwise it falls back to the raw Zoom link (or the
// share-token path, for online modes without Zoom) so reminder emails still
// carry a working link before the portal URLs are set.
func sessionJoinLink(portalURL string, session models.Session) string {
	if portalURL != "" {
		return strings.TrimRight(portalURL, "/") + "/join-session/" + session.ShortID
	}
	if session.ZoomJoinURL != "" {
		return session.ZoomJoinURL
	}
	if session.ShareToken != "" {
		return "/sessions/join/" + session.ShareToken
	}
	return ""
}
