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

func startPostgresWithAuditLogs(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	pool, cleanup := startPostgresWithSessions(t)
	require.NoError(t, RunMigrations(pool, migrations.AuditLogsSQL), "failed to run migration 004")

	return pool, cleanup
}

func TestMigration004_AuditLogsSchema(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()

	rows, err := pool.Query(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'audit_logs'
	`)
	require.NoError(t, err)
	defer rows.Close()

	type column struct{ dataType, nullable string }
	cols := map[string]column{}
	for rows.Next() {
		var name, dataType, nullable string
		require.NoError(t, rows.Scan(&name, &dataType, &nullable))
		cols[name] = column{dataType: dataType, nullable: nullable}
	}
	require.NoError(t, rows.Err())

	expected := map[string]column{
		"id":            {"uuid", "NO"},
		"occurred_at":   {"timestamp with time zone", "NO"},
		"actor_user_id": {"uuid", "YES"},
		"project":       {"character varying", "YES"},
		"action":        {"character varying", "NO"},
		"outcome":       {"character varying", "NO"},
		"entry_count":   {"integer", "NO"},
		"reason_code":   {"text", "YES"},
		"metadata":      {"jsonb", "NO"},
	}

	for name, want := range expected {
		t.Run("column_"+name, func(t *testing.T) {
			got, ok := cols[name]
			require.True(t, ok, "column %q should exist", name)
			assert.Equal(t, want, got)
		})
	}
}

func TestMigration004_AuditLogsIndexes(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT indexname FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'audit_logs'
	`)
	require.NoError(t, err)
	defer rows.Close()

	found := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		found[name] = true
	}
	require.NoError(t, rows.Err())

	for _, idx := range []string{
		"idx_audit_logs_occurred_at",
		"idx_audit_logs_actor_user_id",
		"idx_audit_logs_project",
		"idx_audit_logs_action",
		"idx_audit_logs_outcome",
		"idx_audit_logs_project_occurred_at",
	} {
		t.Run(idx, func(t *testing.T) {
			assert.True(t, found[idx], "index %q should exist", idx)
		})
	}
}

func TestPostgresAuditRepository_InsertListCountAndMetadataRoundTrip(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresAuditRepository(pool)
	actorID := "11111111-1111-1111-1111-111111111111"
	base := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)

	entries := []*model.AuditEntry{
		{
			OccurredAt:  base.Add(-2 * time.Hour),
			ActorUserID: &actorID,
			Project:     stringPtr("alpha"),
			Action:      model.AuditActionUserLevelChange,
			Outcome:     model.AuditOutcomeSuccess,
			EntryCount:  1,
			ReasonCode:  stringPtr("promoted"),
			Metadata: model.AuditMetadata{
				"target_username": "ada",
				"new_level":       "admin",
			},
		},
		{
			OccurredAt: base.Add(-1 * time.Hour),
			Project:    stringPtr("alpha"),
			Action:     model.AuditActionSyncConflict,
			Outcome:    model.AuditOutcomeConflict,
			EntryCount: 2,
			Metadata: model.AuditMetadata{
				"conflict_count": 2,
				"pushed_count":   5,
			},
		},
		{
			OccurredAt: base,
			Project:    stringPtr("beta"),
			Action:     model.AuditActionSyncPush,
			Outcome:    model.AuditOutcomeSuccess,
			EntryCount: 7,
			Metadata:   model.AuditMetadata{"pushed_count": 7},
		},
	}

	for _, entry := range entries {
		require.NoError(t, repo.Insert(ctx, entry))
		require.NotEmpty(t, entry.ID)
	}

	filter := model.AuditFilter{
		Project: stringPtr("alpha"),
		Action:  auditPtr(model.AuditActionSyncConflict),
		Outcome: auditPtr(model.AuditOutcomeConflict),
		Since:   auditPtr(base.Add(-90 * time.Minute)),
		Until:   auditPtr(base),
		Limit:   20,
	}

	count, err := repo.Count(ctx, filter)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)

	got, err := repo.List(ctx, filter)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, model.AuditActionSyncConflict, got[0].Action)
	assert.Equal(t, model.AuditMetadata{"conflict_count": float64(2), "pushed_count": float64(5)}, got[0].Metadata)
}

func TestPostgresAuditRepository_ListPaginationAndOrdering(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresAuditRepository(pool)
	base := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)

	for i, project := range []string{"alpha", "beta", "alpha"} {
		entry := &model.AuditEntry{
			OccurredAt: base.Add(time.Duration(i) * time.Minute),
			Project:    &project,
			Action:     model.AuditActionSyncPush,
			Outcome:    model.AuditOutcomeSuccess,
			EntryCount: i + 1,
			Metadata:   model.AuditMetadata{"pushed_count": i + 1},
		}
		require.NoError(t, repo.Insert(ctx, entry))
	}

	got, err := repo.List(ctx, model.AuditFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 3, got[0].EntryCount, "newest audit entry must be listed first")
	assert.Equal(t, 2, got[1].EntryCount)

	nextPage, err := repo.List(ctx, model.AuditFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	require.Len(t, nextPage, 1)
	assert.Equal(t, 1, nextPage[0].EntryCount)
}

func auditPtr[T any](v T) *T { return &v }
