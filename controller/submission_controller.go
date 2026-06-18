package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type SubmissionController struct {
	repo *repository.SubmissionRepository
}

func NewSubmissionController(repo *repository.SubmissionRepository) *SubmissionController {
	return &SubmissionController{repo: repo}
}

// Create godoc
//
//	@Summary		Submit code solution
//	@Tags			submissions
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateSubmissionInput	true	"Submission"
//	@Success		201		{object}	models.Submission
//	@Security		BearerAuth
//	@Router			/submissions [post]
func (ctrl *SubmissionController) Create(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var in models.CreateSubmissionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s, err := ctrl.repo.Create(c.Request.Context(), userID.(string), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save submission"})
		return
	}
	c.JSON(http.StatusCreated, s)
}

// GetMySubmissions godoc
//
//	@Summary		Get current user's submissions
//	@Tags			submissions
//	@Produce		json
//	@Success		200	{array}		models.Submission
//	@Security		BearerAuth
//	@Router			/submissions/me [get]
func (ctrl *SubmissionController) GetMySubmissions(c *gin.Context) {
	userID, _ := c.Get("user_id")

	list, err := ctrl.repo.FindByUser(c.Request.Context(), userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if list == nil {
		list = []models.Submission{}
	}
	c.JSON(http.StatusOK, list)
}

// GetByQuestion godoc
//
//	@Summary		Get current user's submissions for a specific question
//	@Tags			submissions
//	@Produce		json
//	@Param			short_id	path	string	true	"Question short ID"
//	@Success		200			{array}	models.Submission
//	@Security		BearerAuth
//	@Router			/submissions/question/{short_id} [get]
func (ctrl *SubmissionController) GetByQuestion(c *gin.Context) {
	userID, _ := c.Get("user_id")
	shortID := c.Param("short_id")

	list, err := ctrl.repo.FindByUserAndQuestion(c.Request.Context(), userID.(string), shortID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if list == nil {
		list = []models.Submission{}
	}
	c.JSON(http.StatusOK, list)
}

// GetAllAdmin godoc
//
//	@Summary		List all submissions (admin/mentor)
//	@Description	Returns every submission with student name, email, and question title.
//	@Tags			submissions
//	@Produce		json
//	@Success		200	{array}		models.SubmissionView
//	@Security		BearerAuth
//	@Router			/submissions/admin [get]
func (ctrl *SubmissionController) GetAllAdmin(c *gin.Context) {
	list, err := ctrl.repo.FindAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if list == nil {
		list = []models.SubmissionView{}
	}
	c.JSON(http.StatusOK, list)
}

// GetByUserAdmin godoc
//
//	@Summary		List submissions for a specific user (admin/mentor)
//	@Tags			submissions
//	@Produce		json
//	@Param			user_id	path	string	true	"User ID (UUID)"
//	@Success		200		{array}	models.SubmissionView
//	@Security		BearerAuth
//	@Router			/submissions/user/{user_id} [get]
func (ctrl *SubmissionController) GetByUserAdmin(c *gin.Context) {
	userID := c.Param("user_id")
	list, err := ctrl.repo.FindByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch submissions"})
		return
	}
	if list == nil {
		list = []models.SubmissionView{}
	}
	c.JSON(http.StatusOK, list)
}
