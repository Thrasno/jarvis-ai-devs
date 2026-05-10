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

	"github.com/Thrasno/jarvis-dev/hive-api/internal/config"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/handler"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestWireServices_InjectsAuditRepositoryIntoAdminAndSyncServices(t *testing.T) {
	auditRepo := &repository.MockAuditRepository{}
	factories := defaultServiceFactories()
	factories.newUserRepo = func(*pgxpool.Pool) repository.UserRepository { return nil }
	factories.newMemoryRepo = func(*pgxpool.Pool) repository.MemoryRepository { return nil }
	factories.newPromptRepo = func(*pgxpool.Pool) repository.PromptRepository { return nil }
	factories.newSessionRepo = func(*pgxpool.Pool) repository.SessionRepository { return nil }
	factories.newAuditRepo = func(*pgxpool.Pool) repository.AuditRepository { return auditRepo }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(repository.MemoryRepository, repository.SessionRepository) handler.MemoryService {
		return &mockMemory{}
	}
	factories.newSyncService = func(_ repository.MemoryRepository, _ repository.PromptRepository, _ repository.SessionRepository, got repository.AuditRepository) handler.SyncService {
		require.Same(t, auditRepo, got)
		return &mockSync{}
	}
	factories.newAdminService = func(_ repository.UserRepository, _ repository.MemoryRepository, got repository.AuditRepository, _ repository.TxManager) handler.AdminService {
		require.Same(t, auditRepo, got)
		return &mockAdmin{}
	}

	deps := wireServicesWithFactories(nil, &config.Config{AllowedOrigins: []string{"https://app.example"}}, factories)

	require.NotNil(t, deps.syncSvc)
	require.NotNil(t, deps.adminSvc)
	assert.Equal(t, []string{"https://app.example"}, deps.allowedOrigins)
}
