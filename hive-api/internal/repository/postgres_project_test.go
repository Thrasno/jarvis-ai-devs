package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startPostgresWithProjectSources(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	pool, cleanup := startPostgresWithSessions(t)
	require.NoError(t, RunMigrations(pool, migrations.MemoryMutationsSQL), "failed to run migration 005")
	require.NoError(t, RunMigrations(pool, migrations.DropTopicKeyUniqueConstraintSQL), "failed to run migration 006")
	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptLogsSQL), "failed to run migration 007")

	return pool, cleanup
}

func TestPostgresProjectRepository_ListAggregates(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresProjectRepository(pool)
	base := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)

	insertProjectSession(t, pool, "memory-project-session", "memory-project", base.Add(-3*time.Hour), nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000101", "memory-project", "memory-project-session", base.Add(-4*time.Hour), base.Add(-2*time.Hour), nil)
	deletedAt := base.Add(-30 * time.Minute)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000102", "memory-project", "memory-project-session", base.Add(-5*time.Hour), base.Add(-5*time.Hour), &deletedAt)

	endedAt := base.Add(-90 * time.Minute)
	insertProjectSyncAttempt(t, pool, "sync-only-attempt", "sync-only-project", model.SyncAttemptOutcomeSuccess, base.Add(-2*time.Hour), &endedAt)

	sessionEndedAt := base.Add(-15 * time.Minute)
	insertProjectSession(t, pool, "session-only-session", "session-only-project", base.Add(-45*time.Minute), &sessionEndedAt)

	olderFailureEnd := base.Add(-20 * time.Minute)
	latestSuccessEnd := base.Add(-5 * time.Minute)
	insertProjectSyncAttempt(t, pool, "older-failure", "health-project", model.SyncAttemptOutcomeFailure, base.Add(-25*time.Minute), &olderFailureEnd)
	insertProjectSyncAttempt(t, pool, "latest-success", "health-project", model.SyncAttemptOutcomeSuccess, base.Add(-10*time.Minute), &latestSuccessEnd)

	got, err := repo.ListAggregates(ctx)

	require.NoError(t, err)
	byName := projectAggregatesByName(got)
	require.Contains(t, byName, "memory-project")
	require.Contains(t, byName, "sync-only-project")
	require.Contains(t, byName, "session-only-project")
	require.Contains(t, byName, "health-project")

	assert.EqualValues(t, 1, byName["memory-project"].MemoryCount, "deleted memories must not be counted")
	assert.EqualValues(t, 1, byName["memory-project"].SessionCount)
	assertTimePtrEqual(t, deletedAt, byName["memory-project"].LastMemoryAt, "memory lifecycle timestamp should include tombstones")

	assert.EqualValues(t, 0, byName["sync-only-project"].MemoryCount)
	assert.EqualValues(t, 0, byName["sync-only-project"].SessionCount)
	assertTimePtrEqual(t, endedAt, byName["sync-only-project"].LastSyncAt, "sync-only project should expose sync timestamp")
	require.NotNil(t, byName["sync-only-project"].LatestSyncOutcome)
	assert.Equal(t, model.SyncAttemptOutcomeSuccess, *byName["sync-only-project"].LatestSyncOutcome)

	assert.EqualValues(t, 1, byName["session-only-project"].SessionCount)
	assertTimePtrEqual(t, sessionEndedAt, byName["session-only-project"].LastSessionAt, "session activity should use ended_at when present")
	assert.Nil(t, byName["session-only-project"].LatestSyncOutcome)

	require.NotNil(t, byName["health-project"].LatestSyncOutcome)
	assert.Equal(t, model.SyncAttemptOutcomeSuccess, *byName["health-project"].LatestSyncOutcome, "latest sync outcome should win")
	assertTimePtrEqual(t, latestSuccessEnd, byName["health-project"].LastSyncAt, "latest sync activity should use ended_at when present")
}

func projectAggregatesByName(records []model.ProjectAggregate) map[string]model.ProjectAggregate {
	byName := make(map[string]model.ProjectAggregate, len(records))
	for _, record := range records {
		byName[record.Name] = record
	}
	return byName
}

func insertProjectSession(t *testing.T, pool *pgxpool.Pool, id, project string, startedAt time.Time, endedAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO sessions (id, project, dev_id, client, started_at, ended_at)
		VALUES ($1, $2, 'tester', 'test', $3, $4)`, id, project, startedAt, endedAt)
	require.NoError(t, err)
}

func insertProjectMemory(t *testing.T, pool *pgxpool.Pool, syncID, project, sessionID string, createdAt, updatedAt time.Time, deletedAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO memories (sync_id, project, category, title, content, created_by, created_at, updated_at, session_id, deleted_at)
		VALUES ($1, $2, 'decision', 'project memory', 'content', 'tester', $3, $4, $5, $6)`,
		syncID, project, createdAt, updatedAt, sessionID, deletedAt)
	require.NoError(t, err)
}

func insertProjectSyncAttempt(t *testing.T, pool *pgxpool.Pool, attemptID, project string, outcome model.SyncAttemptOutcome, startedAt time.Time, endedAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO sync_attempt_logs (attempt_id, source_dev_id, project, started_at, ended_at, outcome)
		VALUES ($1, 'tester', $2, $3, $4, $5)`, attemptID, project, startedAt, endedAt, string(outcome))
	require.NoError(t, err)
}

func assertTimePtrEqual(t *testing.T, want time.Time, got *time.Time, msgAndArgs ...interface{}) {
	t.Helper()
	require.NotNil(t, got, msgAndArgs...)
	assert.True(t, got.Equal(want), "got %s want %s", got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
}
