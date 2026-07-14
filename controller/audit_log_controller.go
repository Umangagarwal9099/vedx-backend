package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type AuditLogController struct {
	auditLogRepo *repository.AuditLogRepository
	batchRepo    *repository.BatchRepository
}

func NewAuditLogController(auditLogRepo *repository.AuditLogRepository, batchRepo *repository.BatchRepository) *AuditLogController {
	return &AuditLogController{auditLogRepo: auditLogRepo, batchRepo: batchRepo}
}

// GetAll godoc
//
//	@Summary		Browse the audit log
//	@Description	Returns the 500 most recent audit log entries, newest first. Supports optional filtering by batch_short_id, entity_type, action, and actor_id. Restricted to super_admin / team_lead.
//	@Tags			audit-log
//	@Produce		json
//	@Param			batch_short_id	query	string	false	"Filter by batch short ID"
//	@Param			entity_type		query	string	false	"Filter by entity type (batch, session, assignment, ...)"
//	@Param			action			query	string	false	"Filter by action (create, update, delete, grade, ...)"
//	@Param			actor_id		query	string	false	"Filter by actor user ID"
//	@Success		200	{array}		models.AuditLogEntry
//	@Failure		400	{object}	map[string]string	"Validation error"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/audit-logs [get]
func (ctrl *AuditLogController) GetAll(c *gin.Context) {
	var filter models.AuditLogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries, err := ctrl.auditLogRepo.GetAll(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch audit log"})
		return
	}
	if entries == nil {
		entries = []models.AuditLogEntry{}
	}

	c.JSON(http.StatusOK, entries)
}

// GetForBatch godoc
//
//	@Summary		Get a batch's audit trail
//	@Description	Returns the 500 most recent audit log entries for one batch, newest first. Restricted to super_admin / team_lead / mentor (mentor scoped to batches they manage).
//	@Tags			audit-log
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.AuditLogEntry
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/audit-log [get]
func (ctrl *AuditLogController) GetForBatch(c *gin.Context) {
	shortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	entries, err := ctrl.auditLogRepo.GetForBatch(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch audit log"})
		return
	}
	if entries == nil {
		entries = []models.AuditLogEntry{}
	}

	c.JSON(http.StatusOK, entries)
}
