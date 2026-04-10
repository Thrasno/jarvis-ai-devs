package handler

import (
	"net/http"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

// HealthHandler maneja el endpoint GET /health.
// No requiere autenticación — es el endpoint de liveness probe.
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, model.HealthResponse{
		Status:  "ok",
		DB:      "connected",
		Version: "1.0.0",
	})
}
