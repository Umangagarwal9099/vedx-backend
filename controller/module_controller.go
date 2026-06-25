package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type ModuleController struct {
	moduleRepo *repository.ModuleRepository
}

func NewModuleController(moduleRepo *repository.ModuleRepository) *ModuleController {
	return &ModuleController{moduleRepo: moduleRepo}
}

// CreateModule godoc
//
//	@Summary		Create module
//	@Description	Create a new module. Set max_view_duration to "unlimited" or "restricted". When "restricted", watch_time_minutes is required and defines the maximum minutes a student may view content. watch_time_minutes is ignored when max_view_duration is "unlimited".
//	@Tags			modules
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateModuleInput	true	"Module details"
//	@Success		201		{object}	models.Module
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules [post]
func (ctrl *ModuleController) Create(c *gin.Context) {
	var input models.CreateModuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.MaxViewDuration == "restricted" && input.WatchTimeMinutes == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "watch_time_minutes is required when max_view_duration is restricted"})
		return
	}
	if input.MaxViewDuration == "unlimited" {
		input.WatchTimeMinutes = nil
	}

	module, err := ctrl.moduleRepo.Create(c.Request.Context(), input, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create module: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, module)
}

// GetAllModules godoc
//
//	@Summary		List modules
//	@Description	Returns all non-deleted modules ordered newest first.
//	@Tags			modules
//	@Produce		json
//	@Success		200	{array}		models.Module
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules [get]
func (ctrl *ModuleController) GetAll(c *gin.Context) {
	modules, err := ctrl.moduleRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch modules"})
		return
	}
	if modules == nil {
		modules = []models.Module{}
	}
	c.JSON(http.StatusOK, modules)
}

// FilterModules godoc
//
//	@Summary		Filter modules
//	@Description	Filter non-deleted modules by any combination of fields. All query params are optional.
//	@Tags			modules
//	@Produce		json
//	@Param			module_name			query		string	false	"Partial module name"
//	@Param			module_branch		query		string	false	"Partial branch name"
//	@Param			max_view_duration	query		string	false	"unlimited or restricted"
//	@Param			is_active			query		string	false	"true or false"
//	@Success		200	{array}		models.Module
//	@Failure		400	{object}	map[string]string	"Validation error"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/filter [get]
func (ctrl *ModuleController) Filter(c *gin.Context) {
	var f models.ModuleFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	modules, err := ctrl.moduleRepo.Filter(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not filter modules"})
		return
	}
	if modules == nil {
		modules = []models.Module{}
	}
	c.JSON(http.StatusOK, modules)
}

// UpdateModule godoc
//
//	@Summary		Update module
//	@Description	Partially update a module by its short_id. Send only the fields you want to change. Switching max_view_duration to "unlimited" automatically clears watch_time_minutes.
//	@Tags			modules
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Module short ID"
//	@Param			body		body		models.UpdateModuleInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Module
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Module not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id} [patch]
func (ctrl *ModuleController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateModuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.MaxViewDuration != nil && *input.MaxViewDuration == "restricted" && input.WatchTimeMinutes == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "watch_time_minutes is required when max_view_duration is restricted"})
		return
	}

	if err := ctrl.moduleRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update module"})
		return
	}

	module, err := ctrl.moduleRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || module == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated module"})
		return
	}

	c.JSON(http.StatusOK, module)
}

