package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type CodingQuestionController struct {
	repo *repository.CodingQuestionRepository
}

func NewCodingQuestionController(repo *repository.CodingQuestionRepository) *CodingQuestionController {
	return &CodingQuestionController{repo: repo}
}

// Create godoc
//
//	@Summary		Create coding question
//	@Description	Create a new coding practice question. Restricted to super_admin.
//	@Tags			coding-questions
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateCodingQuestionInput	true	"Question details"
//	@Success		201		{object}	models.CodingQuestion
//	@Failure		400		{object}	map[string]string
//	@Failure		403		{object}	map[string]string
//	@Failure		500		{object}	map[string]string
//	@Security		BearerAuth
//	@Router			/coding-questions [post]
func (ctrl *CodingQuestionController) Create(c *gin.Context) {
	var input models.CreateCodingQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")
	q, err := ctrl.repo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create coding question"})
		return
	}
	c.JSON(http.StatusCreated, q)
}

// GetAll godoc
//
//	@Summary		List coding questions (student view)
//	@Description	Returns all active, non-deleted coding questions ordered by creation date.
//	@Tags			coding-questions
//	@Produce		json
//	@Success		200	{array}		models.CodingQuestion
//	@Failure		500	{object}	map[string]string
//	@Security		BearerAuth
//	@Router			/coding-questions [get]
func (ctrl *CodingQuestionController) GetAll(c *gin.Context) {
	questions, err := ctrl.repo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch coding questions"})
		return
	}
	if questions == nil {
		questions = []models.CodingQuestion{}
	}
	c.JSON(http.StatusOK, questions)
}

// GetAllAdmin godoc
//
//	@Summary		List all coding questions (admin view)
//	@Description	Returns all non-deleted coding questions including inactive ones.
//	@Tags			coding-questions
//	@Produce		json
//	@Success		200	{array}		models.CodingQuestion
//	@Failure		500	{object}	map[string]string
//	@Security		BearerAuth
//	@Router			/coding-questions/admin [get]
func (ctrl *CodingQuestionController) GetAllAdmin(c *gin.Context) {
	questions, err := ctrl.repo.FindAllAdmin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch coding questions"})
		return
	}
	if questions == nil {
		questions = []models.CodingQuestion{}
	}
	c.JSON(http.StatusOK, questions)
}

// GetByShortID godoc
//
//	@Summary		Get coding question by short_id
//	@Description	Returns a single active coding question by its short_id.
//	@Tags			coding-questions
//	@Produce		json
//	@Param			short_id	path		string	true	"Question short ID"
//	@Success		200			{object}	models.CodingQuestion
//	@Failure		404			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Security		BearerAuth
//	@Router			/coding-questions/{short_id} [get]
func (ctrl *CodingQuestionController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	q, err := ctrl.repo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch coding question"})
		return
	}
	if q == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "coding question not found"})
		return
	}
	c.JSON(http.StatusOK, q)
}

// Update godoc
//
//	@Summary		Update coding question
//	@Description	Partially update a coding question by its short_id.
//	@Tags			coding-questions
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Question short ID"
//	@Param			body		body		models.UpdateCodingQuestionInput	true	"Fields to update"
//	@Success		200			{object}	models.CodingQuestion
//	@Failure		400			{object}	map[string]string
//	@Failure		404			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Security		BearerAuth
//	@Router			/coding-questions/{short_id} [patch]
func (ctrl *CodingQuestionController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateCodingQuestionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.repo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "coding question not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update coding question"})
		return
	}

	q, err := ctrl.repo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || q == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated question"})
		return
	}
	c.JSON(http.StatusOK, q)
}

// Delete godoc
//
//	@Summary		Delete coding question
//	@Description	Soft-delete a coding question by its short_id.
//	@Tags			coding-questions
//	@Produce		json
//	@Param			short_id	path	string	true	"Question short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string
//	@Failure		500			{object}	map[string]string
//	@Security		BearerAuth
//	@Router			/coding-questions/{short_id} [delete]
func (ctrl *CodingQuestionController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")
	if err := ctrl.repo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "coding question not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete coding question"})
		return
	}
	c.Status(http.StatusNoContent)
}
