package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryListSelectsTheStoredLiteralAndNothingElse closes the cross-tenant
// read leak on the dashboard's main query.
//
// List used to widen its project filter with an EXISTS over a canonically
// keyed identity registry, so any spelling under the same canonical key
// selected another project's rows: GET /memories?project=foo-bar returned rows
// stored as "Foo.Bar". PullSince matched the literal exactly, so the two
// disagreed on what a project even is.
//
// Symmetrically, the widening was the only reason ?project=Foo.Bar ever reached
// rows stored under a different spelling — dropping it narrows nothing a caller
// could rely on, because those rows were never that project's rows.
func TestMemoryListSelectsTheStoredLiteralAndNothingElse(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	const stored = "Foo.Bar"
	insertProjectSession(t, pool, "identity-session", stored, base, nil)
	insertProjectMemory(t, pool, "00000000-0000-0000-0000-000000000501", stored, "identity-session", base, base, nil)

	repo := NewPostgresMemoryRepository(pool)

	leaked, err := repo.List(ctx, model.MemoryFilter{Project: "foo-bar"})
	require.NoError(t, err)
	assert.Empty(t, leaked, "another spelling is another project and must not read these rows")

	own, err := repo.List(ctx, model.MemoryFilter{Project: stored})
	require.NoError(t, err)
	require.Len(t, own, 1, "the stored literal must still read its own rows")
	assert.Equal(t, stored, own[0].Project)
}
