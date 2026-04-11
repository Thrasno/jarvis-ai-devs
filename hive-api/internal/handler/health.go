package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

// HealthHandler maneja el endpoint GET /health.
// Verifica conectividad con la BD para exponer estado real al load balancer.
type HealthHandler struct {
	db DBPinger // nil en tests unitarios → skip check
}

// NewHealthHandler crea un HealthHandler con el pinger inyectado.
func NewHealthHandler(db DBPinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Check maneja GET /health — liveness + readiness probe.
//
// Respuestas:
//   - 200 {"status":"ok",      "db":"connected"}   — todo bien
//   - 503 {"status":"degraded","db":"unreachable"} — BD caída
func (h *HealthHandler) Check(c *gin.Context) {
	dbStatus := "connected"
	httpStatus := http.StatusOK

	// Si tenemos un pinger configurado, verificamos conectividad real con la BD.
	// timeout corto: 2s — si PostgreSQL no responde en 2s, asumimos que está caído.
	// En los tests unitarios db == nil → skip, no queremos depender de Postgres.
	if h.db != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := h.db.Ping(ctx); err != nil {
			dbStatus = "unreachable"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	c.JSON(httpStatus, model.HealthResponse{
		Status:  map[bool]string{true: "ok", false: "degraded"}[httpStatus == http.StatusOK],
		DB:      dbStatus,
		Version: "1.0.0",
	})
}
