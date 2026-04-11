package middleware

import (
	"net/http"
	"strings"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
)

// claimsKey es la clave usada para guardar los Claims en el contexto de Gin.
// Los handlers la usan para recuperar quién está autenticado:
//
//	claims, _ := c.Get(ClaimsKey)
//	userClaims := claims.(*model.Claims)
//
// Es una constante exportada para que los handlers puedan usarla sin hardcodear el string.
const ClaimsKey = "claims"

// TokenValidator es la interfaz mínima que RequireAuth necesita del AuthService.
// Definirla aquí (en lugar de importar service.AuthService) evita ciclos de importación:
// el paquete handler importa middleware, y ambos importan service — si middleware
// importara service también, Go lo permitiría, pero es una dependencia innecesaria.
// Con esta interfaz local, middleware es independiente de service.
type TokenValidator interface {
	ValidateToken(tokenString string) (*model.Claims, error)
}

// RequireAuth devuelve un middleware que verifica el JWT en el header Authorization.
//
// Flujo:
//  1. Leer el header "Authorization: Bearer <token>"
//  2. Extraer el token (todo lo que va después de "Bearer ")
//  3. Validar el token con el AuthService
//  4. Si es válido → guardar los Claims en el contexto y continuar (c.Next())
//  5. Si no es válido → responder 401 y abortar (c.Abort())
//
// c.Abort() es crucial: sin él, Gin seguiría ejecutando los handlers siguientes
// aunque hayamos respondido. Abort() detiene la cadena de middlewares/handlers.
func RequireAuth(svc TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header requerido"})
			c.Abort()
			return
		}

		// El header debe tener el formato exacto "Bearer <token>"
		// strings.Cut divide el string en dos partes en la primera ocurrencia del separador.
		// Es más claro que Split o HasPrefix + TrimPrefix.
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "formato de autorización inválido"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, prefix)
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token vacío"})
			c.Abort()
			return
		}

		claims, err := svc.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token inválido o expirado"})
			c.Abort()
			return
		}

		// Guardamos los Claims en el contexto para que los handlers los lean.
		// El contexto de Gin es un mapa key→value que viaja con la request.
		c.Set(ClaimsKey, claims)
		c.Next()
	}
}
