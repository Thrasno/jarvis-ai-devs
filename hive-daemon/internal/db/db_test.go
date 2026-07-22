package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestOpen_InMemory(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(':memory:') failed: %v", err)
	}
	defer func() { _ = d.Close() }()
}

func TestOpen_MemoriesTableExists(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	var name string
	err = d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='memories'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("memories table not found: %v", err)
	}
}

func TestOpen_RecoveryTokensTableAndExpiryIndexExist(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	for _, tt := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "recovery_tokens"},
		{kind: "index", name: "idx_recovery_tokens_expires"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type=? AND name=?", tt.kind, tt.name,
			).Scan(&name)
			if err != nil {
				t.Fatalf("%s %q not found: %v", tt.kind, tt.name, err)
			}
		})
	}
}

func TestOpen_MemoryTombstoneAndMutationSchemaExist(t *testing.T) {
	d := openTestDB(t)

	for _, column := range []string{"deleted_at", "deleted_by", "delete_reason", "restored_at"} {
		t.Run("memories column "+column, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM pragma_table_info('memories') WHERE name = ?", column,
			).Scan(&name)
			require.NoErrorf(t, err, "column %s should exist", column)
		})
	}

	for _, tt := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "memory_mutations"},
		{kind: "table", name: "mutation_cursors"},
		{kind: "index", name: "idx_memory_mutations_event_id"},
		{kind: "index", name: "idx_memory_mutations_project_unsynced"},
		{kind: "index", name: "idx_memory_mutations_entity"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type = ? AND name = ?", tt.kind, tt.name,
			).Scan(&name)
			require.NoErrorf(t, err, "%s %s should exist", tt.kind, tt.name)
		})
	}
}

func TestOpen_FTSTableExists(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	var name string
	err = d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='memories_fts'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("memories_fts table not found: %v", err)
	}
}

func TestOpen_AllTriggersExist(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	triggers := []string{"memories_ai", "memories_au", "memories_ad"}
	for _, trigger := range triggers {
		t.Run(trigger, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger,
			).Scan(&name)
			if err != nil {
				t.Errorf("trigger %q not found: %v", trigger, err)
			}
		})
	}
}

func TestValidateSchema_PassesOnValidDB(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	if err := validateSchema(d.sqlDB); err != nil {
		t.Errorf("validateSchema() failed on valid DB: %v", err)
	}
}

func TestValidateSchema_FailsOnMissingTrigger(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	if _, err := d.sqlDB.Exec("DROP TRIGGER memories_ai"); err != nil {
		t.Fatalf("failed to drop trigger: %v", err)
	}

	if err := validateSchema(d.sqlDB); err == nil {
		t.Error("validateSchema() should return error when trigger is missing")
	}
}

func TestOpen_NonExistentDirectory_ReturnsError(t *testing.T) {
	_, err := Open("/nonexistent/path/that/cannot/exist/db.sqlite")
	if err == nil {
		t.Error("Open() should return error when directory does not exist")
	}
}

func TestInitSchema_ClosedDB_ReturnsError(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Close the underlying connection to force initSchema to fail.
	_ = d.Close()

	err = initSchema(d.sqlDB)
	if err == nil {
		t.Error("initSchema() should return error on closed DB")
	}
}

func TestValidateSchema_SixTriggersAfterOpen(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	triggers := []string{
		"memories_ai", "memories_au", "memories_ad",
		"user_prompts_ai", "user_prompts_au", "user_prompts_ad",
	}
	for _, trigger := range triggers {
		t.Run(trigger, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger,
			).Scan(&name)
			if err != nil {
				t.Errorf("trigger %q not found: %v", trigger, err)
			}
		})
	}
}

// TestSchema_NoUniqueTopicKeyIndex asserts that a freshly opened DB does NOT have
// idx_unique_topic_key and DOES have idx_memories_topic_key as a non-unique index.
func TestSchema_NoUniqueTopicKeyIndex(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	// The old UNIQUE index must not exist.
	var oldName string
	err = d.sqlDB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_unique_topic_key'`,
	).Scan(&oldName)
	require.ErrorIs(t, err, sql.ErrNoRows, "idx_unique_topic_key should not exist on fresh schema")

	// The new non-unique index must exist.
	var newName string
	err = d.sqlDB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_memories_topic_key'`,
	).Scan(&newName)
	require.NoError(t, err, "idx_memories_topic_key should exist on fresh schema")
	assert.Equal(t, "idx_memories_topic_key", newName)

	// The index sql must NOT contain the word "UNIQUE".
	var idxSQL string
	err = d.sqlDB.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_memories_topic_key'`,
	).Scan(&idxSQL)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToUpper(idxSQL), "UNIQUE", "idx_memories_topic_key must not be a UNIQUE index")
}

