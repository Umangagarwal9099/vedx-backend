package controller

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// GuestAccessController lets a student who's already enrolled in one batch
// also view another batch's live sessions and recordings — e.g. to sit in
// on a different mentor's teaching of the same course — without a second
// full enrollment. See db/schema_updates_guest_access_v1.sql.
type GuestAccessController struct {
	guestAccessRepo  *repository.GuestAccessRepository
	batchRepo        *repository.BatchRepository
	notificationRepo *repository.NotificationRepository
	auditLogRepo     *repository.AuditLogRepository
}

func NewGuestAccessController(guestAccessRepo *repository.GuestAccessRepository, batchRepo *repository.BatchRepository, notificationRepo *repository.NotificationRepository, auditLogRepo *repository.AuditLogRepository) *GuestAccessController {
	return &GuestAccessController{guestAccessRepo: guestAccessRepo, batchRepo: batchRepo, notificationRepo: notificationRepo, auditLogRepo: auditLogRepo}
}

// Grant godoc
//
//	@Summary		Grant guest access to a batch
//	@Description	Lets each listed student view this batch's live sessions and recordings — no assignments, progress, certificate, or fee requirement, and no effect on any enrollment they already hold elsewhere. Restricted to Admin/Super Admin. Students not in this batch's college are silently skipped, same as POST /batches/{short_id}/students.
//	@Tags			batches
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string					true	"Batch short ID"
//	@Param			body		body	models.GrantGuestAccessInput	true	"Student IDs"
//	@Success		200			{object}	map[string]int	"granted"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/guest-access [post]
func (ctrl *GuestAccessController) Grant(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.GrantGuestAccessInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	grantedBy := c.GetString("user_id")
	granted, err := ctrl.guestAccessRepo.Grant(c.Request.Context(), shortID, input.StudentIDs, grantedBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grant guest access: " + err.Error()})
		return
	}

	if len(granted) > 0 {
		if err := ctrl.notificationRepo.NotifyUsers(c.Request.Context(),
			"Guest access: "+batch.BatchNumber,
			fmt.Sprintf("You can now also view live sessions and recordings from batch %q, in addition to your own batch.", batch.BatchNumber),
			"batch", "batch", shortID, grantedBy, granted,
		); err != nil {
			log.Printf("notify guest access grant: %v", err)
		}

		logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
			Action: "grant_guest_access", EntityType: "batch_guest_access",
			EntityShortID: shortID, EntityLabel: batch.BatchNumber,
			BatchShortID: batch.ShortID,
			Metadata:      map[string]interface{}{"student_ids": granted, "count": len(granted)},
		})
	}

	c.JSON(http.StatusOK, gin.H{"granted": len(granted)})
}

// Revoke godoc
//
//	@Summary		Revoke guest access
//	@Description	Removes one student's guest access to this batch. Never touches any real enrollment. Restricted to Admin/Super Admin.
//	@Tags			batches
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Param			user_id		path	string	true	"Student user ID"
//	@Success		204
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/guest-access/{user_id} [delete]
func (ctrl *GuestAccessController) Revoke(c *gin.Context) {
	shortID := c.Param("short_id")
	userID := c.Param("user_id")

	if err := ctrl.guestAccessRepo.Revoke(c.Request.Context(), shortID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke guest access"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "revoke_guest_access", EntityType: "batch_guest_access",
		EntityShortID: shortID, BatchShortID: shortID,
		Metadata: map[string]interface{}{"student_id": userID},
	})

	c.Status(http.StatusNoContent)
}

// ListForBatch godoc
//
//	@Summary		List a batch's guest-access students
//	@Description	Everyone currently able to view this batch's sessions/recordings as a guest. Restricted to staff.
//	@Tags			batches
//	@Produce		json
//	@Param			short_id	path	string	true	"Batch short ID"
//	@Success		200			{array}	models.GuestAccessStudent
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/guest-access [get]
func (ctrl *GuestAccessController) ListForBatch(c *gin.Context) {
	shortID := c.Param("short_id")
	list, err := ctrl.guestAccessRepo.ListForBatch(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch guest access list"})
		return
	}
	c.JSON(http.StatusOK, list)
}

// ListMine godoc
//
//	@Summary		List batches I have guest access to
//	@Description	Batches the logged-in student can view as a guest, kept separate from their real enrollments (GET /users/{id}/enrollments) — the frontend should label these distinctly, e.g. "Guest Access", never merge them into "My Courses".
//	@Tags			batches
//	@Produce		json
//	@Success		200	{array}	models.GuestBatchAccess
//	@Security		BearerAuth
//	@Router			/batches/guest-access/mine [get]
func (ctrl *GuestAccessController) ListMine(c *gin.Context) {
	userID := c.GetString("user_id")
	list, err := ctrl.guestAccessRepo.ListForStudent(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch guest access"})
		return
	}
	c.JSON(http.StatusOK, list)
}
