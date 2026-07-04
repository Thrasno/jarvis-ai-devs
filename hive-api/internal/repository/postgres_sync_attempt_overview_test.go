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

// startPostgresWithSyncAttemptsAndSessions starts a container with full schema
// including sessions (003/006) and sync_attempt_logs (007).
func startPostgresWithSyncAttemptsAndSessions(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	pool, cleanup := startPostgresWithSessions(t)
	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptLogsSQL))
	return pool, cleanup
}

// --- DaemonHealth tests ---

func TestPostgresSyncAttemptRepository_DaemonHealth_CorrectCounts(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	attempts := []model.SyncAttemptLog{
		// daemon-A: success <24h → counts healthy and total
		{AttemptID: "a1", DevID: "dev1", Project: "p1", DaemonID: "daemon-A", StartedAt: now.Add(-1 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
		// daemon-B: failure <24h → counts total only (not healthy)
		{AttemptID: "b1", DevID: "dev2", Project: "p1", DaemonID: "daemon-B", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure},
		// daemon-C: >24h but <30d → counts in total (30d window), not in healthy (24h window)
		{AttemptID: "c1", DevID: "dev3", Project: "p1", DaemonID: "daemon-C", StartedAt: now.Add(-25 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
	}
	_, err := repo.UpsertBatch(ctx, attempts)
	require.NoError(t, err)

	healthy, total, err := repo.DaemonHealth(ctx, 24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, healthy, "only daemon-A had success within 24h")
	assert.Equal(t, 3, total, "daemon-A, B, C all within 30d")
}

func TestPostgresSyncAttemptRepository_DaemonHealth_EmptyTable(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)

	healthy, total, err := repo.DaemonHealth(ctx, 24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, healthy)
	assert.Equal(t, 0, total)
}

// --- SyncHealthByProject tests ---

func TestPostgresSyncAttemptRepository_SyncHealthByProject_SuccessStatus(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "x1", DevID: "dev1", Project: "proj-X", DaemonID: "d1", StartedAt: now.Add(-1 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
	})
	require.NoError(t, err)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "proj-X", rows[0].Project)
	assert.Equal(t, model.SyncAttemptOutcomeSuccess, rows[0].LastOutcome)
	assert.WithinDuration(t, now.Add(-1*time.Hour), rows[0].LastActivityAt, time.Second)
}

func TestPostgresSyncAttemptRepository_SyncHealthByProject_FailureStatus(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "y1", DevID: "dev1", Project: "proj-Y", DaemonID: "d1", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure},
	})
	require.NoError(t, err)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, model.SyncAttemptOutcomeFailure, rows[0].LastOutcome)
}

func TestPostgresSyncAttemptRepository_SyncHealthByProject_Empty(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Empty(t, rows, "empty table must return empty slice")
}

func TestPostgresSyncAttemptRepository_SyncHealthByProject_LastActivityUsesLatestAttempt(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "latest-1", DevID: "dev-alice", Project: "proj-latest", DaemonID: "d1", StartedAt: now.Add(-4 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure},
		{AttemptID: "latest-2", DevID: "dev-alice", Project: "proj-latest", DaemonID: "d1", StartedAt: now.Add(-15 * time.Minute), Outcome: model.SyncAttemptOutcomeSuccess},
	})
	require.NoError(t, err)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, model.SyncAttemptOutcomeSuccess, rows[0].LastOutcome)
	assert.WithinDuration(t, now.Add(-15*time.Minute), rows[0].LastActivityAt, time.Second)
}

func TestPostgresSyncAttemptRepository_SyncHealthByProject_OrdersProblemProjectsFirst(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "healthy-newer", DevID: "dev-1", Project: "proj-healthy-newer", DaemonID: "d1", StartedAt: now.Add(-5 * time.Minute), Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "failure-older-a", DevID: "dev-1", Project: "proj-failure-a", DaemonID: "d1", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure},
		{AttemptID: "failure-older-b", DevID: "dev-1", Project: "proj-failure-b", DaemonID: "d1", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure},
		{AttemptID: "failure-newer", DevID: "dev-1", Project: "proj-failure-newer", DaemonID: "d1", StartedAt: now.Add(-10 * time.Minute), Outcome: model.SyncAttemptOutcomeFailure},
		{AttemptID: "healthy-older", DevID: "dev-1", Project: "proj-healthy-older", DaemonID: "d1", StartedAt: now.Add(-30 * time.Minute), Outcome: model.SyncAttemptOutcomeSuccess},
	})
	require.NoError(t, err)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 5)
	assert.Equal(t, []string{
		"proj-failure-newer",
		"proj-failure-a",
		"proj-failure-b",
		"proj-healthy-newer",
		"proj-healthy-older",
	}, projectNames(rows))
}

func TestPostgresSyncAttemptRepository_SyncHealthByProject_ContributorCount(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "z1", DevID: "dev-alice", Project: "proj-Z", DaemonID: "d1", StartedAt: now.Add(-1 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "z2", DevID: "dev-bob", Project: "proj-Z", DaemonID: "d2", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "z3", DevID: "dev-alice", Project: "proj-Z", DaemonID: "d1", StartedAt: now.Add(-3 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
	})
	require.NoError(t, err)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "proj-Z", rows[0].Project)
	assert.Equal(t, 2, rows[0].ContributorCount, "must count distinct source_dev_id per project")
}

func projectNames(rows []model.ProjectSyncHealthRow) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Project)
	}
	return names
}
