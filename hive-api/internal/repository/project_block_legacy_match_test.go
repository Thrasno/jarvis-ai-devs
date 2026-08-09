package repository

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// startPostgresWithQuarantineHistory applies every migration a quarantine row
// depends on, so a test can seed pre-contract rows and then run the migrations
// the server replays after them.
func startPostgresWithQuarantineHistory(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	pool, cleanup := startPostgresWithProjectSources(t)
	require.NoError(t, RunMigrations(pool, migrations.DistributedQuarantineSQL), "failed to run migration 018")
	return pool, cleanup
}

// seedLegacyBlock writes a quarantine exactly as the pre-contract code stored
// it: the admin's display spelling in `project`, and in `canonical_project_key`
// the shared Go canonical key, which is also the spelling the daemon pushes.
func seedLegacyBlock(t *testing.T, pool *pgxpool.Pool, project, legacyKey string, generation int64) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO project_blocks (project, canonical_project_key, action, generation, reason, confirmation, export_marker, blocked)
		VALUES ($1, $2, 'block', $3, 'quarantine', $1, 'marker', true)
		RETURNING id::text`, project, legacyKey, generation).Scan(&id))
	return id
}

// TestLegacyQuarantineKeepsBlockingTheSpellingItsRowsCarry is the regression
// guard for the deployment hazard that pre-contract quarantines were assumed to
// have, and do not.
//
// A pre-contract block stored `canonical_project_key = Canonicalize(project)`,
// where Canonicalize was a one-line delegate to projectidentity.Canonical — the
// same function the daemon applies to every project literal before pushing. So
// the rows of that project are stored under exactly the key the block carries,
// and plain equality matches them.
//
// Any startup step that rewrites `canonical_project_key` towards `project` (the
// admin's display spelling, which no stored row carries) silently lifts a live
// quarantine. This test pins that no such step exists in the migrations the
// server replays after the quarantine schema.
func TestLegacyQuarantineKeepsBlockingTheSpellingItsRowsCarry(t *testing.T) {
	pool, cleanup := startPostgresWithQuarantineHistory(t)
	defer cleanup()

	ctx := context.Background()
	seedLegacyBlock(t, pool, "Foo.Bar", "foo-bar", 1)

	// The remaining startup migrations, in the order runStartupMigrations runs
	// them. None of them may touch a block's key.
	for _, migration := range []string{
		migrations.CanonicalProjectRegistrySQL,
		migrations.DropProjectIdentityFoldsSQL,
	} {
		require.NoError(t, RunMigrations(pool, migration))
	}
	require.NoError(t, BackfillProjectIdentityRegistry(ctx, pool))

	require.ErrorIs(t, checkProjectBlocked(ctx, pool, "foo-bar"), ErrProjectBlocked,
		"the quarantine must still block the spelling the daemon pushes")
	require.NoError(t, checkProjectBlocked(ctx, pool, "Foo.Bar"),
		"the admin's display spelling is not a spelling any row is stored under")
}
