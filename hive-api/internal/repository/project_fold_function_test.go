package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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
func TestMigrationsNeverCreateTheSQLProjectFold(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT to_regprocedure('canonical_project_key(text)') IS NOT NULL`).Scan(&exists))
	require.False(t, exists,
		"a migration re-created the SQL project fold; identity is the daemon's contract, not SQL's")
}
