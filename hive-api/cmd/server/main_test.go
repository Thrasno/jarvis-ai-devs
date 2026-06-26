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

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/handler"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildApp_NonNil verifica que buildApp devuelva un router válido
// cuando se le pasan mocks de dependencias.
func TestBuildApp_NonNil(t *testing.T) {
	app := buildApp(buildAppDeps{
		authSvc:     &mockAuth{},
		memorySvc:   &mockMemory{},
		syncSvc:     &mockSync{},
		projectSvc:  &mockProject{},
		adminSvc:    &mockAdmin{},
		overviewSvc: &mockOverview{},
	})
	require.NotNil(t, app)
}

// TestBuildApp_HealthEndpoint verifica que el router construido
// responda 200 en GET /health sin necesitar base de datos.
func TestBuildApp_HealthEndpoint(t *testing.T) {
	app := buildApp(buildAppDeps{
		authSvc:     &mockAuth{},
		memorySvc:   &mockMemory{},
		syncSvc:     &mockSync{},
		projectSvc:  &mockProject{},
		adminSvc:    &mockAdmin{},
		overviewSvc: &mockOverview{},
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

	deps := wireServicesWithFactories(nil, &config.Config{
		AllowedOrigins:     []string{"https://app.example"},
		DashboardAssetsDir: "/app/dashboard",
	}, factories)

	require.NotNil(t, deps.syncSvc)
	require.NotNil(t, deps.adminSvc)
	assert.Equal(t, []string{"https://app.example"}, deps.allowedOrigins)
	assert.Equal(t, "/app/dashboard", deps.dashboardAssetsDir)
}

func TestWireServices_WiresProjectRepositoryIntoRouterDeps(t *testing.T) {
	projectRepo := &repository.MockProjectRepository{}
	projectSvc := &mockProject{}
	factories := defaultServiceFactories()
	factories.newUserRepo = func(*pgxpool.Pool) repository.UserRepository { return nil }
	factories.newMemoryRepo = func(*pgxpool.Pool) repository.MemoryRepository { return nil }
	factories.newPromptRepo = func(*pgxpool.Pool) repository.PromptRepository { return nil }
	factories.newSessionRepo = func(*pgxpool.Pool) repository.SessionRepository { return nil }
	factories.newAuditRepo = func(*pgxpool.Pool) repository.AuditRepository { return nil }
	factories.newSyncAttemptRepo = func(*pgxpool.Pool) repository.SyncAttemptRepository { return nil }
	factories.newProjectRepo = func(*pgxpool.Pool) repository.ProjectRepository { return projectRepo }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(repository.MemoryRepository, repository.SessionRepository) handler.MemoryService {
		return &mockMemory{}
	}
	factories.newSyncService = func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository) handler.SyncService {
		return &mockSync{}
	}
	factories.newSyncAttemptService = func(repository.SyncAttemptRepository) handler.SyncAttemptService { return nil }
	factories.newProjectService = func(got repository.ProjectRepository) handler.ProjectService {
		require.Same(t, projectRepo, got)
		return projectSvc
	}
	factories.newAdminService = func(repository.UserRepository, repository.MemoryRepository, repository.AuditRepository, repository.TxManager) handler.AdminService {
		return &mockAdmin{}
	}

	deps := wireServicesWithFactories(nil, &config.Config{}, factories)

	require.Same(t, projectSvc, deps.projectSvc)
}

func TestWireServices_WiresActivityServiceFromMemoryRepository(t *testing.T) {
	memRepo := &repository.MockMemoryRepository{}
	activitySvc := &mockActivity{}
	factories := defaultServiceFactories()
	factories.newUserRepo = func(*pgxpool.Pool) repository.UserRepository { return nil }
	factories.newMemoryRepo = func(*pgxpool.Pool) repository.MemoryRepository { return memRepo }
	factories.newPromptRepo = func(*pgxpool.Pool) repository.PromptRepository { return nil }
	factories.newSessionRepo = func(*pgxpool.Pool) repository.SessionRepository { return nil }
	factories.newAuditRepo = func(*pgxpool.Pool) repository.AuditRepository { return nil }
	factories.newSyncAttemptRepo = func(*pgxpool.Pool) repository.SyncAttemptRepository { return nil }
	factories.newProjectRepo = func(*pgxpool.Pool) repository.ProjectRepository { return nil }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(got repository.MemoryRepository, _ repository.SessionRepository) handler.MemoryService {
		require.Same(t, memRepo, got)
		return &mockMemory{}
	}
	factories.newSyncService = func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository) handler.SyncService {
		return &mockSync{}
	}
	factories.newSyncAttemptService = func(repository.SyncAttemptRepository) handler.SyncAttemptService { return nil }
	factories.newProjectService = func(repository.ProjectRepository) handler.ProjectService { return &mockProject{} }
	factories.newAdminService = func(repository.UserRepository, repository.MemoryRepository, repository.AuditRepository, repository.TxManager) handler.AdminService {
		return &mockAdmin{}
	}
	factories.newOverviewService = func(repository.MemoryRepository, repository.SyncAttemptRepository, repository.AuditRepository) handler.OverviewService {
		return &mockOverview{}
	}
	factories.newActivityService = func(got repository.MemoryRepository) handler.ActivityService {
		require.Same(t, memRepo, got)
		return activitySvc
	}

	deps := wireServicesWithFactories(nil, &config.Config{}, factories)

	require.Same(t, activitySvc, deps.activitySvc)
}

func TestStartupMigrationSQLIncludesDiscoveryIndexesAfterActivityFeedIndex(t *testing.T) {
	startupMigrations := startupMigrationSQL()

	require.Len(t, startupMigrations, 9)
	assert.Equal(t, migrations.InitialSQL, startupMigrations[0])
	assert.Equal(t, migrations.ActivityFeedIndexSQL, startupMigrations[7])
	assert.Equal(t, migrations.MemoryDiscoveryIndexesSQL, startupMigrations[8])
}
