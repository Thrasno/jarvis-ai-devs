package db

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// ─── T1.1 — sessions table schema ────────────────────────────────────────────

func TestSessionsTableExists(t *testing.T) {
	d := openTestDB(t)

	var name string
	err := d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'",
	).Scan(&name)
	require.NoError(t, err, "sessions table should exist after Open")
	assert.Equal(t, "sessions", name)
}

func TestSessionsTableColumns(t *testing.T) {
	d := openTestDB(t)

	// T4.0c — all 12 columns must be present (including created_at, updated_at added in Slice 4)
	expected := []string{
		"id", "sync_id", "project", "directory", "dev_id", "client",
		"started_at", "ended_at", "summary", "synced_at",
		"created_at", "updated_at",
	}

	for _, col := range expected {
		col := col
		t.Run(col, func(t *testing.T) {
			var colName string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM pragma_table_info('sessions') WHERE name = ?", col,
			).Scan(&colName)
			require.NoErrorf(t, err, "column %q should exist in sessions table", col)
			assert.Equal(t, col, colName)
		})
	}
}

func TestSessionsIndexesExist(t *testing.T) {
	d := openTestDB(t)

	for _, idx := range []string{"idx_sessions_project", "idx_sessions_started_at", "idx_sessions_dev_id"} {
		idx := idx
		t.Run(idx, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='index' AND name=?", idx,
			).Scan(&name)
			require.NoErrorf(t, err, "index %q should exist", idx)
		})
	}
}

// R2-WARN-2 — sessions.sync_id must be UNIQUE (parity with Postgres + Decision 2).
// Without it, idempotent sync re-pushes can create duplicate session rows.
func TestSessionsSyncIDUniqueConstraint(t *testing.T) {
	d := openTestDB(t)

	// First insert succeeds.
	_, err := d.sqlDB.Exec(`
		INSERT INTO sessions (id, sync_id, project, dev_id, client)
		VALUES ('sess-A', 'shared-sync-id', 'proj', 'dev', 'client')`)
	require.NoError(t, err)

	// Second insert with the same sync_id (different id) MUST fail UNIQUE.
	_, err = d.sqlDB.Exec(`
		INSERT INTO sessions (id, sync_id, project, dev_id, client)
		VALUES ('sess-B', 'shared-sync-id', 'proj', 'dev', 'client')`)
	require.Error(t, err, "duplicate sync_id must violate UNIQUE")
	assert.Contains(t, err.Error(), "UNIQUE",
		"the constraint name must be UNIQUE so callers can branch on it")
}

// R2-WARN-3 — memories.sync_id must be UNIQUE so that `INSERT OR IGNORE` in
// SaveFromRemote actually deduplicates re-pulls. Postgres has it; SQLite must too.
func TestMemoriesSyncIDUniqueConstraint(t *testing.T) {
	d := openTestDB(t)

	ensureManualSaveSessions(t, d, "r2w3-proj")

	insertViaRaw := func(syncID string) error {
		_, err := d.sqlDB.Exec(`
			INSERT INTO memories
			    (sync_id, project, category, title, content, created_by, session_id)
			VALUES (?, 'r2w3-proj', 'decision', 't', 'c', 'u', 'manual-save-r2w3-proj')`,
			syncID)
		return err
	}

	require.NoError(t, insertViaRaw("dup-sync-id"))
	err := insertViaRaw("dup-sync-id")
	require.Error(t, err, "second INSERT with duplicate sync_id must violate UNIQUE")
	assert.Contains(t, err.Error(), "UNIQUE")
}

func TestMigration_DevIDIndex_PresentAfterFreshInstall(t *testing.T) {
	d := openTestDB(t)

	var name string
	err := d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_sessions_dev_id'",
	).Scan(&name)
	require.NoError(t, err, "idx_sessions_dev_id must exist on fresh install")
	assert.Equal(t, "idx_sessions_dev_id", name)
}

