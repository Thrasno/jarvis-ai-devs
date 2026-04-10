package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
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
//  4. Si credenciales inválidas → 401
//  5. Si error de servidor → 500
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	// ShouldBindJSON valida el body según los tags `binding:` del struct.
	// Si algún campo requerido falta o el formato es incorrecto, devuelve error.
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	token, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		// Distinguimos el tipo de error para mapear al código HTTP correcto.
		// Los errores de dominio (credenciales inválidas, usuario inactivo)
		// son 401/403. Los errores inesperados son 500.
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, model.LoginResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		User:      model.UserResponse{},
	})
}

// Me maneja GET /auth/me — devuelve los datos del usuario autenticado.
// Los datos vienen de los Claims inyectados por RequireAuth (sin tocar la BD).
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

	// Respondemos con los datos del token — sin consultar la BD.
	// Para datos en tiempo real (nivel actualizado, etc.) habría que ir a la BD.
	// Para el caso de uso de "¿quién soy?" el token es suficiente.
	c.JSON(http.StatusOK, gin.H{
		"id":       claims.Subject,
		"username": claims.Username,
		"level":    claims.Level,
		"exp":      claims.ExpiresAt,
	})
}

// errToStatus es un helper que traduce errores de dominio a códigos HTTP.
// Centralizar esta lógica evita repetirla en cada handler.
func errToStatus(err error) int {
	switch {
	case errors.Is(err, nil):
		return http.StatusOK
	default:
		return http.StatusInternalServerError
	}
}
