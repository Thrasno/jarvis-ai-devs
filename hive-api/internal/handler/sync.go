package handler

import (
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

// SyncHandler maneja el endpoint POST /sync.
type SyncHandler struct {
	svc SyncService
}

// NewSyncHandler crea un SyncHandler con el servicio inyectado.
func NewSyncHandler(svc SyncService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

// Sync maneja POST /sync — sincronización bidireccional Push+Pull.
//
// El flujo completo:
//  1. Bind del body (SyncRequest con memorias + last_sync opcional)
//  2. Push: enviar las memorias del cliente al servidor
//  3. Pull: obtener las memorias del servidor que el cliente no tiene
//  4. Combinar estadísticas de push + memorias pulled en SyncResponse
//
// Push y Pull usan el mismo sync en un solo endpoint para atomicidad
// y reducir la cantidad de requests de red.
func (h *SyncHandler) Sync(c *gin.Context) {
	var req model.SyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	claims := claimsFromCtx(c)
	userID := ""
	if claims != nil {
		userID = claims.Subject
		req.AckSubject = projectBlockAckSubjectFromClaims(claims)
	}

	resp, err := h.svc.Sync(c.Request.Context(), req, userID)
	if err != nil {
		var blockedErr *service.ProjectBlockedError
		if errors.As(err, &blockedErr) {
			cmd := blockedErr.Command.Redacted()
			cmd.AckToken = blockedErr.Command.AckToken
			c.JSON(http.StatusLocked, model.ProjectBlockedErrorResponse{Error: blockedErr.Error(), Command: cmd})
			return
		}
		if errors.Is(err, service.ErrProjectKeyLockBusy) {
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "project is busy; retry sync"})
			return
		}
		// R2-CRIT-6 — clasificar errores de validación del push como 4xx (no 500).
		// El daemon necesita ajustar su payload, no es una falla del servidor.
		if errors.Is(err, service.ErrSessionProjectMismatch) || errors.Is(err, service.ErrPromptProjectMismatch) || errors.Is(err, service.ErrSessionNotFound) || errors.Is(err, model.ErrProjectIdentityVersionUnsupported) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error en sincronización"})
		return
	}

	c.JSON(http.StatusOK, resp)
}
