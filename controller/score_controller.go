package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type ScoreController struct {
	scoreRepo    *repository.ScoreRepository
	batchRepo    *repository.BatchRepository
	auditLogRepo *repository.AuditLogRepository
}

func NewScoreController(scoreRepo *repository.ScoreRepository, batchRepo *repository.BatchRepository, auditLogRepo *repository.AuditLogRepository) *ScoreController {
	return &ScoreController{scoreRepo: scoreRepo, batchRepo: batchRepo, auditLogRepo: auditLogRepo}
}

// UpdateScoreWeights godoc
//
//	@Summary		Set a batch's score weights
//	@Description	Sets how much each category (assignments/exams/projects) contributes to the batch's final score and leaderboard ranking. The three weights must sum to 100. Restricted to super_admin / team_lead.
//	@Tags			scores
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string							true	"Batch short ID"
//	@Param			body		body	models.UpdateScoreWeightsInput	true	"Category weights (must sum to 100)"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error or weights don't sum to 100"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/score-weights [patch]
func (ctrl *ScoreController) UpdateScoreWeights(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateScoreWeightsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.AssignmentsWeight+input.ExamsWeight+input.ProjectsWeight != 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assignments_weight + exams_weight + projects_weight must sum to 100"})
		return
	}

	if err := ctrl.batchRepo.UpdateScoreWeights(c.Request.Context(), shortID, input); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "update", EntityType: "score_weights", EntityShortID: shortID,
		BatchShortID: shortID,
		Metadata: map[string]interface{}{
			"assignments_weight": input.AssignmentsWeight,
			"exams_weight":       input.ExamsWeight,
			"projects_weight":    input.ProjectsWeight,
		},
	})

	c.Status(http.StatusNoContent)
}

// GetBatchLeaderboard godoc
//
//	@Summary		Get a batch's leaderboard
//	@Description	Computes every enrolled student's weighted final score (assignments + exams + projects, per the batch's configured weights) and rank, persists it, and returns the sorted leaderboard.
//	@Tags			scores
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.StudentScoreBreakdown
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/leaderboard [get]
func (ctrl *ScoreController) GetBatchLeaderboard(c *gin.Context) {
	shortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	role := c.GetString("role")
	if role == string(models.RoleStudent) {
		enrolled, err := ctrl.batchRepo.IsStudentEnrolled(c.Request.Context(), shortID, c.GetString("user_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify enrollment"})
			return
		}
		if !enrolled {
			c.JSON(http.StatusForbidden, gin.H{"error": "you're not enrolled in this batch"})
			return
		}
	} else if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	leaderboard, err := ctrl.scoreRepo.GetBatchLeaderboard(
		c.Request.Context(), batch.ID, batch.ShortID, batch.BatchNumber,
		batch.ScoreWeightAssignments, batch.ScoreWeightExams, batch.ScoreWeightProjects,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute leaderboard"})
		return
	}
	if leaderboard == nil {
		leaderboard = []models.StudentScoreBreakdown{}
	}

	c.JSON(http.StatusOK, leaderboard)
}

// GetStudentScoreBreakdown godoc
//
//	@Summary		Get a student's score breakdown
//	@Description	Returns one student's score breakdown (assignments/exams/projects/final score) per batch, across every batch they've ever been added to.
//	@Tags			scores
//	@Produce		json
//	@Param			id	path		string	true	"Student user ID"
//	@Success		200	{array}		models.StudentScoreBreakdown
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/score-breakdown [get]
func (ctrl *ScoreController) GetStudentScoreBreakdown(c *gin.Context) {
	userID := c.Param("id")

	batches, err := ctrl.batchRepo.FindByStudentID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch student's batches"})
		return
	}

	out := make([]models.StudentScoreBreakdown, 0, len(batches))
	for _, batch := range batches {
		b, err := ctrl.scoreRepo.GetStudentBreakdown(
			c.Request.Context(), userID, batch.ID, batch.ShortID, batch.BatchNumber,
			batch.ScoreWeightAssignments, batch.ScoreWeightExams, batch.ScoreWeightProjects,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not compute score breakdown"})
			return
		}
		out = append(out, b)
	}

	c.JSON(http.StatusOK, out)
}
