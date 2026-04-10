package handler

import (
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
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
	}

	// --- Push phase ---
	// Enviamos las memorias del cliente al servidor.
	// pushResp contiene estadísticas (pushed, conflicts).
	pushResp, err := h.svc.Push(c.Request.Context(), req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error en sincronización"})
		return
	}

	// --- Pull phase ---
	// Obtenemos las memorias del servidor que el cliente no tiene.
	// Usamos last_sync como punto de corte temporal.
	// Si no viene last_sync, usamos el tiempo cero → el servidor devuelve todo.
	var since time.Time
	if req.LastSync != nil {
		since = *req.LastSync
	}

	// Excluimos los sync_ids que acabamos de enviar — no tiene sentido devolverlos.
	excludeIDs := make([]string, 0, len(req.Memories))
	for _, m := range req.Memories {
		excludeIDs = append(excludeIDs, m.SyncID)
	}

	pulled, err := h.svc.Pull(c.Request.Context(), req.Project, since, excludeIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error en pull de memorias"})
		return
	}

	if pulled == nil {
		pulled = []*model.Memory{}
	}

	c.JSON(http.StatusOK, model.SyncResponse{
		Pushed:    pushResp.Pushed,
		Pulled:    pulled,
		Conflicts: pushResp.Conflicts,
	})
}
