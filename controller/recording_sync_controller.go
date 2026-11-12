package controller

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/recordingsync"
	"github.com/umangagarwal/vedx-backend/repository"
)

// RecordingSyncController exposes the Zoom recording sync queue: a
// secret-protected trigger for Cloud Scheduler, and read/trigger endpoints
// for admins.
type RecordingSyncController struct {
	syncSvc      *recordingsync.Service
	jobRepo      *repository.ZoomRecordingJobRepository
	secret       string
	lookbackDays int
}

func NewRecordingSyncController(syncSvc *recordingsync.Service, jobRepo *repository.ZoomRecordingJobRepository, secret string, lookbackDays int) *RecordingSyncController {
	return &RecordingSyncController{syncSvc: syncSvc, jobRepo: jobRepo, secret: secret, lookbackDays: lookbackDays}
}

// TriggerSync godoc
//
//	@Summary		Run Zoom recording sync (Cloud Scheduler)
//	@Description	Processes due recording jobs synchronously inside this request — on Cloud Run that is the only way the download/watermark/upload work is guaranteed CPU. Authenticated by the X-Recording-Sync-Secret header (RECORDING_SYNC_SECRET). reconcile=true first lists Zoom recordings for the lookback window and enqueues any the webhook missed. Configure the Cloud Run request timeout to at least budget_minutes+5.
//	@Tags			zoom
//	@Produce		json
//	@Param			reconcile		query	bool	false	"List Zoom recordings and enqueue missing ones first"
//	@Param			max_jobs		query	int		false	"Stop after this many jobs (default 20)"
//	@Param			budget_minutes	query	int		false	"Stop claiming new jobs after this many minutes (default 45)"
//	@Param			process			query	bool	false	"false = reconcile/queue status only, no encoding (default true)"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	map[string]string
//	@Router			/internal/recordings/sync [post]
func (ctrl *RecordingSyncController) TriggerSync(c *gin.Context) {
	if ctrl.secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RECORDING_SYNC_SECRET not configured"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Recording-Sync-Secret")), []byte(ctrl.secret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid sync secret"})
		return
	}
	maxJobs := atoiDefault(c.Query("max_jobs"), 20)
	if c.Query("process") == "false" {
		maxJobs = -1
	}
	ctrl.run(c, c.Query("reconcile") == "true", maxJobs, atoiDefault(c.Query("budget_minutes"), 45))
}

// AdminSync godoc
//
//	@Summary		Run Zoom recording sync now (admin)
//	@Description	Same as the scheduler trigger, for an admin: reconciles against Zoom and processes up to max_jobs recordings within this request.
//	@Tags			zoom
//	@Produce		json
//	@Param			max_jobs		query	int	false	"Stop after this many jobs (default 5)"
//	@Param			budget_minutes	query	int	false	"Stop claiming new jobs after this many minutes (default 20)"
//	@Success		200	{object}	map[string]interface{}
//	@Security		BearerAuth
//	@Router			/recording-sync/run [post]
func (ctrl *RecordingSyncController) AdminSync(c *gin.Context) {
	ctrl.run(c, true, atoiDefault(c.Query("max_jobs"), 5), atoiDefault(c.Query("budget_minutes"), 20))
}

func (ctrl *RecordingSyncController) run(c *gin.Context, reconcile bool, maxJobs, budgetMin int) {
	ctx := c.Request.Context()
	resp := gin.H{}
	if reconcile {
		to := time.Now().AddDate(0, 0, 1)
		from := to.AddDate(0, 0, -ctrl.lookbackDays)
		sum, err := ctrl.syncSvc.Reconcile(ctx, from, to, "reconcile")
		if err != nil {
			resp["reconcile_error"] = err.Error()
		}
		resp["reconcile"] = sum
	}
	if maxJobs >= 0 { // -1 = reconcile / status only
		resp["process"] = ctrl.syncSvc.ProcessDue(ctx, recordingsync.Options{MaxJobs: maxJobs, Budget: time.Duration(budgetMin) * time.Minute})
	}
	if counts, err := ctrl.jobRepo.CountByStatus(ctx); err == nil {
		resp["queue"] = counts
	}
	c.JSON(http.StatusOK, resp)
}

// ListJobs godoc
//
//	@Summary		List Zoom recording sync jobs
//	@Description	Every recorded Zoom instance the LMS knows about and what happened to it (completed / pending / failed with last_error / unmatched / skipped / gone). Filter with status.
//	@Tags			zoom
//	@Produce		json
//	@Param			status	query	string	false	"pending | processing | completed | skipped | unmatched | failed | gone"
//	@Param			limit	query	int		false	"Max rows (default 200)"
//	@Success		200	{object}	map[string]interface{}
//	@Security		BearerAuth
//	@Router			/recording-sync/jobs [get]
func (ctrl *RecordingSyncController) ListJobs(c *gin.Context) {
	jobs, err := ctrl.jobRepo.List(c.Request.Context(), c.Query("status"), atoiDefault(c.Query("limit"), 200))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list recording jobs"})
		return
	}
	counts, _ := ctrl.jobRepo.CountByStatus(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"counts": counts, "jobs": jobs})
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
