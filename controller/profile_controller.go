package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type ProfileController struct {
	profileRepo *repository.ProfileRepository
	userRepo    *repository.UserRepository
}

func NewProfileController(profileRepo *repository.ProfileRepository, userRepo *repository.UserRepository) *ProfileController {
	return &ProfileController{profileRepo: profileRepo, userRepo: userRepo}
}

// GetDetails godoc
//
//	@Summary		Get profile details
//	@Description	Returns the bio/location/education/skills/social-link fields for a user's profile page, plus their current phone. Self, or super_admin/team_lead.
//	@Tags			profile
//	@Produce		json
//	@Param			id	path		string	true	"User ID (UUID)"
//	@Success		200	{object}	models.ProfileDetails
//	@Failure		404	{object}	map[string]string	"User not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/profile-details [get]
func (ctrl *ProfileController) GetDetails(c *gin.Context) {
	id := c.Param("id")

	details, err := ctrl.profileRepo.GetByUserID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch profile details"})
		return
	}
	if details == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, details)
}

// UpdateDetails godoc
//
//	@Summary		Update profile details
//	@Description	Partially update a user's bio/location/education/skills/social links (and phone, which writes through to the users table). Self, or super_admin/team_lead.
//	@Tags			profile
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string								true	"User ID (UUID)"
//	@Param			body	body		models.UpdateProfileDetailsInput	true	"Fields to update (all optional)"
//	@Success		200		{object}	models.ProfileDetails
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/users/{id}/profile-details [patch]
func (ctrl *ProfileController) UpdateDetails(c *gin.Context) {
	id := c.Param("id")

	var input models.UpdateProfileDetailsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Phone != nil {
		if err := ctrl.userRepo.UpdateUser(c.Request.Context(), id, models.UpdateUserInput{Phone: input.Phone}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update phone"})
			return
		}
	}

	if err := ctrl.profileRepo.Upsert(c.Request.Context(), id, input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update profile details: " + err.Error()})
		return
	}

	details, err := ctrl.profileRepo.GetByUserID(c.Request.Context(), id)
	if err != nil || details == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated profile"})
		return
	}
	c.JSON(http.StatusOK, details)
}
