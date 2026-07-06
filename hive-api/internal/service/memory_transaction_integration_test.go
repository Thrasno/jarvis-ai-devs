package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

func startPostgresForMemoryServiceProjectBlocks(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	for _, sql := range []string{
		migrations.InitialSQL,
		migrations.UserPromptsSQL,
		migrations.SessionsSQL,
		migrations.AuditLogsSQL,
		migrations.MemoryMutationsSQL,
		migrations.DropTopicKeyUniqueConstraintSQL,
		migrations.SyncAttemptLogsSQL,
		migrations.ActivityFeedIndexSQL,
		migrations.MemoryDiscoveryIndexesSQL,
		migrations.PullCursorIndexesSQL,
		migrations.ProjectScopedPullCursorIndexesSQL,
		migrations.ProjectBlocksSQL,
	} {
		require.NoError(t, repository.RunMigrations(pool, sql))
	}
	return pool, func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	}
}

func TestMemoryService_CreateConcurrentWithBlockCannotWriteAfterBlock(t *testing.T) {
	pool, cleanup := startPostgresForMemoryServiceProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	memSvc := service.NewMemoryService(
		repository.NewPostgresMemoryRepository(pool),
		repository.NewPostgresSessionRepository(pool),
		repository.NewPostgresProjectBlockRepository(pool),
		repository.NewPostgresTxManager(pool),
	)
	govSvc := service.NewProjectGovernanceService(
		repository.NewPostgresProjectBlockRepository(pool),
		repository.NewPostgresAuditRepository(pool),
		repository.NewPostgresTxManager(pool),
	)

	for i := 0; i < 4; i++ {
		project := fmt.Sprintf("Race Project %d", i)
		canonical := fmt.Sprintf("race-project-%d", i)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var createErr error
		var blockErr error

		go func() {
			defer wg.Done()
			<-start
			_, createErr = memSvc.Create(ctx, &model.Memory{
				SyncID:    fmt.Sprintf("00000000-0000-0000-0000-%012d", i+900),
				Project:   project,
				Category:  model.CatDecision,
				Title:     "direct create",
				Content:   "must not land after block",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			})
		}()

		go func() {
			defer wg.Done()
			<-start
			_, blockErr = govSvc.BlockProject(ctx, model.AdminActor{UserID: "00000000-0000-0000-0000-0000000000a1"}, project, model.ProjectBlockRequest{
				Action:       model.ProjectBlockActionQuarantine,
				Reason:       "garbage project",
				Confirmation: canonical,
				ExportMarker: "export-1",
			})
		}()

		close(start)
		wg.Wait()
		require.NoError(t, blockErr)
		if createErr != nil {
			blockedErr := &service.ProjectBlockedError{}
			require.True(t, errors.As(createErr, &blockedErr), "unexpected create error: %v", createErr)
		}

		_, postBlockErr := memSvc.Create(ctx, &model.Memory{
			SyncID:    fmt.Sprintf("00000000-0000-0000-0000-%012d", i+950),
			Project:   project,
			Category:  model.CatDecision,
			Title:     "post-block direct create",
			Content:   "must never land after block completes",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
		blockedErr := &service.ProjectBlockedError{}
		require.True(t, errors.As(postBlockErr, &blockedErr), "unexpected post-block create error: %v", postBlockErr)

		var postBlockWrites int
		err := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM memories
			WHERE project = $1 AND sync_id = $2`, project, fmt.Sprintf("00000000-0000-0000-0000-%012d", i+950)).Scan(&postBlockWrites)
		require.NoError(t, err)
		require.Zero(t, postBlockWrites)
	}
}
