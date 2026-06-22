package handler

import (
	"context"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

// OverviewService is the handler-local interface for the overview service.
// Defined here following Go convention: define the interface where it is used.
type overviewServiceHandler interface {
	GetStats(ctx context.Context) (*model.OverviewStatsResponse, error)
	GetGrowth(ctx context.Context) (*model.OverviewGrowthResponse, error)
}

// OverviewHandler handles GET /admin/overview/* endpoints.
type OverviewHandler struct {
	svc overviewServiceHandler
}

// NewOverviewHandler creates an OverviewHandler with the given service.
func NewOverviewHandler(svc OverviewService) *OverviewHandler {
	return &OverviewHandler{svc: svc}
}

// GetStats handles GET /admin/overview/stats.
func (h *OverviewHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al obtener overview"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// GetGrowth handles GET /admin/overview/growth.
func (h *OverviewHandler) GetGrowth(c *gin.Context) {
	growth, err := h.svc.GetGrowth(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al obtener crecimiento"})
		return
	}
	c.JSON(http.StatusOK, growth)
}
