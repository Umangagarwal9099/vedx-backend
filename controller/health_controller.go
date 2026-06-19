package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// Health godoc
//
//	@Summary		Health check
//	@Description	Returns server health status
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Router			/health [get]
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
}
