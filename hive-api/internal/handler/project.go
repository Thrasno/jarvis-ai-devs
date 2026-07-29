package handler

import (
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

type ProjectHandler struct {
	svc ProjectService
}

func NewProjectHandler(svc ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) List(c *gin.Context) {
	health := c.Query("health")
	if health != "" && health != model.ProjectSyncHealthDegraded {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "unsupported health filter"})
		return
	}
	resp, err := h.svc.List(c.Request.Context(), health)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al listar proyectos"})
		return
	}
	if resp.Projects == nil {
		resp.Projects = []model.ProjectSummary{}
	}
	c.JSON(http.StatusOK, resp)
}
