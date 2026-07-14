package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// checkBatchAccess enforces that a mentor/employee caller only reads or
// mutates content scoped to a batch they actually manage (batch_manager_id or
// additional_manager_id) — the same scoping already applied to list endpoints
// via FindAllForMentor, now extended to detail/update/delete/grade handlers
// that were previously gated by role alone. team_lead/super_admin and any
// content with no batch (global, batchShortID == "") pass through unchanged.
//
// Writes the error response itself on failure; the caller should return
// immediately when this returns false.
func checkBatchAccess(c *gin.Context, batchRepo *repository.BatchRepository, batchShortID string) bool {
	role := c.GetString("role")
	if role != string(models.RoleMentor) && role != string(models.RoleEmployee) {
		return true
	}
	if batchShortID == "" {
		return true
	}

	batch, err := batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify batch access"})
		return false
	}

	userID := c.GetString("user_id")
	owns := batch != nil && (batch.BatchManagerID == userID || (batch.AdditionalManagerID != "" && batch.AdditionalManagerID == userID))
	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "you don't manage this batch"})
		return false
	}
	return true
}
