package controller

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/recordingsync"
	"github.com/umangagarwal/vedx-backend/service"
)

type ZoomWebhookController struct {
	zoomSvc *service.ZoomService
	syncSvc *recordingsync.Service
	// nudge starts processing right after enqueue. Off on Cloud Run, where a
	// goroutine outliving the request gets throttled CPU (the original bug);
	// there the Cloud Run Job drains the queue instead.
	nudge bool
}

func NewZoomWebhookController(zoomSvc *service.ZoomService, syncSvc *recordingsync.Service, nudge bool) *ZoomWebhookController {
	return &ZoomWebhookController{zoomSvc: zoomSvc, syncSvc: syncSvc, nudge: nudge}
}

type zoomWebhookEnvelope struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

type zoomURLValidationPayload struct {
	PlainToken string `json:"plainToken"`
}

// zoomRecordingCompletedPayload is recording.completed's payload — its
// object has the same shape as one entry of GET /users/{id}/recordings.
type zoomRecordingCompletedPayload struct {
	Object service.ZoomRecordingInstance `json:"object"`
}

// HandleWebhook godoc
//
//	@Summary		Zoom event subscription webhook
//	@Description	Receives Zoom Event Subscription callbacks. Answers Zoom's endpoint.url_validation handshake and verifies the x-zm-signature on all other events before processing them. Not intended to be called directly — configure this URL in the Zoom Marketplace app's Event Subscriptions.
//	@Tags			zoom
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Failure		401	{object}	map[string]string	"Invalid signature"
//	@Router			/zoom/webhook [post]
func (ctrl *ZoomWebhookController) HandleWebhook(c *gin.Context) {
	if !ctrl.zoomSvc.WebhookConfigured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "zoom webhook secret token not configured"})
		return
	}

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read request body"})
		return
	}

	var envelope zoomWebhookEnvelope
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// The URL-validation handshake happens before Zoom signs anything, so it's
	// answered without a signature check — the round-trip HMAC itself is the proof.
	if envelope.Event == "endpoint.url_validation" {
		var payload zoomURLValidationPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url_validation payload"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"plainToken":     payload.PlainToken,
			"encryptedToken": ctrl.zoomSvc.ValidateWebhookURL(payload.PlainToken),
		})
		return
	}

	signature := c.GetHeader("x-zm-signature")
	timestamp := c.GetHeader("x-zm-request-timestamp")
	if signature == "" || !ctrl.zoomSvc.VerifyWebhookSignature(signature, timestamp, rawBody) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	log.Printf("zoom webhook event received: %s", envelope.Event)

	if envelope.Event == "recording.completed" {
		var payload zoomRecordingCompletedPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil || payload.Object.UUID == "" {
			// Malformed but authentic: a 400 would only make Zoom retry the
			// same body. Reconciliation will still pick the recording up.
			log.Printf("zoom webhook: unusable recording.completed payload (err=%v): %.500s", err, string(envelope.Payload))
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}
		// Only persist here — Zoom wants a reply within 3 seconds, and the
		// download/watermark/upload work must not run detached from a
		// request on Cloud Run (CPU is throttled once the response is sent,
		// which is what silently killed recordings from 2026-08-10 on).
		// The durable job is processed by the recording-sync worker.
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		created, err := ctrl.syncSvc.EnqueueInstance(ctx, &payload.Object, "webhook")
		if err != nil {
			// Non-2xx makes Zoom redeliver; reconciliation is the backstop.
			log.Printf("zoom webhook: ERROR enqueue recording meeting=%d uuid=%s: %v", payload.Object.ID, payload.Object.UUID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not queue recording"})
			return
		}
		log.Printf("zoom webhook: recording.completed meeting=%d uuid=%s duration=%dmin files=%d queued=%t",
			payload.Object.ID, payload.Object.UUID, payload.Object.Duration, len(payload.Object.RecordingFiles), created)
		if created && ctrl.nudge {
			// Opportunistic nudge — best effort only. If this process is
			// throttled or recycled the job simply waits for the scheduler
			// or the /internal/recordings/sync trigger.
			go ctrl.syncSvc.ProcessDue(context.Background(), recordingsync.Options{MaxJobs: 1})
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}