// DeleteModule godoc
//
//	@Summary		Delete module
//	@Description	Soft-delete a module by its short_id.
//	@Tags			modules
//	@Produce		json
//	@Param			short_id	path	string	true	"Module short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Module not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id} [delete]
func (ctrl *ModuleController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.moduleRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete module"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ── Sections ──────────────────────────────────────────────────────────────────

// AddSection godoc
//
//	@Summary		Add section to module
//	@Description	Add a new section to an existing module. Set is_prerequisite to true if students must complete this section before accessing the rest of the module.
//	@Tags			modules
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string							true	"Module short ID"
//	@Param			body		body		models.CreateModuleSectionInput	true	"Section details"
//	@Success		201			{object}	models.ModuleSection
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		403			{object}	map[string]string	"Forbidden"
//	@Failure		404			{object}	map[string]string	"Module not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections [post]
func (ctrl *ModuleController) AddSection(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.CreateModuleSectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	section, err := ctrl.moduleRepo.AddSection(c.Request.Context(), shortID, input, c.GetString("user_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add section"})
		return
	}
	c.JSON(http.StatusCreated, section)
}

// GetSections godoc
//
//	@Summary		Get sections of a module
//	@Description	Returns all non-deleted sections for the given module ordered by creation time.
//	@Tags			modules
//	@Produce		json
//	@Param			short_id	path		string	true	"Module short ID"
//	@Success		200			{array}		models.ModuleSection
//	@Failure		404			{object}	map[string]string	"Module not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections [get]
func (ctrl *ModuleController) GetSections(c *gin.Context) {
	shortID := c.Param("short_id")

	sections, err := ctrl.moduleRepo.FindSections(c.Request.Context(), shortID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "module not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch sections"})
		return
	}
	c.JSON(http.StatusOK, sections)
}

// UpdateSection godoc
//
//	@Summary		Update a section
//	@Description	Partially update a section within a module. Send only the fields you want to change.
//	@Tags			modules
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path		string							true	"Module short ID"
//	@Param			section_short_id	path		string							true	"Section short ID"
//	@Param			body				body		models.UpdateModuleSectionInput	true	"Fields to update (all optional)"
//	@Success		200					{object}	models.ModuleSection
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		403					{object}	map[string]string	"Forbidden"
//	@Failure		404					{object}	map[string]string	"Module or section not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections/{section_short_id} [patch]
func (ctrl *ModuleController) UpdateSection(c *gin.Context) {
	shortID := c.Param("short_id")
	sectionShortID := c.Param("section_short_id")

	var input models.UpdateModuleSectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	section, err := ctrl.moduleRepo.UpdateSection(c.Request.Context(), shortID, sectionShortID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "module or section not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update section"})
		return
	}
	c.JSON(http.StatusOK, section)
}

// DeleteSection godoc
//
//	@Summary		Delete a section
//	@Description	Soft-delete a section from a module.
//	@Tags			modules
//	@Produce		json
//	@Param			short_id			path	string	true	"Module short ID"
//	@Param			section_short_id	path	string	true	"Section short ID"
//	@Success		204					"No Content"
//	@Failure		403					{object}	map[string]string	"Forbidden"
//	@Failure		404					{object}	map[string]string	"Module or section not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections/{section_short_id} [delete]
func (ctrl *ModuleController) DeleteSection(c *gin.Context) {
	shortID := c.Param("short_id")
	sectionShortID := c.Param("section_short_id")

	if err := ctrl.moduleRepo.DeleteSection(c.Request.Context(), shortID, sectionShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "module or section not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete section"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Section Materials ──────────────────────────────────────────────────────────

// AddMaterial godoc
//
//	@Summary		Add material to a section
//	@Description	Create a new material inside a section. For file-based materials (video, pdf, image, etc.), first upload the file via POST /upload/material to get a URL, then pass it as file_url. For link type, pass the external URL directly as file_url. max_views must be "unlimited" or "limited"; if "limited", max_views_count is required. allow_access_on must be "both" or "app".
//	@Tags			modules
//	@Accept			json
//	@Produce		json
//	@Param			short_id			path		string						true	"Module short ID"
//	@Param			section_short_id	path		string						true	"Section short ID"
//	@Param			body				body		models.CreateMaterialInput	true	"Material details"
//	@Success		201					{object}	models.SectionMaterial
//	@Failure		400					{object}	map[string]string	"Validation error"
//	@Failure		403					{object}	map[string]string	"Forbidden"
//	@Failure		404					{object}	map[string]string	"Section not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections/{section_short_id}/materials [post]
func (ctrl *ModuleController) AddMaterial(c *gin.Context) {
	sectionShortID := c.Param("section_short_id")

	var input models.CreateMaterialInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.MaxViews == "limited" && input.MaxViewsCount == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_views_count is required when max_views is limited"})
		return
	}
	if input.MaxViews == "unlimited" {
		input.MaxViewsCount = nil
	}

	material, err := ctrl.moduleRepo.CreateMaterial(c.Request.Context(), sectionShortID, c.GetString("user_id"), input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "section not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create material"})
		return
	}
	c.JSON(http.StatusCreated, material)
}

