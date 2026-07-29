package controller

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type BatchRecordingController struct {
	recordingRepo *repository.BatchRecordingRepository
	batchRepo     *repository.BatchRepository
	storage       *service.StorageService
	auditLogRepo  *repository.AuditLogRepository
}

func NewBatchRecordingController(recordingRepo *repository.BatchRecordingRepository, batchRepo *repository.BatchRepository, storage *service.StorageService, auditLogRepo *repository.AuditLogRepository) *BatchRecordingController {
	return &BatchRecordingController{recordingRepo: recordingRepo, batchRepo: batchRepo, storage: storage, auditLogRepo: auditLogRepo}
}

// Upload godoc
//
//	@Summary		Upload batch recordings
//	@Description	Upload one or more recorded lecture videos directly to a batch — for recordings that didn't come from a live Zoom session (e.g. bulk-attaching an existing archive). Each file is stored in Cloudflare R2 and immediately visible to students who are enrolled and fees_paid in this batch via GET /batches/{short_id}/recordings — a student in a different batch can never see them. Attach multiple files under the same "files" form field in one request. Max 3 GB per file. Allowed types: MP4, MOV, AVI, WebM. Restricted to super_admin / team_lead / mentor (mentor must manage this batch).
//	@Tags			sessions
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Param			files		formData	file	true	"One or more video files"
//	@Success		201			{array}		models.BatchRecording
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Forbidden — not your batch"
//	@Failure		500			{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/recordings/upload [post]
func (ctrl *BatchRecordingController) Upload(c *gin.Context) {
	batchShortID := c.Param("short_id")
	if !checkBatchAccess(c, ctrl.batchRepo, batchShortID) {
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expected multipart/form-data with one or more files under 'files'"})
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'files' is required — attach one or more video files"})
		return
	}

	uploadedBy := c.GetString("user_id")
	created := make([]models.BatchRecording, 0, len(files))
	for _, fh := range files {
		url, contentType, size, err := ctrl.storage.UploadBatchRecording(fh, batchShortID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s: %s", fh.Filename, err.Error())})
			return
		}

		title := strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename))
		rec, err := ctrl.recordingRepo.Create(c.Request.Context(), batchShortID, title, url, size, contentType, uploadedBy)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save recording: " + err.Error()})
			return
		}

		logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
			Action: "create", EntityType: "batch_recording",
			EntityID: rec.ID, EntityShortID: rec.ShortID, EntityLabel: rec.Title,
			BatchShortID: batchShortID,
		})

		created = append(created, *rec)
	}

	c.JSON(http.StatusCreated, created)
}

// Delete godoc
//
//	@Summary		Delete a batch recording
//	@Description	Soft-deletes an admin-uploaded batch recording. Does not affect session-linked recordings (those come from GET /sessions, not this table). Restricted to super_admin / team_lead / mentor managing this batch.
//	@Tags			sessions
//	@Produce		json
//	@Param			short_id			path	string	true	"Batch short ID"
//	@Param			recording_short_id	path	string	true	"Recording short ID"
//	@Success		204					"No Content"
//	@Failure		404					{object}	map[string]string	"Recording not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/recordings/{recording_short_id} [delete]
func (ctrl *BatchRecordingController) Delete(c *gin.Context) {
	batchShortID := c.Param("short_id")
	shortID := c.Param("recording_short_id")

	existing, err := ctrl.recordingRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || existing == nil || existing.BatchShortID != batchShortID {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	if !checkBatchAccess(c, ctrl.batchRepo, batchShortID) {
		return
	}

	if err := ctrl.recordingRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete recording"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "delete", EntityType: "batch_recording",
		EntityID: existing.ID, EntityShortID: existing.ShortID, EntityLabel: existing.Title,
		BatchShortID: batchShortID,
	})

	c.Status(http.StatusNoContent)
}
