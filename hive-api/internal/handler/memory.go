package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

const defaultMemoryQueryLimit = 20

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
		SessionID:     req.SessionID,
	}

	created, err := h.svc.Create(c.Request.Context(), mem)
	if err != nil {
		// ErrSyncIDExists → idempotencia: devolvemos el registro existente con 200.
		// El daemon puede reenviar el mismo sync_id sin preocuparse por duplicados.
		if errors.Is(err, service.ErrSyncIDExists) {
			c.JSON(http.StatusOK, created)
			return
		}
		// R3-FIX-2: cross-project session attribution failures and unknown sessions
		// are caller errors (4xx), not server faults.
		if errors.Is(err, service.ErrSessionProjectMismatch) || errors.Is(err, service.ErrSessionNotFound) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
			return
		}
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
	filter, limit, err := buildMemoryDiscoveryFilter(q.Project, q.Category, q.From, q.Until, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	query := strings.TrimSpace(q.Query)
	var mems []*model.Memory
	var total int64
	if query != "" {
		mems, total, err = h.svc.Search(c.Request.Context(), query, filter)
	} else {
		mems, total, err = h.svc.List(c.Request.Context(), filter)
	}
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
		Limit:    limit,
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
	filter, limit, err := buildMemoryDiscoveryFilter(q.Project, q.Category, q.From, q.Until, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	mems, total, err := h.svc.Search(c.Request.Context(), q.Query, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error en la búsqueda"})
		return
	}

	if mems == nil {
		mems = []*model.Memory{}
	}

	c.JSON(http.StatusOK, model.SearchResponse{
		Memories: mems,
		Total:    total,
		Query:    q.Query,
		Limit:    limit,
		Offset:   q.Offset,
	})
}

func buildMemoryDiscoveryFilter(project, category, fromRaw, untilRaw string, limit, offset int) (model.MemoryFilter, int, error) {
	createdFrom, createdUntil, err := parseMemoryDateRange(fromRaw, untilRaw)
	if err != nil {
		return model.MemoryFilter{}, 0, err
	}
	if limit == 0 {
		limit = defaultMemoryQueryLimit
	}
	filter := model.MemoryFilter{Project: project, CreatedFrom: createdFrom, CreatedUntil: createdUntil, Limit: limit, Offset: offset}
	if category != "" {
		cat := model.MemoryCategory(category)
		if !cat.IsValid() {
			return model.MemoryFilter{}, 0, errors.New("invalid category")
		}
		filter.Category = &cat
	}
	return filter, limit, nil
}

func parseMemoryDateRange(fromRaw, untilRaw string) (*time.Time, *time.Time, error) {
	from, err := parseMemoryDateBoundary(fromRaw, false)
	if err != nil {
		return nil, nil, errors.New("invalid from date")
	}
	until, err := parseMemoryDateBoundary(untilRaw, true)
	if err != nil {
		return nil, nil, errors.New("invalid until date")
	}
	if from != nil && until != nil && from.After(*until) {
		return nil, nil, errors.New("from must be before until")
	}
	return from, until, nil
}

func parseMemoryDateBoundary(raw string, endOfDay bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, err
		}
	}
	parsed = parsed.UTC()
	hour, minute, second, nsec := 0, 0, 0, 0
	if endOfDay {
		hour, minute, second, nsec = 23, 59, 59, int(time.Second-time.Nanosecond)
	}
	boundary := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), hour, minute, second, nsec, time.UTC)
	return &boundary, nil
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
