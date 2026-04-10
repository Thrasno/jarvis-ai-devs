package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealth_OK verifica que GET /health devuelva 200 con status "ok".
//
// Este endpoint es el más simple — no requiere autenticación ni base de datos.
// Es el que usan los load balancers y sistemas de monitoreo para saber si
// el servidor está vivo (liveness probe en Kubernetes).
func TestHealth_OK(t *testing.T) {
	r := NewRouter(RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status"`)
}
