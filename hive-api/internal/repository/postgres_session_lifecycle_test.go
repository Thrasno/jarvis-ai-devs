package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionLifecycleQuerier struct {
	sql  string
	args []any
}

func (q *sessionLifecycleQuerier) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.sql = sql
	q.args = args
	return pgconn.CommandTag{}, nil
}

func (*sessionLifecycleQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (*sessionLifecycleQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	return sessionLifecycleNoRows{}
}

type sessionLifecycleNoRows struct{}

func (sessionLifecycleNoRows) Scan(...any) error { return pgx.ErrNoRows }

func TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateAcceptsReopen(t *testing.T) {
	querier := &sessionLifecycleQuerier{}
	repo := newPostgresSessionRepositoryWithQuerier(querier)
	startedAt := time.Date(2026, 8, 9, 10, 11, 12, 0, time.UTC)
	summary := "closed before delayed capture"

	err := repo.UpsertSession(context.Background(), &model.Session{
		ID:        "regular-session",
		SyncID:    "a1000000-0000-0000-0000-000000000001",
		Project:   "alpha",
		Directory: "/work/alpha",
		DevID:     "developer",
		Client:    "mcp",
		StartedAt: startedAt,
		EndedAt:   nil,
		Summary:   &summary,
	})

	require.NoError(t, err)
	assert.Contains(t, querier.sql,
		"ended_at = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.ended_at ELSE sessions.ended_at END")
	assert.Contains(t, querier.sql,
		"summary = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.summary ELSE sessions.summary END")
	assert.Contains(t, querier.sql, "synced_at = now()")
	assert.Contains(t, querier.sql, "WHERE sessions.project = $10")
	assert.NotContains(t, querier.sql, "started_at = EXCLUDED.started_at")
	assert.NotContains(t, querier.sql, "directory = EXCLUDED.directory")
	assert.Len(t, querier.args, 10)
	assert.Equal(t, "regular-session", querier.args[0])
	assert.Equal(t, "a1000000-0000-0000-0000-000000000001", querier.args[1])
	assert.Equal(t, "alpha", querier.args[2])
	assert.Nil(t, querier.args[7], "a reopened session must forward NULL ended_at")
	assert.Equal(t, &summary, querier.args[8])
	assert.Equal(t, "alpha", querier.args[9], "ordinary same-project pushes must target their current project")
	assert.True(t, strings.Contains(querier.sql, "ON CONFLICT (sync_id) DO UPDATE"))
}

func TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdateDistinguishesOrdinaryAndRelocationLifecycle(t *testing.T) {
	lifecycleUpdate := func(fromProject string) *sessionLifecycleQuerier {
		t.Helper()
		querier := &sessionLifecycleQuerier{}
		repo := newPostgresSessionRepositoryWithQuerier(querier)
		endedAt := time.Date(2026, 8, 9, 11, 11, 12, 0, time.UTC)
		summary := "incoming lifecycle"

		require.NoError(t, repo.UpsertSession(context.Background(), &model.Session{
			ID: "regular-session", SyncID: "a1000000-0000-0000-0000-000000000006",
			Project: "alpha", FromProject: fromProject, EndedAt: &endedAt, Summary: &summary,
		}))
		return querier
	}

	ordinary := lifecycleUpdate("")
	relocation := lifecycleUpdate("former-alpha")

	for _, querier := range []*sessionLifecycleQuerier{ordinary, relocation} {
		assert.Contains(t, querier.sql,
			"ended_at = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.ended_at ELSE sessions.ended_at END")
		assert.Contains(t, querier.sql,
			"summary = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.summary ELSE sessions.summary END")
	}
	assert.Equal(t, "alpha", ordinary.args[9], "ordinary updates must apply lifecycle state in place")
	assert.Equal(t, "former-alpha", relocation.args[9], "relocation must preserve the source-project predicate")
}

func TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePreservesRelocationSource(t *testing.T) {
	querier := &sessionLifecycleQuerier{}
	repo := newPostgresSessionRepositoryWithQuerier(querier)

	require.NoError(t, repo.UpsertSession(context.Background(), &model.Session{
		ID: "regular-session", SyncID: "a1000000-0000-0000-0000-000000000007",
		Project: "alpha", FromProject: "former-alpha",
	}))

	require.Len(t, querier.args, 10)
	assert.Equal(t, "former-alpha", querier.args[9], "relocations must retain their source-project predicate")
}

func TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePreservesIdentityAndSentinelBranches(t *testing.T) {
	querier := &sessionLifecycleQuerier{}
	repo := newPostgresSessionRepositoryWithQuerier(querier)
	startedAt := time.Date(2026, 8, 9, 10, 11, 12, 0, time.UTC)
	endedAt := startedAt.Add(time.Hour)
	summary := "closed"

	require.NoError(t, repo.UpsertSession(context.Background(), &model.Session{
		ID: "regular-session", SyncID: "a1000000-0000-0000-0000-000000000002",
		Project: "alpha", Directory: "/work/alpha", DevID: "developer", Client: "mcp",
		StartedAt: startedAt, EndedAt: &endedAt, Summary: &summary, FromProject: "alpha",
	}))

	assert.Contains(t, querier.sql, "ended_at = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.ended_at ELSE sessions.ended_at END")
	assert.Contains(t, querier.sql, "summary = CASE WHEN sessions.project = EXCLUDED.project THEN EXCLUDED.summary ELSE sessions.summary END")
	assert.NotContains(t, querier.sql, "started_at = EXCLUDED.started_at")
	assert.NotContains(t, querier.sql, "sync_id = EXCLUDED.sync_id")
	assert.NotContains(t, querier.sql, "directory = EXCLUDED.directory")
	assert.NotContains(t, querier.sql, "dev_id = EXCLUDED.dev_id")
	assert.NotContains(t, querier.sql, "client = EXCLUDED.client")
	assert.Equal(t, &endedAt, querier.args[7])
	assert.Equal(t, "", querier.args[9], "self-relocation must retain its no-op safeguard")

	require.NoError(t, repo.UpsertSession(context.Background(), &model.Session{
		ID: "manual-save-alpha", SyncID: "a1000000-0000-0000-0000-000000000003",
		Project: "alpha", DevID: "developer", Client: "mcp", StartedAt: startedAt,
	}))
	assert.Contains(t, querier.sql, "ON CONFLICT (id) DO UPDATE")
	assert.NotContains(t, querier.sql, "ended_at = EXCLUDED.ended_at")

	require.NoError(t, repo.UpsertSession(context.Background(), &model.Session{
		ID: "legacy-pre-lifecycle-alpha", SyncID: "a1000000-0000-0000-0000-000000000004",
		Project: "alpha", DevID: "legacy", Client: "legacy", StartedAt: startedAt,
	}))
	assert.Contains(t, querier.sql, "ON CONFLICT (id) DO NOTHING")
	assert.NotContains(t, querier.sql, "ended_at = EXCLUDED.ended_at")
}

func TestPostgresSessionRepository_UpsertSession_RegularLifecycleUpdatePullsReopenedRow(t *testing.T) {
	if testing.Short() {
		t.Skip("PostgreSQL integration requires the repository test database")
	}

	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresSessionRepository(pool)
	startedAt := time.Date(2026, 8, 9, 10, 11, 12, 0, time.UTC)
	endedAt := startedAt.Add(time.Hour)
	originalSummary := "closed"
	beforeReopen := time.Now().UTC()
	_, err := pool.Exec(ctx, `
		INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		"regular-session", "a1000000-0000-0000-0000-000000000005", "alpha", "/work/alpha", "developer", "mcp", startedAt, endedAt, originalSummary, beforeReopen.Add(-time.Hour))
	require.NoError(t, err)

	updatedSummary := "reopened by delayed capture"
	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID: "regular-session", SyncID: "a1000000-0000-0000-0000-000000000005",
		Project: "alpha", Directory: "/other", DevID: "other", Client: "other",
		StartedAt: startedAt.Add(time.Hour), Summary: &updatedSummary,
	}))

	repeatedSummary := "reopened again"
	require.NoError(t, repo.UpsertSession(ctx, &model.Session{
		ID: "regular-session", SyncID: "a1000000-0000-0000-0000-000000000005",
		Project: "alpha", Directory: "/another", DevID: "another", Client: "another",
		StartedAt: startedAt.Add(2 * time.Hour), Summary: &repeatedSummary,
	}))

	got, err := repo.GetSession(ctx, "regular-session")
	require.NoError(t, err)
	assert.Nil(t, got.EndedAt)
	require.Equal(t, &repeatedSummary, got.Summary)
	assert.True(t, got.StartedAt.Equal(startedAt), "reopens must preserve the original start instant")
	assert.Equal(t, "/work/alpha", got.Directory)
	assert.Equal(t, "developer", got.DevID)
	assert.Equal(t, "mcp", got.Client)

	pulled, hasMore, err := repo.ListSessionsSince(ctx, "alpha", beforeReopen, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, pulled, 1)
	assert.Equal(t, "regular-session", pulled[0].ID)
	assert.Nil(t, pulled[0].EndedAt)
}
