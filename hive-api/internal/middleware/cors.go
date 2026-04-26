package middleware

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// CORS devuelve un middleware que añade las cabeceras Access-Control necesarias
// para que el dashboard en hive.hivemem.dev pueda consumir la API.
//
// Solo se permite el origen si está en la lista allowedOrigins. Si el origen
// no está en la lista, la request continúa sin las cabeceras CORS — el browser
// la bloqueará, que es exactamente el comportamiento correcto.
//
// Las preflight requests (OPTIONS) se responden directamente con 204
// sin llegar a ningún handler de negocio.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		if slices.Contains(allowedOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
