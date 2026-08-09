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
	require.NoError(t, RunMigrations(pool, migrations.CanonicalProjectRegistrySQL), "failed to run canonical project registry migration")

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

// TestPostgresProjectRepository_ListAggregatesNamesProjectsByTheStoredSpelling
// proves the aggregate is grouped by the literal on each row, and named through
// the identity registry.
//
// The registry is keyed by that same literal, so the join is exact equality: it
// can attach a display name to a project, and it cannot merge two of them. The
// spelling a remote reported for a project is that project's display name;
// every other project keeps the literal its rows carry.
func TestPostgresProjectRepository_ListAggregatesNamesProjectsByTheStoredSpelling(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()
	require.NoError(t, RunMigrations(pool, migrations.CanonicalProjectRegistrySQL))

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RegisterProjectIdentity(ctx, pool, " Foo_Bar ", "FOO_BAR", base))
	insertProjectSession(t, pool, "aggregate-variant-session", " Foo_Bar ", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000203", " Foo_Bar ", "aggregate-variant-session", base, base, nil)
	insertProjectSyncAttempt(t, pool, "aggregate-variant-sync", " Foo_Bar ", model.SyncAttemptOutcomeSuccess, base, nil)

	// A spelling the daemon would fold onto the same key. It is a different
	// project here, and it borrows neither the aggregate nor the display name.
	insertProjectSession(t, pool, "aggregate-sibling-session", "foo-bar", base, nil)

	aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
	require.NoError(t, err)
	require.Len(t, aggregates, 2, "two spellings are two projects")

	byName := projectAggregatesByName(aggregates)
	named, ok := byName["FOO_BAR"]
	require.True(t, ok, "the registered remote spelling is the project display name")
	assert.EqualValues(t, 1, named.MemoryCount)
	assert.EqualValues(t, 1, named.SessionCount)
	assertTimePtrEqual(t, base, named.LastMemoryAt)
	assertTimePtrEqual(t, base, named.LastSessionAt)
	assertTimePtrEqual(t, base, named.LastSyncAt)
	require.Equal(t, syncOutcomePtr(model.SyncAttemptOutcomeSuccess), named.LatestSyncOutcome)

	sibling, ok := byName["foo-bar"]
	require.True(t, ok, "an unregistered project is named by the literal its rows carry")
	assert.EqualValues(t, 0, sibling.MemoryCount)
	assert.EqualValues(t, 1, sibling.SessionCount)
}

// TestBackfillProjectIdentityRegistryRecordsEveryLegacyLiteral replaces the
// coalescing behaviour this test used to pin.
//
// Coalescing " Foo.Bar " and "foo/bar" into one registry row asserted that the
// API may decide two spellings are the same project. It may not — the daemon is
// the sole authority on identity, and rows are selected by exact equality, so a
// coalesced key made "known" and "readable" different questions.
func TestBackfillProjectIdentityRegistryRecordsEveryLegacyLiteral(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()
	require.NoError(t, RunMigrations(pool, migrations.UserPromptsSQL))
	require.NoError(t, RunMigrations(pool, migrations.ProjectBlocksSQL))
	require.NoError(t, RunMigrations(pool, migrations.QuarantineContractSQL))
	require.NoError(t, RunMigrations(pool, migrations.DistributedQuarantineSQL))
	require.NoError(t, RunMigrations(pool, migrations.CanonicalProjectRegistrySQL))

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	insertProjectSession(t, pool, "registry-oldest", " Foo.Bar ", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000201", "foo/bar", "registry-oldest", base.Add(time.Minute), base.Add(time.Minute), nil)
	insertProjectSyncAttempt(t, pool, "registry-unicode", "STRAßE", model.SyncAttemptOutcomeSuccess, base.Add(2*time.Minute), nil)

	require.NoError(t, BackfillProjectIdentityRegistry(ctx, pool))
	require.NoError(t, BackfillProjectIdentityRegistry(ctx, pool), "backfill must be idempotent")

	rows, err := pool.Query(ctx, `SELECT project_key, first_spelling FROM project_identities ORDER BY project_key`)
	require.NoError(t, err)
	defer rows.Close()

	type identity struct{ key, spelling string }
	got := make([]identity, 0)
	for rows.Next() {
		var row identity
		require.NoError(t, rows.Scan(&row.key, &row.spelling))
		got = append(got, row)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []identity{
		{key: " Foo.Bar ", spelling: " Foo.Bar "},
		{key: "STRAßE", spelling: "STRAßE"},
		{key: "foo/bar", spelling: "foo/bar"},
	}, got)
}

// TestRegisterProjectIdentityKeepsTheRemoteDisplayNamePerLiteral pins that a
// remote display name attaches to the exact literal that reported it. Two
// spellings are two registry rows, so one project's remote name can never
// become another's display name.
func TestRegisterProjectIdentityKeepsTheRemoteDisplayNamePerLiteral(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()
	require.NoError(t, RunMigrations(pool, migrations.CanonicalProjectRegistrySQL))

	ctx := context.Background()
	firstSeen := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "Old Project", "", firstSeen))
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "Old Project", "Remote Project", firstSeen.Add(time.Hour)))
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "old/project", "", firstSeen.Add(time.Hour)))

	var spelling, remote string
	var remoteSeen *time.Time
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT first_spelling, COALESCE(remote_spelling, ''), remote_seen_at
		FROM project_identities WHERE project_key = 'Old Project'`).Scan(&spelling, &remote, &remoteSeen))
	require.Equal(t, "Old Project", spelling)
	require.Equal(t, "Remote Project", remote)
	require.NotNil(t, remoteSeen)

	require.NoError(t, pool.QueryRow(ctx, `
		SELECT first_spelling, COALESCE(remote_spelling, ''), remote_seen_at
		FROM project_identities WHERE project_key = 'old/project'`).Scan(&spelling, &remote, &remoteSeen))
	require.Equal(t, "old/project", spelling)
	require.Empty(t, remote, "a different spelling is a different project and borrows no display name")
	require.Nil(t, remoteSeen)
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
		INSERT INTO sessions (id, project, dev_id, client, started_at, ended_at, created_at, updated_at)
		VALUES ($1, $2, 'tester', 'test', $3, $4, $3, $3)`, id, project, startedAt, endedAt)
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

func syncOutcomePtr(outcome model.SyncAttemptOutcome) *model.SyncAttemptOutcome {
	return &outcome
}
