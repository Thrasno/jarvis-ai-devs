package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockAuthService es el doble de test para service.AuthService.
// Lo definimos aquí (en el paquete middleware) para evitar dependencias cíclicas:
// si importáramos service, y service importara middleware, Go rechazaría el ciclo.
// El middleware solo necesita la interfaz, no la implementación concreta.
type mockAuthService struct {
	mock.Mock
}

func (m *mockAuthService) Login(ctx interface{ Done() <-chan struct{} }, email, password string) (string, error) {
	args := m.Called(email, password)
	return args.String(0), args.Error(1)
}

func (m *mockAuthService) ValidateToken(tokenString string) (*model.Claims, error) {
	args := m.Called(tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Claims), args.Error(1)
}

// tokenValidator es la interfaz mínima que necesita RequireAuth.
// Extraer solo lo que necesitamos evita el import circular.
type tokenValidator interface {
	ValidateToken(tokenString string) (*model.Claims, error)
}

// helper: construye un router de test con RequireAuth y un handler protegido.
func newAuthRouter(svc tokenValidator) *gin.Engine {
	r := gin.New()
	r.Use(Recovery())
	r.GET("/protected", RequireAuth(svc), func(c *gin.Context) {
		claims, _ := c.Get("claims")
		c.JSON(http.StatusOK, gin.H{"user": claims.(*model.Claims).Username})
	})
	return r
}

// TestRequireAuth_MissingHeader verifica que una request sin Authorization devuelva 401.
func TestRequireAuth_MissingHeader(t *testing.T) {
	svc := &mockAuthService{}
	r := newAuthRouter(svc)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/protected", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "error")
	svc.AssertNotCalled(t, "ValidateToken")
}

// TestRequireAuth_MalformedHeader verifica que "Bearer" sin token devuelva 401.
func TestRequireAuth_MalformedHeader(t *testing.T) {
	svc := &mockAuthService{}
	r := newAuthRouter(svc)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/protected", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "NotBearer token")

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	svc.AssertNotCalled(t, "ValidateToken")
}

// TestRequireAuth_InvalidToken verifica que un token que falla validación devuelva 401.
func TestRequireAuth_InvalidToken(t *testing.T) {
	svc := &mockAuthService{}
	svc.On("ValidateToken", "bad-token").Return(nil, errors.New("token inválido"))

	r := newAuthRouter(svc)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/protected", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer bad-token")

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	svc.AssertExpectations(t)
}

// TestRequireAuth_ValidToken verifica que un token válido pase al handler
// con los claims inyectados en el contexto.
func TestRequireAuth_ValidToken(t *testing.T) {
	claims := &model.Claims{
		Username: "testuser",
		Level:    model.LevelMember,
	}
	svc := &mockAuthService{}
	svc.On("ValidateToken", "good-token").Return(claims, nil)

	r := newAuthRouter(svc)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/protected", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer good-token")

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"user":"testuser"}`, w.Body.String())
	svc.AssertExpectations(t)
}