// TestMigration_UserVersion3_DropsUniqueIndex asserts that a DB already at
// user_version 2 (with the old UNIQUE idx_unique_topic_key) is migrated to
// user_version 3, the unique index is removed, the non-unique replacement is
// created, and existing rows survive.
func TestMigration_UserVersion3_DropsUniqueIndex(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "legacy-v2.db")

	// NOTE: This fixture is synthetic. A real daemon-produced DB at user_version=2
	// cannot carry idx_unique_topic_key because migrateMemoriesAddSessionID drops and
	// recreates the memories table with a non-unique idx_memories_topic_key. This test
	// guards against manual or external DB manipulation that could reintroduce the
	// legacy unique index — not a normal production upgrade path.

	// Build a minimal user_version-2 DB that resembles what the daemon creates
	// after migrateMemoriesAddSessionID: memories table with session_id, plus the
	// UNIQUE topic_key index and PRAGMA user_version = 2.
	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Create minimal sessions table (required by FK in memories).
	_, err = rawDB.Exec(`CREATE TABLE sessions (
		id       TEXT PRIMARY KEY,
		sync_id  TEXT NOT NULL,
		project  TEXT NOT NULL,
		directory TEXT NOT NULL DEFAULT '',
		dev_id   TEXT NOT NULL,
		client   TEXT NOT NULL,
		started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		ended_at   DATETIME,
		summary    TEXT,
		synced_at  DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	// Create memories table with session_id (user_version 2 layout).
	_, err = rawDB.Exec(`CREATE TABLE memories (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		sync_id        TEXT NOT NULL,
		project        TEXT NOT NULL,
		topic_key      TEXT,
		category       TEXT NOT NULL DEFAULT '',
		title          TEXT NOT NULL,
		content        TEXT NOT NULL,
		tags           TEXT NOT NULL DEFAULT '[]',
		files_affected TEXT NOT NULL DEFAULT '[]',
		created_by     TEXT NOT NULL DEFAULT 'unknown',
		created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		synced_at      DATETIME,
		deleted_at     DATETIME,
		deleted_by     TEXT,
		delete_reason  TEXT,
		restored_at    DATETIME,
		confidence     TEXT NOT NULL DEFAULT '',
		impact_score   INTEGER NOT NULL DEFAULT 0,
		session_id     TEXT NOT NULL REFERENCES sessions(id)
	)`)
	require.NoError(t, err)

	// Create the OLD unique index (simulates user_version 2 state).
	_, err = rawDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_topic_key
		ON memories(project, topic_key) WHERE topic_key IS NOT NULL`)
	require.NoError(t, err)

	// Insert a sentinel session required by the FK.
	_, err = rawDB.Exec(`INSERT INTO sessions
		(id, sync_id, project, directory, dev_id, client)
		VALUES ('manual-save-test', 'sync-sentinel', 'test', '', 'test', 'test')`)
	require.NoError(t, err)

	// Insert a row to verify row preservation after migration.
	_, err = rawDB.Exec(`INSERT INTO memories
		(sync_id, project, topic_key, title, content, session_id)
		VALUES ('sync-abc', 'test', 'arch/auth', 'Auth design', 'Content', 'manual-save-test')`)
	require.NoError(t, err)

	_, err = rawDB.Exec(`PRAGMA user_version = 2`)
	require.NoError(t, err)
	require.NoError(t, rawDB.Close())

	// Open via our Open() — this triggers initSchema → migrateMemoriesTopicKeyNonUnique.
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	// user_version must be 3.
	var version int
	require.NoError(t, d.sqlDB.QueryRow("PRAGMA user_version").Scan(&version))
	assert.Equal(t, 3, version, "user_version should be bumped to 3 after migration")

	// Old UNIQUE index must be gone.
	var oldName string
	err = d.sqlDB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_unique_topic_key'`,
	).Scan(&oldName)
	require.ErrorIs(t, err, sql.ErrNoRows, "idx_unique_topic_key should be dropped by migration v3")

	// New non-unique index must exist.
	var newName string
	err = d.sqlDB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_memories_topic_key'`,
	).Scan(&newName)
	require.NoError(t, err, "idx_memories_topic_key should be created by migration v3")
	assert.Equal(t, "idx_memories_topic_key", newName)

	// Non-unique: sql must not contain UNIQUE.
	var idxSQL string
	require.NoError(t, d.sqlDB.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_memories_topic_key'`,
	).Scan(&idxSQL))
	assert.NotContains(t, strings.ToUpper(idxSQL), "UNIQUE")

	// Pre-existing row must still be there.
	var count int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = 'sync-abc'`).Scan(&count))
	assert.Equal(t, 1, count, "existing row must survive migration v3")
}

// seedOldShapeMemoryMutations creates a memory_mutations table as it existed
// before the request_id column and its partial unique index were introduced.
// It mirrors the CREATE TABLE in the migrations slice that predates the
// `ALTER TABLE memory_mutations ADD COLUMN request_id TEXT` migration.
func seedOldShapeMemoryMutations(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	_, err := sqlDB.Exec(`CREATE TABLE memory_mutations (
		sequence       INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id       TEXT NOT NULL UNIQUE,
		entity_type    TEXT NOT NULL DEFAULT 'memory',
		entity_sync_id TEXT NOT NULL,
		project        TEXT NOT NULL,
		op             TEXT NOT NULL,
		occurred_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		actor_id       TEXT NOT NULL DEFAULT '',
		base_updated_at DATETIME,
		payload_json   TEXT NOT NULL DEFAULT '{}',
		synced_at      DATETIME
	)`)
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE UNIQUE INDEX idx_memory_mutations_event_id ON memory_mutations(event_id)`,
		`CREATE INDEX idx_memory_mutations_project_unsynced ON memory_mutations(project, sequence) WHERE synced_at IS NULL`,
		`CREATE INDEX idx_memory_mutations_entity ON memory_mutations(entity_type, entity_sync_id, sequence)`,
	} {
		_, err := sqlDB.Exec(stmt)
		require.NoError(t, err)
	}
}

