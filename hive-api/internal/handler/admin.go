package handler

import (
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/gin-gonic/gin"
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

// SetLevel maneja POST /admin/users/:username/level.
// Cambia el nivel de acceso de un usuario.
//
// Posibles respuestas:
//   - 200: nivel cambiado exitosamente
//   - 400: body inválido (falta "level" o nivel no válido)
//   - 404: usuario no encontrado
//   - 409: límite de admins alcanzado
//   - 500: error de servidor
func (h *AdminHandler) SetLevel(c *gin.Context) {
	username := c.Param("username")

	var req model.SetLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.svc.SetLevel(c.Request.Context(), username, req.Level); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "usuario no encontrado"})
		case errors.Is(err, service.ErrMaxAdminsReached):
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al cambiar nivel"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "nivel actualizado"})
}

// Deactivate maneja POST /admin/users/:username/deactivate.
// Desactiva un usuario (is_active = false). No borra el registro.
func (h *AdminHandler) Deactivate(c *gin.Context) {
	username := c.Param("username")

	if err := h.svc.Deactivate(c.Request.Context(), username); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "usuario no encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "error al desactivar usuario"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "usuario desactivado"})
}
