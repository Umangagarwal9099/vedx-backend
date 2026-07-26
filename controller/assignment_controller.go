package controller

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type AssignmentController struct {
	assignmentRepo   *repository.AssignmentRepository
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
	auditLogRepo     *repository.AuditLogRepository
}

func NewAssignmentController(assignmentRepo *repository.AssignmentRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository, auditLogRepo *repository.AuditLogRepository) *AssignmentController {
	return &AssignmentController{assignmentRepo: assignmentRepo, batchRepo: batchRepo, notificationRepo: notificationRepo, auditLogRepo: auditLogRepo}
}

// checkStudentAssignmentAccess mirrors checkStudentProjectAccess — a student
// can only reach an assignment they're enrolled in the batch for, and only
// once it's published. Staff bypass entirely (checkBatchAccess scopes them
// by managed batch elsewhere).
func checkStudentAssignmentAccess(c *gin.Context, assignmentRepo *repository.AssignmentRepository, assignmentShortID string) bool {
	if c.GetString("role") != string(models.RoleStudent) {
		return true
	}
	ok, err := assignmentRepo.StudentHasAccess(c.Request.Context(), assignmentShortID, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify assignment access"})
		return false
	}
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "this assignment is not available for your course or batch"})
		return false
	}
	return true
}

// CreateAssignment godoc
//
//	@Summary		Create assignment
//	@Description	Create a new assignment scoped to a batch, with an optional module/session tie. Restricted to super_admin / team_lead / mentor. Set status to "active" to publish and notify the batch immediately, or "draft" (default) to save without notifying.
//	@Tags			assignments
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateAssignmentInput	true	"Assignment details"
//	@Success		201		{object}	models.Assignment
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments [post]
func (ctrl *AssignmentController) Create(c *gin.Context) {
	var input models.CreateAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, input.BatchShortID) {
		return
	}

	createdBy := c.GetString("user_id")

	assignment, err := ctrl.assignmentRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create assignment: " + err.Error()})
		return
	}

	if assignment.Status == "active" {
		ctrl.notifyPublished(c, assignment, createdBy)
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "create", EntityType: "assignment",
		EntityID: assignment.ID, EntityShortID: assignment.ShortID, EntityLabel: assignment.Title,
		BatchShortID: assignment.BatchShortID,
	})

	c.JSON(http.StatusCreated, assignment)
}

// notifyPublished tells enrolled students and the batch mentor about a newly
// published assignment, and team_lead/super_admin more broadly — same pattern
// used for sessions and communities.
func (ctrl *AssignmentController) notifyPublished(c *gin.Context, a *models.Assignment, actorID string) {
	title := "New assignment: " + a.Title
	message := fmt.Sprintf("A new assignment %q has been published for batch %s. Deadline: %s.", a.Title, a.BatchNumber, a.Deadline.Format("Jan 2, 2006 3:04 PM"))

	students, err := ctrl.batchRepo.GetStudents(c.Request.Context(), a.BatchShortID)
	if err != nil {
		log.Printf("fetch batch students for assignment notify: %v", err)
	}
	recipients := make([]string, 0, len(students))
	for _, s := range students {
		recipients = append(recipients, s.UserID)
	}
	if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
		title, message, "assignment", "assignment", a.ShortID, actorID, recipients,
	); err != nil {
		log.Printf("notify assignment publish (students): %v", err)
	}
	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		title, message, "assignment", "assignment", a.ShortID, actorID,
		[]string{string(models.RoleTeamLead), string(models.RoleSuperAdmin)},
	); err != nil {
		log.Printf("notify assignment publish (team_lead/super_admin): %v", err)
	}
}

// GetAllAssignments godoc
//
//	@Summary		List assignments
//	@Description	Returns assignments scoped to the caller's role — students see published assignments for their enrolled batches; mentors see assignments for batches they manage; team_lead/super_admin see everything. Supports optional filtering by batch_short_id and status.
//	@Tags			assignments
//	@Produce		json
//	@Param			batch_short_id	query	string	false	"Filter by batch short ID"
//	@Param			status			query	string	false	"Filter by status: draft | active | closed"
//	@Success		200	{array}		models.Assignment
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments [get]
func (ctrl *AssignmentController) GetAll(c *gin.Context) {
	var filter models.AssignmentFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	userID := c.GetString("user_id")

	var assignments []models.Assignment
	var err error

	switch role {
	case string(models.RoleStudent):
		assignments, err = ctrl.assignmentRepo.FindAllForStudent(c.Request.Context(), userID)
	case string(models.RoleMentor), string(models.RoleEmployee):
		assignments, err = ctrl.assignmentRepo.FindAllForMentor(c.Request.Context(), userID)
	default:
		var collegeID string
		collegeID, err = repository.CollegeFilter(role, c.GetString("college_id"))
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
			return
		}
		assignments, err = ctrl.assignmentRepo.FindAll(c.Request.Context(), filter, collegeID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assignments"})
		return
	}
	if assignments == nil {
		assignments = []models.Assignment{}
	}
	c.JSON(http.StatusOK, assignments)
}

