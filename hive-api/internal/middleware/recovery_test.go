// Package middleware contiene los middlewares HTTP de la aplicación.
//
// Un middleware en Gin es una función que se ejecuta ANTES (o DESPUÉS) del handler
// principal. La cadena es: request → middleware1 → middleware2 → handler → response.
// Cada middleware puede leer/modificar la request, responder directamente (cortocircuito),
// o ceder el control al siguiente llamando a c.Next().
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Silenciamos los logs de Gin en tests — no queremos output en el terminal.
	// gin.TestMode desactiva los logs de debug/warning que Gin imprime por defecto.
	gin.SetMode(gin.TestMode)
}

// TestRecovery_PanicReturns500 verifica que un panic en el handler
// quede capturado y devuelva 500 con el formato ErrorResponse.
//
// Sin Recovery, un panic en Go mata la goroutine y, en Gin, crashea el servidor.
// Con Recovery, el panic queda contenido y devolvemos un error controlado.
func TestRecovery_PanicReturns500(t *testing.T) {
	// Construimos un router de prueba con nuestro middleware
	r := gin.New()
	r.Use(Recovery())

	// Handler que lanza un panic intencionalmente
	r.GET("/panic", func(c *gin.Context) {
		panic("algo salió muy mal")
	})

	// httptest.NewRecorder captura la respuesta HTTP sin necesitar un servidor real.
	// Es el equivalente a un "mock" de la respuesta HTTP.
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/panic", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	// Verificaciones
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	// El cuerpo debe ser JSON con campo "error" — nunca el stack trace
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
}

// TestRecovery_NoPanicPassesThrough verifica que una request normal
// no sea afectada por el middleware de recovery.
func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(Recovery())

	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/ok", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}
