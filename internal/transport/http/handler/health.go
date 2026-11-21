// Package handler contains HTTP handlers.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthResponse is the health check response body.
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// Health godoc
//
//	@Summary	Health check
//	@Tags		health
//	@Produce	json
//	@Success	200	{object}	HealthResponse
//	@Router		/healthz [get]
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
}