// GetAssignment godoc
//
//	@Summary		Get assignment
//	@Description	Returns a single assignment by its short_id.
//	@Tags			assignments
//	@Produce		json
//	@Param			short_id	path		string	true	"Assignment short ID"
//	@Success		200			{object}	models.Assignment
//	@Failure		404			{object}	map[string]string	"Assignment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id} [get]
func (ctrl *AssignmentController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	a, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assignment"})
		return
	}
	if a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, a.BatchShortID) {
		return
	}
	c.JSON(http.StatusOK, a)
}

// UpdateAssignment godoc
//
//	@Summary		Update assignment
//	@Description	Partially update an assignment. All fields are optional. Restricted to super_admin / team_lead / mentor. Note: publishing a draft via this endpoint (changing status to "active") does not re-trigger student notifications — publish with status "active" on create instead, or notify separately.
//	@Tags			assignments
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Assignment short ID"
//	@Param			body		body		models.UpdateAssignmentInput	true	"Fields to update"
//	@Success		200			{object}	models.Assignment
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404			{object}	map[string]string	"Assignment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id} [patch]
func (ctrl *AssignmentController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assignment"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	var input models.UpdateAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.assignmentRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update assignment"})
		return
	}

	a, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || a == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated assignment"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "assignment",
		EntityID: a.ID, EntityShortID: a.ShortID, EntityLabel: a.Title,
		BatchShortID: a.BatchShortID,
	})

	c.JSON(http.StatusOK, a)
}

// DeleteAssignment godoc
//
//	@Summary		Delete assignment
//	@Description	Soft-deletes an assignment by its short ID. Restricted to super_admin / team_lead / mentor.
//	@Tags			assignments
//	@Produce		json
//	@Param			short_id	path	string	true	"Assignment short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Assignment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id} [delete]
func (ctrl *AssignmentController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assignment"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	if err := ctrl.assignmentRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete assignment"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "delete", EntityType: "assignment",
		EntityID: existing.ID, EntityShortID: existing.ShortID, EntityLabel: existing.Title,
		BatchShortID: existing.BatchShortID,
	})

	c.Status(http.StatusNoContent)
}

// ── Submissions ──────────────────────────────────────────────────────────────

// CreateSubmission godoc
//
//	@Summary		Submit assignment
//	@Description	Submit (or resubmit, if the mentor has requested a resubmission) the calling student's work for an assignment. Upload files first via POST /upload/assignment-file and pass the returned URL as file_url.
//	@Tags			assignments
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Assignment short ID"
//	@Param			body		body		models.CreateAssignmentSubmissionInput	true	"Submission details"
//	@Success		201			{object}	models.AssignmentSubmission
//	@Failure		400			{object}	map[string]string	"Validation error, or already submitted"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id}/submissions [post]
func (ctrl *AssignmentController) CreateSubmission(c *gin.Context) {
	shortID := c.Param("short_id")

	if !checkStudentAssignmentAccess(c, ctrl.assignmentRepo, shortID) {
		return
	}

	var input models.CreateAssignmentSubmissionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	studentID := c.GetString("user_id")

	submission, err := ctrl.assignmentRepo.CreateOrResubmit(c.Request.Context(), shortID, studentID, input)
	if err != nil {
		switch err.Error() {
		case "already submitted":
			c.JSON(http.StatusBadRequest, gin.H{"error": "you've already submitted this assignment; ask your mentor to request a resubmission"})
		case "the submission deadline has passed and late submissions are not allowed":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not submit assignment: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, submission)
}

// GetMySubmission godoc
//
//	@Summary		Get my submission
//	@Description	Returns the calling student's submission for an assignment, if any.
//	@Tags			assignments
//	@Produce		json
//	@Param			short_id	path		string	true	"Assignment short ID"
//	@Success		200			{object}	models.AssignmentSubmission
//	@Failure		404			{object}	map[string]string	"No submission yet"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id}/submissions/me [get]
func (ctrl *AssignmentController) GetMySubmission(c *gin.Context) {
	shortID := c.Param("short_id")
	studentID := c.GetString("user_id")

	if !checkStudentAssignmentAccess(c, ctrl.assignmentRepo, shortID) {
		return
	}

	s, err := ctrl.assignmentRepo.FindMySubmission(c.Request.Context(), shortID, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submission"})
		return
	}
	if s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no submission yet"})
		return
	}
	// Grading is finalized internally the moment a mentor scores it, but the
	// student only sees marks/feedback once results are explicitly published —
	// otherwise this reads as "Submitted — Evaluation Pending".
	if s.Status == "evaluated" {
		publishedAt, found := ctrl.assignmentRepo.GetResultPublishedAt(c.Request.Context(), s.ShortID)
		if !s.ResultsVisibleWith(publishedAt, found) {
			s.Marks = nil
			s.Feedback = ""
		}
	}
	c.JSON(http.StatusOK, s)
}

