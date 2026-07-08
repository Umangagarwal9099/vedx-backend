package controller

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/service"
)

type ZoomWebhookController struct {
	zoomSvc *service.ZoomService
}

func NewZoomWebhookController(zoomSvc *service.ZoomService) *ZoomWebhookController {
	return &ZoomWebhookController{zoomSvc: zoomSvc}
}

type zoomWebhookEnvelope struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

type zoomURLValidationPayload struct {
	PlainToken string `json:"plainToken"`
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

	// No session/recording state is persisted from these yet — logged so delivery
	// can be confirmed end-to-end; extend per event type as features need them.
	log.Printf("zoom webhook event received: %s", envelope.Event)

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}
