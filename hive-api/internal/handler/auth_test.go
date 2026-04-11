package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: configura router y hace una request, devuelve el recorder
func doRequest(t *testing.T, deps RouterDeps, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := NewRouter(deps)
	w := httptest.NewRecorder()

	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, path, reqBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

// helper: configura router y hace una request autenticada
func doAuthRequest(t *testing.T, deps RouterDeps, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := NewRouter(deps)
	w := httptest.NewRecorder()

	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, path, reqBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("Login", context.Background(), "user@test.com", "password123").
		Return("jwt-token-abc", nil)

	deps := RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	w := doRequest(t, deps, http.MethodPost, "/auth/login", map[string]string{
		"email":    "user@test.com",
		"password": "password123",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "jwt-token-abc", resp["token"])
	authSvc.AssertExpectations(t)
}

func TestLogin_InvalidBody(t *testing.T) {
	deps := RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	// Body vacío — faltan campos requeridos
	w := doRequest(t, deps, http.MethodPost, "/auth/login", map[string]string{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("Login", context.Background(), "bad@test.com", "wrongpass").
		Return("", errors.New("credenciales inválidas"))

	deps := RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	w := doRequest(t, deps, http.MethodPost, "/auth/login", map[string]string{
		"email":    "bad@test.com",
		"password": "wrongpass",
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	authSvc.AssertExpectations(t)
}

// TestLogin_InactiveUser verifica que un usuario inactivo reciba 403 (no 401).
// 403 Forbidden indica que las credenciales son válidas pero el acceso está bloqueado.
func TestLogin_InactiveUser(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("Login", context.Background(), "inactive@test.com", "password123").
		Return("", service.ErrUserInactive)

	deps := RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	w := doRequest(t, deps, http.MethodPost, "/auth/login", map[string]string{
		"email":    "inactive@test.com",
		"password": "password123",
	})

	assert.Equal(t, http.StatusForbidden, w.Code)
	authSvc.AssertExpectations(t)
}

// --- Me tests ---

func TestMe_Unauthorized(t *testing.T) {
	deps := RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	w := doRequest(t, deps, http.MethodGet, "/auth/me", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMe_Success(t *testing.T) {
	claims := &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username: "testuser",
		Level:    model.LevelMember,
	}

	user := &model.User{
		ID:       "user-uuid-123",
		Username: "testuser",
		Email:    "testuser@test.com",
		Level:    model.LevelMember,
		IsActive: true,
	}

	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(claims, nil)
	authSvc.On("GetCurrentUser", context.Background(), "user-uuid-123").Return(user, nil)

	deps := RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	w := doAuthRequest(t, deps, http.MethodGet, "/auth/me", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "testuser", resp["username"])
	authSvc.AssertExpectations(t)
}

// TestMe_InactiveUser verifica que un usuario desactivado post-login recibe 403 en GET /me.
func TestMe_InactiveUser(t *testing.T) {
	claims := &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username: "testuser",
		Level:    model.LevelMember,
	}

	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(claims, nil)
	authSvc.On("GetCurrentUser", context.Background(), "user-uuid-123").Return(nil, service.ErrUserInactive)

	deps := RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}

	w := doAuthRequest(t, deps, http.MethodGet, "/auth/me", nil, "valid-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
	authSvc.AssertExpectations(t)
}
