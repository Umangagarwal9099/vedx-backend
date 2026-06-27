package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type BannerController struct {
	bannerRepo *repository.BannerRepository
}

func NewBannerController(bannerRepo *repository.BannerRepository) *BannerController {
	return &BannerController{bannerRepo: bannerRepo}
}

// CreateBanner godoc
//
//	@Summary		Create banner
//	@Description	Create a new banner. Upload the thumbnail image via POST /upload/banner-image first and pass the returned URL in the thumbnail field. branches must be a non-empty array of branch names.
//	@Tags			banners
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateBannerInput	true	"Banner details"
//	@Success		201		{object}	models.Banner
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/banners [post]
func (ctrl *BannerController) Create(c *gin.Context) {
	var input models.CreateBannerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	banner, err := ctrl.bannerRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create banner"})
		return
	}

	c.JSON(http.StatusCreated, banner)
}

// GetAllBanners godoc
//
//	@Summary		List banners
//	@Description	Returns all non-deleted banners ordered newest first. Filter by name (ILIKE, partial match) or exact category.
//	@Tags			banners
//	@Produce		json
//	@Param			name		query		string	false	"Filter by name (case-insensitive, partial match)"
//	@Param			category	query		string	false	"Filter by exact category"
//	@Success		200			{array}		models.Banner
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/banners [get]
func (ctrl *BannerController) GetAll(c *gin.Context) {
	var filter models.BannerFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		banners []models.Banner
		err     error
	)

	if filter.Name != "" || filter.Category != "" {
		banners, err = ctrl.bannerRepo.Search(c.Request.Context(), filter)
	} else {
		banners, err = ctrl.bannerRepo.FindAll(c.Request.Context())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch banners"})
		return
	}
	if banners == nil {
		banners = []models.Banner{}
	}
	c.JSON(http.StatusOK, banners)
}

// UpdateBanner godoc
//
//	@Summary		Update banner
//	@Description	Partially update a banner by its short_id. Send only the fields you want to change. Use is_active to toggle the banner without deleting it. To replace the thumbnail, upload a new image via POST /upload/banner-image and pass the returned URL.
//	@Tags			banners
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Banner short ID"
//	@Param			body		body		models.UpdateBannerInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Banner
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Banner not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/banners/{short_id} [patch]
func (ctrl *BannerController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateBannerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.bannerRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "banner not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update banner"})
		return
	}

	banner, err := ctrl.bannerRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || banner == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated banner"})
		return
	}

	c.JSON(http.StatusOK, banner)
}

// DeleteBanner godoc
//
//	@Summary		Delete banner
//	@Description	Soft-delete a banner by its short_id (sets deleted_at; the row is retained in the database).
//	@Tags			banners
//	@Produce		json
//	@Param			short_id	path	string	true	"Banner short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Banner not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/banners/{short_id} [delete]
func (ctrl *BannerController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.bannerRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "banner not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete banner"})
		return
	}

	c.Status(http.StatusNoContent)
}
