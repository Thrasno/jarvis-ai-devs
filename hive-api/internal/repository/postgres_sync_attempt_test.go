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

func startPostgresWithSyncAttempts(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	pool, cleanup := startPostgres(t)
	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptLogsSQL))
	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptPortalUsersSQL))
	return pool, cleanup
}

func TestPostgresSyncAttemptRepository_UpsertBatchIsIdempotentByDevAndAttempt(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttempts(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()
	_, err := pool.Exec(ctx, `INSERT INTO users (id, username, email, password) VALUES ('00000000-0000-0000-0000-000000000001', 'member', 'dev@example.com', 'hash')`)
	require.NoError(t, err)

	portalUserID, portalUserSource := "00000000-0000-0000-0000-000000000001", model.SyncAttemptPortalUserSourceAuthSubject
	attempt := model.SyncAttemptLog{AttemptID: "attempt-1", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now, Outcome: model.SyncAttemptOutcomeSuccess, PortalUserID: &portalUserID, PortalUserSource: &portalUserSource}
	first, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{attempt})
	require.NoError(t, err)
	second, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{attempt})
	require.NoError(t, err)

	assert.Equal(t, []string{"attempt-1"}, first.AcceptedIDs)
	assert.Empty(t, first.DuplicateIDs)
	assert.Empty(t, second.AcceptedIDs)
	assert.Equal(t, []string{"attempt-1"}, second.DuplicateIDs)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM sync_attempt_logs WHERE source_dev_id=$1 AND attempt_id=$2`, "dev@example.com", "attempt-1").Scan(&count))
	assert.Equal(t, 1, count)
	var persistedUserID, persistedSource string
	require.NoError(t, pool.QueryRow(ctx, `SELECT portal_user_id, portal_user_source FROM sync_attempt_logs WHERE attempt_id = 'attempt-1'`).Scan(&persistedUserID, &persistedSource))
	assert.Equal(t, portalUserID, persistedUserID)
	assert.Equal(t, portalUserSource, persistedSource)
}

func TestSyncAttemptPortalUsersMigration_BackfillsExactEmail(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptLogsSQL))
	_, err := pool.Exec(ctx, `INSERT INTO users (id, username, email, password) VALUES ('00000000-0000-0000-0000-000000000001', 'ada', 'ada@example.com', 'hash')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO sync_attempt_logs (attempt_id, source_dev_id, project, started_at, outcome) VALUES ('legacy', 'ada@example.com', 'jarvis-dev', now(), 'success')`)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(pool, migrations.SyncAttemptPortalUsersSQL))
	var userID, source string
	require.NoError(t, pool.QueryRow(ctx, `SELECT portal_user_id, portal_user_source FROM sync_attempt_logs WHERE attempt_id = 'legacy'`).Scan(&userID, &source))
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", userID)
	assert.Equal(t, "legacy_email", source)
}

func TestPostgresSyncAttemptRepository_DeleteOlderThan(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttempts(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Now().UTC()

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "old", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now.AddDate(0, 0, -91), EndedAt: timePtr(now.AddDate(0, 0, -91)), Outcome: model.SyncAttemptOutcomeFailure},
		{AttemptID: "fresh", DevID: "dev@example.com", Project: "jarvis-dev", StartedAt: now.AddDate(0, 0, -10), EndedAt: timePtr(now.AddDate(0, 0, -10)), Outcome: model.SyncAttemptOutcomeSuccess},
	})
	require.NoError(t, err)

	deleted, err := repo.DeleteOlderThan(ctx, now.AddDate(0, 0, -90))

	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	var remaining []string
	rows, err := pool.Query(ctx, `SELECT attempt_id FROM sync_attempt_logs ORDER BY attempt_id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		remaining = append(remaining, id)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"fresh"}, remaining)
}

func TestPostgresSyncAttemptRepository_ListForSummaryFiltersPersistedAttempts(t *testing.T) {
	pool, cleanup := startPostgresWithSyncAttempts(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresSyncAttemptRepository(pool)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	_, err := repo.UpsertBatch(ctx, []model.SyncAttemptLog{
		{AttemptID: "match", DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("network_error")},
		{AttemptID: "other-dev", DevID: "ben@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-b", StartedAt: now.Add(-2 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("network_error")},
		{AttemptID: "too-old", DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", StartedAt: now.Add(-31 * 24 * time.Hour), Outcome: model.SyncAttemptOutcomeFailure, ErrorCode: stringPtr("network_error")},
	})
	require.NoError(t, err)

	records, err := repo.ListForSummary(ctx, model.SyncAttemptSummaryFilter{Since: now.Add(-30 * 24 * time.Hour), DevID: "ada@example.com", Project: "jarvis-dev", Client: "hive-daemon", DaemonID: "daemon-a", Outcome: "failure", ErrorCode: "network_error"})

	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "ada@example.com", records[0].DevID)
	assert.Equal(t, "jarvis-dev", records[0].Project)
	assert.Equal(t, "hive-daemon", records[0].Client)
	assert.Equal(t, "daemon-a", records[0].DaemonID)
	assert.Equal(t, model.SyncAttemptOutcomeFailure, records[0].Outcome)
	require.NotNil(t, records[0].ErrorCode)
	assert.Equal(t, "network_error", *records[0].ErrorCode)
}

func timePtr(v time.Time) *time.Time { return &v }
