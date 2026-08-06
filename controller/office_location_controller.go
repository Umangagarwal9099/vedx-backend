package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

type OfficeLocationController struct {
	officeLocationRepo *repository.OfficeLocationRepository
}

func NewOfficeLocationController(officeLocationRepo *repository.OfficeLocationRepository) *OfficeLocationController {
	return &OfficeLocationController{officeLocationRepo: officeLocationRepo}
}

// Create godoc
//
//	@Summary		Create an office location
//	@Description	Registers a physical office location that employee check-in/out is geofenced against. Restricted to super_admin/team_lead.
//	@Tags			office-locations
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateOfficeLocationInput	true	"Office location details"
//	@Success		201		{object}	models.OfficeLocation
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/office-locations [post]
func (ctrl *OfficeLocationController) Create(c *gin.Context) {
	var input models.CreateOfficeLocationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	loc, err := ctrl.officeLocationRepo.Create(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create office location: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, loc)
}

// GetAll godoc
//
//	@Summary		List office locations
//	@Description	Returns every office location, active or not. Restricted to super_admin/team_lead.
//	@Tags			office-locations
//	@Produce		json
//	@Success		200	{array}		models.OfficeLocation
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/office-locations [get]
func (ctrl *OfficeLocationController) GetAll(c *gin.Context) {
	locs, err := ctrl.officeLocationRepo.GetAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch office locations"})
		return
	}
	if locs == nil {
		locs = []models.OfficeLocation{}
	}
	c.JSON(http.StatusOK, locs)
}

// Update godoc
//
//	@Summary		Update an office location
//	@Description	Applies a partial update to an office location. Restricted to super_admin/team_lead.
//	@Tags			office-locations
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string								true	"Office location short ID"
//	@Param			body		body		models.UpdateOfficeLocationInput	true	"Fields to update"
//	@Success		200			{object}	models.OfficeLocation
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Office location not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/office-locations/{short_id} [patch]
func (ctrl *OfficeLocationController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateOfficeLocationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	loc, err := ctrl.officeLocationRepo.Update(c.Request.Context(), shortID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "office location not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update office location: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, loc)
}

// ResolveLink godoc
//
//	@Summary		Resolve a Google Maps link to coordinates
//	@Description	Follows a pasted Google Maps URL (short share link or full URL) and extracts the latitude/longitude it encodes, plus a place name when derivable. Restricted to super_admin/team_lead.
//	@Tags			office-locations
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.ResolveMapsLinkInput	true	"Google Maps URL"
//	@Success		200		{object}	models.ResolvedMapsLocation
//	@Failure		400		{object}	map[string]string	"Not a Google Maps link, or validation error"
//	@Failure		422		{object}	map[string]string	"No coordinates found in that link"
//	@Failure		502		{object}	map[string]string	"Could not reach Google Maps"
//	@Security		BearerAuth
//	@Router			/office-locations/resolve-link [post]
func (ctrl *OfficeLocationController) ResolveLink(c *gin.Context) {
	var input models.ResolveMapsLinkInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	loc, err := service.ResolveGoogleMapsLink(c.Request.Context(), input.URL)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotGoogleMapsLink):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrNoCoordinatesFound):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not reach Google Maps: " + err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, loc)
}

// Delete godoc
//
//	@Summary		Delete an office location
//	@Description	Permanently removes an office location. Restricted to super_admin/team_lead.
//	@Tags			office-locations
//	@Param			short_id	path	string	true	"Office location short ID"
//	@Success		204
//	@Failure		404	{object}	map[string]string	"Office location not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/office-locations/{short_id} [delete]
func (ctrl *OfficeLocationController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.officeLocationRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "office location not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete office location"})
		return
	}
	c.Status(http.StatusNoContent)
}
