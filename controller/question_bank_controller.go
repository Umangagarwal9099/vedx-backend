package controller

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type QuestionBankController struct {
	questionBankRepo *repository.QuestionBankRepository
	subjectRepo      *repository.QuestionBankSubjectRepository
	auditLogRepo     *repository.AuditLogRepository
}

func NewQuestionBankController(repo *repository.QuestionBankRepository, subjectRepo *repository.QuestionBankSubjectRepository, auditLogRepo *repository.AuditLogRepository) *QuestionBankController {
	return &QuestionBankController{questionBankRepo: repo, subjectRepo: subjectRepo, auditLogRepo: auditLogRepo}
}

// CreateQuestion godoc
//
//	@Summary		Create question
//	@Description	Add a question to the question bank. For question_type "coding", set coding_question_short_id to link an existing entry from GET /coding-questions instead of duplicating it — grading for coding answers within an exam is manual. Visibility defaults to "private" (only visible to the creator); set "course" or "global" to share it.
//	@Tags			questions
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateQuestionInput	true	"Question details"
//	@Success		201		{object}	models.Question
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/questions [post]
func (ctrl *QuestionBankController) Create(c *gin.Context) {
	var input models.CreateQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	q, err := ctrl.questionBankRepo.Create(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create question: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, q)
}

// GetAllQuestions godoc
//
//	@Summary		List questions
//	@Description	Returns questions visible to the caller — their own private questions plus anything shared at course/global visibility. team_lead/super_admin see every question. Supports optional filtering by question_type, topic, and difficulty.
//	@Tags			questions
//	@Produce		json
//	@Param			question_type	query	string	false	"Filter by type: mcq | multi_select | true_false | fill_blank | short_answer | descriptive | coding"
//	@Param			topic			query	string	false	"Filter by topic (partial match)"
//	@Param			difficulty		query	string	false	"Filter by difficulty: easy | medium | hard"
//	@Success		200	{array}		models.Question
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/questions [get]
func (ctrl *QuestionBankController) GetAll(c *gin.Context) {
	var filter models.QuestionFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	creatorID := c.GetString("user_id")
	if role == string(models.RoleSuperAdmin) || role == string(models.RoleTeamLead) {
		creatorID = "" // staff see everything, not just their own + shared
	}

	questions, err := ctrl.questionBankRepo.FindAll(c.Request.Context(), filter, creatorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch questions"})
		return
	}
	if questions == nil {
		questions = []models.Question{}
	}
	c.JSON(http.StatusOK, questions)
}

// GetQuestion godoc
//
//	@Summary		Get question
//	@Description	Returns a single question-bank entry by its short_id (including its correct answer — restricted to super_admin / team_lead / mentor).
//	@Tags			questions
//	@Produce		json
//	@Param			short_id	path		string	true	"Question short ID"
//	@Success		200			{object}	models.Question
//	@Failure		404			{object}	map[string]string	"Question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/questions/{short_id} [get]
func (ctrl *QuestionBankController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	q, err := ctrl.questionBankRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch question"})
		return
	}
	if q == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
		return
	}
	c.JSON(http.StatusOK, q)
}

// UpdateQuestion godoc
//
//	@Summary		Update question
//	@Description	Partially update a question-bank entry. Restricted to super_admin / team_lead / mentor.
//	@Tags			questions
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Question short ID"
//	@Param			body		body		models.UpdateQuestionInput	true	"Fields to update"
//	@Success		200			{object}	models.Question
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404			{object}	map[string]string	"Question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/questions/{short_id} [patch]
func (ctrl *QuestionBankController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.questionBankRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		log.Printf("update question: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update question"})
		return
	}

	q, err := ctrl.questionBankRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || q == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated question"})
		return
	}
	c.JSON(http.StatusOK, q)
}

