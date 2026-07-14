package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

const moduleReleaseDateLayout = "2006-01-02"

type CurriculumController struct {
	courseRepo   *repository.CourseRepository
	batchRepo    *repository.BatchRepository
	moduleRepo   *repository.ModuleRepository
	scheduleRepo *repository.ModuleScheduleRepository
	auditLogRepo *repository.AuditLogRepository
}

func NewCurriculumController(courseRepo *repository.CourseRepository, batchRepo *repository.BatchRepository, moduleRepo *repository.ModuleRepository, scheduleRepo *repository.ModuleScheduleRepository, auditLogRepo *repository.AuditLogRepository) *CurriculumController {
	return &CurriculumController{courseRepo: courseRepo, batchRepo: batchRepo, moduleRepo: moduleRepo, scheduleRepo: scheduleRepo, auditLogRepo: auditLogRepo}
}

// GetBatchCurriculum godoc
//
//	@Summary		Get a batch's curriculum (with module release status)
//	@Description	Returns the batch's course curriculum, annotating every module with is_released and (if scheduled) release_date. A module with no configured schedule is always released. Students don't see sections/materials for a module that isn't released yet.
//	@Tags			curriculum
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.ModuleWithSections
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/curriculum [get]
func (ctrl *CurriculumController) GetBatchCurriculum(c *gin.Context) {
	shortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	if c.GetString("role") == string(models.RoleStudent) {
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

	curriculum, err := ctrl.courseRepo.GetCurriculum(c.Request.Context(), batch.CourseShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch curriculum"})
		return
	}
	schedule, err := ctrl.scheduleRepo.GetForBatch(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch module schedule"})
		return
	}

	role := c.GetString("role")
	now := time.Now()

	for i := range curriculum {
		released := true
		releaseDateStr, scheduled := schedule[curriculum[i].ShortID]
		if scheduled {
			curriculum[i].ReleaseDate = &releaseDateStr
			if rd, err := time.Parse(moduleReleaseDateLayout, releaseDateStr); err == nil && now.Before(rd) {
				released = false
			}
		}
		curriculum[i].IsReleased = &released

		if role == string(models.RoleStudent) && !released {
			curriculum[i].Sections = []models.SectionSummary{}
		}
	}

	c.JSON(http.StatusOK, curriculum)
}

// GetModuleSchedule godoc
//
//	@Summary		List a batch's module release schedule
//	@Description	Returns every module that has a configured release date for this batch.
//	@Tags			curriculum
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}		models.ModuleScheduleEntry
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/modules/schedule [get]
func (ctrl *CurriculumController) GetModuleSchedule(c *gin.Context) {
	shortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, shortID) {
		return
	}

	entries, err := ctrl.scheduleRepo.ListForBatch(c.Request.Context(), batch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch module schedule"})
		return
	}
	if entries == nil {
		entries = []models.ModuleScheduleEntry{}
	}

	c.JSON(http.StatusOK, entries)
}

// SetModuleSchedule godoc
//
//	@Summary		Schedule a module's release for a batch
//	@Description	Sets (or updates) the date a module becomes visible to this batch's students. Restricted to super_admin / team_lead / mentor.
//	@Tags			curriculum
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path	string							true	"Batch short ID"
//	@Param			module_short_id		path	string							true	"Module short ID"
//	@Param			body				body	models.SetModuleScheduleInput	true	"Release date"
//	@Success		204					"No Content"
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		404					{object}	map[string]string	"Batch or module not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/modules/{module_short_id}/schedule [patch]
func (ctrl *CurriculumController) SetModuleSchedule(c *gin.Context) {
	batchShortID := c.Param("short_id")
	moduleShortID := c.Param("module_short_id")

	var input models.SetModuleScheduleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := time.Parse(moduleReleaseDateLayout, input.ReleaseDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "release_date must be in YYYY-MM-DD format"})
		return
	}

	if !checkBatchAccess(c, ctrl.batchRepo, batchShortID) {
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	module, err := ctrl.moduleRepo.FindByShortID(c.Request.Context(), moduleShortID)
	if err != nil || module == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
		return
	}

	if err := ctrl.scheduleRepo.Upsert(c.Request.Context(), batch.ID, module.ID, input.ReleaseDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save module schedule"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "schedule", EntityType: "module_schedule",
		EntityShortID: module.ShortID, EntityLabel: module.ModuleName,
		BatchShortID: batchShortID,
		Metadata: map[string]interface{}{"release_date": input.ReleaseDate},
	})

	c.Status(http.StatusNoContent)
}

// DeleteModuleSchedule godoc
//
//	@Summary		Unschedule a module's release for a batch
//	@Description	Removes a module's release schedule for this batch, reverting it to "released from day one". Restricted to super_admin / team_lead / mentor.
//	@Tags			curriculum
//	@Produce		json
//	@Param			short_id			path	string	true	"Batch short ID"
//	@Param			module_short_id		path	string	true	"Module short ID"
//	@Success		204					"No Content"
//	@Failure		404					{object}	map[string]string	"Batch, module, or schedule not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/modules/{module_short_id}/schedule [delete]
func (ctrl *CurriculumController) DeleteModuleSchedule(c *gin.Context) {
	batchShortID := c.Param("short_id")
	moduleShortID := c.Param("module_short_id")

	if !checkBatchAccess(c, ctrl.batchRepo, batchShortID) {
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	module, err := ctrl.moduleRepo.FindByShortID(c.Request.Context(), moduleShortID)
	if err != nil || module == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
		return
	}

	if err := ctrl.scheduleRepo.Delete(c.Request.Context(), batch.ID, module.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no schedule configured for this module in this batch"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove module schedule"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "unschedule", EntityType: "module_schedule",
		EntityShortID: module.ShortID, EntityLabel: module.ModuleName,
		BatchShortID: batchShortID,
	})

	c.Status(http.StatusNoContent)
}
