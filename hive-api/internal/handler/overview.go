package handler

import (
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

// OverviewHandler handles capability-aware and legacy Admin overview endpoints.
type OverviewHandler struct {
	authSvc AuthService
	svc     OverviewService
}

// NewOverviewHandler creates an OverviewHandler with the required services.
func NewOverviewHandler(authSvc AuthService, svc OverviewService) *OverviewHandler {
	return &OverviewHandler{authSvc: authSvc, svc: svc}
}

// Get handles GET /overview using the current persisted user capability.
func (h *OverviewHandler) Get(c *gin.Context) {
	rawClaims, exists := c.Get(middleware.ClaimsKey)
	claims, ok := rawClaims.(*model.Claims)
	if !exists || !ok || claims == nil || claims.Subject == "" {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	user, err := h.authSvc.GetCurrentUser(c.Request.Context(), claims.Subject)
	if err != nil {
		if errors.Is(err, service.ErrUserInactive) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}
	if user == nil || !user.IsActive || (user.Level != model.LevelMember && user.Level != model.LevelAdmin) {
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "forbidden"})
		return
	}

	overview, err := h.svc.GetForLevel(c.Request.Context(), user.Level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}
	c.JSON(http.StatusOK, overview)
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
