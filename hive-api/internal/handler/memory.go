package handler

import (
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/gin-gonic/gin"
)

// MemoryHandler maneja los endpoints CRUD de memorias.
type MemoryHandler struct {
	svc MemoryService
}

// NewMemoryHandler crea un MemoryHandler con el servicio inyectado.
func NewMemoryHandler(svc MemoryService) *MemoryHandler {
	return &MemoryHandler{svc: svc}
}

// Create maneja POST /memories.
func (h *MemoryHandler) Create(c *gin.Context) {
	var req model.CreateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	// Extraemos el userID del token para asignarlo como creador de la memoria.
	// El token ya fue validado por RequireAuth — los Claims están en el contexto.
	claims := claimsFromCtx(c)
	userID := ""
	if claims != nil {
		userID = claims.Subject
	}

	mem := &model.Memory{
		SyncID:        req.SyncID,
		Project:       req.Project,
		TopicKey:      req.TopicKey,
		Category:      req.Category,
		Title:         req.Title,
		Content:       req.Content,
		Tags:          req.Tags,
		FilesAffected: req.FilesAffected,
		CreatedBy:     userID,
		Confidence:    derefFloat32(req.Confidence),
		ImpactScore:   derefFloat32(req.ImpactScore),
	}

	created, err := h.svc.Create(c.Request.Context(), mem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al crear memoria"})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// List maneja GET /memories con paginación y filtros opcionales.
func (h *MemoryHandler) List(c *gin.Context) {
	var q model.ListMemoriesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	filter := model.MemoryFilter{
		Project: q.Project,
		Limit:   q.Limit,
		Offset:  q.Offset,
	}
	if q.Category != "" {
		cat := model.MemoryCategory(q.Category)
		filter.Category = &cat
	}

	mems, total, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al listar memorias"})
		return
	}

	// Garantizamos [] en lugar de null cuando no hay memorias
	if mems == nil {
		mems = []*model.Memory{}
	}

	c.JSON(http.StatusOK, model.ListMemoriesResponse{
		Memories: mems,
		Total:    total,
		Limit:    q.Limit,
		Offset:   q.Offset,
	})
}

// GetByID maneja GET /memories/:id.
func (h *MemoryHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "id requerido"})
		return
	}

	mem, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "memoria no encontrada"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al obtener memoria"})
		return
	}

	c.JSON(http.StatusOK, mem)
}

// Search maneja GET /memories/search?query=...
func (h *MemoryHandler) Search(c *gin.Context) {
	var q model.SearchQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	filter := model.MemoryFilter{
		Project: q.Project,
		Limit:   q.Limit,
		Offset:  q.Offset,
	}

	mems, err := h.svc.Search(c.Request.Context(), q.Query, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error en la búsqueda"})
		return
	}

	if mems == nil {
		mems = []*model.Memory{}
	}

	c.JSON(http.StatusOK, model.SearchResponse{
		Memories: mems,
		Total:    int64(len(mems)),
		Query:    q.Query,
		Limit:    q.Limit,
	})
}

// --- helpers privados ---

// claimsFromCtx extrae los Claims del contexto de Gin.
// Devuelve nil si no existen (no debería ocurrir si RequireAuth está en la cadena).
func claimsFromCtx(c *gin.Context) *model.Claims {
	raw, exists := c.Get(middleware.ClaimsKey)
	if !exists {
		return nil
	}
	claims, _ := raw.(*model.Claims)
	return claims
}

// derefFloat32 desreferencia un *float32, devolviendo 0 si es nil.
func derefFloat32(f *float32) float32 {
	if f == nil {
		return 0
	}
	return *f
}