func TestMigration_DevIDIndex_PresentAfterMigration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "devid_idx_test.db")

	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "s1", "project": "alpha", "title": "t1", "content": "c1", "created_at": "2026-01-01 10:00:00"},
	})

	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	var name string
	err = d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_sessions_dev_id'",
	).Scan(&name)
	require.NoError(t, err, "idx_sessions_dev_id must exist after migration from v1 DB")
	assert.Equal(t, "idx_sessions_dev_id", name)
}

func TestMigration_DevIDIndex_IdempotentOnReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "devid_idx_idempotent.db")

	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "s1", "project": "alpha", "title": "t1", "content": "c1", "created_at": "2026-01-01 10:00:00"},
	})

	d1, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, d1.Close())

	// Second open must not error (idempotent index creation)
	d2, err := Open(dbPath)
	require.NoError(t, err, "re-opening a migrated DB must succeed (idempotent idx_sessions_dev_id)")
	t.Cleanup(func() { _ = d2.Close() })

	var name string
	err = d2.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_sessions_dev_id'",
	).Scan(&name)
	require.NoError(t, err, "idx_sessions_dev_id must still exist after re-open")
}

// ─── T1.2 — memories table recreation with session_id NOT NULL + FTS5 rebuild ─

// seedV1DB creates a file-based SQLite DB with the pre-migration schema (v1):
// memories table has NO session_id column. The caller is responsible for the
// temp dir lifetime; the returned *sql.DB must be closed before Open() is called.
func seedV1DB(t *testing.T, dbPath string, memories []map[string]string) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	_, err = sqlDB.Exec(`CREATE TABLE memories (
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
		confidence     TEXT NOT NULL DEFAULT '',
		impact_score   INTEGER NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE VIRTUAL TABLE memories_fts USING fts5(
		title, content, tags,
		content='memories', content_rowid='id', tokenize='unicode61'
	)`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts(rowid, title, content, tags)
		VALUES (new.id, new.title, new.content, new.tags);
	END`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TRIGGER memories_au AFTER UPDATE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
		VALUES ('delete', old.id, old.title, old.content, old.tags);
		INSERT INTO memories_fts(rowid, title, content, tags)
		VALUES (new.id, new.title, new.content, new.tags);
	END`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
		VALUES ('delete', old.id, old.title, old.content, old.tags);
	END`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TABLE sync_state (
		project TEXT PRIMARY KEY, last_sync_at DATETIME, jwt_token TEXT,
		jwt_expires_at DATETIME, last_attempt_at DATETIME, last_success_at DATETIME,
		last_failure_at DATETIME, consecutive_failures INTEGER NOT NULL DEFAULT 0,
		backoff_until DATETIME, last_error TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TABLE user_prompts (
		id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT NOT NULL DEFAULT '',
		project TEXT NOT NULL DEFAULT '', content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME
	)`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE VIRTUAL TABLE user_prompts_fts USING fts5(
		content, content='user_prompts', content_rowid='id', tokenize='unicode61'
	)`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_ai AFTER INSERT ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content);
	END`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_au AFTER UPDATE ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
		VALUES ('delete', old.id, old.content);
		INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content);
	END`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_ad AFTER DELETE ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
		VALUES ('delete', old.id, old.content);
	END`)
	require.NoError(t, err)

	// user_prompts_project_created index (added by existing migration)
	_, err = sqlDB.Exec(`CREATE INDEX IF NOT EXISTS idx_user_prompts_project_created ON user_prompts(project, created_at DESC)`)
	require.NoError(t, err)

	for _, m := range memories {
		_, err = sqlDB.Exec(
			`INSERT INTO memories (sync_id, project, title, content, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			m["sync_id"], m["project"], m["title"], m["content"],
			m["created_at"],
		)
		require.NoError(t, err)
	}

	require.NoError(t, sqlDB.Close())
}

func TestMigration_MemoriesSessionIDColumnExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration_test.db")

	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "sync-1", "project": "alpha", "title": "t1", "content": "c1", "created_at": "2026-01-01 10:00:00"},
	})

	// Open runs migrations — this is the system under test
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	// session_id column must exist after migration
	var colName string
	err = d.sqlDB.QueryRow(
		"SELECT name FROM pragma_table_info('memories') WHERE name = 'session_id'",
	).Scan(&colName)
	require.NoError(t, err, "memories.session_id column should exist after migration")
	assert.Equal(t, "session_id", colName)
}

func TestMigration_FTS5TriggersIntactAfterRecreate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration_fts_test.db")

	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "sync-2", "project": "beta", "title": "t2", "content": "c2", "created_at": "2026-01-01 11:00:00"},
	})

	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	// All three FTS5 triggers must be present after the recreate-table dance
	for _, trigger := range []string{"memories_ai", "memories_au", "memories_ad"} {
		trigger := trigger
		t.Run(trigger, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger,
			).Scan(&name)
			require.NoErrorf(t, err, "FTS5 trigger %q should exist after migration", trigger)
		})
	}
}

// ─── T1.3 — sentinel session creation ────────────────────────────────────────

func TestMigration_SentinelSessionCreated(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sentinel_test.db")

	// minCreatedAt is the earliest memory for project 'alpha'; sentinel must use it.
	const minCreatedAt = "2026-01-01 08:00:00"
	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "s1", "project": "alpha", "title": "early", "content": "c", "created_at": minCreatedAt},
		{"sync_id": "s2", "project": "alpha", "title": "late", "content": "c", "created_at": "2026-01-02 10:00:00"},
	})

	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	var id, devID, startedAtStr string
	var endedAt sql.NullString

	err = d.sqlDB.QueryRow(`
		SELECT id, dev_id, ended_at, started_at
		FROM sessions WHERE id = 'legacy-pre-lifecycle-alpha'
	`).Scan(&id, &devID, &endedAt, &startedAtStr)
	require.NoError(t, err, "sentinel session should exist for project 'alpha'")

	assert.Equal(t, "legacy-pre-lifecycle-alpha", id)
	assert.Equal(t, "legacy", devID)
	assert.True(t, endedAt.Valid, "sentinel ended_at should be set (not NULL)")

	// Compare as parsed times to be format-agnostic (SQLite may return either layout)
	wantTime, _ := parseTimeStr(minCreatedAt)
	gotTime, err := parseTimeStr(startedAtStr)
	require.NoError(t, err, "started_at should be a parseable timestamp")
	assert.Equal(t, wantTime, gotTime, "sentinel started_at should equal MIN(memories.created_at)")
}

// ─── T1.4 — backfill memories with sentinel session_id ───────────────────────

func TestMigration_AllMemoriesBackfilled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "backfill_test.db")

	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "s1", "project": "alpha", "title": "t1", "content": "c", "created_at": "2026-01-01 10:00:00"},
		{"sync_id": "s2", "project": "alpha", "title": "t2", "content": "c", "created_at": "2026-01-02 10:00:00"},
		{"sync_id": "s3", "project": "beta", "title": "t3", "content": "c", "created_at": "2026-01-03 10:00:00"},
	})

	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	// Every memory must have a non-NULL session_id after migration
	rows, err := d.sqlDB.Query(`SELECT project, session_id FROM memories ORDER BY sync_id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var project, sessionID string
		require.NoError(t, rows.Scan(&project, &sessionID))
		expected := "legacy-pre-lifecycle-" + project
		assert.Equal(t, expected, sessionID,
			"memory in project %q should have session_id %q", project, expected)
	}
	require.NoError(t, rows.Err())
}

