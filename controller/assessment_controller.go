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

type AssessmentController struct {
	assessmentRepo   *repository.AssessmentRepository
	questionBankRepo *repository.QuestionBankRepository
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
}

func NewAssessmentController(repo *repository.AssessmentRepository, questionBankRepo *repository.QuestionBankRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository) *AssessmentController {
	return &AssessmentController{assessmentRepo: repo, questionBankRepo: questionBankRepo, batchRepo: batchRepo, notificationRepo: notificationRepo}
}

// CreateAssessment godoc
//
//	@Summary		Create assessment
//	@Description	Create a new assessment — either static content (leave start_at/end_at/duration_minutes unset) or a real timed exam. Attach questions afterwards via POST /assessments/{short_id}/questions. Leave batch_short_id empty for global visibility. Upload thumbnail via POST /upload/assessment-thumbnail and files via POST /upload/assessment-file first, then pass the returned URLs here.
//	@Tags			assessments
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateAssessmentInput	true	"Assessment details"
//	@Success		201		{object}	models.Assessment
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments [post]
func (ctrl *AssessmentController) Create(c *gin.Context) {
	var input models.CreateAssessmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	assessment, err := ctrl.assessmentRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create assessment: " + err.Error()})
		return
	}

	if assessment.BatchShortID != "" {
		ctrl.notifyBatch(c, assessment, createdBy)
	} else if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		"New assessment: "+assessment.Name,
		fmt.Sprintf("A new assessment %q has been added.", assessment.Name),
		"assessment", "assessment", assessment.ShortID, createdBy,
		[]string{"student", "mentor", "team_lead"},
	); err != nil {
		log.Printf("notify assessment create: %v", err)
	}

	c.JSON(http.StatusCreated, assessment)
}

func (ctrl *AssessmentController) notifyBatch(c *gin.Context, a *models.Assessment, actorID string) {
	title := "New assessment: " + a.Name
	message := fmt.Sprintf("A new assessment %q has been published for batch %s.", a.Name, a.BatchNumber)

	students, err := ctrl.batchRepo.GetStudents(c.Request.Context(), a.BatchShortID)
	if err != nil {
		log.Printf("fetch batch students for assessment notify: %v", err)
	}
	recipients := make([]string, 0, len(students))
	for _, s := range students {
		recipients = append(recipients, s.UserID)
	}
	if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
		title, message, "assessment", "assessment", a.ShortID, actorID, recipients,
	); err != nil {
		log.Printf("notify assessment publish (students): %v", err)
	}
	if err := ctrl.notificationRepo.NotifyRoles(c.Request.Context(),
		title, message, "assessment", "assessment", a.ShortID, actorID,
		[]string{string(models.RoleTeamLead), string(models.RoleSuperAdmin)},
	); err != nil {
		log.Printf("notify assessment publish (team_lead/super_admin): %v", err)
	}
}

// GetAllAssessments godoc
//
//	@Summary		List assessments
//	@Description	Returns assessments scoped to the caller's role — students see active global assessments plus ones for their enrolled batches; mentors see global plus their managed batches; team_lead/super_admin see everything (with optional name/description/is_active/batch_short_id filters).
//	@Tags			assessments
//	@Produce		json
//	@Param			name		query	string	false	"Filter by name (partial match)"
//	@Param			description	query	string	false	"Filter by description (partial match)"
//	@Param			is_active	query	string	false	"Filter by active status: true or false"
//	@Param			batch_short_id	query	string	false	"Filter by batch short ID"
//	@Success		200	{array}		models.Assessment
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments [get]
func (ctrl *AssessmentController) GetAll(c *gin.Context) {
	var filter models.AssessmentFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	userID := c.GetString("user_id")

	var assessments []models.Assessment
	var err error

	switch role {
	case string(models.RoleStudent):
		assessments, err = ctrl.assessmentRepo.FindAllForStudent(c.Request.Context(), userID)
	case string(models.RoleMentor), string(models.RoleEmployee):
		assessments, err = ctrl.assessmentRepo.FindAllForMentor(c.Request.Context(), userID)
	default:
		if filter.Name != "" || filter.Description != "" || filter.IsActive != "" || filter.BatchShortID != "" {
			assessments, err = ctrl.assessmentRepo.Search(c.Request.Context(), filter)
		} else {
			assessments, err = ctrl.assessmentRepo.FindAll(c.Request.Context())
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assessments"})
		return
	}
	if assessments == nil {
		assessments = []models.Assessment{}
	}
	c.JSON(http.StatusOK, assessments)
}

// GetAssessment godoc
//
//	@Summary		Get assessment
//	@Description	Returns a single assessment by its short_id.
//	@Tags			assessments
//	@Produce		json
//	@Param			short_id	path		string	true	"Assessment short ID"
//	@Success		200			{object}	models.Assessment
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id} [get]
func (ctrl *AssessmentController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	a, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assessment"})
		return
	}
	if a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, a.BatchShortID) {
		return
	}
	c.JSON(http.StatusOK, a)
}

