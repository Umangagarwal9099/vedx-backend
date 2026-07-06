package controller

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type NotificationController struct {
	notificationRepo *repository.NotificationRepository
}

func NewNotificationController(notificationRepo *repository.NotificationRepository) *NotificationController {
	return &NotificationController{notificationRepo: notificationRepo}
}

// CreateNotification godoc
//
//	@Summary		Create notification
//	@Description	Manually broadcast a notification to one or more roles. Use role "all" to target every active user (the caller is never notified about their own action). Restricted to super_admin / team_lead.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateNotificationInput	true	"Notification details"
//	@Success		201		{object}	models.Notification
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/notifications [post]
func (ctrl *NotificationController) Create(c *gin.Context) {
	var input models.CreateNotificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	notif, err := ctrl.notificationRepo.Create(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid role") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create notification: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, notif)
}

// GetInbox godoc
//
//	@Summary		List my notifications
//	@Description	Returns every notification addressed to the logged-in user, newest first, each with an is_read flag.
//	@Tags			notifications
//	@Produce		json
//	@Success		200	{array}		models.NotificationView
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/notifications [get]
func (ctrl *NotificationController) GetInbox(c *gin.Context) {
	notifications, err := ctrl.notificationRepo.GetInbox(c.Request.Context(), c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch notifications"})
		return
	}
	if notifications == nil {
		notifications = []models.NotificationView{}
	}
	c.JSON(http.StatusOK, notifications)
}

// UpdateNotification godoc
//
//	@Summary		Update notification
//	@Description	Edit the title/message of a notification by its short_id. This changes the content seen by every recipient. Restricted to super_admin / team_lead.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Notification short ID"
//	@Param			body		body		models.UpdateNotificationInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Notification
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Notification not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/notifications/{short_id} [patch]
func (ctrl *NotificationController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateNotificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.notificationRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update notification"})
		return
	}

	notif, err := ctrl.notificationRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || notif == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated notification"})
		return
	}

	c.JSON(http.StatusOK, notif)
}

// MarkNotificationRead godoc
//
//	@Summary		Mark notification read/unread
//	@Description	Marks a notification read (default) or unread for the logged-in recipient only. Body is optional — omit it to mark read.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path	string					false	"Notification short ID"
//	@Param			body		body	models.MarkReadInput	false	"Optional — defaults to is_read: true"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Notification not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/notifications/{short_id}/read [patch]
func (ctrl *NotificationController) MarkRead(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.MarkReadInput
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isRead := true
	if input.IsRead != nil {
		isRead = *input.IsRead
	}

	if err := ctrl.notificationRepo.MarkRead(c.Request.Context(), shortID, c.GetString("user_id"), isRead); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update notification"})
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteNotification godoc
//
//	@Summary		Delete notification
//	@Description	Soft-delete a notification by its short_id. Removes it from every recipient's inbox. Restricted to super_admin / team_lead.
//	@Tags			notifications
//	@Produce		json
//	@Param			short_id	path	string	true	"Notification short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Notification not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/notifications/{short_id} [delete]
func (ctrl *NotificationController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.notificationRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete notification"})
		return
	}

	c.Status(http.StatusNoContent)
}
