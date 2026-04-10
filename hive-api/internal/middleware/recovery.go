package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Recovery devuelve un middleware que captura panics y responde con 500.
//
// ¿Por qué no usar gin.Recovery() directamente?
// Gin tiene su propio middleware de recovery, pero devuelve HTML y escribe el
// stack trace en el log. Nosotros queremos:
//   1. Siempre responder JSON (nunca HTML ni texto plano)
//   2. Nunca filtrar información interna (stack trace) al cliente
//   3. Usar el formato ErrorResponse estándar de esta API
//
// El stack trace sigue siendo útil en el servidor — lo imprimimos en stderr
// para que aparezca en los logs del sistema, pero NO en la respuesta HTTP.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		// Respondemos con el mensaje genérico — nunca el detalle del panic.
		// El cliente no necesita saber qué salió mal internamente.
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
	})
}
