package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockProject quarantines a project under the canonical key the Go identity
// contract produces, exactly as the governance service stores it.
func blockProject(t *testing.T, pool pgxQuerier, project, canonicalKey string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO project_blocks (project, canonical_project_key, action, reason, confirmation, export_marker, blocked)
		VALUES ($1, $2, 'block', 'quarantine', $1, 'marker', true)`, project, canonicalKey)
	require.NoError(t, err)
}

// TestQuarantineFailsClosedForUnregisteredSpelling proves that a stored project
// spelling with no row in project_identity_spellings still reads as BLOCKED
// through every real consumer query. Quarantine enforcement must never depend
// on the registry being complete.
func TestQuarantineFailsClosedForUnregisteredSpelling(t *testing.T) {
	for _, tc := range []struct {
		name          string
		storedProject string
	}{
		{name: "canonical spelling stored by the sync path", storedProject: "foo-bar"},
		{name: "raw legacy spelling never registered", storedProject: "Foo.Bar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, cleanup := startPostgresWithProjectSources(t)
			defer cleanup()

			ctx := context.Background()
			base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)

			blockProject(t, pool, "Foo.Bar", "foo-bar")

			insertProjectSession(t, pool, "blocked-session", tc.storedProject, base, nil)
			insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000301", tc.storedProject, "blocked-session", base, base, nil)
			insertProjectSyncAttempt(t, pool, "blocked-attempt", tc.storedProject, model.SyncAttemptOutcomeSuccess, base, nil)

			var spellings int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM project_identity_spellings`).Scan(&spellings))
			require.Zero(t, spellings, "the registry must stay empty so the test exercises the fail-closed path")

			memories, _, err := NewPostgresMemoryRepository(pool).PullSince(ctx, "Foo.Bar", time.Time{}, nil, model.PullCursor{}, model.UnboundedPullLimit)
			require.NoError(t, err)
			assert.Empty(t, memories, "quarantined memories must not be pullable")

			sessions, _, err := NewPostgresSessionRepository(pool).ListSessionsSince(ctx, "Foo.Bar", time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
			require.NoError(t, err)
			assert.Empty(t, sessions, "quarantined sessions must not be pullable")

			aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
			require.NoError(t, err)
			assert.Empty(t, aggregates, "quarantined projects must not be listed")
		})
	}
}

// TestQuarantineBlocksNonASCIISpellingThroughRegistry covers the residual the
// SQL fold cannot reach: Unicode case folding only happens in Go, so the
// registry has to carry non-ASCII spellings.
func TestQuarantineBlocksNonASCIISpellingThroughRegistry(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)

	require.NoError(t, RegisterProjectIdentity(ctx, pool, "STRAßE", "", base))
	blockProject(t, pool, "STRAßE", "strasse")

	insertProjectSession(t, pool, "unicode-session", "STRAßE", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000302", "STRAßE", "unicode-session", base, base, nil)

	memories, _, err := NewPostgresMemoryRepository(pool).PullSince(ctx, "STRAßE", time.Time{}, nil, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.Empty(t, memories)

	aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
	require.NoError(t, err)
	assert.Empty(t, aggregates)
}

// TestRegisterProjectIdentityRegistersCanonicalSpelling proves the registry can
// resolve the canonical key the sync path actually stores in project columns,
// not only the raw spelling the client sent.
func TestRegisterProjectIdentityRegistersCanonicalSpelling(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "Foo.Bar", "", base))

	var key string
	require.NoError(t, pool.QueryRow(ctx, `SELECT project_key FROM project_identity_spellings WHERE spelling = 'foo-bar'`).Scan(&key))
	assert.Equal(t, "foo-bar", key)

	require.NoError(t, pool.QueryRow(ctx, `SELECT project_key FROM project_identity_spellings WHERE spelling = 'Foo.Bar'`).Scan(&key))
	assert.Equal(t, "foo-bar", key)

	var firstSpelling string
	require.NoError(t, pool.QueryRow(ctx, `SELECT first_spelling FROM project_identities WHERE project_key = 'foo-bar'`).Scan(&firstSpelling))
	assert.Equal(t, "Foo.Bar", firstSpelling, "registering the canonical key must not steal display precedence")
}
