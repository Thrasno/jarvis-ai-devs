package handler

import (
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

type ActivityHandler struct {
	svc ActivityService
}

func NewActivityHandler(svc ActivityService) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

func (h *ActivityHandler) List(c *gin.Context) {
	var query model.ActivityFeedQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	response, err := h.svc.List(c.Request.Context(), query)
	if err != nil {
		if errors.Is(err, service.ErrInvalidActivityCursor) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: service.ErrInvalidActivityCursor.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error listing activity feed"})
		return
	}
	if response == nil {
		response = &model.ActivityFeedResponse{}
	}
	if response.Entries == nil {
		response.Entries = []model.ActivityFeedEntry{}
	}

	c.JSON(http.StatusOK, response)
}
