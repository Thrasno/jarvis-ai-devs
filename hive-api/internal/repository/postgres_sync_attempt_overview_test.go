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
	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptPortalUsersSQL))
	return pool, cleanup
}

func startPostgresWithSyncAttemptsAndProjectBlocks(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	pool, cleanup := startPostgresWithSyncAttemptsAndSessions(t)
	require.NoError(t, RunMigrations(pool, migrations.ProjectBlocksSQL))
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

func TestPostgresSyncAttemptRepository_SyncHealthByProjectExcludesBlockedProjects(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndProjectBlocks(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	blockRepo := NewPostgresProjectBlockRepository(pool)
	now := time.Now().UTC()
	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "visible-sync-health", DevID: "dev-1", Project: "visible-sync", DaemonID: "d1", StartedAt: now.Add(-1 * time.Hour), Outcome: model.SyncAttemptOutcomeSuccess},
		{AttemptID: "blocked-sync-health", DevID: "dev-2", Project: "Blocked Sync", DaemonID: "d2", StartedAt: now.Add(-30 * time.Minute), Outcome: model.SyncAttemptOutcomeFailure},
	})
	require.NoError(t, err)
	_, err = blockRepo.BlockProject(ctx, model.ProjectBlockCreate{
		Project:             "Blocked Sync",
		CanonicalProjectKey: "blocked-sync",
		Action:              model.ProjectBlockActionQuarantine,
		Reason:              "garbage",
		Confirmation:        "blocked-sync",
		ExportMarker:        "export-1",
		ActorUserID:         "admin-1",
	})
	require.NoError(t, err)

	rows, err := repo.SyncHealthByProject(ctx, 30*24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, []string{"visible-sync"}, projectNames(rows))
}

func TestPostgresSyncAttemptRepository_ProjectSyncHealth(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndProjectBlocks(t)
	defer cleanup()
	ctx, repo := context.Background(), NewPostgresSyncAttemptRepository(pool)
	const alice = "00000000-0000-0000-0000-000000000011"
	const bob = "00000000-0000-0000-0000-000000000012"
	const disabled = "00000000-0000-0000-0000-000000000013"
	_, err := pool.Exec(ctx, `INSERT INTO users (id, username, email, password, is_active) VALUES
		($1, 'alice', 'alice@example.com', 'hash', true), ($2, 'bob', 'bob@example.com', 'hash', true), ($3, 'disabled', 'disabled@example.com', 'hash', false)`, alice, bob, disabled)
	require.NoError(t, err)
	now, source := time.Now().UTC(), model.SyncAttemptPortalUserSourceAuthSubject
	_, err = repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "healthy-old", DevID: "device-a", Project: "healthy", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: healthStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "healthy-new", DevID: "device-b", Project: "healthy", StartedAt: now.Add(-time.Hour), Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: healthStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "degraded-a", DevID: "device-a", Project: "degraded", StartedAt: now.Add(-time.Hour), Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: healthStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "degraded-b", DevID: "device-b", Project: "degraded", StartedAt: now.Add(-time.Hour), Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: healthStringPtr(bob), PortalUserSource: &source},
		{AttemptID: "tie-success", DevID: "device-a", Project: "tie", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: healthStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "tie-failure", DevID: "device-b", Project: "tie", StartedAt: now, Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: healthStringPtr(alice), PortalUserSource: &source},
		{AttemptID: "disabled", DevID: "device-c", Project: "disabled", StartedAt: now, Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: healthStringPtr(disabled), PortalUserSource: &source},
		{AttemptID: "blocked", DevID: "device-d", Project: "blocked", StartedAt: now, Outcome: model.SyncAttemptOutcomeFailure, PortalUserID: healthStringPtr(alice), PortalUserSource: &source},
	})
	require.NoError(t, err)
	_, err = NewPostgresProjectBlockRepository(pool).BlockProject(ctx, model.ProjectBlockCreate{Project: "blocked", CanonicalProjectKey: "blocked", Action: model.ProjectBlockActionQuarantine, Reason: "test", Confirmation: "blocked", ExportMarker: "test", ActorUserID: alice})
	require.NoError(t, err)

	projection, err := repo.ProjectSyncHealth(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, projection.Degraded)
	assert.Equal(t, 3, projection.Total)
	assert.Equal(t, []string{"tie", "degraded", "healthy"}, projectNames(projection.Rows))
	assert.Equal(t, []model.SyncAttemptOutcome{model.SyncAttemptOutcomeFailure, model.SyncAttemptOutcomeFailure, model.SyncAttemptOutcomeSuccess}, []model.SyncAttemptOutcome{projection.Rows[0].LastOutcome, projection.Rows[1].LastOutcome, projection.Rows[2].LastOutcome})
	assert.Equal(t, 2, projection.Rows[1].ContributorCount)
}

func TestPostgresSyncAttemptRepository_ProjectSyncHealth_Empty(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttemptsAndProjectBlocks(t)
	defer cleanup()

	projection, err := NewPostgresSyncAttemptRepository(pool).ProjectSyncHealth(context.Background())

	require.NoError(t, err)
	assert.Empty(t, projection.Rows)
	assert.Zero(t, projection.Degraded)
	assert.Zero(t, projection.Total)
}

func healthStringPtr(value string) *string { return &value }

func projectNames(rows []model.ProjectSyncHealthRow) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Project)
	}
	return names
}
