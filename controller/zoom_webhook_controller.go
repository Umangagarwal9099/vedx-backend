package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type ZoomWebhookController struct {
	zoomSvc     *service.ZoomService
	storageSvc  *service.StorageService
	sessionRepo *repository.SessionRepository
	timezone    string
}

func NewZoomWebhookController(zoomSvc *service.ZoomService, storageSvc *service.StorageService, sessionRepo *repository.SessionRepository, timezone string) *ZoomWebhookController {
	return &ZoomWebhookController{zoomSvc: zoomSvc, storageSvc: storageSvc, sessionRepo: sessionRepo, timezone: timezone}
}

type zoomWebhookEnvelope struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	// DownloadToken authorizes downloading this meeting's recording files —
	// sent by Zoom only alongside recording.completed events.
	DownloadToken string `json:"download_token"`
}

type zoomURLValidationPayload struct {
	PlainToken string `json:"plainToken"`
}

// zoomRecordingFile is one entry in recording.completed's recording_files —
// Zoom sends a separate file per view/format (combined gallery view, audio
// only, transcript, chat, etc.).
type zoomRecordingFile struct {
	ID            string `json:"id"`
	FileType      string `json:"file_type"`
	RecordingType string `json:"recording_type"`
	DownloadURL   string `json:"download_url"`
}

// zoomRecordingCompletedPayload is the subset of the recording.completed
// payload.object we care about.
type zoomRecordingCompletedPayload struct {
	Object struct {
		ID             int64               `json:"id"`
		StartTime      string              `json:"start_time"`
		RecordingFiles []zoomRecordingFile `json:"recording_files"`
	} `json:"object"`
}

// pickPrimaryRecordingFile returns the combined screen+speaker view Zoom
// records by default, falling back to any other MP4 file if that view isn't
// present. Audio-only, transcript and chat files are never selected.
func pickPrimaryRecordingFile(files []zoomRecordingFile) *zoomRecordingFile {
	for i := range files {
		if files[i].RecordingType == "shared_screen_with_speaker_view" {
			return &files[i]
		}
	}
	for i := range files {
		if files[i].FileType == "MP4" {
			return &files[i]
		}
	}
	return nil
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
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			log.Printf("zoom webhook: invalid recording.completed payload: %v", err)
		} else if file := pickPrimaryRecordingFile(payload.Object.RecordingFiles); file == nil {
			log.Printf("zoom webhook: no video file in recording.completed for meeting %d", payload.Object.ID)
		} else {
			// Run in the background: Zoom expects a fast 200 OK here, and a
			// full lecture recording can take well over a minute to download
			// and re-upload.
			meetingID := payload.Object.ID
			downloadToken := envelope.DownloadToken
			go ctrl.processRecording(meetingID, *file, downloadToken, payload.Object.StartTime)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// formatRecordingTimestamp converts Zoom's UTC start_time into the app's
// configured timezone for display, replacing Zoom's own fixed-size "add a
// timestamp to the recording" feature with one we control the size of.
// Returns "" (skipping the overlay) if start_time is missing or unparseable.
func (ctrl *ZoomWebhookController) formatRecordingTimestamp(startTime string) string {
	if startTime == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return ""
	}
	loc, err := time.LoadLocation(ctrl.timezone)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2 Jan 2006, 3:04 PM")
}

// processRecording downloads a completed Zoom cloud recording, burns a
// watermark into it, and re-uploads the result to our own storage, then
// points the session's recording_url at it instead of Zoom's share link.
// Watermarking happens here rather than relying on Zoom's own recording
// watermark feature because that only renders inside Zoom's own web player —
// once we re-host the raw file and play it back in our own frontend player,
// Zoom's overlay never applies.
func (ctrl *ZoomWebhookController) processRecording(meetingID int64, file zoomRecordingFile, downloadToken string, startTime string) {
	if already, err := ctrl.sessionRepo.HasRecording(context.Background(), meetingID); err != nil {
		log.Printf("zoom webhook: check existing recording for meeting %d: %v", meetingID, err)
		return
	} else if already {
		log.Printf("zoom webhook: meeting %d already has a recording attached, ignoring additional recording.completed event (file %s)", meetingID, file.ID)
		return
	}

	body, contentType, err := ctrl.zoomSvc.DownloadRecording(file.DownloadURL, downloadToken)
	if err != nil {
		log.Printf("zoom webhook: download recording for meeting %d: %v", meetingID, err)
		return
	}
	defer body.Close()

	watermarked, err := service.WatermarkRecording(context.Background(), body, service.DefaultWatermarkText, ctrl.formatRecordingTimestamp(startTime))
	if err != nil {
		log.Printf("zoom webhook: watermark recording for meeting %d: %v", meetingID, err)
		return
	}
	defer watermarked.Close()

	key := fmt.Sprintf("%d/%s.mp4", meetingID, file.ID)
	url, err := ctrl.storageSvc.UploadRecording(context.Background(), watermarked, key, contentType)
	if err != nil {
		log.Printf("zoom webhook: upload recording for meeting %d: %v", meetingID, err)
		return
	}

	wrote, err := ctrl.sessionRepo.UpdateRecordingURL(context.Background(), meetingID, url)
	if err != nil {
		log.Printf("zoom webhook: store recording url for meeting %d: %v", meetingID, err)
	} else if !wrote {
		// Lost a race against another recording.completed event processed
		// between the HasRecording check above and this write.
		log.Printf("zoom webhook: meeting %d gained a recording while this one was processing, discarding file %s", meetingID, file.ID)
	}
}
