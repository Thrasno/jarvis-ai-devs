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
// List used to widen its project filter with an EXISTS over the identity
// registry, so any spelling registered under the same canonical key selected
// another project's rows: GET /memories?project=foo-bar returned rows stored as
// "Foo.Bar". PullSince matched the literal exactly, so the two disagreed on what
// a project even is.
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
	require.NoError(t, RegisterProjectIdentity(ctx, pool, stored, "", base))
	require.NoError(t, RegisterProjectIdentity(ctx, pool, "foo-bar", "", base))
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

// TestProjectIdentityRegistryIsKeyedByTheLiteralSpelling replaces the coverage
// that pinned canonical keying.
//
// The API never derives identity, so the registry cannot be a canonical
// grouping: it is a table of the project literals the API has observed. Keying
// it canonically was what made the read leak reachable, and it also made
// "is this project known?" disagree with "does this project have rows?" —
// GET /memories?project=known/project answered 200-empty forever for a project
// whose rows are stored under a different spelling.
func TestProjectIdentityRegistryIsKeyedByTheLiteralSpelling(t *testing.T) {
	pool, cleanup := startPostgresWithProjectSources(t)
	defer cleanup()

	ctx := context.Background()
	seenAt := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RegisterProjectIdentity(ctx, pool, " Foo.Bar ", "", seenAt))

	var key, spelling string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT project_key, first_spelling FROM project_identities`).Scan(&key, &spelling))
	assert.Equal(t, " Foo.Bar ", key, "the registry key is the literal, byte for byte")
	assert.Equal(t, " Foo.Bar ", spelling)

	repo := NewPostgresMemoryRepository(pool)
	known, err := repo.ProjectExists(ctx, " Foo.Bar ")
	require.NoError(t, err)
	assert.True(t, known)

	folded, err := repo.ProjectExists(ctx, "foo-bar")
	require.NoError(t, err)
	assert.False(t, folded, "a spelling the API never saw is not a known project")
}
