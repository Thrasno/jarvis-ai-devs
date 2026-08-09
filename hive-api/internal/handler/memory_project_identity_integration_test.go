package handler

import (
	"context"
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

// TestMemoryHandler_ProjectFilterAnswersEveryLiteralWithAnEmptyResult pins that
// the API has no opinion about which projects exist.
//
// It used to answer 404 project_unknown for any literal absent from the
// identity registry, which is a whitelist: the API deciding which projects are
// real. The daemon is the sole authority on project identity, so ?project= is a
// query and nothing else — a literal with no rows has no rows, which is a
// 200 with an empty list, not a missing resource.
func TestMemoryHandler_ProjectFilterAnswersEveryLiteralWithAnEmptyResult(t *testing.T) {
	pool, cleanup := startMemoryHandlerPostgres(t)
	defer cleanup()

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

	// Every one of these answers 200 with zero memories. The API cannot tell a
	// project it has never heard of from one whose rows simply do not match, and
	// that is the point: both are projects with nothing to return.
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "list", path: "/memories?project=Ghost.Project"},
		{name: "search", path: "/memories/search?query=needle&project=ghost/project"},
		{name: "another spelling list", path: "/memories?project=KNOWN_project"},
		{name: "another spelling search", path: "/memories/search?query=needle&project=known/project"},
		{name: "dotted list", path: "/memories?project=Known.Project"},
		{name: "dotted search", path: "/memories/search?query=needle&project=Known.Project"},
		{name: "global list", path: "/memories"},
		{name: "global search", path: "/memories/search?query=needle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, req)

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"total":0`)
		})
	}
	authSvc.AssertExpectations(t)
}

// TestMemoryReadsAnswerOnlyForTheSyncedLiteral proves the service read path
// scopes on the exact literal a sync stored, and that every other spelling is
// answered — with nothing — rather than refused.
func TestMemoryReadsAnswerOnlyForTheSyncedLiteral(t *testing.T) {
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
	// The API never folds spellings together, so a session payload belongs to
	// the request project only when its spelling is byte-for-byte identical.
	_, err := syncService.Sync(ctx, model.SyncRequest{
		Project:  " Fresh.Project ",
		Sessions: []model.SyncSessionPayload{{ID: "session-fresh", SyncID: "10000000-0000-0000-0000-000000000001", Project: " Fresh.Project ", DevID: "dev", Client: "test", StartedAt: now}},
	}, "00000000-0000-0000-0000-000000000001")
	require.NoError(t, err)
	sessionID := "session-fresh"
	_, err = memoryService.Create(ctx, &model.Memory{SyncID: "10000000-0000-0000-0000-000000000002", Project: " Fresh.Project ", SessionID: &sessionID, Category: model.CatDecision, Title: "needle", Content: "needle", CreatedBy: "user", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)

	listed, total, err := memoryService.List(ctx, model.MemoryFilter{Project: " Fresh.Project "})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.EqualValues(t, 1, total)
	found, total, err := memoryService.Search(ctx, "needle", model.MemoryFilter{Project: " Fresh.Project "})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.EqualValues(t, 1, total)

	// No other spelling reads a single one of those rows, and asking about one
	// is an ordinary empty answer rather than an error.
	for _, other := range []string{"FRESH_project", "fresh-project", "Fresh.Project"} {
		listed, total, err = memoryService.List(ctx, model.MemoryFilter{Project: other})
		require.NoError(t, err, other)
		require.Empty(t, listed, other)
		require.Zero(t, total, other)

		found, total, err = memoryService.Search(ctx, "needle", model.MemoryFilter{Project: other})
		require.NoError(t, err, other)
		require.Empty(t, found, other)
		require.Zero(t, total, other)
	}
}

// TestMemoryCreateStoresTheLiteralProjectAndRollsBackOnWriteFailure proves a
// create stores the project spelling verbatim and that a failed create leaves
// nothing behind.
func TestMemoryCreateStoresTheLiteralProjectAndRollsBackOnWriteFailure(t *testing.T) {
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
	// The pull selects on the stored literal.
	pulled, err := syncService.PullAll(ctx, " Direct.Project ", time.Time{}, nil, 10, model.PullCursor{}, model.PullCursor{})
	require.NoError(t, err)
	require.Len(t, pulled.Memories, 1)
	require.Equal(t, " Direct.Project ", pulled.Memories[0].Project, "pull retains the stored display spelling")

	var ghost int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM memories WHERE project = 'Ghost.Project'`).Scan(&ghost))
	require.Zero(t, ghost, "the rejected create left no row behind")
	// Another spelling reads none of those rows, and says so with an empty list
	// rather than by refusing the query.
	listed, total, err := memoryService.List(ctx, model.MemoryFilter{Project: "direct/project"})
	require.NoError(t, err)
	require.Empty(t, listed)
	require.Zero(t, total)
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
		migrations.DropProjectIdentityFoldsSQL, migrations.DropProjectIdentityRegistrySQL,
	} {
		require.NoError(t, repository.RunMigrations(pool, sql))
	}
	return pool, func() {
		pool.Close()
		require.NoError(t, container.Terminate(ctx))
	}
}
