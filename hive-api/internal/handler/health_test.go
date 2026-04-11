package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestHealth_OK verifica que GET /health devuelva 200 cuando la BD responde.
func TestHealth_OK(t *testing.T) {
	db := &mockDBPinger{}
	// mock.Anything porque el handler pasa un context.WithTimeout, no context.Background()
	db.On("Ping", mock.Anything).Return(nil)

	r := NewRouter(RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
		DB:        db,
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status"`)
	assert.Contains(t, w.Body.String(), `"ok"`)
	db.AssertExpectations(t)
}

// TestHealth_DBDown verifica que GET /health devuelva 503 cuando la BD no responde.
func TestHealth_DBDown(t *testing.T) {
	db := &mockDBPinger{}
	db.On("Ping", mock.Anything).Return(errors.New("connection refused"))

	r := NewRouter(RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
		DB:        db,
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), `"degraded"`)
	assert.Contains(t, w.Body.String(), `"unreachable"`)
	db.AssertExpectations(t)
}

// TestHealth_NoDB verifica que GET /health devuelva 200 cuando no hay DB configurada (nil).
// Esto garantiza compatibilidad con tests unitarios que no inyectan DB.
func TestHealth_NoDB(t *testing.T) {
	r := NewRouter(RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
		DB:        nil,
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
