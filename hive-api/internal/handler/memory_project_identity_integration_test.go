package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