// ─── T1.5 — migration idempotency ────────────────────────────────────────────

func TestMigration_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "idempotent_test.db")

	seedV1DB(t, dbPath, []map[string]string{
		{"sync_id": "s1", "project": "alpha", "title": "t", "content": "c", "created_at": "2026-01-01 10:00:00"},
	})

	// First open — runs migration
	d1, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, d1.Close())

	// Second open — must be a no-op (idempotent)
	d2, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d2.Close() })

	// Sentinel row count must be exactly 1 (no duplicates)
	var count int
	err = d2.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE id = 'legacy-pre-lifecycle-alpha'`,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "re-running migration must not create duplicate sentinel rows")

	// Memory count unchanged
	var memCount int
	err = d2.sqlDB.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&memCount)
	require.NoError(t, err)
	assert.Equal(t, 1, memCount, "re-running migration must not duplicate memory rows")
}

// ─── T1.6 — Session model + db/session.go CRUD ───────────────────────────────

func TestCreateSession_HappyPath(t *testing.T) {
	d := openTestDB(t)
	directory := t.TempDir()

	err := d.CreateSession("sess-001", "jarvis-dev", directory, "andres", "claude-code")
	require.NoError(t, err)

	var id, project, gotDirectory, devID, client string
	var endedAt sql.NullString
	err = d.sqlDB.QueryRow(
		`SELECT id, project, directory, dev_id, client, ended_at FROM sessions WHERE id = ?`, "sess-001",
	).Scan(&id, &project, &gotDirectory, &devID, &client, &endedAt)
	require.NoError(t, err)

	assert.Equal(t, "sess-001", id)
	assert.Equal(t, "jarvis-dev", project)
	assert.Equal(t, directory, gotDirectory)
	assert.Equal(t, "andres", devID)
	assert.Equal(t, "claude-code", client)
	assert.False(t, endedAt.Valid, "ended_at should be NULL for a new session")
}

func TestGetSession_HappyPath(t *testing.T) {
	d := openTestDB(t)
	directory := t.TempDir()

	require.NoError(t, d.CreateSession("sess-get", "proj", directory, "dev", "cli"))

	sess, err := d.GetSession("sess-get")
	require.NoError(t, err)
	require.NotNil(t, sess)

	assert.Equal(t, "sess-get", sess.ID)
	assert.Equal(t, "proj", sess.Project)
	assert.Equal(t, directory, sess.Directory)
	assert.Equal(t, "dev", sess.DevID)
	assert.Equal(t, "cli", sess.Client)
	assert.Nil(t, sess.EndedAt, "EndedAt should be nil for an open session")
}

func TestGetSession_NotFound_ReturnsErrSessionNotFound(t *testing.T) {
	d := openTestDB(t)

	_, err := d.GetSession("does-not-exist")
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestEndSession_HappyPath(t *testing.T) {
	d := openTestDB(t)

	require.NoError(t, d.CreateSession("sess-end", "proj", t.TempDir(), "dev", "cli"))

	err := d.EndSession("sess-end", "all done")
	require.NoError(t, err)

	sess, err := d.GetSession("sess-end")
	require.NoError(t, err)
	require.NotNil(t, sess.EndedAt)
	assert.Equal(t, "all done", sess.Summary)
}

func TestListSessions_HappyPath(t *testing.T) {
	d := openTestDB(t)
	dirRoot := t.TempDir()

	require.NoError(t, d.CreateSession("ls-1", "projA", filepath.Join(dirRoot, "d1"), "dev", "cli"))
	require.NoError(t, d.CreateSession("ls-2", "projA", filepath.Join(dirRoot, "d2"), "dev", "cli"))
	require.NoError(t, d.CreateSession("ls-3", "projB", filepath.Join(dirRoot, "d3"), "dev", "cli"))

	sessions, err := d.ListSessions("projA", 10)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	for _, s := range sessions {
		assert.Equal(t, "projA", s.Project)
	}
}

// ─── T1.7 — EnsureManualSaveSession ──────────────────────────────────────────

func TestEnsureManualSaveSession_CreatesRowOnFirstCall(t *testing.T) {
	d := openTestDB(t)

	id, err := d.EnsureManualSaveSession("jarvis-dev")
	require.NoError(t, err)
	assert.Equal(t, "manual-save-jarvis-dev", id)

	sess, err := d.GetSession("manual-save-jarvis-dev")
	require.NoError(t, err)
	assert.Equal(t, "jarvis-dev", sess.Project)
	assert.Equal(t, "manual", sess.Client)
	assert.Nil(t, sess.EndedAt, "manual-save session ended_at must stay NULL")
}

func TestEnsureManualSaveSession_IdempotentOnSecondCall(t *testing.T) {
	d := openTestDB(t)

	id1, err := d.EnsureManualSaveSession("jarvis-dev")
	require.NoError(t, err)

	id2, err := d.EnsureManualSaveSession("jarvis-dev")
	require.NoError(t, err)

	assert.Equal(t, id1, id2)

	var count int
	err = d.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'manual-save-jarvis-dev'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "second call must not create a second row")
}

// ─── T1.8 — stale-session scanner ────────────────────────────────────────────

func TestAutoCloseStale_ClosesOldSessions(t *testing.T) {
	d := openTestDB(t)
	directory := t.TempDir()

	now := time.Now().UTC()
	staleTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-1 * time.Hour)
	manualTime := now.Add(-7 * 24 * time.Hour)

	insertSessionAt := func(id, started string) {
		t.Helper()
		_, err := d.sqlDB.Exec(
			`INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at)
			 VALUES (?, ?, 'proj', ?, 'dev', 'cli', ?)`,
			id, "sync-"+id, directory, started,
		)
		require.NoError(t, err)
	}

	insertSessionAt("sess-stale", staleTime.Format("2006-01-02 15:04:05"))
	insertSessionAt("sess-recent", recentTime.Format("2006-01-02 15:04:05"))
	insertSessionAt("manual-save-proj", manualTime.Format("2006-01-02 15:04:05"))

	_, err := d.AutoCloseStale(24*time.Hour, func() time.Time { return now })
	require.NoError(t, err)

	// Stale session must be closed
	staleSess, err := d.GetSession("sess-stale")
	require.NoError(t, err)
	require.NotNil(t, staleSess.EndedAt, "stale session should be closed")
	assert.Equal(t, "[auto-closed: daemon restart]", staleSess.Summary)

	// Recent session must remain open
	recentSess, err := d.GetSession("sess-recent")
	require.NoError(t, err)
	assert.Nil(t, recentSess.EndedAt, "recent session must NOT be auto-closed")

	// manual-save-* session must remain open regardless of age
	manualSess, err := d.GetSession("manual-save-proj")
	require.NoError(t, err)
	assert.Nil(t, manualSess.EndedAt, "manual-save session must never be auto-closed")
}

// ─── T1.9 — concurrent EnsureManualSaveSession race test ─────────────────────

func TestEnsureManualSaveSession_ConcurrentCreate_ExactlyOneRow(t *testing.T) {
	t.Parallel()

	d := openTestDB(t)

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = d.EnsureManualSaveSession("concurrent-proj")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoErrorf(t, err, "goroutine %d returned error", i)
	}

	var count int
	err := d.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'manual-save-concurrent-proj'`).Scan(&count)
	require.NoError(t, err)
	// SetMaxOpenConns(1) serializes writes; INSERT OR IGNORE ensures exactly one row
	assert.Equal(t, 1, count, "concurrent calls must produce exactly one session row")
}

