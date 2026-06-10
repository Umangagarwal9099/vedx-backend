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
		// Surface validation errors (type/size) as 400, storage errors as 500
		status := http.StatusInternalServerError
		if err.Error()[:4] == "file" || err.Error()[:4] == "unsu" {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}