// UpdateAssessment godoc
//
//	@Summary		Update assessment
//	@Description	Partially update an assessment. All fields are optional — only provided fields are updated. To replace files, upload new ones via POST /upload/assessment-file and pass the full updated file_urls array. To replace the thumbnail, upload via POST /upload/assessment-thumbnail.
//	@Tags			assessments
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Assessment short ID"
//	@Param			body		body		models.UpdateAssessmentInput	true	"Fields to update"
//	@Success		200			{object}	models.Assessment
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id} [patch]
func (ctrl *AssessmentController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assessment"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	var input models.UpdateAssessmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.assessmentRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update assessment"})
		return
	}

	assessment, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || assessment == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated assessment"})
		return
	}

	c.JSON(http.StatusOK, assessment)
}

// DeleteAssessment godoc
//
//	@Summary		Delete assessment
//	@Description	Soft-deletes an assessment by its short ID. The record is retained in the database with deleted_at set.
//	@Tags			assessments
//	@Produce		json
//	@Param			short_id	path	string	true	"Assessment short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Assessment not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id} [delete]
func (ctrl *AssessmentController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	existing, err := ctrl.assessmentRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch assessment"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, existing.BatchShortID) {
		return
	}

	if err := ctrl.assessmentRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "assessment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete assessment"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ── Questions attached to an assessment ─────────────────────────────────────

// AttachQuestion godoc
//
//	@Summary		Attach question to assessment
//	@Description	Attaches an existing question-bank entry (see POST /questions) to an assessment, with a display order and optional marks override. Restricted to super_admin / team_lead / mentor.
//	@Tags			assessments
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string							true	"Assessment short ID"
//	@Param			body		body	models.AttachQuestionInput	true	"Question to attach"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/questions [post]
func (ctrl *AssessmentController) AttachQuestion(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.AttachQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.questionBankRepo.AttachQuestion(c.Request.Context(), shortID, input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not attach question: " + err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetQuestions godoc
//
//	@Summary		List assessment questions
//	@Description	Returns every question attached to an assessment, in display order. Restricted to super_admin / team_lead / mentor (students never see the raw question bank with answers — they get a sanitized view via the attempt endpoints).
//	@Tags			assessments
//	@Produce		json
//	@Param			short_id	path	string	true	"Assessment short ID"
//	@Success		200			{array}	models.AssessmentQuestion
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/questions [get]
func (ctrl *AssessmentController) GetQuestions(c *gin.Context) {
	shortID := c.Param("short_id")
	questions, err := ctrl.questionBankRepo.GetQuestions(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch questions"})
		return
	}
	if questions == nil {
		questions = []models.AssessmentQuestion{}
	}
	c.JSON(http.StatusOK, questions)
}

// UpdateAttachedQuestion godoc
//
//	@Summary		Update attached question
//	@Description	Updates the display order or marks override for a question already attached to an assessment. Restricted to super_admin / team_lead / mentor.
//	@Tags			assessments
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string								true	"Assessment short ID"
//	@Param			question_short_id	path	string								true	"Question short ID"
//	@Param			body				body	models.UpdateAttachedQuestionInput	true	"Fields to update"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		404					{object}	map[string]string	"Not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/questions/{question_short_id} [patch]
func (ctrl *AssessmentController) UpdateAttachedQuestion(c *gin.Context) {
	shortID := c.Param("short_id")
	questionShortID := c.Param("question_short_id")

	var input models.UpdateAttachedQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.questionBankRepo.UpdateAttachedQuestion(c.Request.Context(), shortID, questionShortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update"})
		return
	}
	c.Status(http.StatusNoContent)
}

// DetachQuestion godoc
//
//	@Summary		Detach question from assessment
//	@Description	Removes a question from an assessment (the question itself remains in the bank). Restricted to super_admin / team_lead / mentor.
//	@Tags			assessments
//	@Produce		json
//	@Param			short_id			path	string	true	"Assessment short ID"
//	@Param			question_short_id	path	string	true	"Question short ID"
//	@Success		204					"No Content"
//	@Failure		404					{object}	map[string]string	"Not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/assessments/{short_id}/questions/{question_short_id} [delete]
func (ctrl *AssessmentController) DetachQuestion(c *gin.Context) {
	shortID := c.Param("short_id")
	questionShortID := c.Param("question_short_id")
	if err := ctrl.questionBankRepo.DetachQuestion(c.Request.Context(), shortID, questionShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not detach question"})
		return
	}
	c.Status(http.StatusNoContent)
}
