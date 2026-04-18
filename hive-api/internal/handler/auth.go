package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler maneja los endpoints de autenticación.
type AuthHandler struct {
	svc AuthService
}

// NewAuthHandler crea un AuthHandler con el servicio inyectado.
func NewAuthHandler(svc AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login maneja POST /auth/login.
//
// Flujo:
//  1. Bind del body JSON al LoginRequest
//  2. Llamar a AuthService.Login
//  3. Si ok → 200 con el token
//  4. Si usuario inactivo → 403 (distinto de 401: el usuario existe pero está bloqueado)
//  5. Si credenciales inválidas → 401
//  6. Si error de servidor → 500
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	token, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		// Distinguimos los errores de dominio para mapear al código HTTP correcto.
		// ErrUserInactive → 403: el usuario existe pero está desactivado (no es un problema de credenciales).
		// ErrInvalidCredentials → 401: email/password no coinciden.
		// Resto → 500: fallo interno (DB/JWT/etc).
		if errors.Is(err, service.ErrUserInactive) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: err.Error()})
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	claims, err := h.svc.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	user, err := h.svc.GetCurrentUser(c.Request.Context(), claims.Subject)
	if err != nil {
		if errors.Is(err, service.ErrUserInactive) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	const jwtTTL = 30 * 24 * time.Hour // debe coincidir con el TTL del service
	c.JSON(http.StatusOK, model.LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(jwtTTL),
		User: model.UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Level:     user.Level,
			IsActive:  user.IsActive,
			CreatedAt: user.CreatedAt,
		},
	})
}

// Me maneja GET /auth/me — devuelve los datos frescos del usuario autenticado.
//
// A diferencia del approach de solo-Claims (que usa los datos del token, potencialmente
// desactualizados), aquí consultamos la BD para verificar que el usuario sigue activo
// y devolver su nivel actual — importante si un admin cambió su nivel entre requests.
func (h *AuthHandler) Me(c *gin.Context) {
	raw, exists := c.Get(middleware.ClaimsKey)
	if !exists {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	claims, ok := raw.(*model.Claims)
	if !ok {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	// Re-validamos contra la BD: si el usuario fue desactivado DESPUÉS de emitir el token,
	// GetCurrentUser devuelve ErrUserInactive y rechazamos el request.
	user, err := h.svc.GetCurrentUser(c.Request.Context(), claims.Subject)
	if err != nil {
		if errors.Is(err, service.ErrUserInactive) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "usuario inactivo"})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	c.JSON(http.StatusOK, model.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Level:     user.Level,
		IsActive:  user.IsActive,
		CreatedAt: user.CreatedAt,
	})
}
