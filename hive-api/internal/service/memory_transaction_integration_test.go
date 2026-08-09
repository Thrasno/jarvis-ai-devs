package service_test

import (
	"context"
	"encoding/json"
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
		migrations.QuarantineContractSQL,
		migrations.DistributedQuarantineSQL,
		migrations.CanonicalProjectRegistrySQL,
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

func TestProjectGovernanceService_ConcurrentTransitionsPreserveStrictGenerationHistory(t *testing.T) {
	pool, cleanup := startPostgresForMemoryServiceProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	service := service.NewProjectGovernanceService(
		repository.NewPostgresProjectBlockRepository(pool),
		repository.NewPostgresAuditRepository(pool),
		repository.NewPostgresTxManager(pool),
	)
	project := "Concurrent Project"
	canonical := project
	requests := []model.ProjectBlockRequest{
		{Action: model.ProjectBlockActionBlock, Reason: "first transition", Confirmation: canonical},
		{Action: model.ProjectBlockActionUnblock, Reason: "second transition", Confirmation: canonical},
	}
	start := make(chan struct{})
	errs := make(chan error, len(requests))
	var wg sync.WaitGroup
	for index, request := range requests {
		wg.Add(1)
		go func(index int, request model.ProjectBlockRequest) {
			defer wg.Done()
			<-start
			_, err := service.BlockProject(ctx, model.AdminActor{UserID: fmt.Sprintf("00000000-0000-0000-0000-%012d", index+81)}, project, request)
			errs <- err
		}(index, request)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	rows, err := pool.Query(ctx, `SELECT generation, action FROM project_quarantine_commands WHERE canonical_project_key = $1 ORDER BY generation ASC`, canonical)
	require.NoError(t, err)
	defer rows.Close()
	var generations []int64
	var actions []string
	for rows.Next() {
		var generation int64
		var action string
		require.NoError(t, rows.Scan(&generation, &action))
		generations = append(generations, generation)
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int64{1, 2}, generations)
	require.ElementsMatch(t, []string{model.ProjectBlockActionBlock, model.ProjectBlockActionUnblock}, actions)

	var headGeneration int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT generation FROM project_blocks WHERE canonical_project_key = $1`, canonical).Scan(&headGeneration))
	require.Equal(t, int64(2), headGeneration)
}

func TestProjectGovernanceService_ListQuarantinesLoadsRetainedCurrentGenerationAfterRelease(t *testing.T) {
	pool, cleanup := startPostgresForMemoryServiceProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	governance := service.NewProjectGovernanceService(
		repository.NewPostgresProjectBlockRepository(pool),
		repository.NewPostgresAuditRepository(pool),
		repository.NewPostgresTxManager(pool),
	)
	actor := model.AdminActor{UserID: "00000000-0000-0000-0000-000000000091"}
	_, err := governance.BlockProject(ctx, actor, "Retained Project", model.ProjectBlockRequest{Action: model.ProjectBlockActionBlock, Reason: "quarantine", Confirmation: "Retained Project"})
	require.NoError(t, err)
	_, err = governance.BlockProject(ctx, actor, "Retained Project", model.ProjectBlockRequest{Action: model.ProjectBlockActionUnblock, Reason: "release", Confirmation: "Retained Project"})
	require.NoError(t, err)

	summaries, err := governance.ListQuarantines(ctx)

	require.NoError(t, err)
	require.Equal(t, []model.QuarantineSummary{{
		Project: "Retained Project", CanonicalProjectKey: "Retained Project", Generation: 2,
		Action: model.ProjectBlockActionUnblock, State: model.ProjectBlockProgressPending,
		TransitionedAt: summaries[0].TransitionedAt,
	}}, summaries)
	require.NotEmpty(t, summaries[0].TransitionedAt)
	payload, err := json.Marshal(summaries[0])
	require.NoError(t, err)
	forbidden := []string{"actor_user_id", "ack_token", "auth_subject"}
	for _, field := range forbidden {
		require.NotContains(t, string(payload), field)
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
		canonical := project
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
				Action:       model.ProjectBlockActionBlock,
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

// TestMemoryCreateDerivesTheManualSaveSessionFromTheLiteralProject drives the
// whole seam with the real repository: Create -> validateSessionAttribution ->
// EnsureManualSaveSession, with a project spelling that is not its own
// canonical form.
//
// The id derivation was pinned only at the repository seam, while every
// service-level test mocked a pair the real repository can no longer produce
// ("Jarvis Dev" -> "manual-save-jarvis-dev"). Under that mocking, reverting the
// derivation to a canonical key breaks nothing here — yet it is exactly what
// would hand one spelling a session owned by another, because the attribution
// check builds its expected id from the literal project.
func TestMemoryCreateDerivesTheManualSaveSessionFromTheLiteralProject(t *testing.T) {
	pool, cleanup := startPostgresForMemoryServiceProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	svc := service.NewMemoryService(
		repository.NewPostgresMemoryRepository(pool),
		repository.NewPostgresSessionRepository(pool),
		repository.NewPostgresProjectBlockRepository(pool),
		repository.NewPostgresTxManager(pool),
	)
	const project = "Jarvis Dev"
	now := time.Now().UTC()

	created, err := svc.Create(ctx, &model.Memory{
		SyncID: "40000000-0000-0000-0000-000000000001", Project: project,
		Category: model.CatDecision, Title: "lazy fallback", Content: "content",
		CreatedBy: "user", CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	require.NotNil(t, created.SessionID)
	require.Equal(t, "manual-save-"+project, *created.SessionID,
		"the lazy-fallback session id is derived from the literal project")

	var sessionProject string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT project FROM sessions WHERE id = $1`, "manual-save-"+project).Scan(&sessionProject))
	require.Equal(t, project, sessionProject)

	// The attribution check builds its expected id from the same literal, so a
	// memory pointing at another spelling's manual-save session is rejected.
	foreign := "manual-save-jarvis-dev"
	_, err = svc.Create(ctx, &model.Memory{
		SyncID: "40000000-0000-0000-0000-000000000002", Project: project, SessionID: &foreign,
		Category: model.CatDecision, Title: "cross project", Content: "content",
		CreatedBy: "user", CreatedAt: now, UpdatedAt: now,
	})
	require.ErrorIs(t, err, service.ErrSessionProjectMismatch)
}
