package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminHandler maneja los endpoints de administración.
// Todos requieren RequireAuth + RequireAdmin en la cadena de middlewares.
type AdminHandler struct {
	svc AdminService
}

// NewAdminHandler crea un AdminHandler con el servicio inyectado.
func NewAdminHandler(svc AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

// ListUsers maneja GET /admin/users.
// Devuelve todos los usuarios del sistema.
func (h *AdminHandler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al obtener usuarios"})
		return
	}

	if users == nil {
		users = []*model.User{}
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	filter, err := auditFilterFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	resp, err := h.svc.ListAuditLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al obtener auditoría"})
		return
	}
	if resp.AuditLogs == nil {
		resp.AuditLogs = []*model.AuditEntry{}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if err := model.ValidateTemporaryPasswordBytes(req.TemporaryPassword); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.svc.CreateUser(c.Request.Context(), adminActorFromCtx(c), req); err != nil {
		h.writeAdminMutationError(c, err, "error al crear usuario")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "usuario creado"})
}

func (h *AdminHandler) ResetTemporaryPassword(c *gin.Context) {
	username := c.Param("username")
	var req model.ResetTemporaryPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}
	if err := model.ValidateTemporaryPasswordBytes(req.TemporaryPassword); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.svc.ResetTemporaryPassword(c.Request.Context(), adminActorFromCtx(c), username, req); err != nil {
		h.writeAdminMutationError(c, err, "error al resetear contraseña")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "contraseña temporal actualizada"})
}

func (h *AdminHandler) Activate(c *gin.Context) {
	username := c.Param("username")
	if err := h.svc.Activate(c.Request.Context(), adminActorFromCtx(c), username); err != nil {
		h.writeAdminMutationError(c, err, "error al activar usuario")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "usuario activado"})
}

// SetLevel maneja POST /admin/users/:username/level.
// Cambia el nivel de acceso de un usuario.
//
// Posibles respuestas:
//   - 200: nivel cambiado exitosamente
//   - 400: body inválido (falta "level" o nivel no válido)
//   - 403: el admin intenta cambiar su propio nivel (no permitido)
//   - 404: usuario no encontrado
//   - 409: límite de admins alcanzado
//   - 500: error de servidor
func (h *AdminHandler) SetLevel(c *gin.Context) {
	username := c.Param("username")

	// Prevención de auto-modificación: un admin no puede cambiar su propio nivel.
	// Esto evita que un admin se quite permisos por accidente y que se usen
	// las rutas de admin como bypass de la lógica de negocio.
	if claims := claimsFromCtx(c); claims != nil && claims.Username == username {
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "no puedes cambiar tu propio nivel"})
		return
	}

	var req model.SetLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.svc.SetLevel(c.Request.Context(), adminActorFromCtx(c), username, req.Level); err != nil {
		switch {
		case errors.Is(err, service.ErrSelfAdminMutation):
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "usuario no encontrado"})
		case errors.Is(err, service.ErrMaxAdminsReached):
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: err.Error()})
		case errors.Is(err, service.ErrInsufficientAdmins):
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al cambiar nivel"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "nivel actualizado"})
}

// GrantAdmin maneja POST /admin/users/:username/grant-admin.
// Asciende a un usuario a nivel admin. Es idempotente: si ya es admin, devuelve 200.
//
// Posibles respuestas:
//   - 200: ascendido (o ya era admin — idempotente)
//   - 403: el admin intenta ascenderse a sí mismo
//   - 404: usuario no encontrado
//   - 409: límite de 3 admins alcanzado
//   - 500: error de servidor
func (h *AdminHandler) GrantAdmin(c *gin.Context) {
	username := c.Param("username")

	if claims := claimsFromCtx(c); claims != nil && claims.Username == username {
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "no puedes cambiar tu propio nivel"})
		return
	}

	if err := h.svc.GrantAdmin(c.Request.Context(), adminActorFromCtx(c), username); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "usuario no encontrado"})
		case errors.Is(err, service.ErrMaxAdminsReached):
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al ascender usuario"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "usuario ascendido a admin"})
}

// Deactivate maneja POST /admin/users/:username/deactivate.
// Desactiva un usuario (is_active = false). No borra el registro.
func (h *AdminHandler) Deactivate(c *gin.Context) {
	username := c.Param("username")

	if claims := claimsFromCtx(c); claims != nil && claims.Username == username {
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "cannot deactivate your own account"})
		return
	}

	if err := h.svc.Deactivate(c.Request.Context(), adminActorFromCtx(c), username); err != nil {
		switch {
		case errors.Is(err, service.ErrSelfAdminMutation):
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "usuario no encontrado"})
		case errors.Is(err, service.ErrInsufficientAdmins):
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al desactivar usuario"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "usuario desactivado"})
}

func (h *AdminHandler) writeAdminMutationError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, model.ErrTemporaryPasswordTooLong):
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrSelfAdminMutation):
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "usuario no encontrado"})
	case errors.Is(err, repository.ErrConflict), errors.Is(err, service.ErrMaxAdminsReached), errors.Is(err, service.ErrInsufficientAdmins):
		c.JSON(http.StatusConflict, model.ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: fallback})
	}
}

// GetStats maneja GET /admin/stats.
// Devuelve estadísticas agregadas del sistema (usuarios y memorias).
func (h *AdminHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al obtener estadísticas"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// claimsFromCtx — definido en memory.go (mismo paquete handler, sin duplicar)

func adminActorFromCtx(c *gin.Context) model.AdminActor {
	claims := claimsFromCtx(c)
	if claims == nil {
		return model.AdminActor{}
	}
	return model.AdminActor{UserID: claims.Subject, Username: claims.Username}
}

func auditFilterFromQuery(c *gin.Context) (model.AuditFilter, error) {
	filter := model.AuditFilter{}
	if value := c.Query("project"); value != "" {
		filter.Project = &value
	}
	if value := c.Query("actor_user_id"); value != "" {
		if _, err := uuid.Parse(value); err != nil {
			return model.AuditFilter{}, errors.New("actor_user_id debe ser un UUID válido")
		}
		filter.ActorUserID = &value
	}
	if value := c.Query("action"); value != "" {
		action := model.AuditAction(value)
		filter.Action = &action
	}
	if value := c.Query("outcome"); value != "" {
		outcome := model.AuditOutcome(value)
		filter.Outcome = &outcome
	}
	since, err := parseAuditTimeQuery(c, "since")
	if err != nil {
		return model.AuditFilter{}, err
	}
	filter.Since = since
	until, err := parseAuditTimeQuery(c, "until")
	if err != nil {
		return model.AuditFilter{}, err
	}
	filter.Until = until
	if since != nil && until != nil && since.After(*until) {
		return model.AuditFilter{}, errors.New("since debe ser anterior o igual a until")
	}
	limit, err := parseAuditIntQuery(c, "limit")
	if err != nil {
		return model.AuditFilter{}, err
	}
	filter.Limit = limit
	offset, err := parseAuditIntQuery(c, "offset")
	if err != nil {
		return model.AuditFilter{}, err
	}
	filter.Offset = offset
	return filter.Normalize(), nil
}

func parseAuditTimeQuery(c *gin.Context, key string) (*time.Time, error) {
	value := c.Query(key)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, errors.New(key + " debe tener formato RFC3339")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func parseAuditIntQuery(c *gin.Context, key string) (int, error) {
	value := c.Query(key)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New(key + " debe ser un entero")
	}
	return parsed, nil
}
