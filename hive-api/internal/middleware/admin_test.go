package middleware

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCurrentUserProvider struct {
	user *model.User
	err  error
}

func (m mockCurrentUserProvider) GetCurrentUser(ctx context.Context, userID string) (*model.User, error) {
	return m.user, m.err
}

// helper: construye un router que simula la cadena completa:
// Recovery → RequireAuth (ya con claims inyectados) → RequireAdmin → handler
func newAdminRouter(claims *model.Claims) *gin.Engine {
	provider := mockCurrentUserProvider{user: &model.User{ID: "admin-1", Username: "adminuser", Level: model.LevelAdmin, IsActive: true}}
	return newAdminRouterWithProvider(claims, provider)
}

func newAdminRouterWithProvider(claims *model.Claims, provider CurrentUserProvider) *gin.Engine {
	r := gin.New()
	r.Use(Recovery())
	r.GET("/admin/only", func(c *gin.Context) {
		// Simulamos que RequireAuth ya puso los claims en el contexto
		if claims != nil {
			c.Set(ClaimsKey, claims)
		}
		c.Next()
	}, RequireAdmin(provider), func(c *gin.Context) {
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
		RegisteredClaims: jwtClaimsSubject("user-1"),
		Username:         "normaluser",
		Level:            model.LevelAdmin,
	}
	provider := mockCurrentUserProvider{user: &model.User{ID: "user-1", Username: "normaluser", Level: model.LevelMember, IsActive: true}}
	r := newAdminRouterWithProvider(claims, provider)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestRequireAdmin_InactiveCurrentUserRejected(t *testing.T) {
	claims := &model.Claims{
		RegisteredClaims: jwtClaimsSubject("admin-1"),
		Username:         "adminuser",
		Level:            model.LevelAdmin,
	}
	provider := mockCurrentUserProvider{err: service.ErrUserInactive}
	r := newAdminRouterWithProvider(claims, provider)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireAdmin_CurrentUserServiceFailureReturnsInternalServerError(t *testing.T) {
	var logs bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})

	claims := &model.Claims{
		RegisteredClaims: jwtClaimsSubject("admin-1"),
		Username:         "adminuser",
		Level:            model.LevelAdmin,
	}
	currentUserErr := errors.New("database unavailable")
	provider := mockCurrentUserProvider{err: currentUserErr}
	r := newAdminRouterWithProvider(claims, provider)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
	output := logs.String()
	assert.Contains(t, output, "warn: admin current user lookup failed")
	assert.Contains(t, output, currentUserErr.Error())
	assert.NotContains(t, output, claims.Username)
}

// TestRequireAdmin_IsAdmin verifica que un admin pueda acceder al endpoint.
func TestRequireAdmin_IsAdmin(t *testing.T) {
	claims := &model.Claims{
		RegisteredClaims: jwtClaimsSubject("admin-1"),
		Username:         "adminuser",
		Level:            model.LevelAdmin,
	}
	r := newAdminRouter(claims)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/admin/only", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"ok":true}`, w.Body.String())
}

func jwtClaimsSubject(subject string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{Subject: subject}
}
