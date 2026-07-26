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
	courseRepo   *repository.CourseRepository
	collegeRepo  *repository.CollegeRepository
	auditLogRepo *repository.AuditLogRepository
}

func NewCourseController(courseRepo *repository.CourseRepository, collegeRepo *repository.CollegeRepository, auditLogRepo *repository.AuditLogRepository) *CourseController {
	return &CourseController{courseRepo: courseRepo, collegeRepo: collegeRepo, auditLogRepo: auditLogRepo}
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

	collegeID, err := resolveTargetCollege(c.Request.Context(), ctrl.collegeRepo, c.GetString("role"), c.GetString("college_id"), input.CollegeShortID)
	if err != nil {
		if errors.Is(err, repository.ErrCollegeScopeRequired) {
			c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope to create a course under"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not resolve target college: " + err.Error()})
		return
	}

	course, err := ctrl.courseRepo.Create(c.Request.Context(), input, createdBy, collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create course"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "course_created", EntityType: "course", EntityID: course.ID, EntityShortID: course.ShortID, EntityLabel: course.Name,
	})

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
	collegeID, err := repository.CollegeFilter(c.GetString("role"), c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	courses, err := ctrl.courseRepo.FindAll(c.Request.Context(), collegeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch courses"})
		return
	}
	if courses == nil {
		courses = []models.Course{}
	}
	c.JSON(http.StatusOK, courses)
}

// GetByShortID godoc
//
//	@Summary		Get course
//	@Description	Returns a single non-deleted course by its short_id.
//	@Tags			courses
//	@Produce		json
//	@Param			short_id	path		string	true	"Course short ID"
//	@Success		200			{object}	models.Course
//	@Failure		404			{object}	map[string]string	"Course not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses/{short_id} [get]
func (ctrl *CourseController) GetByShortID(c *gin.Context) {
	shortID := c.Param("short_id")

	course, err := ctrl.courseRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch course"})
		return
	}
	if course == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
		return
	}
	c.JSON(http.StatusOK, course)
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

	collegeID, err := repository.CollegeFilter(c.GetString("role"), c.GetString("college_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "COLLEGE_SCOPE_VIOLATION", "error": "you have no college scope"})
		return
	}

	courses, err := ctrl.courseRepo.Search(c.Request.Context(), q, collegeID)
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
	if !checkCourseAccess(c, ctrl.courseRepo, shortID) {
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

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "course_updated", EntityType: "course", EntityID: course.ID, EntityShortID: course.ShortID, EntityLabel: course.Name,
	})

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

	if !checkCourseAccess(c, ctrl.courseRepo, shortID) {
		return
	}

	if err := ctrl.courseRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete course"})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{Action: "course_deleted", EntityType: "course", EntityShortID: shortID})

	c.Status(http.StatusNoContent)
}

// AssignCollegeInput carries the target college and optional access window
// for assigning a global course to a college.
type AssignCollegeInput struct {
	CollegeShortID  string  `json:"college_short_id" binding:"required" example:"ABCENG"`
	AccessStartDate *string `json:"access_start_date" example:"2026-08-01"`
	AccessEndDate   *string `json:"access_end_date"   example:"2027-07-31"`
}

// AssignToCollege godoc
//
//	@Summary		Assign a global course to a college
//	@Description	Grants a college access to a course marked course_scope=global, with an optional access window. Super_admin only. Has no effect on organization-scoped courses (those are only ever usable by the one college that owns them).
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string				true	"Course short ID"
//	@Param			body		body		AssignCollegeInput	true	"Target college"
//	@Success		204			"No Content"
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Course or college not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/courses/{short_id}/assign [post]
func (ctrl *CourseController) AssignToCollege(c *gin.Context) {
	shortID := c.Param("short_id")

	var input AssignCollegeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	course, err := ctrl.courseRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || course == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "course not found"})
		return
	}
	college, err := ctrl.collegeRepo.FindByShortID(c.Request.Context(), input.CollegeShortID)
	if err != nil || college == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "college not found"})
		return
	}

	var startDate, endDate string
	if input.AccessStartDate != nil {
		startDate = *input.AccessStartDate
	}
	if input.AccessEndDate != nil {
		endDate = *input.AccessEndDate
	}

	if err := ctrl.courseRepo.AssignToCollege(c.Request.Context(), course.ID, college.ID, c.GetString("user_id"), &startDate, &endDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not assign course: " + err.Error()})
		return
	}

	logAudit(c, ctrl.auditLogRepo, models.AuditEntry{
		Action: "course_assigned_to_college", EntityType: "course", EntityID: course.ID, EntityShortID: course.ShortID, EntityLabel: course.Name,
		Metadata: map[string]interface{}{"college_short_id": college.ShortID, "college_name": college.Name},
	})

	c.Status(http.StatusNoContent)
}
