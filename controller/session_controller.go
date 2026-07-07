package controller

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type SessionController struct {
	sessionRepo      *repository.SessionRepository
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
	publicBaseURL    string
}

func NewSessionController(repo *repository.SessionRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository, publicBaseURL string) *SessionController {
	return &SessionController{sessionRepo: repo, batchRepo: batchRepo, notificationRepo: notificationRepo, publicBaseURL: publicBaseURL}
}

// withShareLink fills in ShareLink from ShareToken for API responses.
func (ctrl *SessionController) withShareLink(s *models.Session) *models.Session {
	if s != nil && s.ShareToken != "" {
		base := strings.TrimRight(ctrl.publicBaseURL, "/")
		s.ShareLink = base + "/sessions/join/" + s.ShareToken
	}
	return s
}

// CreateSession godoc
//
//	@Summary		Create session
//	@Description	Schedule a new session. mode must be online | offline; meeting_platform (zoom | google_meet | teams) is required when mode is online. session_type currently only supports "batch" — pass batch_short_id from GET /batches. Pick mentor_id from GET /mentors. Set generate_shareable_link to true to receive a public join link, and feedback_form_short_id (from GET /feedback-forms) to attach a class feedback form. send_confirmation_email and session_reminder_notifications are stored preferences; actual email/reminder dispatch is handled by a separate notification worker. In-app notifications go only to students enrolled in the target batch, the assigned mentor, and team_leads — not every student in the system.
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateSessionInput	true	"Session details"
//	@Success		201		{object}	models.Session
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions [post]
func (ctrl *SessionController) Create(c *gin.Context) {
	var input models.CreateSessionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Mode == "online" && input.MeetingPlatform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "meeting_platform is required when mode is online"})
		return
	}

	createdBy := c.GetString("user_id")

	session, err := ctrl.sessionRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}

	title := "New session: " + session.Name
	message := fmt.Sprintf("A new session %q has been scheduled for batch %s.", session.Name, session.BatchNumber)

	// Notify only the students enrolled in this batch, plus the assigned mentor —
	// not every student/mentor in the system.
	students, err := ctrl.batchRepo.GetStudents(c.Request.Context(), session.BatchShortID)
	if err != nil {
		log.Printf("fetch batch students for session notify: %v", err)
	}
	recipients := make([]string, 0, len(students)+1)
	for _, s := range students {
		recipients = append(recipients, s.UserID)
	}
	recipients = append(recipients, session.MentorID)

	if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
		title, message, "session", "session", session.ShortID, createdBy, recipients,
	); err != nil {
		log.Printf("notify session create (students/mentor): %v", err)
	}

	// team_lead oversees all batches, so they're notified broadly rather than per-batch.
	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		title, message, "session", "session", session.ShortID, createdBy,
		[]string{"team_lead"},
	); err != nil {
		log.Printf("notify session create (team_lead): %v", err)
	}

	c.JSON(http.StatusCreated, ctrl.withShareLink(session))
}

// GetAllSessions godoc
//
//	@Summary		List sessions
//	@Description	Returns all non-deleted sessions ordered by session date/time (newest first). Supports optional filtering by batch_short_id, mentor_id, date (YYYY-MM-DD), and is_active.
//	@Tags			sessions
//	@Produce		json
//	@Param			batch_short_id	query	string	false	"Filter by batch short ID"
//	@Param			mentor_id		query	string	false	"Filter by mentor user ID"
//	@Param			date			query	string	false	"Filter by session date (YYYY-MM-DD)"
//	@Param			is_active		query	string	false	"Filter by active status: true or false"
//	@Success		200	{array}		models.Session
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions [get]
func (ctrl *SessionController) GetAll(c *gin.Context) {
	var filter models.SessionFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var sessions []models.Session
	var err error

	if filter.BatchShortID != "" || filter.MentorID != "" || filter.Date != "" || filter.IsActive != "" {
		sessions, err = ctrl.sessionRepo.Filter(c.Request.Context(), filter)
	} else {
		sessions, err = ctrl.sessionRepo.FindAll(c.Request.Context())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch sessions"})
		return
	}
	if sessions == nil {
		sessions = []models.Session{}
	}
	for i := range sessions {
		ctrl.withShareLink(&sessions[i])
	}
	c.JSON(http.StatusOK, sessions)
}

// UpdateSession godoc
//
//	@Summary		Update session
//	@Description	Partially update a session — including mentor, date/time, and batch. All fields are optional; only provided fields are updated. Set generate_shareable_link to false to revoke the existing share link, or to true to (re)generate one.
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Session short ID"
//	@Param			body		body		models.UpdateSessionInput	true	"Fields to update"
//	@Success		200			{object}	models.Session
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404			{object}	map[string]string	"Session not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions/{short_id} [patch]
func (ctrl *SessionController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateSessionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.sessionRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update session"})
		return
	}

	session, err := ctrl.sessionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || session == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated session"})
		return
	}

	c.JSON(http.StatusOK, ctrl.withShareLink(session))
}

// DeleteSession godoc
//
//	@Summary		Delete session
//	@Description	Soft-deletes a session by its short ID. The record is retained in the database with deleted_at set.
//	@Tags			sessions
//	@Produce		json
//	@Param			short_id	path	string	true	"Session short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Session not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions/{short_id} [delete]
func (ctrl *SessionController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.sessionRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete session"})
		return
	}

	c.Status(http.StatusNoContent)
}
