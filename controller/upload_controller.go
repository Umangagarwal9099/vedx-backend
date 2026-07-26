package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/service"
)

type UploadController struct {
	storage *service.StorageService
}

func NewUploadController(storage *service.StorageService) *UploadController {
	return &UploadController{storage: storage}
}

// UploadEventImage godoc
//
//	@Summary		Upload event image
//	@Description	Upload an image from a device file picker or camera capture. Returns the public URL to use in create/update event. Max size 10 MB. Allowed types: JPEG, PNG, WebP, GIF.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			image	formData	file	true	"Image file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded image"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/image [post]
func (ctrl *UploadController) UploadEventImage(c *gin.Context) {
	fh, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'image' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadEventImage(fh)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error()[:4] == "file" || err.Error()[:4] == "unsu" {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadBlogImage godoc
//
//	@Summary		Upload blog featured image
//	@Description	Upload an image for a blog post. Returns the public URL to use in the featured_image field when creating or updating a blog. Max size 10 MB. Allowed types: JPEG, PNG, WebP, GIF.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			image	formData	file	true	"Image file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded image"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/blog-image [post]
func (ctrl *UploadController) UploadBlogImage(c *gin.Context) {
	fh, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'image' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadBlogImage(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadBannerImage godoc
//
//	@Summary		Upload banner thumbnail
//	@Description	Upload a thumbnail image for a banner. Returns the public URL to use in the thumbnail field when creating or updating a banner. Max size 10 MB. Allowed types: JPEG, PNG, WebP, GIF.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			image	formData	file	true	"Image file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded image"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/banner-image [post]
func (ctrl *UploadController) UploadBannerImage(c *gin.Context) {
	fh, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'image' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadBannerImage(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadAssessmentThumbnail godoc
//
//	@Summary		Upload assessment thumbnail
//	@Description	Upload a thumbnail image for an assessment. Returns the public URL to use in the thumbnail field when creating or updating an assessment. Max size 10 MB. Allowed types: JPEG, PNG, WebP, GIF.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			image	formData	file	true	"Thumbnail image file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded thumbnail"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/assessment-thumbnail [post]
func (ctrl *UploadController) UploadAssessmentThumbnail(c *gin.Context) {
	fh, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'image' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadAssessmentThumbnail(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadAssessmentFile godoc
//
//	@Summary		Upload assessment file
//	@Description	Upload a file (PDF, Word, Excel, PowerPoint, image, video, audio, zip, etc.) to attach to an assessment. Returns the public URL to include in the file_urls array when creating or updating an assessment. Max size 500 MB. Call this endpoint once per file and collect all returned URLs.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Assessment file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded file"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/assessment-file [post]
func (ctrl *UploadController) UploadAssessmentFile(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'file' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadAssessmentFile(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadAssignmentFile godoc
//
//	@Summary		Upload assignment submission file
//	@Description	Upload a student's assignment submission file. Returns the public URL to pass as file_url when submitting via POST /assignments/{short_id}/submissions. Max size 500 MB.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Submission file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded file"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/assignment-file [post]
func (ctrl *UploadController) UploadAssignmentFile(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'file' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadAssignmentFile(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadResourceFile godoc
//
//	@Summary		Upload resource file
//	@Description	Upload a learning resource file (PDF, doc, video, image, code, etc.) to attach to a resource. Returns the public URL to pass as url when creating or updating a resource via POST/PATCH /resources. Max size 500 MB.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Resource file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded file"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/resource-file [post]
func (ctrl *UploadController) UploadResourceFile(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'file' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadResourceFile(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadProjectFile godoc
//
//	@Summary		Upload project submission file
//	@Description	Upload a project submission file (report, zip, presentation, etc.). Returns the public URL to pass as file_url when submitting via POST /projects/{short_id}/milestones/{milestone_short_id}/submissions. Max size 500 MB.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Submission file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded file"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/project-file [post]
func (ctrl *UploadController) UploadProjectFile(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'file' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadProjectFile(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// UploadMaterial godoc
//
//	@Summary		Upload a material file
//	@Description	Upload any supported material file (image, video, audio, PDF, doc, sheet, slide, zip, etc.) to Cloudflare R2. Returns the public URL to use when creating a section material. Max size 500 MB.
//	@Tags			upload
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Material file"
//	@Success		200		{object}	map[string]string	"url: public URL of the uploaded file"
//	@Failure		400		{object}	map[string]string	"Validation error"
//	@Failure		500		{object}	map[string]string	"Upload failed"
//	@Security		BearerAuth
//	@Router			/upload/material [post]
func (ctrl *UploadController) UploadMaterial(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 'file' is required (multipart/form-data)"})
		return
	}

	url, err := ctrl.storage.UploadMaterial(fh)
	if err != nil {
		status := http.StatusInternalServerError
		msg := err.Error()
		if len(msg) >= 4 && (msg[:4] == "file" || msg[:4] == "unsu" || msg[:4] == "cann") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}
