package middleware

import (
	"net/http"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

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
func RequireAdmin() gin.HandlerFunc {
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

		// Verificamos el nivel de acceso.
		// Solo LevelAdmin puede acceder a los endpoints de administración.
		if claims.Level != model.LevelAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "acceso denegado: se requiere nivel admin"})
			c.Abort()
			return
		}

		c.Next()
	}
}
