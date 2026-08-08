package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionQueriesResolveSpellingsThroughTheGoContract proves the session
// read path round-trips spellings whose SQL-derived key disagrees with the
// shared Go canonical key: dots are separators in Go but not in SQL, and only
// Go applies Unicode full case folding.
func TestSessionQueriesResolveSpellingsThroughTheGoContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spelling string
	}{
		{name: "dot separator", spelling: "Foo.Bar"},
		{name: "non-ascii case folding", spelling: "STRAßE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, cleanup := startPostgresWithProjectSources(t)
			defer cleanup()

			ctx := context.Background()
			base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
			require.NoError(t, RegisterProjectIdentity(ctx, pool, tc.spelling, "", base))
			insertProjectSession(t, pool, "identity-session", tc.spelling, base, nil)

			repo := NewPostgresSessionRepository(pool)

			byProject, err := repo.ListSessionsByProject(ctx, tc.spelling)
			require.NoError(t, err)
			require.Len(t, byProject, 1, "ListSessionsByProject must find the stored spelling")

			since, _, err := repo.ListSessionsSince(ctx, tc.spelling, time.Time{}, model.PullCursor{}, model.UnboundedPullLimit)
			require.NoError(t, err)
			require.Len(t, since, 1, "ListSessionsSince must find the stored spelling")
			assert.Equal(t, "identity-session", since[0].ID)
		})
	}
}

// TestProjectAggregatesGroupOnTheGoCanonicalKey proves the aggregate query
// groups rows on the same key the identity registry joins on. Grouping on the
// SQL key split dotted and non-ASCII spellings into phantom projects.
func TestProjectAggregatesGroupOnTheGoCanonicalKey(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "Foo.Bar", "Foo.Bar", base))
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "foo_bar", "", base.Add(time.Minute)))

	insertProjectSession(t, pool, "dotted-session", "Foo.Bar", base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000401", "foo_bar", "dotted-session", base, base, nil)
	insertProjectSyncAttempt(t, pool, "dotted-sync", "foo-bar", model.SyncAttemptOutcomeSuccess, base, nil)

	aggregates, err := NewPostgresProjectRepository(pool).ListAggregates(ctx)
	require.NoError(t, err)
	require.Len(t, aggregates, 1, "every spelling of one project must coalesce into one aggregate")
	assert.Equal(t, "Foo.Bar", aggregates[0].Name)
	assert.EqualValues(t, 1, aggregates[0].MemoryCount)
	assert.EqualValues(t, 1, aggregates[0].SessionCount)
	require.NotNil(t, aggregates[0].LatestSyncOutcome)
}
