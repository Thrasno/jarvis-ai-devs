// Package main contiene el punto de entrada del servidor hive-api.
//
// Los tests de main son intencionales: verificamos que la función buildApp
// (que construye el router sin iniciar el servidor) funciona correctamente
// en un entorno controlado. No podemos testear main() directamente porque
// llama a log.Fatal (terminaría el proceso de test), pero sí podemos testear
// todo lo que hace main() antes de esa llamada.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildApp_NonNil verifica que buildApp devuelva un router válido
// cuando se le pasan mocks de dependencias.
func TestBuildApp_NonNil(t *testing.T) {
	app := buildApp(buildAppDeps{
		authSvc:   &mockAuth{},
		memorySvc: &mockMemory{},
		syncSvc:   &mockSync{},
		adminSvc:  &mockAdmin{},
	})
	require.NotNil(t, app)
}

// TestBuildApp_HealthEndpoint verifica que el router construido
// responda 200 en GET /health sin necesitar base de datos.
func TestBuildApp_HealthEndpoint(t *testing.T) {
	app := buildApp(buildAppDeps{
		authSvc:   &mockAuth{},
		memorySvc: &mockMemory{},
		syncSvc:   &mockSync{},
		adminSvc:  &mockAdmin{},
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	require.NoError(t, err)

	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
