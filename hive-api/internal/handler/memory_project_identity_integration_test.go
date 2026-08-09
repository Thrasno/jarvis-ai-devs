package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMemoryHandler_ProjectIdentityDistinguishesUnknownKnownEmptyAndGlobalQueries(t *testing.T) {
	pool, cleanup := startMemoryHandlerPostgres(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, repository.RegisterProjectIdentity(ctx, pool, "Known.Project", "", time.Now().UTC()))

	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	router := NewRouter(RouterDeps{
		AuthSvc: authSvc,
		MemorySvc: service.NewMemoryService(
			repository.NewPostgresMemoryRepository(pool),
			repository.NewPostgresSessionRepository(pool),
			repository.NewPostgresProjectBlockRepository(pool),
			repository.NewPostgresTxManager(pool),
		),
		SyncSvc:  &mockSyncSvc{},
		AdminSvc: &mockAdminSvc{},
	})

	for _, tc := range []struct {
		name string
		path string
		want int
	}{
		{name: "unknown list", path: "/memories?project=Ghost.Project", want: http.StatusNotFound},
		{name: "unknown search", path: "/memories/search?query=needle&project=ghost/project", want: http.StatusNotFound},
		{name: "known empty list", path: "/memories?project=KNOWN_project", want: http.StatusOK},
		{name: "known empty search", path: "/memories/search?query=needle&project=known/project", want: http.StatusOK},
		{name: "global list", path: "/memories", want: http.StatusOK},
		{name: "global search", path: "/memories/search?query=needle", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, req)

			require.Equal(t, tc.want, response.Code)
			if tc.want == http.StatusNotFound {
				require.JSONEq(t, `{"error":"project_unknown"}`, response.Body.String())
			}
		})
	}
	authSvc.AssertExpectations(t)
}

func TestMemorySyncRegistersNewProjectBeforeFilteredReadInSameServerLifetime(t *testing.T) {
	pool, cleanup := startMemoryHandlerPostgres(t)
	defer cleanup()
	ctx := context.Background()

	memoryRepo := repository.NewPostgresMemoryRepository(pool)
	sessionRepo := repository.NewPostgresSessionRepository(pool)
	blockRepo := repository.NewPostgresProjectBlockRepository(pool)
	tx := repository.NewPostgresTxManager(pool)
	syncService := service.NewSyncService(memoryRepo, repository.NewPostgresPromptRepository(pool), sessionRepo, repository.NewPostgresAuditRepository(pool), blockRepo, tx)
	memoryService := service.NewMemoryService(memoryRepo, sessionRepo, blockRepo, tx)
	now := time.Now().UTC()
	// The API no longer folds spellings together, so a session payload belongs to
	// the request project only when its spelling is byte-for-byte identical.
	_, err := syncService.Sync(ctx, model.SyncRequest{
		Project:  " Fresh.Project ",
		Sessions: []model.SyncSessionPayload{{ID: "session-fresh", SyncID: "10000000-0000-0000-0000-000000000001", Project: " Fresh.Project ", DevID: "dev", Client: "test", StartedAt: now}},
	}, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)

	_, _, err = memoryService.List(ctx, model.MemoryFilter{Project: "FRESH_project"})
	require.NoError(t, err)
	_, _, err = memoryService.Search(ctx, "needle", model.MemoryFilter{Project: "fresh-project"})
	require.NoError(t, err)
}

