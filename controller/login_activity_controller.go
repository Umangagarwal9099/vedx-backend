package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type LoginActivityController struct {
	repo *repository.LoginActivityRepository
}

func NewLoginActivityController(repo *repository.LoginActivityRepository) *LoginActivityController {
	return &LoginActivityController{repo: repo}
}

// GetForUser godoc
//
//	@Summary		Get a user's login/device activity
//	@Description	Returns total logins, registered/removed device counts, and the device list. Restricted to super_admin/team_lead/mentor.
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string	true	"User ID (UUID)"
//	@Success		200	{object}	models.LoginActivitySummary
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/login-activity [get]
func (ctrl *LoginActivityController) GetForUser(c *gin.Context) {
	id := c.Param("id")

	devices, err := ctrl.repo.FindAllForUser(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch login activity"})
		return
	}
	if devices == nil {
		devices = []models.LoginActivityDevice{}
	}

	summary := models.LoginActivitySummary{Devices: devices}
	for _, d := range devices {
		summary.TotalLogins += d.LoginCount
		if d.RemovedAt != nil {
			summary.RemovedDeviceCount++
		} else {
			summary.RegisteredDeviceCount++
		}
	}
	c.JSON(http.StatusOK, summary)
}

// RemoveDevice godoc
//
//	@Summary		Remove a device from a user's login activity
//	@Description	Soft-removes a device — it stops counting as a registered device, but its history is retained. Restricted to super_admin/team_lead.
//	@Tags			users
//	@Produce		json
//	@Param			id			path	string	true	"User ID (UUID)"
//	@Param			deviceId	path	string	true	"Device ID"
//	@Success		204			"No Content"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/login-activity/{deviceId} [delete]
func (ctrl *LoginActivityController) RemoveDevice(c *gin.Context) {
	id := c.Param("id")
	deviceID := c.Param("deviceId")

	if err := ctrl.repo.RemoveDevice(c.Request.Context(), id, deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove device"})
		return
	}
	c.Status(http.StatusNoContent)
}