// TestOpen_UpgradesMemoryMutationsWithoutRequestID reproduces issue #459: on an
// upgraded DB whose memory_mutations table predates request_id, the base schema
// CREATE TABLE IF NOT EXISTS is a no-op, so a base-schema index referencing
// request_id fails fatally with "no such column: request_id" before the
// log-and-continue migrations slice (which adds the column and the index) can
// run. Open must succeed and leave both the column and the partial unique
// index in place.
func TestOpen_UpgradesMemoryMutationsWithoutRequestID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "legacy-mutations.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	seedOldShapeMemoryMutations(t, rawDB)
	require.NoError(t, rawDB.Close())

	d, err := Open(dbPath)
	require.NoError(t, err, "Open must not fail on a DB whose memory_mutations predates request_id")
	defer func() { _ = d.Close() }()

	var column string
	err = d.sqlDB.QueryRow(
		`SELECT name FROM pragma_table_info('memory_mutations') WHERE name = 'request_id'`,
	).Scan(&column)
	require.NoError(t, err, "request_id column should be added by the migrations slice")
	assert.Equal(t, "request_id", column)

	var idxSQL string
	err = d.sqlDB.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_memory_mutations_request_id'`,
	).Scan(&idxSQL)
	require.NoError(t, err, "idx_memory_mutations_request_id should exist after migrations")
	upper := strings.ToUpper(idxSQL)
	assert.Contains(t, upper, "UNIQUE", "index must be UNIQUE")
	assert.Contains(t, upper, "WHERE REQUEST_ID IS NOT NULL", "index must keep its partial predicate")
}

