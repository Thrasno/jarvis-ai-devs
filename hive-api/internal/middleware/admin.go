package middleware

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

type CurrentUserProvider interface {
	GetCurrentUser(ctx context.Context, userID string) (*model.User, error)
}

// RequireAdmin devuelve un middleware que verifica que el usuario autenticado
// tenga nivel LevelAdmin.
//
// IMPORTANTE: RequireAdmin SIEMPRE debe ir después de RequireAuth en la cadena.
// RequireAuth inyecta los Claims en el contexto. Si RequireAdmin se usa sin
// RequireAuth, no habrá Claims y devolverá 500 (error de configuración).
//
// Diseño deliberado: devolver 500 (no 401) cuando faltan los Claims es
// una señal de error de programación, no de autenticación fallida.
// Un 401 sugiere "inicia sesión", pero el problema real es que el desarrollador
// olvidó poner RequireAuth antes de RequireAdmin.
func RequireAdmin(svc CurrentUserProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Intentamos obtener los Claims del contexto (puestos por RequireAuth).
		raw, exists := c.Get(ClaimsKey)
		if !exists {
			// No hay claims — RequireAuth no se ejecutó. Error de configuración.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			c.Abort()
			return
		}

		claims, ok := raw.(*model.Claims)
		if !ok || claims == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			c.Abort()
			return
		}

		if claims.Subject == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "acceso denegado: se requiere nivel admin"})
			c.Abort()
			return
		}

		if claims.Level != model.LevelAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "acceso denegado: se requiere nivel admin"})
			c.Abort()
			return
		}

		user, err := svc.GetCurrentUser(c.Request.Context(), claims.Subject)
		if err != nil {
			if errors.Is(err, service.ErrUserInactive) {
				c.JSON(http.StatusForbidden, gin.H{"error": "acceso denegado: se requiere nivel admin"})
				c.Abort()
				return
			}

			log.Printf("warn: admin current user lookup failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			c.Abort()
			return
		}

		if user == nil || !user.IsActive || user.Level != model.LevelAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "acceso denegado: se requiere nivel admin"})
			c.Abort()
			return
		}

		c.Next()
	}
}
