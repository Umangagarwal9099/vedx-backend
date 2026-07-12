package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type SessionController struct {
	sessionRepo      *repository.SessionRepository
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
	zoomSvc          *service.ZoomService
	publicBaseURL    string
	timezone         string
}

func NewSessionController(repo *repository.SessionRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository, zoomSvc *service.ZoomService, publicBaseURL, timezone string) *SessionController {
	return &SessionController{
		sessionRepo:      repo,
		batchRepo:        batchRepo,
		notificationRepo: notificationRepo,
		zoomSvc:          zoomSvc,
		publicBaseURL:    publicBaseURL,
		timezone:         timezone,
	}
}

// withShareLink fills in ShareLink from ShareToken for API responses.
func (ctrl *SessionController) withShareLink(s *models.Session) *models.Session {
	if s != nil && s.ShareToken != "" {
		base := strings.TrimRight(ctrl.publicBaseURL, "/")
		s.ShareLink = base + "/sessions/join/" + s.ShareToken
	}
	return s
}

// sanitizeForRole strips fields the given role must never see. ZoomStartURL is a
// host token — anyone holding it can start/control the meeting as host — so it's
// only ever returned to staff (mentor/team_lead/super_admin), never students.
func sanitizeForRole(s *models.Session, role string) *models.Session {
	if s != nil && role == string(models.RoleStudent) {
		s.ZoomStartURL = ""
	}
	return s
}

// stripRecordingIfUnpaid removes RecordingURL for students unless they've been
// marked fees_paid for the session's batch. Live-class access (ZoomJoinURL) is
// never touched here — only recordings are fee-gated. Staff roles always see it.
func (ctrl *SessionController) stripRecordingIfUnpaid(ctx context.Context, s *models.Session, role, userID string) *models.Session {
	if s == nil || s.RecordingURL == "" || role != string(models.RoleStudent) {
		return s
	}
	paid, err := ctrl.batchRepo.IsFeesPaid(ctx, s.BatchID, userID)
	if err != nil {
		log.Printf("check fees paid for session %s: %v", s.ShortID, err)
	}
	if err != nil || !paid {
		s.RecordingURL = ""
	}
	return s
}

// sessionStartTime combines a session's date ("2025-09-15") and start time ("10:00")
// into a time.Time in the app's configured timezone.
func (ctrl *SessionController) sessionStartTime(sessionDate, startTime string) (time.Time, error) {
	loc, err := time.LoadLocation(ctrl.timezone)
	if err != nil {
		loc = time.UTC
	}
	return time.ParseInLocation("2006-01-02 15:04", sessionDate+" "+startTime, loc)
}

