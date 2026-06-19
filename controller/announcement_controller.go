package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type AnnouncementController struct {
	announcementRepo *repository.AnnouncementRepository
}

func NewAnnouncementController(repo *repository.AnnouncementRepository) *AnnouncementController {
	return &AnnouncementController{announcementRepo: repo}
}

// CreateAnnouncement godoc
//
//	@Summary		Create announcement
//	@Description	Create a new announcement. Description supports rich text (HTML/Markdown). Use POST /upload/image first to get the image_url.
//	@Tags			announcements
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateAnnouncementInput	true	"Announcement details"
//	@Success		201		{object}	models.Announcement
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/announcements [post]
func (ctrl *AnnouncementController) Create(c *gin.Context) {
	var input models.CreateAnnouncementInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	announcement, err := ctrl.announcementRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create announcement"})
		return
	}

	c.JSON(http.StatusCreated, announcement)
}

// GetAllAnnouncements godoc
//
//	@Summary		List announcements
//	@Description	Returns all non-deleted announcements. Use query params to filter: name (partial match), urgency (low|medium|high), is_active (true|false).
//	@Tags			announcements
//	@Produce		json
//	@Param			name		query		string	false	"Filter by name (case-insensitive, partial match)"
//	@Param			urgency		query		string	false	"Filter by urgency: low | medium | high"
//	@Param			is_active	query		string	false	"Filter by status: true | false"
//	@Success		200			{array}		models.Announcement
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/announcements [get]
func (ctrl *AnnouncementController) GetAll(c *gin.Context) {
	var filter models.AnnouncementFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		announcements []models.Announcement
		err           error
	)

	if filter.Name != "" || filter.Urgency != "" || filter.IsActive != "" {
		announcements, err = ctrl.announcementRepo.Search(c.Request.Context(), filter)
	} else {
		announcements, err = ctrl.announcementRepo.FindAll(c.Request.Context())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch announcements"})
		return
	}
	if announcements == nil {
		announcements = []models.Announcement{}
	}
	c.JSON(http.StatusOK, announcements)
}

// UpdateAnnouncement godoc
//
//	@Summary		Update announcement
//	@Description	Partially update an announcement by its short_id. Send only the fields you want to change.
//	@Tags			announcements
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Announcement short ID"
//	@Param			body		body		models.UpdateAnnouncementInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Announcement
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Announcement not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/announcements/{short_id} [patch]
func (ctrl *AnnouncementController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateAnnouncementInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.announcementRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update announcement"})
		return
	}

	announcement, err := ctrl.announcementRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || announcement == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated announcement"})
		return
	}

	c.JSON(http.StatusOK, announcement)
}

// DeleteAnnouncement godoc
//
//	@Summary		Delete announcement
//	@Description	Soft-delete an announcement by its short_id (sets deleted_at; the row is retained).
//	@Tags			announcements
//	@Produce		json
//	@Param			short_id	path	string	true	"Announcement short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Announcement not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/announcements/{short_id} [delete]
func (ctrl *AnnouncementController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.announcementRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "announcement not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete announcement"})
		return
	}

	c.Status(http.StatusNoContent)
}
