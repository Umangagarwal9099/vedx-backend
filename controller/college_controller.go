package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type CollegeController struct {
	collegeRepo  *repository.CollegeRepository
	auditLogRepo *repository.AuditLogRepository
}

func NewCollegeController(collegeRepo *repository.CollegeRepository, auditLogRepo *repository.AuditLogRepository) *CollegeController {
	return &CollegeController{collegeRepo: collegeRepo, auditLogRepo: auditLogRepo}
}

// CreateCollege godoc
//
//	@Summary		Create a college
//	@Description	Creates a college with an optional starting feature-toggle set. Super_admin only.
//	@Tags			colleges
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateCollegeInput	true	"College details"
//	@Success		201		{object}	models.College
//	@Failure		400		{object}	map[string]string	"Validation error / code already in use"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges [post]
func (ctrl *CollegeController) Create(c *gin.Context) {
	var input models.CreateCollegeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	college, err := ctrl.collegeRepo.Create(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "college_created", EntityType: "college", EntityID: college.ID, EntityShortID: college.ShortID, EntityLabel: college.Name,
	})
	c.JSON(http.StatusCreated, college)
}

// MyFeaturesResponse is the caller's own resolved feature-access — used by
// every frontend to decide which nav sections/routes to show, without
// needing the super_admin-only permission to read a college's full profile.
type MyFeaturesResponse struct {
	CollegeID   string          `json:"college_id,omitempty"`
	CollegeName string          `json:"college_name,omitempty"`
	Features    map[string]bool `json:"features"`
	Unscoped    bool            `json:"unscoped"` // true for super_admin / legacy users with no college_id — treated as full access
}

// GetMyFeatures godoc
//
//	@Summary		Get my resolved feature access
//	@Description	Returns the calling user's own college's enabled features — used to drive sidebar/nav visibility on every frontend. super_admin and users with no college_id get unscoped=true (full access, since they either manage every college or predate multi-tenancy).
//	@Tags			colleges
//	@Produce		json
//	@Success		200	{object}	MyFeaturesResponse
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges/me/features [get]
func (ctrl *CollegeController) GetMyFeatures(c *gin.Context) {
	if c.GetString("role") == string(models.RoleSuperAdmin) {
		c.JSON(http.StatusOK, MyFeaturesResponse{Unscoped: true, Features: map[string]bool{}})
		return
	}

	collegeID := c.GetString("college_id")
	if collegeID == "" {
		c.JSON(http.StatusOK, MyFeaturesResponse{Unscoped: true, Features: map[string]bool{}})
		return
	}

	college, err := ctrl.collegeRepo.FindByID(c.Request.Context(), collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not resolve college features"})
		return
	}
	if college == nil {
		c.JSON(http.StatusOK, MyFeaturesResponse{Unscoped: true, Features: map[string]bool{}})
		return
	}
	c.JSON(http.StatusOK, MyFeaturesResponse{CollegeID: college.ShortID, CollegeName: college.Name, Features: college.EnabledFeatures})
}

// GetAllColleges godoc
//
//	@Summary		List colleges
//	@Description	Returns every non-deleted college, newest first. Super_admin only.
//	@Tags			colleges
//	@Produce		json
//	@Success		200	{array}		models.College
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges [get]
func (ctrl *CollegeController) GetAll(c *gin.Context) {
	colleges, err := ctrl.collegeRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch colleges"})
		return
	}
	if colleges == nil {
		colleges = []models.College{}
	}
	c.JSON(http.StatusOK, colleges)
}

// GetCollege godoc
//
//	@Summary		Get college
//	@Description	Returns a single non-deleted college by its short_id. Super_admin only.
//	@Tags			colleges
//	@Produce		json
//	@Param			short_id	path		string	true	"College short ID"
//	@Success		200			{object}	models.College
//	@Failure		404			{object}	map[string]string	"College not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges/{short_id} [get]
func (ctrl *CollegeController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")

	college, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch college"})
		return
	}
	if college == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
		return
	}
	c.JSON(http.StatusOK, college)
}

