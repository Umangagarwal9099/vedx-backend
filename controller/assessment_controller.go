package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type AssessmentController struct {
	assessmentRepo *repository.AssessmentRepository
}

func NewAssessmentController(repo *repository.AssessmentRepository) *AssessmentController {
	return &AssessmentController{assessmentRepo: repo}
}

// CreateAssessment godoc
//
//	@Summary		Create assessment
//	@Description	Create a new assessment. Upload thumbnail via POST /upload/assessment-thumbnail and files via POST /upload/assessment-file first, then pass the returned URLs here. result_declaration must be one of: manual | automatic. result_display must be one of: marks_and_status | status_only.
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create assessment"})
		return
	}

	c.JSON(http.StatusCreated, assessment)
}

// GetAllAssessments godoc
//
//	@Summary		List assessments
//	@Description	Returns all active and inactive assessments. Supports optional filtering by name (partial match), description (partial match), and is_active (true|false).
//	@Tags			assessments
//	@Produce		json
//	@Param			name		query	string	false	"Filter by name (partial match)"
//	@Param			description	query	string	false	"Filter by description (partial match)"
//	@Param			is_active	query	string	false	"Filter by active status: true or false"
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

	var assessments []models.Assessment
	var err error

	if filter.Name != "" || filter.Description != "" || filter.IsActive != "" {
		assessments, err = ctrl.assessmentRepo.Search(c.Request.Context(), filter)
	} else {
		assessments, err = ctrl.assessmentRepo.FindAll(c.Request.Context())
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