// DeleteQuestion godoc
//
//	@Summary		Delete question
//	@Description	Soft-deletes a question-bank entry. Restricted to super_admin / team_lead / mentor.
//	@Tags			questions
//	@Produce		json
//	@Param			short_id	path	string	true	"Question short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Question not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/questions/{short_id} [delete]
func (ctrl *QuestionBankController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")
	if err := ctrl.questionBankRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete question"})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetTaxonomy godoc
//
//	@Summary		Get question bank taxonomy
//	@Description	Returns the fixed Subject -> Topic -> Subtopic reference tree used to drive the browse UI and cascading selects.
//	@Tags			questions
//	@Produce		json
//	@Success		200	{array}	models.QuestionBankTaxonomy
//	@Security		BearerAuth
//	@Router			/question-bank/taxonomy [get]
func (ctrl *QuestionBankController) GetTaxonomy(c *gin.Context) {
	subjects, err := ctrl.subjectRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch subjects"})
		return
	}

	tree := make([]models.QuestionBankTaxonomy, 0, len(subjects))
	for _, s := range subjects {
		topics := models.TopicsForSubject(s.Name)
		if topics == nil {
			topics = []models.QuestionBankTopic{} // never null — the frontend type isn't optional
		}
		tree = append(tree, models.QuestionBankTaxonomy{
			Subject: s.Name,
			Topics:  topics,
			ShortID: s.ShortID,
		})
	}
	c.JSON(http.StatusOK, tree)
}

// CreateSubject godoc
//
//	@Summary		Add a question bank subject
//	@Description	Adds a new subject tile to the question bank browse page.
//	@Tags			questions
//	@Accept			json
//	@Produce		json
//	@Param			body	body		object{name=string}	true	"Subject name"
//	@Success		201		{object}	models.QuestionBankSubject
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		409		{object}	map[string]string	"Name already exists"
//	@Security		BearerAuth
//	@Router			/question-bank/subjects [post]
func (ctrl *QuestionBankController) CreateSubject(c *gin.Context) {
	var input struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be blank"})
		return
	}

	subject, err := ctrl.subjectRepo.Create(c.Request.Context(), name, c.GetString("user_id"))
	if err != nil {
		if errors.Is(err, repository.ErrSubjectNameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "a subject with this name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create subject"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "create", EntityType: "question_bank_subject",
		EntityID: subject.ID, EntityShortID: subject.ShortID, EntityLabel: subject.Name,
	})

	c.JSON(http.StatusCreated, subject)
}

// DeleteSubject godoc
//
//	@Summary		Delete a question bank subject
//	@Description	Removes a subject tile. Refuses if any question still uses this subject.
//	@Tags			questions
//	@Produce		json
//	@Param			short_id	path	string	true	"Subject short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Not found"
//	@Failure		409			{object}	map[string]string	"Subject still has questions"
//	@Security		BearerAuth
//	@Router			/question-bank/subjects/{short_id} [delete]
func (ctrl *QuestionBankController) DeleteSubject(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.subjectRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, repository.ErrSubjectInUse) {
			c.JSON(http.StatusConflict, gin.H{"error": "this subject still has questions assigned to it — move or delete them first"})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "subject not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete subject"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "delete", EntityType: "question_bank_subject", EntityShortID: shortID,
	})

	c.Status(http.StatusNoContent)
}

// GetStats godoc
//
//	@Summary		Get question bank stats for a subject
//	@Description	Returns aggregate counts (total, by question_type, by difficulty, by topic) for the given subject.
//	@Tags			questions
//	@Produce		json
//	@Param			subject	query		string	true	"Subject name, e.g. Python"
//	@Success		200		{object}	models.QuestionBankStats
//	@Failure		400		{object}	map[string]string	"Missing subject param"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/question-bank/stats [get]
func (ctrl *QuestionBankController) GetStats(c *gin.Context) {
	subject := c.Query("subject")
	if subject == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'subject' is required"})
		return
	}

	stats, err := ctrl.questionBankRepo.GetStats(c.Request.Context(), subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch stats"})
		return
	}
	c.JSON(http.StatusOK, stats)
}
