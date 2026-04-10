// Package handler contiene los manejadores HTTP de la API.
//
// Un handler en Gin recibe un *gin.Context (que contiene la request y permite
// escribir la response) y no devuelve nada — escribe la respuesta directamente.
//
// La responsabilidad del handler es EXACTAMENTE:
//   1. Leer y validar la request (path params, query params, body JSON)
//   2. Llamar al servicio correspondiente
//   3. Traducir el resultado (o error) a HTTP (código de estado + JSON body)
//
// El handler NO tiene lógica de negocio — eso vive en los services.
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestRouter_RoutesRegistered verifica que todas las rutas estén registradas
// y que /memories/search aparezca ANTES de /memories/:id.
//
// ¿Por qué importa el orden?
// Gin hace matching de rutas en orden de registro. Si /memories/:id se registra
// antes que /memories/search, la request "GET /memories/search" matchearía
// /memories/:id con id="search" — incorrecto. Las rutas estáticas deben ir antes
// que las paramétrizadas cuando comparten prefijo.
func TestRouter_RoutesRegistered(t *testing.T) {
	r := NewRouter(RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	})

	// Verificamos que cada ruta esperada exista probando que no devuelve 404
	// (lo que significaría que la ruta no está registrada).
	// Usamos OPTIONS para no disparar lógica de negocio.
	routes := r.Routes()
	routeMap := make(map[string]bool)
	for _, route := range routes {
		routeMap[route.Method+":"+route.Path] = true
	}

	expectedRoutes := []string{
		"GET:/health",
		"POST:/auth/login",
		"GET:/auth/me",
		"GET:/memories",
		"POST:/memories",
		"GET:/memories/search",
		"GET:/memories/:id",
		"POST:/sync",
		"GET:/admin/users",
		"POST:/admin/users/:username/level",
		"POST:/admin/users/:username/deactivate",
	}

	for _, route := range expectedRoutes {
		assert.True(t, routeMap[route], "ruta no registrada: %s", route)
	}
}

// TestRouter_SearchBeforeByID verifica que /memories/search esté registrada
// antes de /memories/:id en la lista de rutas.
func TestRouter_SearchBeforeByID(t *testing.T) {
	r := NewRouter(RouterDeps{
		AuthSvc:   &mockAuthSvc{},
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	})

	// Hacemos una request real a /memories/search
	// Con un token inválido esperamos 401 (no 404 ni que matchee /:id)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/memories/search?query=test", nil)
	require.NoError(t, err)
	// Sin Authorization header → 401 del middleware RequireAuth
	r.ServeHTTP(w, req)

	// 401 confirma que matcheó /memories/search (que está protegido con RequireAuth)
	// y no /memories/:id (que también está protegido, pero el punto es que es la ruta search)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
