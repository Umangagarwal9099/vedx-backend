package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type CourseController struct {
	courseRepo *repository.CourseRepository
}

func NewCourseController(courseRepo *repository.CourseRepository) *CourseController {
	return &CourseController{courseRepo: courseRepo}
}

// CreateCourse godoc
//
//	@Summary		Create course
//	@Description	Create a new course. Restricted to super_admin role. A short unique ID is generated automatically.
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateCourseInput	true	"Course details"
//	@Success		201		{object}	models.Course
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		403		{object}	map[string]string	"Forbidden"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses [post]
func (ctrl *CourseController) Create(c *gin.Context) {
	var input models.CreateCourseInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdBy := c.GetString("user_id")

	course, err := ctrl.courseRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create course"})
		return
	}

	c.JSON(http.StatusCreated, course)
}

// GetAllCourses godoc
//
//	@Summary		List courses
//	@Description	Returns all active and inactive non-deleted courses.
//	@Tags			courses
//	@Produce		json
//	@Success		200	{array}		models.Course
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses [get]
func (ctrl *CourseController) GetAll(c *gin.Context) {
	courses, err := ctrl.courseRepo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch courses"})
		return
	}
	if courses == nil {
		courses = []models.Course{}
	}
	c.JSON(http.StatusOK, courses)
}

// SearchCourses godoc
//
//	@Summary		Search courses
//	@Description	Search non-deleted courses by name or description. Pass the search term as query param `q`.
//	@Tags			courses
//	@Produce		json
//	@Param			q	query		string	true	"Search term"
//	@Success		200	{array}		models.Course
//	@Failure		400	{object}	map[string]string	"Missing query param"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses/search [get]
func (ctrl *CourseController) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query param 'q' is required"})
		return
	}

	courses, err := ctrl.courseRepo.Search(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search courses"})
		return
	}
	if courses == nil {
		courses = []models.Course{}
	}
	c.JSON(http.StatusOK, courses)
}

// UpdateCourse godoc
//
//	@Summary		Update course
//	@Description	Partially update a course by its short_id. Send only the fields you want to change. Use is_active to activate or deactivate.
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string						true	"Course short ID (e.g. A3F72C1D)"
//	@Param			body		body		models.UpdateCourseInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Course
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Course not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses/{short_id} [patch]
func (ctrl *CourseController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateCourseInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.courseRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update course"})
		return
	}

	course, err := ctrl.courseRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || course == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated course"})
		return
	}

	c.JSON(http.StatusOK, course)
}

// GetCurriculum returns all modules (with sections + materials) assigned to a course.
func (ctrl *CourseController) GetCurriculum(c *gin.Context) {
	shortID := c.Param("short_id")
	curriculum, err := ctrl.courseRepo.GetCurriculum(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch curriculum"})
		return
	}
	if curriculum == nil {
		curriculum = []models.ModuleWithSections{}
	}
	c.JSON(http.StatusOK, curriculum)
}

// AssignModule assigns a module to a course.
func (ctrl *CourseController) AssignModule(c *gin.Context) {
	shortID := c.Param("short_id")
	var input models.AssignModuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.courseRepo.AssignModule(c.Request.Context(), shortID, input.ModuleShortID, input.OrderIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not assign module: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "module assigned"})
}

// UnassignModule removes a module from a course.
func (ctrl *CourseController) UnassignModule(c *gin.Context) {
	courseShortID := c.Param("short_id")
	moduleShortID := c.Param("module_short_id")
	if err := ctrl.courseRepo.UnassignModule(c.Request.Context(), courseShortID, moduleShortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not unassign module"})
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteCourse godoc
//
//	@Summary		Delete course
//	@Description	Soft-delete a course by its short_id (sets deleted_at; the row is retained).
//	@Tags			courses
//	@Produce		json
//	@Param			short_id	path	string	true	"Course short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Course not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses/{short_id} [delete]
func (ctrl *CourseController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.courseRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete course"})
		return
	}

	c.Status(http.StatusNoContent)
}
