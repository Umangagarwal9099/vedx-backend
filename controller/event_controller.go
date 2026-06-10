package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type EventController struct {
	eventRepo *repository.EventRepository
}

func NewEventController(eventRepo *repository.EventRepository) *EventController {
	return &EventController{eventRepo: eventRepo}
}

// CreateEvent godoc
//
//	@Summary		Create event
//	@Description	Create a new event. The image_url should be the URL returned after uploading to storage. Description supports rich text (HTML/Markdown).
//	@Tags			events
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateEventInput	true	"Event details"
//	@Success		201		{object}	models.Event
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/events [post]
func (ctrl *EventController) Create(c *gin.Context) {
	var input models.CreateEventInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	event, err := ctrl.eventRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create event"})
		return
	}

	c.JSON(http.StatusCreated, event)
}

// GetAllEvents godoc
//
//	@Summary		List events
//	@Description	Returns all non-deleted events ordered newest first. Use query params `name` and `status` to filter results.
//	@Tags			events
//	@Produce		json
//	@Param			name	query		string	false	"Filter by event name (case-insensitive, partial match)"
//	@Param			status	query		string	false	"Filter by status: published | unpublished"
//	@Success		200		{array}		models.Event
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/events [get]
func (ctrl *EventController) GetAll(c *gin.Context) {
	var filter models.EventFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		events []models.Event
		err    error
	)

	if filter.Name != "" || filter.Status != "" {
		events, err = ctrl.eventRepo.Search(c.Request.Context(), filter)
	} else {
		events, err = ctrl.eventRepo.FindAll(c.Request.Context())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch events"})
		return
	}
	if events == nil {
		events = []models.Event{}
	}
	c.JSON(http.StatusOK, events)
}

// UpdateEvent godoc
//
//	@Summary		Update event
//	@Description	Partially update an event by its short_id. Send only the fields you want to change.
//	@Tags			events
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Event short ID"
//	@Param			body		body		models.UpdateEventInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Event
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Event not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/events/{short_id} [patch]
func (ctrl *EventController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateEventInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.eventRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update event"})
		return
	}

	event, err := ctrl.eventRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || event == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated event"})
		return
	}

	c.JSON(http.StatusOK, event)
}

// DeleteEvent godoc
//
//	@Summary		Delete event
//	@Description	Soft-delete an event by its short_id (sets deleted_at; the row is retained).
//	@Tags			events
//	@Produce		json
//	@Param			short_id	path	string	true	"Event short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Event not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/events/{short_id} [delete]
func (ctrl *EventController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.eventRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete event"})
		return
	}

	c.Status(http.StatusNoContent)
}
