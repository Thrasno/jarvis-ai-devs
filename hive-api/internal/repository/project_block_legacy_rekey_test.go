package repository

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startPostgresWithQuarantineHistory applies every quarantine-related migration
// EXCEPT the legacy rekey, so a test can seed pre-rekey rows and then apply it.
func startPostgresWithQuarantineHistory(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	pool, cleanup := startPostgresWithProjectSources(t)
	require.NoError(t, RunMigrations(pool, migrations.DistributedQuarantineSQL), "failed to run migration 018")
	return pool, cleanup
}

// seedLegacyBlock writes a quarantine exactly as the pre-fix code stored it: the
// admin's literal in `project`, a canonicalized key in `canonical_project_key`.
func seedLegacyBlock(t *testing.T, pool *pgxpool.Pool, project, legacyKey string, generation int64) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO project_blocks (project, canonical_project_key, action, generation, reason, confirmation, export_marker, blocked)
		VALUES ($1, $2, 'block', $3, 'quarantine', $1, 'marker', true)
		RETURNING id::text`, project, legacyKey, generation).Scan(&id))
	return id
}

// TestLegacyQuarantineRekeyRepointsBlocksAtTheirStoredSpelling covers the
// deployment hazard the exact-equality contract created.
//
// Every quarantine written before the contract stores the admin's literal in
// `project` and a canonicalized key in `canonical_project_key`. The predicate
// now compares a stored row literal against `canonical_project_key` alone, so a
// legacy block matches nothing at all — while Status and ListQuarantines still
// read `blocked = true` and report the project as quarantined. The admin console
// shows a quarantine that does not quarantine.
//
// Rekeying at startup was correctly deleted: under exact equality it would
// repoint a live block on every boot. This is the one-time migration instead.
func TestLegacyQuarantineRekeyRepointsBlocksAtTheirStoredSpelling(t *testing.T) {
	pool, cleanup := startPostgresWithQuarantineHistory(t)
	defer cleanup()

	ctx := context.Background()
	blockID := seedLegacyBlock(t, pool, "Foo.Bar", "foo-bar", 1)
	var commandID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT command_id::text FROM project_blocks WHERE id = $1`, blockID).Scan(&commandID))
	_, err := pool.Exec(ctx, `
		INSERT INTO project_block_acks (command_id, canonical_project_key, ack_token, ack_auth_subject, status)
		VALUES ($1, 'foo-bar', 'token', 'user-1', 'applied')`, commandID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO project_block_ack_deliveries (command_id, canonical_project_key, ack_auth_subject, ack_daemon_id)
		VALUES ($1, 'foo-bar', 'user-1', 'daemon-1')`, commandID)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(pool, migrations.LegacyQuarantineRekeySQL))

	var key string
	require.NoError(t, pool.QueryRow(ctx, `SELECT canonical_project_key FROM project_blocks WHERE id = $1`, blockID).Scan(&key))
	assert.Equal(t, "Foo.Bar", key, "the block must quarantine the literal the admin named")

	// ON UPDATE CASCADE carries the acknowledgement trail with the block.
	for _, table := range []string{"project_block_acks", "project_block_ack_deliveries"} {
		var count int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE canonical_project_key = 'Foo.Bar'`).Scan(&count))
		assert.Equal(t, 1, count, table+" must follow the rekeyed block")
	}

	// The immutable command history has no FK, so the migration moves it too —
	// otherwise QuarantineProgress loses the rekeyed block's generations.
	var commands int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM project_quarantine_commands WHERE canonical_project_key = 'Foo.Bar'`).Scan(&commands))
	assert.Equal(t, 1, commands)

	// And the rekeyed block now actually quarantines its project.
	require.ErrorIs(t, checkProjectBlocked(ctx, pool, "Foo.Bar"), ErrProjectBlocked)
	require.NoError(t, checkProjectBlocked(ctx, pool, "foo-bar"),
		"the canonical fold was never a project an admin named")
}

// TestLegacyQuarantineRekeyLeavesCollidingLegacyBlocksUntouched pins the
// UNIQUE(canonical_project_key) edge.
//
// A block written after the exact-equality contract already keys on its literal.
// If a legacy block's literal is that same key, rekeying it would violate the
// unique constraint. The live literal-keyed block already quarantines that exact
// project, so the migration skips the legacy row rather than failing the whole
// deployment — and skipping changes nothing, because that row quarantines
// exactly what it quarantined before.
func TestLegacyQuarantineRekeyLeavesCollidingLegacyBlocksUntouched(t *testing.T) {
	pool, cleanup := startPostgresWithQuarantineHistory(t)
	defer cleanup()

	ctx := context.Background()
	legacyID := seedLegacyBlock(t, pool, "Foo-Bar", "foo-bar-legacy", 1)
	currentID := seedLegacyBlock(t, pool, "Foo-Bar", "Foo-Bar", 2)

	require.NoError(t, RunMigrations(pool, migrations.LegacyQuarantineRekeySQL))

	var legacyKey, currentKey string
	require.NoError(t, pool.QueryRow(ctx, `SELECT canonical_project_key FROM project_blocks WHERE id = $1`, legacyID).Scan(&legacyKey))
	require.NoError(t, pool.QueryRow(ctx, `SELECT canonical_project_key FROM project_blocks WHERE id = $1`, currentID).Scan(&currentKey))
	assert.Equal(t, "foo-bar-legacy", legacyKey, "the colliding legacy row keeps its key instead of failing the migration")
	assert.Equal(t, "Foo-Bar", currentKey, "the literal-keyed block stays authoritative for its project")
}

// TestLegacyQuarantineRekeyIsIdempotent guards the replay-on-every-boot
// migration strategy this module uses.
func TestLegacyQuarantineRekeyIsIdempotent(t *testing.T) {
	pool, cleanup := startPostgresWithQuarantineHistory(t)
	defer cleanup()

	ctx := context.Background()
	blockID := seedLegacyBlock(t, pool, "Foo.Bar", "foo-bar", 1)

	require.NoError(t, RunMigrations(pool, migrations.LegacyQuarantineRekeySQL))
	require.NoError(t, RunMigrations(pool, migrations.LegacyQuarantineRekeySQL))

	var key string
	require.NoError(t, pool.QueryRow(ctx, `SELECT canonical_project_key FROM project_blocks WHERE id = $1`, blockID).Scan(&key))
	assert.Equal(t, "Foo.Bar", key)
}