func TestOpen_MigratesLegacySyncStateHealthColumns(t *testing.T) {
	tests := []struct {
		name         string
		seedRows     func(t *testing.T, sqlDB *sql.DB)
		assertHealth func(t *testing.T, d *DB)
	}{
		{
			name: "existing auth and sync rows keep values and gain defaults",
			seedRows: func(t *testing.T, sqlDB *sql.DB) {
				_, err := sqlDB.Exec(`
				CREATE TABLE sync_state (
					project TEXT PRIMARY KEY,
					last_sync_at DATETIME,
					jwt_token TEXT,
					jwt_expires_at DATETIME
				);
				INSERT INTO sync_state (project, last_sync_at, jwt_token, jwt_expires_at)
				VALUES ('__auth__', NULL, 'jwt-token', '2030-01-02 03:04:05');
				INSERT INTO sync_state (project, last_sync_at)
				VALUES ('project-a', '2026-05-08 10:00:00');
				`)
				require.NoError(t, err)
			},
			assertHealth: func(t *testing.T, d *DB) {
				assert.Equal(t, "jwt-token", d.GetJWT())

				lastSync, err := d.GetLastSync("project-a")
				require.NoError(t, err)
				assert.Equal(t, time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC), lastSync)

				health, err := d.GetSyncHealth("project-a")
				require.NoError(t, err)
				assert.Equal(t, "project-a", health.Project)
				assert.Zero(t, health.ConsecutiveFailures)
				assert.True(t, health.LastAttemptAt.IsZero())
				assert.True(t, health.LastSuccessAt.IsZero())
				assert.True(t, health.LastFailureAt.IsZero())
				assert.True(t, health.BackoffUntil.IsZero())
				assert.Empty(t, health.LastError)

				authHealth, err := d.GetSyncHealth("__auth__")
				require.NoError(t, err)
				assert.Equal(t, "__auth__", authHealth.Project)
				assert.Zero(t, authHealth.ConsecutiveFailures)
				assert.Empty(t, authHealth.LastError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "legacy-sync-state.db")

			sqlDB, err := sql.Open("sqlite", dbPath)
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TABLE memories (id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT NOT NULL, project TEXT NOT NULL, topic_key TEXT, category TEXT NOT NULL DEFAULT '', title TEXT NOT NULL, content TEXT NOT NULL, tags TEXT NOT NULL DEFAULT '[]', files_affected TEXT NOT NULL DEFAULT '[]', created_by TEXT NOT NULL DEFAULT 'unknown', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, confidence TEXT NOT NULL DEFAULT '', impact_score INTEGER NOT NULL DEFAULT 0)")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE VIRTUAL TABLE memories_fts USING fts5(title, content, tags, content='memories', content_rowid='id', tokenize='unicode61')")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN INSERT INTO memories_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags); END")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TRIGGER memories_au AFTER UPDATE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, title, content, tags) VALUES ('delete', old.id, old.title, old.content, old.tags); INSERT INTO memories_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags); END")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, title, content, tags) VALUES ('delete', old.id, old.title, old.content, old.tags); END")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TABLE user_prompts (id INTEGER PRIMARY KEY AUTOINCREMENT, content TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME)")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE VIRTUAL TABLE user_prompts_fts USING fts5(content, content='user_prompts', content_rowid='id', tokenize='unicode61')")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TRIGGER user_prompts_ai AFTER INSERT ON user_prompts BEGIN INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TRIGGER user_prompts_au AFTER UPDATE ON user_prompts BEGIN INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content) VALUES ('delete', old.id, old.content); INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END")
			require.NoError(t, err)
			_, err = sqlDB.Exec("CREATE TRIGGER user_prompts_ad AFTER DELETE ON user_prompts BEGIN INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content) VALUES ('delete', old.id, old.content); END")
			require.NoError(t, err)

			tt.seedRows(t, sqlDB)
			require.NoError(t, sqlDB.Close())

			d, err := Open(dbPath)
			require.NoError(t, err)
			defer func() { require.NoError(t, d.Close()) }()

			for _, column := range []string{"last_attempt_at", "last_success_at", "last_failure_at", "consecutive_failures", "backoff_until", "last_error"} {
				var name string
				err = d.sqlDB.QueryRow("SELECT name FROM pragma_table_info('sync_state') WHERE name = ?", column).Scan(&name)
				require.NoErrorf(t, err, "column %s should exist after migration", column)
			}

			tt.assertHealth(t, d)
		})
	}
}