// sessionDurationMinutes returns the gap between two "HH:MM" times, defaulting to
// 60 minutes if either is unparseable or the range is non-positive.
func sessionDurationMinutes(startTime, endTime string) int {
	const layout = "15:04"
	st, err1 := time.Parse(layout, startTime)
	et, err2 := time.Parse(layout, endTime)
	if err1 != nil || err2 != nil {
		return 60
	}
	d := et.Sub(st)
	if d <= 0 {
		return 60
	}
	return int(d.Minutes())
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

	// Every online session should always be joinable via a link — don't leave it
	// to the mentor remembering to toggle generate_shareable_link.
	if input.Mode == "online" {
		input.GenerateShareableLink = true
	}

	createdBy := c.GetString("user_id")

	// Create the Zoom meeting (if applicable) before persisting, so the join/start
	// URLs can be stored on the session row in the same insert. If Zoom isn't
	// configured yet, or the call fails, the session is still created without a
	// Zoom link rather than failing the whole request.
	var zoom *models.ZoomMeetingInfo
	if input.Mode == "online" && input.MeetingPlatform == "zoom" {
		if ctrl.zoomSvc.Configured() {
			start, err := ctrl.sessionStartTime(input.SessionDate, input.StartTime)
			if err != nil {
				log.Printf("parse session start time for zoom: %v", err)
			} else if meeting, err := ctrl.zoomSvc.CreateMeeting(
				input.Name, start, sessionDurationMinutes(input.StartTime, input.EndTime), ctrl.timezone,
			); err != nil {
				log.Printf("create zoom meeting: %v", err)
			} else {
				zoom = &models.ZoomMeetingInfo{ID: meeting.ID, JoinURL: meeting.JoinURL, StartURL: meeting.StartURL}
			}
		} else {
			log.Printf("zoom not configured, skipping meeting creation for session %q", input.Name)
		}
	}

	session, err := ctrl.sessionRepo.Create(c.Request.Context(), input, createdBy, zoom)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	session = ctrl.withShareLink(session)

	title := "New session: " + session.Name
	message := fmt.Sprintf("A new session %q has been scheduled for batch %s.", session.Name, session.BatchNumber)
	// Prefer the direct Zoom join link — clicking it joins the meeting immediately,
	// no intermediate app page required. Falls back to the share page (which itself
	// resolves to the Zoom link) for non-Zoom online modes or if Zoom wasn't configured.
	if session.ZoomJoinURL != "" {
		message = fmt.Sprintf("%s Join: %s", message, session.ZoomJoinURL)
	} else if session.ShareLink != "" {
		message = fmt.Sprintf("%s Join: %s", message, session.ShareLink)
	}

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

	// team_lead and super_admin oversee all batches, so they're notified broadly rather than per-batch.
	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		title, message, "session", "session", session.ShortID, createdBy,
		[]string{string(models.RoleTeamLead), string(models.RoleSuperAdmin)},
	); err != nil {
		log.Printf("notify session create (team_lead/super_admin): %v", err)
	}

	c.JSON(http.StatusCreated, sanitizeForRole(session, c.GetString("role")))
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
	role := c.GetString("role")
	userID := c.GetString("user_id")
	for i := range sessions {
		ctrl.withShareLink(&sessions[i])
		sanitizeForRole(&sessions[i], role)
		ctrl.stripRecordingIfUnpaid(c.Request.Context(), &sessions[i], role, userID)
	}
	c.JSON(http.StatusOK, sessions)
}

// GetSessionsByBatch godoc
//
//	@Summary		List sessions for a batch
//	@Description	Returns all non-deleted sessions scheduled for a batch, ordered by session date/time (newest first).
//	@Tags			sessions
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200	{array}		models.Session
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/sessions [get]
func (ctrl *SessionController) GetByBatch(c *gin.Context) {
	batchShortID := c.Param("short_id")

	sessions, err := ctrl.sessionRepo.FindByBatchShortID(c.Request.Context(), batchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch sessions"})
		return
	}
	if sessions == nil {
		sessions = []models.Session{}
	}
	role := c.GetString("role")
	userID := c.GetString("user_id")
	for i := range sessions {
		ctrl.withShareLink(&sessions[i])
		sanitizeForRole(&sessions[i], role)
		ctrl.stripRecordingIfUnpaid(c.Request.Context(), &sessions[i], role, userID)
	}
	c.JSON(http.StatusOK, sessions)
}

// GetBatchRecordings godoc
//
//	@Summary		List a batch's session recordings
//	@Description	Student-facing view of every recorded session in a batch. If the caller is a student who hasn't been marked fees_paid, recordings is empty and message explains why — staff always see the full list, regardless of any student's payment status.
//	@Tags			sessions
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Success		200			{object}	models.BatchRecordingsResponse
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/recordings [get]
func (ctrl *SessionController) GetBatchRecordings(c *gin.Context) {
	batchShortID := c.Param("short_id")
	role := c.GetString("role")
	userID := c.GetString("user_id")

	if role == string(models.RoleStudent) {
		paid, err := ctrl.batchRepo.IsFeesPaidByBatchShortID(c.Request.Context(), batchShortID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch fee status"})
			return
		}
		if !paid {
			c.JSON(http.StatusOK, models.BatchRecordingsResponse{
				FeesPaid:   false,
				Message:    "Please pay your fees for this batch to access session recordings.",
				Recordings: []models.RecordingListItem{},
			})
			return
		}
	}

	sessions, err := ctrl.sessionRepo.FindByBatchShortID(c.Request.Context(), batchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch sessions"})
		return
	}

	recordings := []models.RecordingListItem{}
	for _, s := range sessions {
		if s.RecordingURL == "" {
			continue
		}
		recordings = append(recordings, models.RecordingListItem{
			SessionShortID: s.ShortID,
			Name:           s.Name,
			SessionDate:    s.SessionDate,
			RecordingURL:   s.RecordingURL,
		})
	}

	c.JSON(http.StatusOK, models.BatchRecordingsResponse{FeesPaid: true, Recordings: recordings})
}