// UpdateCollege godoc
//
//	@Summary		Update college
//	@Description	Partially update a college's profile fields. Super_admin only. Use PATCH /colleges/{short_id}/features to change feature toggles.
//	@Tags			colleges
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"College short ID"
//	@Param			body		body		models.UpdateCollegeInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.College
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"College not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges/{short_id} [patch]
func (ctrl *CollegeController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateCollegeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.collegeRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update college"})
		return
	}

	college, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || college == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated college"})
		return
	}
	action := "college_updated"
	if input.Status != nil {
		action = "college_status_changed"
	}
	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: action, EntityType: "college", EntityID: college.ID, EntityShortID: college.ShortID, EntityLabel: college.Name,
		Metadata: map[string]interface{}{"status": college.Status},
	})
	c.JSON(http.StatusOK, college)
}

// UpdateFeatures godoc
//
//	@Summary		Update a college's feature toggles
//	@Description	Merges the given feature-key/enabled pairs into the college's configuration — send only the keys you're changing. Super_admin only. Disabled features are hidden from that college's sidebar and blocked server-side via RequireFeature.
//	@Tags			colleges
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"College short ID"
//	@Param			body		body		models.UpdateFeaturesInput		true	"Feature toggles to change"
//	@Success		200			{object}	models.College
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"College not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges/{short_id}/features [patch]
func (ctrl *CollegeController) UpdateFeatures(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateFeaturesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.collegeRepo.UpdateFeatures(c.Request.Context(), shortID, input.Features, c.GetString("user_id")); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update features"})
		return
	}

	college, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || college == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated college"})
		return
	}
	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "college_features_updated", EntityType: "college", EntityID: college.ID, EntityShortID: college.ShortID, EntityLabel: college.Name,
		Metadata: map[string]interface{}{"features": input.Features},
	})
	c.JSON(http.StatusOK, college)
}

// DeleteCollege godoc
//
//	@Summary		Delete college
//	@Description	Soft-delete a college by its short_id. Super_admin only.
//	@Tags			colleges
//	@Produce		json
//	@Param			short_id	path	string	true	"College short ID"
//	@Success		204	"No Content"
//	@Failure		404	{object}	map[string]string	"College not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges/{short_id} [delete]
func (ctrl *CollegeController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.collegeRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete college"})
		return
	}
	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{Action: "college_deleted", EntityType: "college", EntityShortID: shortID})
	c.Status(http.StatusNoContent)
}

// CreateSubscriptionInput carries a new subscription period for a college.
type CreateSubscriptionInput struct {
	PlanID    string `json:"plan_id" example:"standard-annual"`
	StartDate string `json:"start_date" binding:"required" example:"2026-08-01"`
	EndDate   string `json:"end_date"   binding:"required" example:"2027-07-31"`
}

// CreateSubscription godoc
//
//	@Summary		Record a college subscription period
//	@Description	Creates a new subscription period for a college (a renewal creates a new row rather than editing the old one, preserving history). Super_admin only.
//	@Tags			colleges
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"College short ID"
//	@Param			body		body		CreateSubscriptionInput	true	"Subscription period"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"College not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/colleges/{short_id}/subscriptions [post]
func (ctrl *CollegeController) CreateSubscription(c *gin.Context) {
	shortID := c.Param("short_id")

	var input CreateSubscriptionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	college, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || college == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
		return
	}

	if err := ctrl.collegeRepo.CreateSubscription(c.Request.Context(), college.ID, input.PlanID, input.StartDate, input.EndDate, c.GetString("user_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create subscription: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "subscription_created", EntityType: "college", EntityID: college.ID, EntityShortID: college.ShortID, EntityLabel: college.Name,
		Metadata: map[string]interface{}{"plan_id": input.PlanID, "start_date": input.StartDate, "end_date": input.EndDate},
	})

	c.Status(http.StatusNoContent)
}
