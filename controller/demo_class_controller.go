package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// DemoClassController manages which session recordings are marked as demo
// videos and serves them out publicly (to any authenticated user, regardless
// of batch enrollment or fee status). It reuses SessionRepository and
// BatchRecordingRepository directly rather than duplicating their queries —
// a demo recording is just an existing Session/BatchRecording row with
// is_demo set.
type DemoClassController struct {
	sessionRepo        *repository.SessionRepository
	batchRepo          *repository.BatchRepository
	batchRecordingRepo *repository.BatchRecordingRepository
	demoClassRepo      *repository.DemoClassRepository
}

func NewDemoClassController(sessionRepo *repository.SessionRepository, batchRepo *repository.BatchRepository, batchRecordingRepo *repository.BatchRecordingRepository, demoClassRepo *repository.DemoClassRepository) *DemoClassController {
	return &DemoClassController{
		sessionRepo:        sessionRepo,
		batchRepo:          batchRepo,
		batchRecordingRepo: batchRecordingRepo,
		demoClassRepo:      demoClassRepo,
	}
}

// SetBatchDemoSelection godoc
//
//	@Summary		Set a batch's demo-class recordings
//	@Description	Replaces the full set of recordings (from either source — session-linked or uploaded) marked as demo videos for this batch. Any recording in the batch not included in items is unmarked. Demo recordings bypass the usual fees_paid gate — every authenticated student can watch them via GET /demo-classes/{short_id}, whether or not they're enrolled in this batch.
//	@Tags			demo-classes
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string						true	"Batch short ID"
//	@Param			body		body	models.SetDemoSelectionInput	true	"Recordings to mark as demo"
//	@Success		200	{object}	map[string]string
//	@Failure		400	{object}	map[string]string	"Invalid request body"
//	@Failure		404	{object}	map[string]string	"Batch not found"
//	@Security		BearerAuth
//	@Router			/batches/{short_id}/demo-recordings [put]
func (ctrl *DemoClassController) SetBatchDemoSelection(c *gin.Context) {
	batchShortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	var input models.SetDemoSelectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sessionShortIDs := []string{}
	recordingShortIDs := []string{}
	for _, item := range input.Items {
		if item.Source == "session" {
			sessionShortIDs = append(sessionShortIDs, item.ShortID)
		} else {
			recordingShortIDs = append(recordingShortIDs, item.ShortID)
		}
	}

	if err := ctrl.sessionRepo.SetDemoFlags(c.Request.Context(), batchShortID, sessionShortIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update demo sessions"})
		return
	}
	if err := ctrl.batchRecordingRepo.SetDemoFlags(c.Request.Context(), batchShortID, recordingShortIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update demo recordings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// ListDemoBatches godoc
//
//	@Summary		List batches with demo classes
//	@Description	Every batch that has at least one recording marked as a demo video — this is the "Demo Classes" landing list shown to students (including ones not yet enrolled anywhere), so they can preview real class content before enrolling or paying.
//	@Tags			demo-classes
//	@Produce		json
//	@Success		200	{array}	models.DemoBatchSummary
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/demo-classes [get]
func (ctrl *DemoClassController) ListDemoBatches(c *gin.Context) {
	batches, err := ctrl.demoClassRepo.ListDemoBatches(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch demo classes"})
		return
	}
	c.JSON(http.StatusOK, batches)
}

// GetBatchDemoRecordings godoc
//
//	@Summary		List a batch's demo-class recordings
//	@Description	The demo-marked recordings for one batch — open to any authenticated user regardless of enrollment or fees_paid status, unlike GET /batches/{short_id}/recordings.
//	@Tags			demo-classes
//	@Produce		json
//	@Param			short_id	path		string	true	"Batch short ID"
//	@Success		200			{object}	models.DemoClassesBatchResponse
//	@Failure		404			{object}	map[string]string	"Batch not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/demo-classes/{short_id} [get]
func (ctrl *DemoClassController) GetBatchDemoRecordings(c *gin.Context) {
	batchShortID := c.Param("short_id")

	batch, err := ctrl.batchRepo.FindByShortID(c.Request.Context(), batchShortID)
	if err != nil || batch == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	sessions, err := ctrl.sessionRepo.FindByBatchShortID(c.Request.Context(), batchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch sessions"})
		return
	}
	uploaded, err := ctrl.batchRecordingRepo.FindByBatchShortID(c.Request.Context(), batchShortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch uploaded recordings"})
		return
	}

	recordings := []models.RecordingListItem{}
	for _, s := range sessions {
		if !s.IsDemo || s.RecordingURL == "" {
			continue
		}
		recordings = append(recordings, models.RecordingListItem{
			SessionShortID: s.ShortID,
			Source:         "session",
			Name:           s.Name,
			SessionDate:    s.SessionDate,
			RecordingURL:   apiBaseURL(c) + "/api/v1/stream/sessions/" + s.ShortID,
			IsDemo:         true,
		})
	}
	for _, u := range uploaded {
		if !u.IsDemo {
			continue
		}
		recordings = append(recordings, models.RecordingListItem{
			RecordingShortID: u.ShortID,
			Source:           "upload",
			Name:             u.Title,
			SessionDate:      u.CreatedAt.Format("2006-01-02"),
			RecordingURL:     apiBaseURL(c) + "/api/v1/stream/batch-recordings/" + u.ShortID,
			IsDemo:           true,
		})
	}

	c.JSON(http.StatusOK, models.DemoClassesBatchResponse{
		BatchShortID: batch.ShortID,
		BatchNumber:  batch.BatchNumber,
		CourseName:   batch.CourseName,
		Recordings:   recordings,
	})
}