// GetMaterials godoc
//
//	@Summary		List materials in a section
//	@Description	Returns all active materials for the given section, ordered by creation date.
//	@Tags			modules
//	@Produce		json
//	@Param			short_id			path		string	true	"Module short ID"
//	@Param			section_short_id	path		string	true	"Section short ID"
//	@Success		200					{array}		models.SectionMaterial
//	@Failure		404					{object}	map[string]string	"Section not found"
//	@Failure		500					{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections/{section_short_id}/materials [get]
func (ctrl *ModuleController) GetMaterials(c *gin.Context) {
	sectionShortID := c.Param("section_short_id")

	materials, err := ctrl.moduleRepo.GetMaterials(c.Request.Context(), sectionShortID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "section not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch materials"})
		return
	}
	if materials == nil {
		materials = []models.SectionMaterial{}
	}
	c.JSON(http.StatusOK, materials)
}

// UpdateMaterial godoc
//
//	@Summary		Update a material
//	@Description	Partially update a material. Send only the fields you want to change.
//	@Tags			modules
//	@Accept			json
//	@Produce		json
//	@Param			short_id				path		string						true	"Module short ID"
//	@Param			section_short_id		path		string						true	"Section short ID"
//	@Param			material_short_id		path		string						true	"Material short ID"
//	@Param			body					body		models.UpdateMaterialInput	true	"Fields to update"
//	@Success		200						{object}	models.SectionMaterial
//	@Failure		400						{object}	map[string]string	"Validation error"
//	@Failure		403						{object}	map[string]string	"Forbidden"
//	@Failure		404						{object}	map[string]string	"Section or material not found"
//	@Failure		500						{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections/{section_short_id}/materials/{material_short_id} [patch]
func (ctrl *ModuleController) UpdateMaterial(c *gin.Context) {
	sectionShortID := c.Param("section_short_id")
	materialShortID := c.Param("material_short_id")

	var input models.UpdateMaterialInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	material, err := ctrl.moduleRepo.UpdateMaterial(c.Request.Context(), sectionShortID, materialShortID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "section or material not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update material"})
		return
	}
	c.JSON(http.StatusOK, material)
}

// DeleteMaterial godoc
//
//	@Summary		Delete a material
//	@Description	Soft-delete a material from a section.
//	@Tags			modules
//	@Produce		json
//	@Param			short_id				path	string	true	"Module short ID"
//	@Param			section_short_id		path	string	true	"Section short ID"
//	@Param			material_short_id		path	string	true	"Material short ID"
//	@Success		204						"No Content"
//	@Failure		403						{object}	map[string]string	"Forbidden"
//	@Failure		404						{object}	map[string]string	"Section or material not found"
//	@Failure		500						{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/modules/{short_id}/sections/{section_short_id}/materials/{material_short_id} [delete]
func (ctrl *ModuleController) DeleteMaterial(c *gin.Context) {
	sectionShortID := c.Param("section_short_id")
	materialShortID := c.Param("material_short_id")

	if err := ctrl.moduleRepo.DeleteMaterial(c.Request.Context(), sectionShortID, materialShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "section or material not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete material"})
		return
	}
	c.Status(http.StatusNoContent)
}
