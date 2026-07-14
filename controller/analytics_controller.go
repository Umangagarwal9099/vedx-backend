package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type AnalyticsController struct {
	analyticsRepo *repository.AnalyticsRepository
	batchRepo     *repository.BatchRepository
}

func NewAnalyticsController(analyticsRepo *repository.AnalyticsRepository, batchRepo *repository.BatchRepository) *AnalyticsController {
	return &AnalyticsController{analyticsRepo: analyticsRepo, batchRepo: batchRepo}
}

// GetBatchAnalytics godoc
//
//	@Summary		Batch & Progress analytics
//	@Description	Returns one row per batch — student count, average completion %, average attendance rate, an attendance-based at-risk count, and certificates issued. Mentors/employees see only batches they manage.
//	@Tags			analytics
//	@Produce		json
//	@Success		200	{array}		models.BatchAnalyticsRow
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/analytics/batches [get]
func (ctrl *AnalyticsController) GetBatchAnalytics(c *gin.Context) {
	mentorID := ""
	role := c.GetString("role")
	if role == string(models.RoleMentor) || role == string(models.RoleEmployee) {
		mentorID = c.GetString("user_id")
	}

	rows, err := ctrl.analyticsRepo.GetBatchAnalytics(c.Request.Context(), mentorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute batch analytics"})
		return
	}
	if rows == nil {
		rows = []models.BatchAnalyticsRow{}
	}
	c.JSON(http.StatusOK, rows)
}

// GetBatchAttendanceTrend godoc
//
//	@Summary		Batch attendance trend
//	@Description	Returns average attendance % per week for one batch, oldest first.
//	@Tags			analytics
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Success		200			{array}		models.BatchAttendanceTrendPoint
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/analytics/batches/{short_id}/attendance-trend [get]
func (ctrl *AnalyticsController) GetBatchAttendanceTrend(c *gin.Context) {
	shortID := c.Param("short_id")
	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	trend, err := ctrl.analyticsRepo.GetBatchAttendanceTrend(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute attendance trend"})
		return
	}
	if trend == nil {
		trend = []models.BatchAttendanceTrendPoint{}
	}
	c.JSON(http.StatusOK, trend)
}