// PublishResults godoc
//
//	@Summary		Publish this assignment's results
//	@Description	Makes every graded submission's marks/feedback visible to students at once. Grading itself never publishes — this is a deliberate, separate action. Restricted to super_admin / team_lead / mentor (of a batch they manage).
//	@Tags			assignments
//	@Produce		json
//	@Param			short_id	path	string	true	"Assignment short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Assignment not found, or nothing to publish"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id}/publish-results [post]
func (ctrl *AssignmentController) PublishResults(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	if err := ctrl.assignmentRepo.PublishResults(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no graded submissions to publish"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not publish results"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "publish_results", EntityType: "assignment",
		EntityID: existing.ID, EntityShortID: existing.ShortID, EntityLabel: existing.Title,
		BatchShortID: existing.BatchShortID,
	})

	c.Status(http.StatusNoContent)
}

// GetAllSubmissions godoc
//
//	@Summary		List submissions
//	@Description	Returns every submission for an assignment, newest first. Restricted to super_admin / team_lead / mentor.
//	@Tags			assignments
//	@Produce		json
//	@Param			short_id	path		string	true	"Assignment short ID"
//	@Success		200			{array}		models.AssignmentSubmission
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id}/submissions [get]
func (ctrl *AssignmentController) GetAllSubmissions(c *gin.Context) {
	shortID := c.Param("short_id")

	assignment, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, assignment.BatchShortID) {
		return
	}

	submissions, err := ctrl.assignmentRepo.FindAllSubmissions(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if submissions == nil {
		submissions = []models.AssignmentSubmission{}
	}
	c.JSON(http.StatusOK, submissions)
}

// GetAllSubmissionsGlobal godoc
//
//	@Summary		List every assignment submission (cross-assignment)
//	@Description	Returns every assignment submission across every assignment, newest first — mentors see only submissions in batches they manage; team_lead/super_admin see everything. Powers the unified Submissions workspace.
//	@Tags			assignments
//	@Produce		json
//	@Success		200	{array}		models.AssignmentSubmission
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/submissions/assignments [get]
func (ctrl *AssignmentController) GetAllSubmissionsGlobal(c *gin.Context) {
	mentorID := ""
	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		mentorID = c.GetString("user_id")
	}
	collegeID, err := repository.CollegeFilter(role, c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	submissions, err := ctrl.assignmentRepo.FindAllSubmissionsForMentor(c.Request.Context(), mentorID, collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if submissions == nil {
		submissions = []models.AssignmentSubmission{}
	}
	c.JSON(http.StatusOK, submissions)
}

// GradeSubmission godoc
//
//	@Summary		Grade submission
//	@Description	Records marks and feedback for a student's submission, or requests a resubmission. Restricted to super_admin / team_lead / mentor.
//	@Tags			assignments
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string							true	"Assignment short ID"
//	@Param			submission_short_id	path	string							true	"Submission short ID"
//	@Param			body				body	models.GradeSubmissionInput	true	"Grade details"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		404					{object}	map[string]string	"Submission not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assignments/{short_id}/submissions/{submission_short_id} [patch]
func (ctrl *AssignmentController) GradeSubmission(c *gin.Context) {
	shortID := c.Param("short_id")
	submissionShortID := c.Param("submission_short_id")

	assignment, err := ctrl.assignmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, assignment.BatchShortID) {
		return
	}

	var input models.GradeSubmissionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	evaluatedBy := c.GetString("user_id")

	if err := ctrl.assignmentRepo.Grade(c.Request.Context(), shortID, submissionShortID, evaluatedBy, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grade submission"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "grade", EntityType: "assignment_submission",
		EntityShortID: submissionShortID, EntityLabel: assignment.Title,
		BatchShortID: assignment.BatchShortID,
		Metadata:     map[string]interface{}{"marks": input.Marks, "status": input.Status},
	})

	c.Status(http.StatusNoContent)
}