// CRIT-5 — fresh-install schema must enforce NOT NULL on memories.session_id.
// The migration short-circuits when pragma_table_info shows the column already
// exists; that path bumps user_version=2 without touching the column. So the
// base CREATE TABLE in `schema` MUST declare NOT NULL from the start.
func TestFreshInstall_MemoriesSessionIDIsNotNull(t *testing.T) {
	d := openTestDB(t)

	var notNull int
	err := d.sqlDB.QueryRow(
		"SELECT \"notnull\" FROM pragma_table_info('memories') WHERE name = 'session_id'",
	).Scan(&notNull)
	require.NoError(t, err, "memories.session_id column must exist on fresh install")
	assert.Equal(t, 1, notNull,
		"fresh-install memories.session_id must be NOT NULL (FR-D-2). Migration short-circuits on this path so the base schema must enforce it.")
}

// R3-FIX-4 — UNIQUE index retrofit on sessions.sync_id MUST fail loudly with
// an actionable error message when duplicate sync_ids exist in a pre-R2 DB.
// Previously the loop swallowed the error via `warn:` and the daemon started
// without the UNIQUE protection — silent regression of R2-WARN-2 parity.
func TestUniqueIndexRetrofit_FailsLoudlyOnDuplicateSyncIDs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "r3f4-dup-syncids.db")

	// Hand-craft a v1-ish DB whose `sessions` table has NO UNIQUE on sync_id and
	// already contains duplicate rows. We bypass Open()/initSchema by writing the
	// schema directly with the constraint REMOVED, and seed the duplicates.
	sqlDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Minimal pre-R2 schema (sessions WITHOUT UNIQUE on sync_id).
	_, err = sqlDB.Exec(`CREATE TABLE memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT NOT NULL,
		project TEXT NOT NULL, topic_key TEXT, category TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL, content TEXT NOT NULL,
		tags TEXT NOT NULL DEFAULT '[]', files_affected TEXT NOT NULL DEFAULT '[]',
		created_by TEXT NOT NULL DEFAULT 'unknown',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		confidence TEXT NOT NULL DEFAULT '', impact_score INTEGER NOT NULL DEFAULT 0)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE VIRTUAL TABLE memories_fts USING fts5(
		title, content, tags, content='memories', content_rowid='id', tokenize='unicode61')`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts(rowid, title, content, tags)
		VALUES (new.id, new.title, new.content, new.tags); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER memories_au AFTER UPDATE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
		VALUES ('delete', old.id, old.title, old.content, old.tags);
		INSERT INTO memories_fts(rowid, title, content, tags)
		VALUES (new.id, new.title, new.content, new.tags); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
		VALUES ('delete', old.id, old.title, old.content, old.tags); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TABLE sync_state (
		project TEXT PRIMARY KEY, last_sync_at DATETIME, jwt_token TEXT, jwt_expires_at DATETIME)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TABLE user_prompts (
		id INTEGER PRIMARY KEY AUTOINCREMENT, content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE VIRTUAL TABLE user_prompts_fts USING fts5(
		content, content='user_prompts', content_rowid='id', tokenize='unicode61')`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_ai AFTER INSERT ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_au AFTER UPDATE ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
		VALUES ('delete', old.id, old.content);
		INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_ad AFTER DELETE ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
		VALUES ('delete', old.id, old.content); END`)
	require.NoError(t, err)

	// Pre-R2 sessions: NO UNIQUE on sync_id.
	_, err = sqlDB.Exec(`CREATE TABLE sessions (
		id          TEXT PRIMARY KEY,
		sync_id     TEXT NOT NULL,
		project     TEXT NOT NULL,
		directory   TEXT NOT NULL DEFAULT '',
		dev_id      TEXT NOT NULL,
		client      TEXT NOT NULL,
		started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		ended_at    DATETIME,
		summary     TEXT,
		synced_at   DATETIME)`)
	require.NoError(t, err)

	// Seed two sessions with the SAME sync_id — pre-R2 had no UNIQUE so this is allowed.
	_, err = sqlDB.Exec(
		`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES (?, ?, ?, ?, ?)`,
		"sess-1", "duplicate-sync-id", "p", "d", "c")
	require.NoError(t, err)
	_, err = sqlDB.Exec(
		`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES (?, ?, ?, ?, ?)`,
		"sess-2", "duplicate-sync-id", "p", "d", "c")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// Open MUST now fail with an actionable error (not silently warn-and-continue).
	_, err = Open(dbPath)
	require.Error(t, err, "Open() must fail loudly when duplicate sync_ids prevent UNIQUE retrofit")
	assert.Contains(t, err.Error(), "duplicate sync_id",
		"error must mention duplicate sync_id so operators know what's wrong")
	assert.Contains(t, err.Error(), "manual cleanup required",
		"error must instruct operators that manual cleanup is required")
}

