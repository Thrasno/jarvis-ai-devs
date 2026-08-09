package repository

import (
	"context"
	_ "embed"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// legacyProjectIdentityFoldsSQL recreates the state an upgraded production
// database is in: the SQL fold and the spelling registry, exactly as migrations
// 012 and 019 shipped them before they were removed from those files.
//
//go:embed testdata/legacy_project_identity_folds.sql
var legacyProjectIdentityFoldsSQL string

// TestMigrationsNeverCreateTheSQLProjectFold pins that no migration brings the
// canonical_project_key(text) fold into existence.
//
// Migration 021 drops it, but this module has no migration ledger: every boot
// replays every migration file in order, so a CREATE in an earlier file
// re-creates on every startup what a later file drops. The function then exists
// for the whole window between them, and any statement issued in that window
// can resolve it. Removing the CREATE is what makes the drop final; 021 stays
// authoritative for databases that already have the function.
//
// startPostgresWithSessions applies migration 012, the file that used to create
// it.
//
// This proves only that nothing CREATES the fold on a fresh database. That is
// half the contract, and on its own it passes even if migration 021 is deleted:
// TestFullMigrationSetRemovesTheLegacyProjectIdentityFolds proves the other
// half, that an existing database has both constructs taken away.
func TestMigrationsNeverCreateTheSQLProjectFold(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT to_regprocedure('canonical_project_key(text)') IS NOT NULL`).Scan(&exists))
	require.False(t, exists,
		"a migration re-created the SQL project fold; identity is the daemon's contract, not SQL's")
}

// TestFullMigrationSetRemovesTheLegacyProjectIdentityFolds is the test that
// migration 021 is answerable to.
//
// Every production database upgraded from an earlier release still carries the
// SQL fold and project_identity_spellings; a fresh one never had them. Only 021
// closes that gap, so a test that starts from an empty database asserts nothing
// about it — delete 021 and such a test stays green while every deployed
// database keeps both constructs forever.
//
// So this starts from the intermediate state instead: bring the schema up, seed
// the two constructs as the released 012 and 019 created them, and then replay
// the WHOLE ordered set the way the next boot does. Both must be gone
// afterwards, and the replay must not put either back.
func TestFullMigrationSetRemovesTheLegacyProjectIdentityFolds(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()
	ordered := migrations.Ordered()
	require.NotEmpty(t, ordered)

	// Bring the schema up, then add the two constructs back. That is precisely
	// what an upgraded production database looks like: today's tables, plus the
	// objects an earlier release created and nothing since has removed.
	for _, sql := range ordered {
		require.NoError(t, RunMigrations(pool, sql))
	}
	_, err := pool.Exec(ctx, legacyProjectIdentityFoldsSQL)
	require.NoError(t, err, "the fixture must reproduce the released schema")
	require.True(t, sqlProjectFoldExists(t, pool), "the fixture must leave the fold in place")
	require.True(t, spellingRegistryExists(t, pool), "the fixture must leave the spelling registry in place")

	for _, sql := range ordered {
		require.NoError(t, RunMigrations(pool, sql))
	}

	require.False(t, sqlProjectFoldExists(t, pool),
		"an upgraded database kept the SQL project fold; it survives wherever no migration takes it away")
	require.False(t, spellingRegistryExists(t, pool),
		"an upgraded database kept the legacy spelling registry; the join that leaked one project's rows to another is still joinable")
}

func sqlProjectFoldExists(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT to_regprocedure('canonical_project_key(text)') IS NOT NULL`).Scan(&exists))
	return exists
}

func spellingRegistryExists(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT to_regclass('project_identity_spellings') IS NOT NULL`).Scan(&exists))
	return exists
}
