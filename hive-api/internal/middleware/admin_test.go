package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: construye un router que simula la cadena completa:
// Recovery → RequireAuth (ya con claims inyectados) → RequireAdmin → handler
func newAdminRouter(claims *model.Claims) *gin.Engine {
	r := gin.New()
	r.Use(Recovery())
	r.GET("/admin/only", func(c *gin.Context) {
		// Simulamos que RequireAuth ya puso los claims en el contexto
		if claims != nil {
			c.Set(ClaimsKey, claims)
		}
		c.Next()
	}, RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// TestRequireAdmin_NoClaims verifica que si no hay claims en el contexto
// (RequireAuth no se ejecutó antes, o fue omitido) devuelva 500.
// Esto es un error de configuración — el desarrollador olvidó poner RequireAuth antes.
func TestRequireAdmin_NoClaims(t *testing.T) {
	r := newAdminRouter(nil) // nil = sin claims

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestRequireAdmin_NotAdmin verifica que un usuario con nivel insuficiente reciba 403.
func TestRequireAdmin_NotAdmin(t *testing.T) {
	claims := &model.Claims{
		Username: "normaluser",
		Level:    model.LevelMember,
	}
	r := newAdminRouter(claims)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

// TestRequireAdmin_IsAdmin verifica que un admin pueda acceder al endpoint.
func TestRequireAdmin_IsAdmin(t *testing.T) {
	claims := &model.Claims{
		Username: "adminuser",
		Level:    model.LevelAdmin,
	}
	r := newAdminRouter(claims)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"ok":true}`, w.Body.String())
}
