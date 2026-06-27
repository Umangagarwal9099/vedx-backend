package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

type BlogController struct {
	blogRepo *repository.BlogRepository
}

func NewBlogController(blogRepo *repository.BlogRepository) *BlogController {
	return &BlogController{blogRepo: blogRepo}
}

// CreateBlog godoc
//
//	@Summary		Create blog
//	@Description	Create a new blog post. Upload featured_image via POST /upload/blog-image first and pass the returned URL here. status must be one of: published | draft | scheduled. When status is "scheduled", publish_at (RFC3339) is required. When status is "published" and publish_at is omitted, it defaults to now.
//	@Tags			blogs
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.CreateBlogInput	true	"Blog details"
//	@Success		201		{object}	models.Blog
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/blogs [post]
func (ctrl *BlogController) Create(c *gin.Context) {
	var input models.CreateBlogInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Status == "scheduled" && input.PublishAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "publish_at is required when status is 'scheduled'"})
		return
	}

	createdBy := c.GetString("user_id")

	blog, err := ctrl.blogRepo.Create(c.Request.Context(), input, createdBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create blog"})
		return
	}

	c.JSON(http.StatusCreated, blog)
}

// GetAllBlogs godoc
//
//	@Summary		List blogs
//	@Description	Returns all non-deleted blogs ordered newest first. Filter by title (ILIKE), status, or date (YYYY-MM-DD based on created_at).
//	@Tags			blogs
//	@Produce		json
//	@Param			title	query		string	false	"Filter by title (case-insensitive, partial match)"
//	@Param			status	query		string	false	"Filter by status: published | draft | scheduled"
//	@Param			date	query		string	false	"Filter by creation date (YYYY-MM-DD)"
//	@Success		200		{array}		models.Blog
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/blogs [get]
func (ctrl *BlogController) GetAll(c *gin.Context) {
	var filter models.BlogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var (
		blogs []models.Blog
		err   error
	)

	if filter.Title != "" || filter.Status != "" || filter.Date != "" {
		blogs, err = ctrl.blogRepo.Search(c.Request.Context(), filter)
	} else {
		blogs, err = ctrl.blogRepo.FindAll(c.Request.Context())
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch blogs"})
		return
	}
	if blogs == nil {
		blogs = []models.Blog{}
	}
	c.JSON(http.StatusOK, blogs)
}

// UpdateBlog godoc
//
//	@Summary		Update blog
//	@Description	Partially update a blog by its short_id. Send only the fields you want to change. Use is_active to toggle the blog active/inactive without deleting it.
//	@Tags			blogs
//	@Accept			json
//	@Produce		json
//	@Param			short_id	path		string					true	"Blog short ID"
//	@Param			body		body		models.UpdateBlogInput	true	"Fields to update (all optional)"
//	@Success		200			{object}	models.Blog
//	@Failure		400			{object}	map[string]string	"Validation error"
//	@Failure		404			{object}	map[string]string	"Blog not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/blogs/{short_id} [patch]
func (ctrl *BlogController) Update(c *gin.Context) {
	shortID := c.Param("short_id")

	var input models.UpdateBlogInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.blogRepo.Update(c.Request.Context(), shortID, input); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "blog not found"})
			return
		}
		if err.Error() == "no fields to update" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one field to update"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update blog"})
		return
	}

	blog, err := ctrl.blogRepo.FindByShortID(c.Request.Context(), shortID)
	if err != nil || blog == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch updated blog"})
		return
	}

	c.JSON(http.StatusOK, blog)
}

// DeleteBlog godoc
//
//	@Summary		Delete blog
//	@Description	Soft-delete a blog by its short_id (sets deleted_at; the row is retained in the database).
//	@Tags			blogs
//	@Produce		json
//	@Param			short_id	path	string	true	"Blog short ID"
//	@Success		204			"No Content"
//	@Failure		404			{object}	map[string]string	"Blog not found"
//	@Failure		500			{object}	map[string]string	"Internal server error"
//	@Security		BearerAuth
//	@Router			/blogs/{short_id} [delete]
func (ctrl *BlogController) Delete(c *gin.Context) {
	shortID := c.Param("short_id")

	if err := ctrl.blogRepo.Delete(c.Request.Context(), shortID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "blog not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete blog"})
		return
	}

	c.Status(http.StatusNoContent)
}
