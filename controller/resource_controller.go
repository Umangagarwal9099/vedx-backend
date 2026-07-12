package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type ResourceController struct {
	resourceRepo *repository.ResourceRepository
}

func NewResourceController(resourceRepo *repository.ResourceRepository) *ResourceController {
	return &ResourceController{resourceRepo: resourceRepo}
}

// CreateResource godoc
//
//	@Summary		Create resource
//	@Description	Publish a new learning resource. Upload a file first via POST /upload/resource-file and pass the returned URL, or pass an external link directly. Leave batch_short_id empty for global visibility (all students), or set it to scope to one batch. Restricted to super_admin / team_lead / mentor.
//	@Tags			resources
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateResourceInput	true	"Resource details"
//	@Success		201		{object}	models.Resource
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/resources [post]
func (ctrl *ResourceController) Create(c *gin.Context) {
	var input models.CreateResourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := ctrl.resourceRepo.Create(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create resource: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, res)
}

// GetAllResources godoc
//
//	@Summary		List resources
//	@Description	Returns resources scoped to the caller's role — students see global resources plus those for their enrolled batches (expired resources excluded); mentors see global resources plus those for batches they manage; team_lead/super_admin see everything. Supports optional filtering by batch_short_id and resource_type.
//	@Tags			resources
//	@Produce		json
//	@Param			batch_short_id	query	string	false	"Filter by batch short ID"
//	@Param			resource_type	query	string	false	"Filter by type: pdf | document | video | link | image | code | other"
//	@Success		200	{array}		models.Resource
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/resources [get]
func (ctrl *ResourceController) GetAll(c *gin.Context) {
	var filter models.ResourceFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := c.GetString("role")
	userID := c.GetString("user_id")

	var resources []models.Resource
	var err error

	switch role {
	case string(models.RoleStudent):
		resources, err = ctrl.resourceRepo.FindAllForStudent(c.Request.Context(), userID)
	case string(models.RoleMentor), string(models.RoleEmployee):
		resources, err = ctrl.resourceRepo.FindAllForMentor(c.Request.Context(), userID)
	default:
		resources, err = ctrl.resourceRepo.FindAll(c.Request.Context(), filter)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch resources"})
		return
	}
	if resources == nil {
		resources = []models.Resource{}
	}
	c.JSON(http.StatusOK, resources)
}

// GetResource godoc
//
//	@Summary		Get resource
//	@Description	Returns a single resource by its short_id.
//	@Tags			resources
//	@Produce		json
//	@Param			short_id	path		string	true	"Resource short ID"
//	@Success		200			{object}	models.Resource
//	@Failure		404			{object}	map[string]string	"Resource not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/resources/{short_id} [get]
func (ctrl *ResourceController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")
	res, err := ctrl.resourceRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch resource"})
		return
	}
	if res == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}
	c.JSON(http.StatusOK, res)
}

// UpdateResource godoc
//
//	@Summary		Update resource
//	@Description	Partially update a resource. All fields are optional. Send an empty string for batch_short_id/module_short_id/session_short_id to clear that scoping. Restricted to super_admin / team_lead / mentor.
//	@Tags			resources
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Resource short ID"
//	@Param			body		body		models.UpdateResourceInput	true	"Fields to update"
//	@Success		200			{object}	models.Resource
//	@Failure		400			{object}	map[string]string	"Validation error or no fields provided"
//	@Failure		404			{object}	map[string]string	"Resource not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/resources/{short_id} [patch]
func (ctrl *ResourceController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateResourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.resourceRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update resource"})
		return
	}

	res, err := ctrl.resourceRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || res == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated resource"})
		return
	}
	c.JSON(http.StatusOK, res)
}

// DeleteResource godoc
//
//	@Summary		Delete resource
//	@Description	Soft-deletes a resource by its short ID. Restricted to super_admin / team_lead / mentor.
//	@Tags			resources
//	@Produce		json
//	@Param			short_id	path	string	true	"Resource short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Resource not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/resources/{short_id} [delete]
func (ctrl *ResourceController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")
	if err := ctrl.resourceRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete resource"})
		return
	}
	c.Status(http.StatusNoContent)
}
