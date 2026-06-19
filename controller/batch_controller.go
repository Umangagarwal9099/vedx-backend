package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type BatchController struct {
	batchRepo *repository.BatchRepository
}

func NewBatchController(batchRepo *repository.BatchRepository) *BatchController {
	return &BatchController{batchRepo: batchRepo}
}

// CreateBatch godoc
//
//	@Summary		Create batch
//	@Description	Create a new batch. Restricted to super_admin. A short unique ID is generated automatically. Provide course_short_id to link to a course.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateBatchInput	true	"Batch details"
//	@Success		201		{object}	models.Batch
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches [post]
func (ctrl *BatchController) Create(c *gin.Context) {
	var input models.CreateBatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	batch, err := ctrl.batchRepo.Create(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create batch: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, batch)
}

// GetAllBatches godoc
//
//	@Summary		List batches
//	@Description	Returns all non-deleted batches with full course and manager details.
//	@Tags			batches
//	@Produce		json
//	@Success		200	{array}		models.Batch
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches [get]
func (ctrl *BatchController) GetAll(c *gin.Context) {
	batches, err := ctrl.batchRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch batches"})
		return
	}
	if batches == nil {
		batches = []models.Batch{}
	}
	c.JSON(http.StatusOK, batches)
}

// FilterBatches godoc
//
//	@Summary		Filter batches
//	@Description	Filter non-deleted batches by any combination of fields. All query params are optional.
//	@Tags			batches
//	@Produce		json
//	@Param			batch_number		query		string	false	"Partial batch number"
//	@Param			course_short_id		query		string	false	"Exact course short ID"
//	@Param			batch_manager_id	query		string	false	"Exact batch manager user ID (UUID)"
//	@Param			module				query		string	false	"Partial module name"
//	@Param			start_date			query		string	false	"Exact start date (YYYY-MM-DD)"
//	@Param			end_date			query		string	false	"Exact end date (YYYY-MM-DD)"
//	@Param			is_active			query		string	false	"true or false"
//	@Success		200	{array}		models.Batch
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/filter [get]
func (ctrl *BatchController) Filter(c *gin.Context) {
	var f models.BatchFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	batches, err := ctrl.batchRepo.Filter(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not filter batches"})
		return
	}
	if batches == nil {
		batches = []models.Batch{}
	}
	c.JSON(http.StatusOK, batches)
}

// UpdateBatch godoc
//
//	@Summary		Update batch
//	@Description	Partially update a batch by its short_id. Send only the fields you want to change. Use is_active to activate or deactivate. Set additional_manager_id to "" to clear it.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Batch short ID"
//	@Param			body		body		models.UpdateBatchInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Batch
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id} [patch]
func (ctrl *BatchController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateBatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.batchRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update batch"})
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated batch"})
		return
	}

	c.JSON(http.StatusOK, batch)
}

// DeleteBatch godoc
//
//	@Summary		Delete batch
//	@Description	Soft-delete a batch by its short_id.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id} [delete]
func (ctrl *BatchController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.batchRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete batch"})
		return
	}

	c.Status(http.StatusNoContent)
}
