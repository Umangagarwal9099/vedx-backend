package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type QuestionBankController struct {
	questionBankRepo *repository.QuestionBankRepository
}

func NewQuestionBankController(repo *repository.QuestionBankRepository) *QuestionBankController {
	return &QuestionBankController{questionBankRepo: repo}
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