func TestMemoryCreateRegistersCanonicalProjectAndRollsBackOnWriteFailure(t *testing.T) {
	pool, cleanup := startMemoryHandlerPostgres(t)
	defer cleanup()
	ctx := context.Background()
	memoryRepo := repository.NewPostgresMemoryRepository(pool)
	sessionRepo := repository.NewPostgresSessionRepository(pool)
	blockRepo := repository.NewPostgresProjectBlockRepository(pool)
	tx := repository.NewPostgresTxManager(pool)
	memoryService := service.NewMemoryService(memoryRepo, sessionRepo, blockRepo, tx)
	syncService := service.NewSyncService(memoryRepo, repository.NewPostgresPromptRepository(pool), sessionRepo, repository.NewPostgresAuditRepository(pool), blockRepo, tx)
	now := time.Now().UTC()

	_, err := memoryService.Create(ctx, &model.Memory{SyncID: "20000000-0000-0000-0000-000000000001", Project: " Direct.Project ", Category: model.CatDecision, Title: "registered", Content: "content", CreatedBy: "user", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	_, err = memoryService.Create(ctx, &model.Memory{SyncID: "20000000-0000-0000-0000-000000000001", Project: "Ghost.Project", Category: model.CatDecision, CreatedBy: "user", CreatedAt: now, UpdatedAt: now})
	require.Error(t, err)
	pulled, err := syncService.PullAll(ctx, "direct-project", time.Time{}, nil, 10, model.PullCursor{}, model.PullCursor{})
	require.NoError(t, err)
	require.Len(t, pulled.Memories, 1)
	require.Equal(t, " Direct.Project ", pulled.Memories[0].Project, "pull retains the stored display spelling")

	var key, spelling string
	require.NoError(t, pool.QueryRow(ctx, `SELECT project_key, first_spelling FROM project_identities WHERE project_key = 'direct-project'`).Scan(&key, &spelling))
	require.Equal(t, "direct-project", key)
	require.Equal(t, " Direct.Project ", spelling)
	var ghost bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_identities WHERE project_key = 'ghost-project')`).Scan(&ghost))
	require.False(t, ghost)
	_, _, err = memoryService.List(ctx, model.MemoryFilter{Project: "direct/project"})
	require.False(t, errors.Is(err, service.ErrProjectUnknown))
}

func TestMemoryCreateRollsBackDomainWriteWhenIdentityRegistrationFails(t *testing.T) {
	pool, cleanup := startMemoryHandlerPostgres(t)
	defer cleanup()
	ctx := context.Background()
	memoryService := service.NewMemoryService(repository.NewPostgresMemoryRepository(pool), repository.NewPostgresSessionRepository(pool), repository.NewPostgresProjectBlockRepository(pool), repository.NewPostgresTxManager(pool))
	require.NoError(t, repository.RunMigrations(pool, `DROP TABLE project_identity_spellings`))
	now := time.Now().UTC()

	_, err := memoryService.Create(ctx, &model.Memory{SyncID: "30000000-0000-0000-0000-000000000001", Project: "Broken.Registry", Category: model.CatDecision, Title: "must rollback", Content: "content", CreatedBy: "user", CreatedAt: now, UpdatedAt: now})
	require.Error(t, err)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM memories WHERE project = 'Broken.Registry'`).Scan(&count))
	require.Zero(t, count)
}

func startMemoryHandlerPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)),
	)
	require.NoError(t, err)
	connectionString, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, connectionString)
	require.NoError(t, err)
	for _, sql := range []string{
		migrations.InitialSQL, migrations.UserPromptsSQL, migrations.SessionsSQL, migrations.AuditLogsSQL,
		migrations.MemoryMutationsSQL, migrations.DropTopicKeyUniqueConstraintSQL, migrations.SyncAttemptLogsSQL,
		migrations.ActivityFeedIndexSQL, migrations.MemoryDiscoveryIndexesSQL, migrations.PullCursorIndexesSQL,
		migrations.ProjectScopedPullCursorIndexesSQL, migrations.ProjectBlocksSQL, migrations.QuarantineContractSQL,
		migrations.DistributedQuarantineSQL, migrations.CanonicalProjectRegistrySQL,
	} {
		require.NoError(t, repository.RunMigrations(pool, sql))
	}
	return pool, func() {
		pool.Close()
		require.NoError(t, container.Terminate(ctx))
	}
}
