package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type AttendanceController struct {
	attendanceRepo *repository.AttendanceRepository
	sessionRepo    *repository.SessionRepository
	batchRepo      *repository.BatchRepository
	auditLogRepo   *repository.AuditLogRepository
}

func NewAttendanceController(attendanceRepo *repository.AttendanceRepository, sessionRepo *repository.SessionRepository, batchRepo *repository.BatchRepository, auditLogRepo *repository.AuditLogRepository) *AttendanceController {
	return &AttendanceController{attendanceRepo: attendanceRepo, sessionRepo: sessionRepo, batchRepo: batchRepo, auditLogRepo: auditLogRepo}
}

// GetSessionAttendance godoc
//
//	@Summary		Get attendance for a session
//	@Description	Returns the batch roster alongside whatever attendance has been marked so far for this session. Students not yet marked simply won't appear in "attendance" — the caller treats them as unmarked, not absent.
//	@Tags			attendance
//	@Produce		json
//	@Param			short_id	path	string	true	"Session short ID"
//	@Success		200			{object}	map[string]interface{}	"roster[] and attendance[]"
//	@Failure		404			{object}	map[string]string	"Session not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions/{short_id}/attendance [get]
func (ctrl *AttendanceController) GetSessionAttendance(c *gin.Context) {
	shortID := c.Param("short_id")

	session, err := ctrl.sessionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, session.BatchShortID) {
		return
	}

	roster, err := ctrl.batchRepo.GetStudents(c.Request.Context(), session.BatchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batch roster"})
		return
	}
	if roster == nil {
		roster = []models.BatchStudent{}
	}

	attendance, err := ctrl.attendanceRepo.GetForSession(c.Request.Context(), session.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance"})
		return
	}
	if attendance == nil {
		attendance = []models.SessionAttendance{}
	}

	c.JSON(http.StatusOK, gin.H{"roster": roster, "attendance": attendance})
}

// BulkMarkAttendance godoc
//
//	@Summary		Mark attendance for a whole session
//	@Description	Marks (or re-marks) attendance for every student listed in the request for one session — this is what the "take attendance" screen submits. Restricted to super_admin / team_lead / mentor.
//	@Tags			attendance
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string							true	"Session short ID"
//	@Param			body		body	models.BulkMarkAttendanceInput	true	"Per-student attendance"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Session not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions/{short_id}/attendance/bulk [post]
func (ctrl *AttendanceController) BulkMarkAttendance(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.BulkMarkAttendanceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := ctrl.sessionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, session.BatchShortID) {
		return
	}

	markedBy := c.GetString("user_id")
	if err := ctrl.attendanceRepo.BulkMark(c.Request.Context(), session.ID, session.BatchID, markedBy, input.Records); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not mark attendance: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "grade", EntityType: "attendance",
		EntityShortID: session.ShortID, EntityLabel: session.Name,
		BatchShortID: session.BatchShortID,
		Metadata: map[string]interface{}{"student_count": len(input.Records)},
	})

	c.Status(http.StatusNoContent)
}

// MarkStudentAttendance godoc
//
//	@Summary		Mark one student's attendance for a session
//	@Description	Updates a single student's attendance for one session without resubmitting the whole roster. Restricted to super_admin / team_lead / mentor.
//	@Tags			attendance
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string						true	"Session short ID"
//	@Param			student_id	path	string						true	"Student user ID"
//	@Param			body		body	models.MarkAttendanceInput	true	"Status"
//	@Success		200			{object}	models.SessionAttendance
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Session not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/sessions/{short_id}/attendance/{student_id} [patch]
func (ctrl *AttendanceController) MarkStudentAttendance(c *gin.Context) {
	shortID := c.Param("short_id")
	studentID := c.Param("student_id")

	var input models.MarkAttendanceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := ctrl.sessionRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, session.BatchShortID) {
		return
	}

	markedBy := c.GetString("user_id")
	attendance, err := ctrl.attendanceRepo.MarkOne(c.Request.Context(), session.ID, session.BatchID, studentID, input.Status, input.Notes, markedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not mark attendance: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "grade", EntityType: "attendance",
		EntityShortID: studentID, EntityLabel: session.Name,
		BatchShortID: session.BatchShortID,
		Metadata: map[string]interface{}{"status": input.Status},
	})

	c.JSON(http.StatusOK, attendance)
}

// GetBatchAttendanceSummary godoc
//
//	@Summary		Get a batch's attendance summary
//	@Description	Returns every enrolled student's attendance rollup (present/absent/late/excused counts and percentage) across every held session of the batch.
//	@Tags			attendance
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.StudentAttendanceSummary
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/attendance-summary [get]
func (ctrl *AttendanceController) GetBatchAttendanceSummary(c *gin.Context) {
	shortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	summary, err := ctrl.attendanceRepo.GetBatchSummary(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance summary"})
		return
	}
	if summary == nil {
		summary = []models.StudentAttendanceSummary{}
	}

	c.JSON(http.StatusOK, summary)
}

// GetStudentAttendanceSummary godoc
//
//	@Summary		Get a student's attendance summary
//	@Description	Returns one student's attendance rollup per batch, across every batch they've ever been added to.
//	@Tags			attendance
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{array}		models.StudentAttendanceSummary
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/attendance-summary [get]
func (ctrl *AttendanceController) GetStudentAttendanceSummary(c *gin.Context) {
	userID := c.Param("id")

	summary, err := ctrl.attendanceRepo.GetSummaryForStudent(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance summary"})
		return
	}
	if summary == nil {
		summary = []models.StudentAttendanceSummary{}
	}

	c.JSON(http.StatusOK, summary)
}

// GetStudentAttendanceHistory godoc
//
//	@Summary		Get a student's attendance history
//	@Description	Returns every attendance record ever marked for a student, newest first — the session-by-session log behind "My Attendance".
//	@Tags			attendance
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{array}		models.SessionAttendance
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/attendance-history [get]
func (ctrl *AttendanceController) GetStudentAttendanceHistory(c *gin.Context) {
	userID := c.Param("id")

	history, err := ctrl.attendanceRepo.GetHistoryForStudent(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance history"})
		return
	}
	if history == nil {
		history = []models.SessionAttendance{}
	}

	c.JSON(http.StatusOK, history)
}

// GetSessionReports godoc
//
//	@Summary		Get cross-batch attendance reports
//	@Description	Returns a rollup per session (across every batch, most recent first) — mentors see only sessions in batches they manage; team_lead/super_admin see everything. Only sessions with at least one marked record appear.
//	@Tags			attendance
//	@Produce		json
//	@Success		200	{array}		models.SessionAttendanceReport
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/attendance/reports [get]
func (ctrl *AttendanceController) GetSessionReports(c *gin.Context) {
	mentorID := ""
	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		mentorID = c.GetString("user_id")
	}

	reports, err := ctrl.attendanceRepo.GetSessionReports(c.Request.Context(), mentorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch attendance reports"})
		return
	}
	if reports == nil {
		reports = []models.SessionAttendanceReport{}
	}

	c.JSON(http.StatusOK, reports)
}
