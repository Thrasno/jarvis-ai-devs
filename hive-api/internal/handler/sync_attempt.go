package handler

import (
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

type SyncAttemptHandler struct {
	svc     SyncAttemptService
	authSvc AuthService
}

func NewSyncAttemptHandler(svc SyncAttemptService, authSvc AuthService) *SyncAttemptHandler {
	return &SyncAttemptHandler{svc: svc, authSvc: authSvc}
}

func (h *SyncAttemptHandler) Ingest(c *gin.Context) {
	var req model.SyncAttemptIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if !h.authorizedDevID(c, req) {
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "sync attempt dev_id is not authorized for this caller"})
		return
	}
	resp, err := h.svc.Ingest(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrSyncAttemptBatchTooLarge) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error ingesting sync attempts"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SyncAttemptHandler) authorizedDevID(c *gin.Context, req model.SyncAttemptIngestRequest) bool {
	claims := claimsFromCtx(c)
	if claims == nil {
		return false
	}
	if claims.Level == model.LevelAdmin {
		return true
	}
	user, err := h.authSvc.GetCurrentUser(c.Request.Context(), claims.Subject)
	if err != nil || user == nil {
		return false
	}
	for _, attempt := range req.Attempts {
		if attempt.DevID != "" && attempt.DevID != user.Email {
			return false
		}
	}
	return true
}

func (h *SyncAttemptHandler) Summary(c *gin.Context) {
	query := model.SyncAttemptSummaryQuery{
		Window:    c.Query("window"),
		Project:   c.Query("project"),
		DevID:     c.Query("dev_id"),
		Client:    c.Query("client"),
		DaemonID:  c.Query("daemon_id"),
		Outcome:   c.Query("outcome"),
		ErrorCode: c.Query("error_code"),
	}
	if query.Window != "" && query.Window != "24h" && query.Window != "7d" && query.Window != "30d" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "window must be one of 24h, 7d, or 30d"})
		return
	}
	if query.Outcome != "" && query.Outcome != string(model.SyncAttemptOutcomeSuccess) && query.Outcome != string(model.SyncAttemptOutcomeFailure) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "outcome must be success or failure"})
		return
	}

	resp, err := h.svc.Summary(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error reading sync attempt summary"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
