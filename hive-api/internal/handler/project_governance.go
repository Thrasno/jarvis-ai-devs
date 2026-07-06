package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProjectGovernanceHandler struct {
	svc ProjectGovernanceService
}

func NewProjectGovernanceHandler(svc ProjectGovernanceService) *ProjectGovernanceHandler {
	return &ProjectGovernanceHandler{svc: svc}
}

func (h *ProjectGovernanceHandler) BlockProject(c *gin.Context) {
	var req model.ProjectBlockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	claims := claimsFromCtx(c)
	actor := model.AdminActor{}
	if claims != nil {
		actor.UserID = claims.Subject
		actor.Username = claims.Username
	}
	project := req.Project
	if project == "" {
		project = c.Param("project")
	}
	resp, err := h.svc.BlockProject(c.Request.Context(), actor, project, req)
	if err != nil {
		if errors.Is(err, service.ErrProjectBlockInvalidRequest) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
			return
		}
		if errors.Is(err, service.ErrProjectKeyLockBusy) {
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "project is busy; retry block request"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error blocking project"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *ProjectGovernanceHandler) Status(c *gin.Context) {
	resp, err := h.svc.Status(c.Request.Context(), projectFromQueryOrParam(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error loading project block status"})
		return
	}
	if !claimsAreAdmin(c) {
		resp.Reason = ""
		resp.Command = nil
		resp.Ack = nil
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ProjectGovernanceHandler) Acknowledge(c *gin.Context) {
	var req model.ProjectBlockAck
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	claims := claimsFromCtx(c)
	req.AckSubject = projectBlockAckSubjectFromClaims(claims)
	if !req.AckSubject.Valid() {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: service.ErrProjectBlockInvalidRequest.Error()})
		return
	}
	ack, err := h.svc.Acknowledge(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrProjectBlockInvalidRequest) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
			return
		}
		if errors.Is(err, service.ErrProjectKeyLockBusy) {
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "project is busy; retry acknowledgement"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error recording project block acknowledgement"})
		return
	}
	c.JSON(http.StatusOK, ack)
}

func projectFromQueryOrParam(c *gin.Context) string {
	if project := c.Query("project"); project != "" {
		return project
	}
	return c.Param("project")
}

func claimsAreAdmin(c *gin.Context) bool {
	claims := claimsFromCtx(c)
	return claims != nil && strings.EqualFold(string(claims.Level), string(model.LevelAdmin))
}

func projectBlockAckSubjectFromClaims(claims *model.Claims) model.ProjectBlockAckSubject {
	if claims == nil {
		return model.ProjectBlockAckSubject{}
	}
	return model.ProjectBlockAckSubject{AuthSubject: claims.Subject, DaemonID: claims.DaemonID, Client: claims.Client}
}
