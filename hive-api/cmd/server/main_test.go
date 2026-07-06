// Package main contiene el punto de entrada del servidor hive-api.
//
// Los tests de main son intencionales: verificamos que la función buildApp
// (que construye el router sin iniciar el servidor) funciona correctamente
// en un entorno controlado. No podemos testear main() directamente porque
// llama a log.Fatal (terminaría el proceso de test), pero sí podemos testear
// todo lo que hace main() antes de esa llamada.
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/handler"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestBuildApp_NonNil verifica que buildApp devuelva un router válido
// cuando se le pasan mocks de dependencias.
func TestBuildApp_NonNil(t *testing.T) {
	app := buildApp(buildAppDeps{
		authSvc:              &mockAuth{},
		memorySvc:            &mockMemory{},
		syncSvc:              &mockSync{},
		projectSvc:           &mockProject{},
		projectGovernanceSvc: &mockProjectGovernance{},
		adminSvc:             &mockAdmin{},
		overviewSvc:          &mockOverview{},
	})
	require.NotNil(t, app)
}

// TestBuildApp_HealthEndpoint verifica que el router construido
// responda 200 en GET /health sin necesitar base de datos.
func TestBuildApp_HealthEndpoint(t *testing.T) {
	app := buildApp(buildAppDeps{
		authSvc:              &mockAuth{},
		memorySvc:            &mockMemory{},
		syncSvc:              &mockSync{},
		projectSvc:           &mockProject{},
		projectGovernanceSvc: &mockProjectGovernance{},
		adminSvc:             &mockAdmin{},
		overviewSvc:          &mockOverview{},
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
	factories.newProjectBlockRepo = func(*pgxpool.Pool) repository.ProjectBlockRepository { return nil }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(repository.MemoryRepository, repository.SessionRepository, repository.ProjectBlockRepository, repository.TxManager) handler.MemoryService {
		return &mockMemory{}
	}
	factories.newSyncService = func(_ repository.MemoryRepository, _ repository.PromptRepository, _ repository.SessionRepository, got repository.AuditRepository, _ repository.ProjectBlockRepository, _ repository.TxManager) handler.SyncService {
		require.Same(t, auditRepo, got)
		return &mockSync{}
	}
	factories.newProjectGovernanceService = func(repository.ProjectBlockRepository, repository.AuditRepository, repository.TxManager) handler.ProjectGovernanceService {
		return &mockProjectGovernance{}
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

func TestWireServices_ProjectBlockAckWiredWhenAdminMutationDisabledByDefault(t *testing.T) {
	called := false
	factories := defaultServiceFactories()
	factories.newUserRepo = func(*pgxpool.Pool) repository.UserRepository { return nil }
	factories.newMemoryRepo = func(*pgxpool.Pool) repository.MemoryRepository { return nil }
	factories.newPromptRepo = func(*pgxpool.Pool) repository.PromptRepository { return nil }
	factories.newSessionRepo = func(*pgxpool.Pool) repository.SessionRepository { return nil }
	factories.newAuditRepo = func(*pgxpool.Pool) repository.AuditRepository { return nil }
	factories.newSyncAttemptRepo = func(*pgxpool.Pool) repository.SyncAttemptRepository { return nil }
	factories.newProjectRepo = func(*pgxpool.Pool) repository.ProjectRepository { return nil }
	factories.newProjectBlockRepo = func(*pgxpool.Pool) repository.ProjectBlockRepository { return nil }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(repository.MemoryRepository, repository.SessionRepository, repository.ProjectBlockRepository, repository.TxManager) handler.MemoryService {
		return &mockMemory{}
	}
	factories.newSyncService = func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository, repository.ProjectBlockRepository, repository.TxManager) handler.SyncService {
		return &mockSync{}
	}
	factories.newSyncAttemptService = func(repository.SyncAttemptRepository) handler.SyncAttemptService { return nil }
	factories.newProjectService = func(repository.ProjectRepository) handler.ProjectService { return &mockProject{} }
	factories.newProjectGovernanceService = func(repository.ProjectBlockRepository, repository.AuditRepository, repository.TxManager) handler.ProjectGovernanceService {
		called = true
		return &mockProjectGovernance{}
	}
	factories.newAdminService = func(repository.UserRepository, repository.MemoryRepository, repository.AuditRepository, repository.TxManager) handler.AdminService {
		return &mockAdmin{}
	}
	factories.newOverviewService = func(repository.MemoryRepository, repository.SyncAttemptRepository, repository.AuditRepository) handler.OverviewService {
		return &mockOverview{}
	}
	factories.newActivityService = func(repository.MemoryRepository) handler.ActivityService { return &mockActivity{} }

	deps := wireServicesWithFactories(nil, &config.Config{}, factories)

	assert.True(t, called)
	require.NotNil(t, deps.projectGovernanceSvc)
	assert.False(t, deps.projectBlockAdminEnabled)
}

func TestBuildApp_ProjectBlockAckRouteReachableWhenAdminMutationDisabled(t *testing.T) {
	authSvc := &mockAuth{}
	authSvc.On("ValidateToken", "valid-token").Return(&model.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "daemon-user"}, Username: "daemon", Level: model.LevelMember, DaemonID: "daemon-1", Client: "hive-daemon"}, nil)
	ackAt := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	govSvc := &mockProjectGovernance{}
	govSvc.On("Acknowledge", mock.Anything, model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AckSubject: model.ProjectBlockAckSubject{AuthSubject: "daemon-user", DaemonID: "daemon-1", Client: "hive-daemon"}}).
		Return(model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AppliedAt: ackAt}, nil)
	app := buildApp(buildAppDeps{
		authSvc:                  authSvc,
		memorySvc:                &mockMemory{},
		syncSvc:                  &mockSync{},
		projectSvc:               &mockProject{},
		projectGovernanceSvc:     govSvc,
		projectBlockAdminEnabled: false,
		adminSvc:                 &mockAdmin{},
		overviewSvc:              &mockOverview{},
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/project-blocks/ack", bytes.NewBufferString(`{"command_id":"cmd-1","canonical_project_key":"jarvis-dev","ack_token":"ack-token-1","status":"applied"}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	authSvc.AssertExpectations(t)
	govSvc.AssertExpectations(t)
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
	factories.newProjectBlockRepo = func(*pgxpool.Pool) repository.ProjectBlockRepository { return nil }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(repository.MemoryRepository, repository.SessionRepository, repository.ProjectBlockRepository, repository.TxManager) handler.MemoryService {
		return &mockMemory{}
	}
	factories.newSyncService = func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository, repository.ProjectBlockRepository, repository.TxManager) handler.SyncService {
		return &mockSync{}
	}
	factories.newSyncAttemptService = func(repository.SyncAttemptRepository) handler.SyncAttemptService { return nil }
	factories.newProjectService = func(got repository.ProjectRepository) handler.ProjectService {
		require.Same(t, projectRepo, got)
		return projectSvc
	}
	factories.newProjectGovernanceService = func(repository.ProjectBlockRepository, repository.AuditRepository, repository.TxManager) handler.ProjectGovernanceService {
		return &mockProjectGovernance{}
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
	factories.newProjectBlockRepo = func(*pgxpool.Pool) repository.ProjectBlockRepository { return nil }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAuthService = func(repository.UserRepository, string) handler.AuthService { return &mockAuth{} }
	factories.newMemoryService = func(got repository.MemoryRepository, _ repository.SessionRepository, _ repository.ProjectBlockRepository, _ repository.TxManager) handler.MemoryService {
		require.Same(t, memRepo, got)
		return &mockMemory{}
	}
	factories.newSyncService = func(repository.MemoryRepository, repository.PromptRepository, repository.SessionRepository, repository.AuditRepository, repository.ProjectBlockRepository, repository.TxManager) handler.SyncService {
		return &mockSync{}
	}
	factories.newSyncAttemptService = func(repository.SyncAttemptRepository) handler.SyncAttemptService { return nil }
	factories.newProjectService = func(repository.ProjectRepository) handler.ProjectService { return &mockProject{} }
	factories.newAdminService = func(repository.UserRepository, repository.MemoryRepository, repository.AuditRepository, repository.TxManager) handler.AdminService {
		return &mockAdmin{}
	}
	factories.newProjectGovernanceService = func(repository.ProjectBlockRepository, repository.AuditRepository, repository.TxManager) handler.ProjectGovernanceService {
		return &mockProjectGovernance{}
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

	require.Len(t, startupMigrations, 13)
	assert.Equal(t, migrations.InitialSQL, startupMigrations[0])
	assert.Equal(t, migrations.ActivityFeedIndexSQL, startupMigrations[7])
	assert.Equal(t, migrations.MemoryDiscoveryIndexesSQL, startupMigrations[8])
	assert.Contains(t, migrations.MemoryDiscoveryIndexesSQL, "created_at DESC, synced_at DESC, id DESC")
}

func TestStartupMigrationSQLIncludesPullCursorIndexesAfterDiscoveryIndexes(t *testing.T) {
	startupMigrations := startupMigrationSQL()

	require.Len(t, startupMigrations, 13)
	assert.Equal(t, migrations.PullCursorIndexesSQL, startupMigrations[9])
	assert.Contains(t, migrations.PullCursorIndexesSQL, "idx_memories_synced_at_sync_id")
	assert.Contains(t, migrations.PullCursorIndexesSQL, "idx_sessions_synced_at_sync_id")
}

func TestStartupMigrationSQLIncludesProjectScopedPullCursorIndexesAfterLegacyPullCursorIndexes(t *testing.T) {
	startupMigrations := startupMigrationSQL()

	require.Len(t, startupMigrations, 13)
	assert.Equal(t, migrations.ProjectScopedPullCursorIndexesSQL, startupMigrations[10])
	assert.Contains(t, migrations.ProjectScopedPullCursorIndexesSQL, "idx_memories_project_synced_at_sync_id")
	assert.Contains(t, migrations.ProjectScopedPullCursorIndexesSQL, "idx_sessions_project_synced_at_sync_id")
	assert.Contains(t, migrations.ProjectScopedPullCursorIndexesSQL, "DROP INDEX IF EXISTS idx_memories_synced_at")
}

func TestStartupMigrationSQLIncludesProjectBlockAckSubjectsAfterProjectBlocks(t *testing.T) {
	startupMigrations := startupMigrationSQL()

	require.Len(t, startupMigrations, 13)
	assert.Equal(t, migrations.ProjectBlocksSQL, startupMigrations[11])
	assert.Equal(t, migrations.ProjectBlockAckSubjectsSQL, startupMigrations[12])
}