// GetSession godoc
//
//	@Summary		Get session
//	@Description	Returns a single non-deleted session by its short ID. recording_url is omitted for students who haven't been marked fees_paid for the session's batch; live-class access is unaffected.
//	@Tags			sessions
//	@Produce		json
//	@Param			short_id	path		string	true	"Session short ID"
//	@Success		200			{object}	models.Session
//	@Failure		404			{object}	map[string]string	"Session not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions/{short_id} [get]
func (ctrl *SessionController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")

	session, err := ctrl.sessionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch session"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	role := c.GetString("role")
	userID := c.GetString("user_id")
	ctrl.withShareLink(session)
	sanitizeForRole(session, role)
	ctrl.stripRecordingIfUnpaid(c.Request.Context(), session, role, userID)

	c.JSON(http.StatusOK, session)
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

	// Keep the Zoom meeting in sync when name/date/time changed on a session that
	// already has one. Best-effort — DB is the source of truth, so a Zoom-side
	// failure here is logged but doesn't fail the request.
	if session.ZoomMeetingID != nil && (input.Name != nil || input.SessionDate != nil || input.StartTime != nil || input.EndTime != nil) {
		if start, err := ctrl.sessionStartTime(session.SessionDate, session.StartTime); err != nil {
			log.Printf("parse session start time for zoom update: %v", err)
		} else if err := ctrl.zoomSvc.UpdateMeeting(
			*session.ZoomMeetingID, session.Name, start, sessionDurationMinutes(session.StartTime, session.EndTime), ctrl.timezone,
		); err != nil {
			log.Printf("update zoom meeting: %v", err)
		}
	}

	c.JSON(http.StatusOK, sanitizeForRole(ctrl.withShareLink(session), c.GetString("role")))
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

	// Best-effort: cancel the Zoom meeting before soft-deleting the session.
	if session, err := ctrl.sessionRepo.FindByShortID(c.Request.Context(), shortID); err != nil {
		log.Printf("fetch session before delete: %v", err)
	} else if session != nil && session.ZoomMeetingID != nil {
		if err := ctrl.zoomSvc.DeleteMeeting(*session.ZoomMeetingID); err != nil {
			log.Printf("delete zoom meeting: %v", err)
		}
	}

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

// JoinByToken godoc
//
//	@Summary		Resolve a session share link
//	@Description	Public, unauthenticated lookup of a session by its share token (the value embedded in share_link, e.g. /sessions/join/{token}). Never returns the Zoom host start URL.
//	@Tags			sessions
//	@Produce		json
//	@Param			token	path		string	true	"Share token"
//	@Success		200		{object}	models.SessionJoinInfo
//	@Failure		404		{object}	map[string]string	"Not found"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Router			/sessions/by-token/{token} [get]
func (ctrl *SessionController) JoinByToken(c *gin.Context) {
	token := c.Param("token")

	session, err := ctrl.sessionRepo.FindByShareToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch session"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, models.SessionJoinInfo{
		Name:            session.Name,
		SessionDate:     session.SessionDate,
		StartTime:       session.StartTime,
		EndTime:         session.EndTime,
		MentorName:      session.MentorName,
		Mode:            session.Mode,
		MeetingPlatform: session.MeetingPlatform,
		ZoomJoinURL:     session.ZoomJoinURL,
	})
}
