package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjectQueriesMatchTheStoredSpellingExactly proves the read path selects
// rows by exact equality on the stored project literal. The daemon owns project
// identity; the API never derives, folds or reconciles a key to widen a match.
//
// "Foo.Bar" and "foo-bar" are therefore two DISTINCT projects here. That is
// deliberate: folding them together on a shared backend let one tenant read
// another tenant's rows.
func TestProjectQueriesMatchTheStoredSpellingExactly(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	const stored = "Foo.Bar"
	const other = "foo-bar"

	require.NoError(t, RegisterProjectIdentity(ctx, pool, stored, "", base))
	insertProjectSession(t, pool, "identity-session", stored, base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000501", stored, "identity-session", base, base, nil)

	sessions := NewPostgresSessionRepository(pool)
	memories := NewPostgresMemoryRepository(pool)

	since, _, err := sessions.ListSessionsSince(ctx, stored, time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, since, 1, "ListSessionsSince must find the stored spelling")
	assert.Equal(t, "identity-session", since[0].ID)

	pulled, _, err := memories.PullSince(ctx, stored, time.Time{}, nil, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	require.Len(t, pulled, 1, "PullSince must find the stored spelling")
	assert.Equal(t, stored, pulled[0].Project, "the stored literal is returned verbatim")

	otherSince, _, err := sessions.ListSessionsSince(ctx, other, time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.Empty(t, otherSince, "a different spelling is a different project")

	otherPulled, _, err := memories.PullSince(ctx, other, time.Time{}, nil, model.PullCursor{}, model.UnboundedPullLimit)
	require.NoError(t, err)
	assert.Empty(t, otherPulled, "a different spelling is a different project")
}

// TestProjectAggregatesGroupOnTheStoredSpelling proves the dashboard aggregate
// groups rows on the literal each row carries. Distinct spellings stay distinct
// projects until an admin merges them; nothing in the API folds them together.
func TestProjectAggregatesGroupOnTheStoredSpelling(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "Foo.Bar", "Foo.Bar", base))

	insertProjectSession(t, pool, "dotted-session", "Foo.Bar", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000401", "foo_bar", "dotted-session", base, base, nil)
	insertProjectSyncAttempt(t, pool, "dotted-sync", "foo-bar", model.SyncAttemptOutcomeSuccess, base, nil)

	aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
	require.NoError(t, err)
	require.Len(t, aggregates, 3, "each stored spelling is its own project")

	byName := make(map[string]model.ProjectAggregate, len(aggregates))
	for _, aggregate := range aggregates {
		byName[aggregate.Name] = aggregate
	}
	require.Contains(t, byName, "Foo.Bar")
	require.Contains(t, byName, "foo_bar")
	require.Contains(t, byName, "foo-bar")
	assert.EqualValues(t, 1, byName["Foo.Bar"].SessionCount)
	assert.EqualValues(t, 0, byName["Foo.Bar"].MemoryCount)
	assert.EqualValues(t, 1, byName["foo_bar"].MemoryCount)
	require.NotNil(t, byName["foo-bar"].LatestSyncOutcome)
}
