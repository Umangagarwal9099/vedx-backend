package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type DevicePushTokenController struct {
	repo *repository.DevicePushTokenRepository
}

func NewDevicePushTokenController(repo *repository.DevicePushTokenRepository) *DevicePushTokenController {
	return &DevicePushTokenController{repo: repo}
}

// RegisterToken godoc
//
//	@Summary		Register a mobile push token
//	@Description	Registers (or re-registers) this device's Expo push token against the logged-in user, so they receive push notifications from the mobile app.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.RegisterPushTokenInput	true	"Push token"
//	@Success		204		"No Content"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Security		BearerAuth
//	@Router			/notifications/push-token [post]
func (ctrl *DevicePushTokenController) RegisterToken(c *gin.Context) {
	var input models.RegisterPushTokenInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.repo.Upsert(c.Request.Context(), c.GetString("user_id"), input.Token, input.Platform); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not register push token"})
		return
	}
	c.Status(http.StatusNoContent)
}

// UnregisterToken godoc
//
//	@Summary		Unregister a mobile push token
//	@Description	Removes a device's push token (e.g. on logout) so it stops receiving pushes.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.UnregisterPushTokenInput	true	"Push token"
//	@Success		204		"No Content"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Security		BearerAuth
//	@Router			/notifications/push-token [delete]
func (ctrl *DevicePushTokenController) UnregisterToken(c *gin.Context) {
	var input models.UnregisterPushTokenInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.repo.Delete(c.Request.Context(), input.Token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not unregister push token"})
		return
	}
	c.Status(http.StatusNoContent)
}