// R4-FIX-5 — migrateMemoriesAddSessionID's recreate-table dance creates
// `idx_memories_sync_id` as a UNIQUE index. On a pre-Slice2 DB with duplicate
// memories.sync_id values the index creation fails. Pre-fix the failure was
// wrapped only with "recreate index: %w" (cryptic SQLite UNIQUE message).
// The fix detects the UNIQUE-on-sync_id failure and surfaces the actionable
// cleanup hint (parity with R3-FIX-4).
func TestMigrateMemoriesAddSessionID_DuplicateMemoriesSyncIDs_FailsLoudly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "r4f5-dup-mem-sync.db")

	// Hand-craft a pre-Slice2 DB: memories table WITHOUT session_id, no sessions
	// table, no idx_memories_sync_id. Bypass Open()/initSchema by writing the
	// minimal schema directly and seeding duplicate sync_ids.
	sqlDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	_, err = sqlDB.Exec(`CREATE TABLE memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT NOT NULL,
		project TEXT NOT NULL, topic_key TEXT, category TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL, content TEXT NOT NULL,
		tags TEXT NOT NULL DEFAULT '[]', files_affected TEXT NOT NULL DEFAULT '[]',
		created_by TEXT NOT NULL DEFAULT 'unknown',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		synced_at DATETIME,
		confidence TEXT NOT NULL DEFAULT '', impact_score INTEGER NOT NULL DEFAULT 0)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE VIRTUAL TABLE memories_fts USING fts5(
		title, content, tags, content='memories', content_rowid='id', tokenize='unicode61')`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts(rowid, title, content, tags)
		VALUES (new.id, new.title, new.content, new.tags); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER memories_au AFTER UPDATE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
		VALUES ('delete', old.id, old.title, old.content, old.tags);
		INSERT INTO memories_fts(rowid, title, content, tags)
		VALUES (new.id, new.title, new.content, new.tags); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
		VALUES ('delete', old.id, old.title, old.content, old.tags); END`)
	require.NoError(t, err)

	// Pre-create idx_memories_sync_id as NON-unique so the schema's
	// `CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_sync_id` is a no-op (name
	// already taken, definition irrelevant for IF NOT EXISTS). This forces the
	// failure path into migrateMemoriesAddSessionID's recreate-table dance,
	// which is the path R4-FIX-5 protects.
	_, err = sqlDB.Exec(`CREATE INDEX idx_memories_sync_id ON memories(sync_id)`)
	require.NoError(t, err)

	// sync_state and sync_state-related tables required by the schema.
	_, err = sqlDB.Exec(`CREATE TABLE sync_state (
		project TEXT PRIMARY KEY, last_sync_at DATETIME, jwt_token TEXT, jwt_expires_at DATETIME)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TABLE user_prompts (
		id INTEGER PRIMARY KEY AUTOINCREMENT, content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME)`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE VIRTUAL TABLE user_prompts_fts USING fts5(
		content, content='user_prompts', content_rowid='id', tokenize='unicode61')`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_ai AFTER INSERT ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_au AFTER UPDATE ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
		VALUES ('delete', old.id, old.content);
		INSERT INTO user_prompts_fts(rowid, content) VALUES (new.id, new.content); END`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TRIGGER user_prompts_ad AFTER DELETE ON user_prompts BEGIN
		INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
		VALUES ('delete', old.id, old.content); END`)
	require.NoError(t, err)

	// Seed two memories with the SAME sync_id — pre-Slice2 had no UNIQUE.
	for i := 0; i < 2; i++ {
		_, err = sqlDB.Exec(
			`INSERT INTO memories (sync_id, project, title, content) VALUES (?, ?, ?, ?)`,
			"duplicate-sync-id", "p", "t", "c")
		require.NoError(t, err)
	}
	require.NoError(t, sqlDB.Close())

	// Open() runs initSchema → migrations loop → migrateMemoriesAddSessionID.
	// The recreate-table dance creates idx_memories_sync_id which MUST fail loudly
	// because the seeded memories have duplicate sync_ids.
	_, err = Open(dbPath)
	require.Error(t, err, "Open() must fail when migrateMemoriesAddSessionID can't create UNIQUE on duplicate sync_ids")
	assert.Contains(t, err.Error(), "duplicate sync_id",
		"error must mention duplicate sync_id (actionable, not cryptic UNIQUE failure)")
	assert.Contains(t, err.Error(), "manual cleanup required",
		"error must instruct operators that manual cleanup is required")
}

// CRIT-5 / Suspect-A — fresh-install must create idx_memories_session.
// Without it, FR-D-2 is unmet and `WHERE session_id = ?` queries are full
// table scans.
func TestFreshInstall_MemoriesSessionIndexExists(t *testing.T) {
	d := openTestDB(t)

	var name string
	err := d.sqlDB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_memories_session'",
	).Scan(&name)
	require.NoError(t, err, "idx_memories_session must exist on fresh install (FR-D-2)")
}
