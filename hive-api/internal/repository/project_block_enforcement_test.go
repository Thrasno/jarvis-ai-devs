package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockProject quarantines a project under the exact literal an admin supplied,
// exactly as the governance service stores it.
func blockProject(t *testing.T, pool pgxQuerier, project, canonicalKey string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO project_blocks (project, canonical_project_key, action, reason, confirmation, export_marker, blocked)
		VALUES ($1, $2, 'block', 'quarantine', $1, 'marker', true)`, project, canonicalKey)
	require.NoError(t, err)
}

// TestQuarantineBlocksTheExactStoredSpelling proves the quarantine predicate is
// plain equality against the literal each row carries. It asks one question —
// is this literal quarantined? — and no longer over-approximates the answer
// across a registry lookup, an ASCII fold and a COALESCE sentinel.
func TestQuarantineBlocksTheExactStoredSpelling(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	const stored = "Foo.Bar"

	blockProject(t, pool, stored, stored)
	insertProjectSession(t, pool, "blocked-session", stored, base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000301", stored, "blocked-session", base, base, nil)
	insertProjectSyncAttempt(t, pool, "blocked-attempt", stored, model.SyncAttemptOutcomeSuccess, base, nil)

	memories, _, err := NewPostgresMemoryRepository(pool).PullSince(ctx, stored, time.Time{}, nil, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.Empty(t, memories, "quarantined memories must not be pullable")

	sessions, _, err := NewPostgresSessionRepository(pool).ListSessionsSince(ctx, stored, time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.Empty(t, sessions, "quarantined sessions must not be pullable")

	aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
	require.NoError(t, err)
	assert.Empty(t, aggregates, "quarantined projects must not be listed")
}

// TestQuarantineOfAnotherSpellingLeavesTheProjectReadable pins DELIBERATE
// behaviour: "foo-bar" and "Foo.Bar" are two distinct projects, so blocking one
// must not block the other. This is asserted explicitly so a future reader does
// not mistake it for a hole and "fix" it by folding the keys back together —
// that fold quarantined unrelated projects and, through its COALESCE fallback
// to the empty string, could quarantine an entire backend.
func TestQuarantineOfAnotherSpellingLeavesTheProjectReadable(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	const stored = "foo.bar"

	blockProject(t, pool, "foo-bar", "foo-bar")
	insertProjectSession(t, pool, "readable-session", stored, base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000302", stored, "readable-session", base, base, nil)

	memories, _, err := NewPostgresMemoryRepository(pool).PullSince(ctx, stored, time.Time{}, nil, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, memories, 1, "blocking a different spelling must not block this project")

	sessions, _, err := NewPostgresSessionRepository(pool).ListSessionsSince(ctx, stored, time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "blocking a different spelling must not block this project")

	aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
	require.NoError(t, err)
	require.Len(t, aggregates, 1)
	assert.Equal(t, stored, aggregates[0].Name)
}

// TestQuarantineWriteCheckAgreesWithTheReadPredicate proves the write-side
// check and the read-side predicate resolve the same block for the same
// literal. A disagreement here would let writes into a project whose rows are
// already unreadable, or reject writes to a project nobody blocked.
func TestQuarantineWriteCheckAgreesWithTheReadPredicate(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	blockProject(t, pool, "Foo.Bar", "Foo.Bar")

	require.ErrorIs(t, checkProjectBlocked(ctx, pool, "Foo.Bar"), ErrProjectBlocked,
		"the blocked literal must be rejected on write")
	require.NoError(t, checkProjectBlocked(ctx, pool, "foo-bar"),
		"a different spelling is a different project and stays writable")
}
