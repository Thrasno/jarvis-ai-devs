package db

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

-- project_identities is the authoritative local mapping from canonical storage
-- keys to their stable display metadata. Project-bearing tables retain TEXT keys
-- because SQLite cannot add a foreign key to an existing column in place.
CREATE TABLE IF NOT EXISTS project_identities (
    project_key       TEXT PRIMARY KEY,
    first_spelling    TEXT NOT NULL,
    first_seen_at     DATETIME NOT NULL,
    first_source      TEXT NOT NULL,
    remote_spelling   TEXT NOT NULL DEFAULT '',
    remote_seen_at    DATETIME,
    remote_source     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_project_identities_first_seen
ON project_identities(first_seen_at, project_key);

CREATE TABLE IF NOT EXISTS memories (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id         TEXT NOT NULL,
    project         TEXT NOT NULL,
    topic_key       TEXT,
    category        TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    tags            TEXT NOT NULL DEFAULT '[]',
    files_affected  TEXT NOT NULL DEFAULT '[]',
    created_by      TEXT NOT NULL DEFAULT 'unknown',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    synced_at       DATETIME,
    deleted_at      DATETIME,
    deleted_by      TEXT,
    delete_reason   TEXT,
    restored_at     DATETIME,
    confidence      TEXT NOT NULL DEFAULT '',
    impact_score    INTEGER NOT NULL DEFAULT 0,
    session_id      TEXT NOT NULL REFERENCES sessions(id)
);

-- sync_state guarda el JWT y el timestamp del último sync por proyecto.
-- La fila con project='__auth__' almacena el token global.
CREATE TABLE IF NOT EXISTS sync_state (
    project         TEXT PRIMARY KEY,
    last_sync_at    DATETIME,
    jwt_token       TEXT,
	jwt_expires_at  DATETIME,
	last_attempt_at DATETIME,
	last_success_at DATETIME,
	last_failure_at DATETIME,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	backoff_until   DATETIME,
	last_error      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_memories_topic_key
ON memories(project, topic_key)
WHERE topic_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project);
CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at DESC);
-- R2-WARN-3 — UNIQUE on sync_id makes INSERT OR IGNORE actually deduplicate
-- re-pulls. Without it, the same remote memory could be inserted multiple times.
-- Postgres has the equivalent constraint on memories.sync_id; this is the parity.
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_sync_id ON memories(sync_id) WHERE sync_id != '';
-- idx_memories_session is created AFTER migrateMemoriesAddSessionID runs so the
-- column always exists. See initSchema. Suspect-A / FR-D-2.

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    title, content, tags,
    content='memories',
    content_rowid='id',
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, title, content, tags)
    VALUES (new.id, new.title, new.content, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
    VALUES ('delete', old.id, old.title, old.content, old.tags);
    INSERT INTO memories_fts(rowid, title, content, tags)
    VALUES (new.id, new.title, new.content, new.tags);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
    VALUES ('delete', old.id, old.title, old.content, old.tags);
END;

-- ─── sessions ────────────────────────────────────────────────────────────────
-- Tracks explicit sessions (mem_session_start/end) and implicit manual-save-*
-- sessions created by the lazy fallback path.
-- R2-WARN-2 — sync_id es UNIQUE para paridad con Postgres (Decisión 2). Esto
-- garantiza que reenvíos idempotentes desde el daemon no producen filas duplicadas.
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    sync_id     TEXT NOT NULL UNIQUE,
    project     TEXT NOT NULL,
    directory   TEXT NOT NULL DEFAULT '',
    dev_id      TEXT NOT NULL,
    client      TEXT NOT NULL,
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at    DATETIME,
    summary     TEXT,
    synced_at   DATETIME,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- sync_from_project is a pending write-side precondition, not history: it
    -- names the project literal the SERVER still holds for this row after the
    -- local identity migration renamed it here. The push sends it as
    -- from_project so the server moves that exact row and nothing else, and it
    -- is cleared the moment the row is acked. Empty means "no relocation
    -- pending", which is every row's normal state.
    sync_from_project TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_project    ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_dev_id     ON sessions(dev_id);
-- idx_sessions_sync_id mirrors Postgres and accelerates conflict-target lookups.
-- The UNIQUE constraint above already implies an index, but we declare it explicitly
-- under a stable name so daemon/server queries can rely on the same name.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_sync_id ON sessions(sync_id);

CREATE TABLE IF NOT EXISTS memory_mutations (
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
    request_id     TEXT,
    synced_at      DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_mutations_event_id ON memory_mutations(event_id);
CREATE INDEX IF NOT EXISTS idx_memory_mutations_project_unsynced ON memory_mutations(project, sequence) WHERE synced_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_memory_mutations_entity ON memory_mutations(entity_type, entity_sync_id, sequence);
-- idx_memory_mutations_request_id is created in the migrations slice, AFTER the
-- ALTER TABLE that adds request_id. Declaring it here breaks upgraded DBs whose
-- memory_mutations predates the column: CREATE TABLE IF NOT EXISTS is a no-op,
-- the index references a missing column, and the fatal base-schema exec kills
-- the daemon before migrations run (issue #459). Same class as idx_memories_session.

CREATE TABLE IF NOT EXISTS mutation_receipts (
    request_id     TEXT PRIMARY KEY,
    operation      TEXT NOT NULL,
    target_id      INTEGER NOT NULL,
    project        TEXT NOT NULL,
    entity_sync_id TEXT NOT NULL,
    event_id       TEXT NOT NULL,
    actor_id       TEXT NOT NULL DEFAULT '',
    reason         TEXT NOT NULL DEFAULT '',
    local_status   TEXT NOT NULL,
    shared_status  TEXT NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mutation_cursors (
    consumer       TEXT NOT NULL,
    project        TEXT NOT NULL,
    sequence       INTEGER NOT NULL DEFAULT 0,
    event_id       TEXT NOT NULL DEFAULT '',
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consumer, project)
);

-- pull_cursors persists the bounded legacy-pull resume position (PR 2a/2b,
-- hive-sync-batched-drain) per (consumer, project, channel). "channel"
-- distinguishes the two independently-paginated legacy pull channels —
-- "memories" and "sessions" — mirroring mutation_cursors' shape one level
-- deeper, since pull pagination needs one cursor per channel per project
-- rather than a single cursor per project.
CREATE TABLE IF NOT EXISTS pull_cursors (
    consumer       TEXT NOT NULL,
    project        TEXT NOT NULL,
    channel        TEXT NOT NULL,
    synced_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sync_id        TEXT NOT NULL DEFAULT '',
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consumer, project, channel)
);

CREATE TABLE IF NOT EXISTS memory_prompt_links (
    memory_id  INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    prompt_id  INTEGER NOT NULL REFERENCES user_prompts(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (memory_id, prompt_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_prompt_links_prompt_id ON memory_prompt_links(prompt_id);

CREATE TABLE IF NOT EXISTS user_prompts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id    TEXT    NOT NULL DEFAULT '',
    project    TEXT    NOT NULL DEFAULT '',
    session_id TEXT    NOT NULL DEFAULT '',
    content    TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    synced_at  DATETIME,
    -- See sessions.sync_from_project: same pending relocation precondition.
    sync_from_project TEXT NOT NULL DEFAULT ''
);

CREATE VIRTUAL TABLE IF NOT EXISTS user_prompts_fts USING fts5(
    content,
    content='user_prompts',
    content_rowid='id',
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS user_prompts_ai AFTER INSERT ON user_prompts BEGIN
    INSERT INTO user_prompts_fts(rowid, content)
    VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS user_prompts_au AFTER UPDATE ON user_prompts BEGIN
    INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
    VALUES ('delete', old.id, old.content);
    INSERT INTO user_prompts_fts(rowid, content)
    VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS user_prompts_ad AFTER DELETE ON user_prompts BEGIN
    INSERT INTO user_prompts_fts(user_prompts_fts, rowid, content)
    VALUES ('delete', old.id, old.content);
END;

CREATE TABLE IF NOT EXISTS recovery_tokens (
    token             TEXT PRIMARY KEY,
    reason            TEXT NOT NULL,
    requested_project TEXT NOT NULL,
    selected_project  TEXT NOT NULL DEFAULT '',
    candidates_json   TEXT NOT NULL,
    context_hash      TEXT NOT NULL,
    created_at        DATETIME NOT NULL,
    expires_at        DATETIME NOT NULL,
    consumed_at       DATETIME
);

CREATE INDEX IF NOT EXISTS idx_recovery_tokens_expires ON recovery_tokens(expires_at);

CREATE TABLE IF NOT EXISTS hive_warnings (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    severity         TEXT NOT NULL,
    source           TEXT NOT NULL,
    message          TEXT NOT NULL,
    resolution_state TEXT NOT NULL DEFAULT 'active' CHECK (resolution_state IN ('active', 'resolved')),
    resolved_at      DATETIME
);

CREATE INDEX IF NOT EXISTS idx_hive_warnings_created_at ON hive_warnings(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_hive_warnings_resolution_state ON hive_warnings(resolution_state, created_at DESC);

CREATE TABLE IF NOT EXISTS sync_attempt_logs (
    attempt_id       TEXT PRIMARY KEY,
    dev_id           TEXT NOT NULL DEFAULT '',
    project          TEXT NOT NULL,
    client           TEXT NOT NULL DEFAULT '',
    daemon_id        TEXT NOT NULL DEFAULT '',
    started_at       DATETIME NOT NULL,
    ended_at         DATETIME NOT NULL,
    outcome          TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
    http_status      INTEGER NOT NULL DEFAULT 0,
    error_code       TEXT NOT NULL DEFAULT '',
    error_message    TEXT NOT NULL DEFAULT '',
    request_id       TEXT NOT NULL DEFAULT '',
    sync_counts_json TEXT NOT NULL DEFAULT '{}',
    metadata_json    TEXT NOT NULL DEFAULT '{}',
    delivered_at     DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_pending
ON sync_attempt_logs(delivered_at, started_at)
WHERE delivered_at IS NULL AND dev_id != '';

CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_retention
ON sync_attempt_logs(ended_at);

CREATE TABLE IF NOT EXISTS project_blocks (
    canonical_project_key TEXT PRIMARY KEY,
    project               TEXT NOT NULL,
    command_id            TEXT NOT NULL,
    ack_token             TEXT NOT NULL DEFAULT '',
    reason                TEXT NOT NULL DEFAULT '',
    action                TEXT NOT NULL DEFAULT 'block',
    generation            INTEGER NOT NULL DEFAULT 1,
    blocked               INTEGER NOT NULL DEFAULT 1,
    blocked_at            DATETIME NOT NULL,
    ack_pending           INTEGER NOT NULL DEFAULT 1,
    ack_status            TEXT NOT NULL DEFAULT '',
    ack_warning           TEXT NOT NULL DEFAULT '',
    ack_applied_at        DATETIME,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_project_blocks_canonical ON project_blocks(canonical_project_key);
CREATE INDEX IF NOT EXISTS idx_project_blocks_pending_ack ON project_blocks(ack_pending, blocked_at);

CREATE TABLE IF NOT EXISTS project_quarantine_archives (
    canonical_project_key TEXT PRIMARY KEY,
    project               TEXT NOT NULL,
    command_id            TEXT NOT NULL,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hive_project_governance (
    project        TEXT PRIMARY KEY,
    archived_at    DATETIME,
    archived_by    TEXT NOT NULL DEFAULT '',
    archive_reason TEXT NOT NULL DEFAULT '',
    merge_target   TEXT NOT NULL DEFAULT '',
    merged_at      DATETIME,
    merged_by      TEXT NOT NULL DEFAULT '',
    merge_reason   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS import_runs (
    id                 TEXT PRIMARY KEY,
    source_system      TEXT NOT NULL,
    source_path        TEXT NOT NULL DEFAULT '',
    source_fingerprint TEXT NOT NULL DEFAULT '',
    mode               TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT 'completed',
    started_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at       DATETIME,
    report_json        TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS import_source_aliases (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    source_system  TEXT NOT NULL,
    source_table   TEXT NOT NULL,
    source_id      TEXT NOT NULL,
    source_project TEXT NOT NULL DEFAULT '',
    hive_table     TEXT NOT NULL,
    hive_pk        TEXT NOT NULL,
    hive_sync_id   TEXT NOT NULL,
    content_hash   TEXT NOT NULL DEFAULT '',
    run_id         TEXT NOT NULL REFERENCES import_runs(id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_import_source_aliases_source
ON import_source_aliases(source_system, source_table, source_id, source_project);

CREATE INDEX IF NOT EXISTS idx_import_source_aliases_hive
ON import_source_aliases(hive_table, hive_pk);

-- passive_observations: stores raw stdout captured by the subagent-stop hook.
-- sync_id is nullable and reserved for future Hive sync integration.
CREATE TABLE IF NOT EXISTS passive_observations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL DEFAULT '',
    project    TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL,
    sync_id    TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_passive_observations_session
ON passive_observations(session_id);

CREATE INDEX IF NOT EXISTS idx_passive_observations_project
ON passive_observations(project, created_at DESC);
`

// DB wraps an SQLite connection with schema validation.
type DB struct {
	sqlDB       *sql.DB
	migrationMu sync.Mutex

	// Observability only — see LastProjectMigrationSummary.
	migrationSummaryMu sync.Mutex
	migrationSummary   ProjectMigrationSummary
}

// sqliteBusyTimeout bounds how long a connection waits for a lock held by
// another process before reporting SQLITE_BUSY. hive-daemon is an MCP stdio
// server, so several client sessions run several processes over the same
// memory.db, and the startup identity migration holds one long exclusive write
// transaction. Without a wait the other processes fail instantly and gate their
// whole session off. 15s comfortably covers that rebuild while staying well
// under the startup budget an MCP client allows before it gives up on us.
const sqliteBusyTimeout = 15 * time.Second

// Open opens (or creates) a SQLite database at dsn, initializes the schema,
// and validates that all required triggers exist. Use ":memory:" for tests.
func Open(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", sqliteDSNWithBusyTimeout(dsn))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single connection to avoid WAL issues with in-memory DB
	sqlDB.SetMaxOpenConns(1)

	if err := initSchema(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if err := validateSchema(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("validate schema: %w", err)
	}
	return &DB{sqlDB: sqlDB}, nil
}

// sqliteDSNWithBusyTimeout applies the busy timeout through the DSN so every
// connection the pool opens carries it, including reconnects after an idle
// connection is dropped.
func sqliteDSNWithBusyTimeout(dsn string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%s_pragma=busy_timeout(%d)", dsn, separator, sqliteBusyTimeout.Milliseconds())
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.sqlDB.Close()
}

// RawDB exposes the underlying *sql.DB. Used by test packages outside db/ that
// need to seed rows directly (e.g. cmd/hive-daemon tests seeding stale sessions).
func (d *DB) RawDB() *sql.DB { return d.sqlDB }

func initSchema(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec(schema); err != nil {
		// R3-FIX-4 — UNIQUE-constraint failures during schema bootstrap on legacy
		// DBs indicate duplicate sync_id values exist that block the retrofit.
		// Surface an actionable error rather than letting the cryptic SQLite text
		// reach operators.
		if isUniqueSyncIDFailure(err) {
			return wrapUniqueSyncIDError(err, "sessions/memories")
		}
		return fmt.Errorf("exec schema: %w", err)
	}
	// Migraciones incrementales: añadimos columnas si no existen todavía.
	// SQLite no soporta ALTER TABLE ADD COLUMN IF NOT EXISTS — ignoramos el error
	// si la columna ya existe (error "duplicate column name").
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS project_identities (project_key TEXT PRIMARY KEY, first_spelling TEXT NOT NULL, first_seen_at DATETIME NOT NULL, first_source TEXT NOT NULL, remote_spelling TEXT NOT NULL DEFAULT '', remote_seen_at DATETIME, remote_source TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_project_identities_first_seen ON project_identities(first_seen_at, project_key)`,
		// SQLite no acepta DEFAULT CURRENT_TIMESTAMP en ALTER TABLE — solo defaults constantes.
		// Usamos epoch como placeholder; las rows existentes se actualizan abajo.
		`ALTER TABLE memories ADD COLUMN updated_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00'`,
		`ALTER TABLE memories ADD COLUMN synced_at DATETIME`,
		`ALTER TABLE memories ADD COLUMN deleted_at DATETIME`,
		`ALTER TABLE memories ADD COLUMN deleted_by TEXT`,
		`ALTER TABLE memories ADD COLUMN delete_reason TEXT`,
		`ALTER TABLE memories ADD COLUMN restored_at DATETIME`,
		// Backfill: copiar created_at a updated_at para las filas pre-migración.
		`UPDATE memories SET updated_at = created_at WHERE updated_at = '1970-01-01 00:00:00'`,
		// Fix FTS5 content-table triggers: UPDATE y DELETE FROM no funcionan en FTS5.
		// Se deben usar drop+recreate para bases de datos existentes con los triggers incorrectos.
		`DROP TRIGGER IF EXISTS memories_au`,
		`DROP TRIGGER IF EXISTS memories_ad`,
		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, title, content, tags) VALUES ('delete', old.id, old.title, old.content, old.tags); INSERT INTO memories_fts(rowid, title, content, tags) VALUES (new.id, new.title, new.content, new.tags); END`,
		`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, title, content, tags) VALUES ('delete', old.id, old.title, old.content, old.tags); END`,
		// user_prompts: add project and sync_id columns for context filtering
		`ALTER TABLE user_prompts ADD COLUMN project TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE user_prompts ADD COLUMN sync_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE user_prompts ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_user_prompts_project_created ON user_prompts(project, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_user_prompts_project_session_created ON user_prompts(project, session_id, created_at DESC, id DESC)`,
		`ALTER TABLE sync_state ADD COLUMN last_attempt_at DATETIME`,
		`ALTER TABLE sync_state ADD COLUMN last_success_at DATETIME`,
		`ALTER TABLE sync_state ADD COLUMN last_failure_at DATETIME`,
		`ALTER TABLE sync_state ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sync_state ADD COLUMN backoff_until DATETIME`,
		`ALTER TABLE sync_state ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`,
		// Additive index — safe to run on any DB including those migrated pre-Slice 2.
		`CREATE INDEX IF NOT EXISTS idx_sessions_dev_id ON sessions(dev_id)`,
		// R2-WARN-2 — UNIQUE index on sessions.sync_id for Postgres parity. UNIQUE
		// added to base schema; this migration retrofits existing DBs that were
		// created without the constraint. If duplicate sync_ids already exist, the
		// CREATE INDEX fails and we log+continue (caller may need to dedupe manually).
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_sync_id ON sessions(sync_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_project_active ON memories(project, created_at DESC) WHERE deleted_at IS NULL`,
		`CREATE TABLE IF NOT EXISTS memory_mutations (sequence INTEGER PRIMARY KEY AUTOINCREMENT, event_id TEXT NOT NULL UNIQUE, entity_type TEXT NOT NULL DEFAULT 'memory', entity_sync_id TEXT NOT NULL, project TEXT NOT NULL, op TEXT NOT NULL, occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, actor_id TEXT NOT NULL DEFAULT '', base_updated_at DATETIME, payload_json TEXT NOT NULL DEFAULT '{}', synced_at DATETIME)`,
		`ALTER TABLE memory_mutations ADD COLUMN request_id TEXT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_mutations_event_id ON memory_mutations(event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_mutations_project_unsynced ON memory_mutations(project, sequence) WHERE synced_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_memory_mutations_entity ON memory_mutations(entity_type, entity_sync_id, sequence)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_mutations_request_id ON memory_mutations(request_id) WHERE request_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS mutation_receipts (request_id TEXT PRIMARY KEY, operation TEXT NOT NULL, target_id INTEGER NOT NULL, project TEXT NOT NULL, entity_sync_id TEXT NOT NULL, event_id TEXT NOT NULL, actor_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', local_status TEXT NOT NULL, shared_status TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS mutation_cursors (consumer TEXT NOT NULL, project TEXT NOT NULL, sequence INTEGER NOT NULL DEFAULT 0, event_id TEXT NOT NULL DEFAULT '', updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (consumer, project))`,
		`CREATE TABLE IF NOT EXISTS memory_prompt_links (memory_id INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE, prompt_id INTEGER NOT NULL REFERENCES user_prompts(id) ON DELETE CASCADE, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (memory_id, prompt_id))`,
		`CREATE INDEX IF NOT EXISTS idx_memory_prompt_links_prompt_id ON memory_prompt_links(prompt_id)`,
		`CREATE TABLE IF NOT EXISTS sync_attempt_logs (attempt_id TEXT PRIMARY KEY, dev_id TEXT NOT NULL DEFAULT '', project TEXT NOT NULL, client TEXT NOT NULL DEFAULT '', daemon_id TEXT NOT NULL DEFAULT '', started_at DATETIME NOT NULL, ended_at DATETIME NOT NULL, outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')), http_status INTEGER NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '', request_id TEXT NOT NULL DEFAULT '', sync_counts_json TEXT NOT NULL DEFAULT '{}', metadata_json TEXT NOT NULL DEFAULT '{}', delivered_at DATETIME, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_pending ON sync_attempt_logs(delivered_at, started_at) WHERE delivered_at IS NULL AND dev_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_sync_attempt_logs_retention ON sync_attempt_logs(ended_at)`,
		`CREATE TABLE IF NOT EXISTS project_blocks (canonical_project_key TEXT PRIMARY KEY, project TEXT NOT NULL, command_id TEXT NOT NULL, ack_token TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', blocked_at DATETIME NOT NULL, ack_pending INTEGER NOT NULL DEFAULT 1, ack_status TEXT NOT NULL DEFAULT '', ack_warning TEXT NOT NULL DEFAULT '', ack_applied_at DATETIME, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`ALTER TABLE project_blocks ADD COLUMN ack_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE project_blocks ADD COLUMN action TEXT NOT NULL DEFAULT 'block'`,
		`ALTER TABLE project_blocks ADD COLUMN generation INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE project_blocks ADD COLUMN blocked INTEGER NOT NULL DEFAULT 1`,
		`CREATE INDEX IF NOT EXISTS idx_project_blocks_canonical ON project_blocks(canonical_project_key)`,
		`CREATE INDEX IF NOT EXISTS idx_project_blocks_pending_ack ON project_blocks(ack_pending, blocked_at)`,
		`CREATE TABLE IF NOT EXISTS project_quarantine_archives (canonical_project_key TEXT PRIMARY KEY, project TEXT NOT NULL, command_id TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS hive_project_governance (project TEXT PRIMARY KEY, archived_at DATETIME, archived_by TEXT NOT NULL DEFAULT '', archive_reason TEXT NOT NULL DEFAULT '', merge_target TEXT NOT NULL DEFAULT '', merged_at DATETIME, merged_by TEXT NOT NULL DEFAULT '', merge_reason TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS import_runs (id TEXT PRIMARY KEY, source_system TEXT NOT NULL, source_path TEXT NOT NULL DEFAULT '', source_fingerprint TEXT NOT NULL DEFAULT '', mode TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'completed', started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, completed_at DATETIME, report_json TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE IF NOT EXISTS import_source_aliases (id INTEGER PRIMARY KEY AUTOINCREMENT, source_system TEXT NOT NULL, source_table TEXT NOT NULL, source_id TEXT NOT NULL, source_project TEXT NOT NULL DEFAULT '', hive_table TEXT NOT NULL, hive_pk TEXT NOT NULL, hive_sync_id TEXT NOT NULL, content_hash TEXT NOT NULL DEFAULT '', run_id TEXT NOT NULL REFERENCES import_runs(id), created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_import_source_aliases_source ON import_source_aliases(source_system, source_table, source_id, source_project)`,
		`CREATE INDEX IF NOT EXISTS idx_import_source_aliases_hive ON import_source_aliases(hive_table, hive_pk)`,
		`ALTER TABLE hive_project_governance ADD COLUMN merge_target TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hive_project_governance ADD COLUMN merged_at DATETIME`,
		`ALTER TABLE hive_project_governance ADD COLUMN merged_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hive_project_governance ADD COLUMN merge_reason TEXT NOT NULL DEFAULT ''`,
		// project_aliases: durable source→target redirect table. Additive, idempotent.
		// source_project is the PK (one alias per source). scope supports future
		// global/cloud aliases without a schema change. No user_version bump needed.
		`CREATE TABLE IF NOT EXISTS project_aliases (
			source_project TEXT PRIMARY KEY,
			target_project TEXT NOT NULL,
			scope          TEXT NOT NULL DEFAULT 'local',
			reason         TEXT NOT NULL DEFAULT '',
			created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_by     TEXT NOT NULL DEFAULT '',
			synced_at      DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_project_aliases_target ON project_aliases(target_project)`,
		// passive_observations: additive table for hook-captured subagent output.
		// sync_id nullable for forward-compat with Hive sync (local-only for now).
		`CREATE TABLE IF NOT EXISTS passive_observations (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL DEFAULT '', project TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '', content TEXT NOT NULL, sync_id TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_passive_observations_session ON passive_observations(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_passive_observations_project ON passive_observations(project, created_at DESC)`,
		// pull_cursors: additive table for bounded legacy-pull pagination resume
		// positions (PR 2a/2b, hive-sync-batched-drain). See the base schema
		// declaration above for field semantics.
		`CREATE TABLE IF NOT EXISTS pull_cursors (consumer TEXT NOT NULL, project TEXT NOT NULL, channel TEXT NOT NULL, synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, sync_id TEXT NOT NULL DEFAULT '', updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (consumer, project, channel))`,
		// sync_state: nullable columns persisting the most recently recorded
		// Drain outcome per project (PR 3, task 3.4, hive-sync-batched-drain).
		// Nullable/no default beyond '' for the reason column — a project that
		// never called RecordDrainOutcome (only RecordSyncSuccess/Failure) must
		// read back empty/zero, not fail.
		`ALTER TABLE sync_state ADD COLUMN last_drain_state TEXT`,
		`ALTER TABLE sync_state ADD COLUMN last_drain_reason TEXT`,
		`ALTER TABLE sync_state ADD COLUMN last_drain_remaining INTEGER`,
		// BUG-DEVID-EMPTY — heal sessions poisoned with dev_id='' by the hook
		// path (handleSessionsCreate forwarded body.DevID with no fallback).
		// hive-api rejects dev_id='' via binding:"required", and one poisoned
		// Sessions[0] blocks the whole batched sync push. CreateSession now
		// guards new inserts; this migration fixes existing rows. TRIM matches
		// CreateSession's TrimSpace guard so whitespace-only values heal too.
		// Idempotent — safe to run on every daemon start.
		`UPDATE sessions SET dev_id = 'unknown' WHERE TRIM(dev_id) = ''`,
		// sync_from_project: the pending relocation precondition the project
		// identity migration stamps on rows the server already holds. Added
		// last on both tables so an upgraded DB ends up with the same column
		// order as a fresh one — rebuildContentProjectOwnershipTables copies
		// these tables by name, but keeping the two shapes identical keeps any
		// positional reader honest.
		`ALTER TABLE sessions ADD COLUMN sync_from_project TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE user_prompts ADD COLUMN sync_from_project TEXT NOT NULL DEFAULT ''`,
	}
	for _, m := range migrations {
		if _, err := sqlDB.Exec(m); err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "duplicate column name") || strings.Contains(errMsg, "already has column named") {
				continue // column already exists — safe to ignore
			}
			// R3-FIX-4 — UNIQUE retrofit failure on idx_sessions_sync_id is NOT
			// recoverable: continuing means the daemon runs without the dedup
			// guarantee R2-WARN-2 promised. Fail loudly. (R4-FIX-6 removed the
			// idx_memories_sync_id branch — that index is created inside
			// migrateMemoriesAddSessionID, never in this slice.)
			if isUniqueSyncIDFailure(err) && strings.Contains(m, "idx_sessions_sync_id") {
				return wrapUniqueSyncIDError(err, "sessions")
			}
			logger.Log.Printf("warn: migration failed: %v — sql: %.120s", err, m)
		}
	}

	if err := migrateMemoriesAddSessionID(sqlDB); err != nil {
		return fmt.Errorf("migrate memories session_id: %w", err)
	}

	// Suspect-A — idx_memories_session is required by FR-D-2. Create after the
	// session_id migration runs so the column always exists by this point. Safe
	// on fresh install (column declared in base schema) and on migrated DBs.
	if _, err := sqlDB.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id)`); err != nil {
		return fmt.Errorf("create idx_memories_session: %w", err)
	}

	if err := migrateMemoriesTopicKeyNonUnique(sqlDB); err != nil {
		return fmt.Errorf("migrate memories topic_key non-unique: %w", err)
	}

	return nil
}

// migrateMemoriesAddSessionID performs the recreate-table dance for memories,
// adding session_id NOT NULL. Gated by PRAGMA user_version — skips if already 2.
//
// Order of operations (must be atomic in a transaction):
//  1. Insert sentinel sessions per project (INSERT OR IGNORE).
//  2. Set session_id on existing memories (UPDATE via temp column or direct).
//  3. CREATE memories_new with session_id NOT NULL + FK.
//  4. Copy all rows from memories → memories_new.
//  5. Drop memories_fts, memories_ai/au/ad triggers, memories table.
//  6. Rename memories_new → memories.
//  7. Recreate memories_fts + all three triggers (must match schema const exactly).
//  8. SET PRAGMA user_version = 2.
func migrateMemoriesAddSessionID(sqlDB *sql.DB) error {
	var version int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= 2 {
		return nil // already migrated
	}

	// Check whether memories table already has session_id — detects fresh installs
	// where the CREATE TABLE IF NOT EXISTS in schema already includes session_id.
	var hasSessionID int
	_ = sqlDB.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('memories') WHERE name='session_id'`,
	).Scan(&hasSessionID)
	if hasSessionID > 0 {
		// Fresh install: memories was created with session_id already.
		// Bump version so we skip on next open.
		if _, err := sqlDB.Exec("PRAGMA user_version = 2"); err != nil {
			return fmt.Errorf("set user_version: %w", err)
		}
		return nil
	}

	// --- recreate-table dance inside a single transaction ---
	tx, err := sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Disable FK enforcement during the dance — we re-enable after rename.
	if _, err = tx.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign_keys: %w", err)
	}

	if err = insertSentinelSessions(tx); err != nil {
		return err
	}

	// Step 3: create the new table with session_id NOT NULL + FK.
	if _, err = tx.Exec(`CREATE TABLE memories_new (
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
	)`); err != nil {
		return fmt.Errorf("create memories_new: %w", err)
	}

	// Step 4: copy rows; session_id is already set by insertSentinelSessions backfill.
	// We use a temp column trick: the backfill UPDATE ran on the old table which
	// does NOT have session_id yet. We compute it inline in the SELECT.
	if _, err = tx.Exec(`
		INSERT INTO memories_new
		    (id, sync_id, project, topic_key, category, title, content, tags,
		     files_affected, created_by, created_at, updated_at, synced_at,
		     deleted_at, deleted_by, delete_reason, restored_at,
		     confidence, impact_score, session_id)
		SELECT id, sync_id, project, topic_key, category, title, content, tags,
		       files_affected, created_by, created_at, updated_at, synced_at,
		       NULL, NULL, NULL, NULL,
		       confidence, impact_score,
		       'legacy-pre-lifecycle-' || project
		FROM memories
	`); err != nil {
		return fmt.Errorf("copy memories to memories_new: %w", err)
	}

	// Step 5: drop FTS virtual table, triggers, and old table.
	// Trigger drop order: ai/au/ad, then fts, then table.
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS memories_ai",
		"DROP TRIGGER IF EXISTS memories_au",
		"DROP TRIGGER IF EXISTS memories_ad",
		"DROP TABLE IF EXISTS memories_fts",
		"DROP TABLE memories",
	} {
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("drop step (%s): %w", stmt, err)
		}
	}

	// Step 6: rename.
	if _, err = tx.Exec("ALTER TABLE memories_new RENAME TO memories"); err != nil {
		return fmt.Errorf("rename memories_new: %w", err)
	}

	// Recreate indexes that were on the old table. Also ensure idx_sessions_dev_id
	// and idx_memories_session exist (they may be missing on DBs migrated before
	// these indexes were added — Suspect-A: idx_memories_session is required by FR-D-2).
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_memories_topic_key
		 ON memories(project, topic_key) WHERE topic_key IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_project_active ON memories(project, created_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_dev_id ON sessions(dev_id)`,
		// R2-WARN-3 — recreated table needs the UNIQUE sync_id index too.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_sync_id ON memories(sync_id) WHERE sync_id != ''`,
	} {
		if _, err = tx.Exec(stmt); err != nil {
			// R4-FIX-5 — UNIQUE-on-sync_id failure here means a pre-Slice2 DB
			// has duplicate memories.sync_id values. Surface the actionable
			// cleanup hint instead of "recreate index: constraint failed: ...".
			if isUniqueSyncIDFailure(err) {
				return wrapUniqueSyncIDError(err, "memories")
			}
			return fmt.Errorf("recreate index: %w", err)
		}
	}

	// Step 7: recreate FTS5 virtual table + all three triggers.
	// These must match the schema const exactly to keep validateSchema happy.
	for _, stmt := range []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			title, content, tags,
			content='memories', content_rowid='id', tokenize='unicode61'
		)`,
		`INSERT INTO memories_fts(memories_fts) VALUES('rebuild')`,
		`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, title, content, tags)
			VALUES (new.id, new.title, new.content, new.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
			VALUES ('delete', old.id, old.title, old.content, old.tags);
			INSERT INTO memories_fts(rowid, title, content, tags)
			VALUES (new.id, new.title, new.content, new.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, title, content, tags)
			VALUES ('delete', old.id, old.title, old.content, old.tags);
		END`,
	} {
		if _, err = tx.Exec(stmt); err != nil {
			return fmt.Errorf("recreate fts/trigger: %w", err)
		}
	}

	// Re-enable FK enforcement.
	if _, err = tx.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("re-enable foreign_keys: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}

	// Step 8: bump schema version outside the transaction (PRAGMA ignores transactions).
	if _, err = sqlDB.Exec("PRAGMA user_version = 2"); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	return nil
}

// migrateMemoriesTopicKeyNonUnique drops the legacy UNIQUE index on
// (project, topic_key) and replaces it with a non-unique index. topic_key is
// now a grouping/context key, not an identity key (Issue #119). Gated by
// PRAGMA user_version — skips if already >= 3. Idempotent.
func migrateMemoriesTopicKeyNonUnique(sqlDB *sql.DB) error {
	var version int
	if err := sqlDB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= 3 {
		return nil // already migrated
	}
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_unique_topic_key`,
		`CREATE INDEX IF NOT EXISTS idx_memories_topic_key
		     ON memories(project, topic_key) WHERE topic_key IS NOT NULL`,
	} {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return fmt.Errorf("topic_key non-unique migration (%.60s): %w", stmt, err)
		}
	}
	if _, err := sqlDB.Exec("PRAGMA user_version = 3"); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	return nil
}

// insertSentinelSessions inserts one legacy-pre-lifecycle-{project} session per
// distinct project found in memories, then backfills memories.session_id.
// Uses INSERT OR IGNORE so re-running is safe.
func insertSentinelSessions(tx *sql.Tx) error {
	rows, err := tx.Query(
		`SELECT project, MIN(created_at) FROM memories GROUP BY project`,
	)
	if err != nil {
		return fmt.Errorf("query projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type sentinelRow struct {
		project   string
		minCreate string
	}
	var sentinels []sentinelRow
	for rows.Next() {
		var s sentinelRow
		if err := rows.Scan(&s.project, &s.minCreate); err != nil {
			return fmt.Errorf("scan project row: %w", err)
		}
		sentinels = append(sentinels, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate projects: %w", err)
	}

	for _, s := range sentinels {
		id := "legacy-pre-lifecycle-" + s.project
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO sessions
			    (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
			VALUES (?, lower(hex(randomblob(16))), ?, '', 'legacy', 'legacy', ?,
			        CURRENT_TIMESTAMP,
			        'Backfilled placeholder for memories created before session lifecycle.')
		`, id, s.project, s.minCreate)
		if err != nil {
			return fmt.Errorf("insert sentinel for %q: %w", s.project, err)
		}
	}

	return nil
}

// isUniqueSyncIDFailure reports whether err is a SQLite UNIQUE-constraint
// violation against a sync_id column. R3-FIX-4 — these happen when retrofitting
// a UNIQUE index onto a pre-R2 sessions/memories table that already contains
// duplicate sync_ids. We surface them as fatal rather than swallowing the
// retrofit and silently regressing the dedup guarantee.
func isUniqueSyncIDFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unique") {
		return false
	}
	return strings.Contains(msg, "sync_id")
}

// wrapUniqueSyncIDError returns an actionable fatal error explaining the manual
// cleanup operators must perform before reopening the daemon DB. The `table`
// argument selects the SELECT hint shown to the operator (sessions vs memories);
// callers that don't know which table failed pass "sessions/memories".
func wrapUniqueSyncIDError(err error, table string) error {
	if table == "" {
		table = "sessions/memories"
	}
	hint := table
	if table == "sessions/memories" {
		hint = "sessions"
	}
	return fmt.Errorf(
		"duplicate sync_id values exist in %s table; manual cleanup required before upgrading. "+
			"Run: SELECT sync_id, COUNT(*) FROM %s GROUP BY sync_id HAVING COUNT(*) > 1; "+
			"underlying error: %w", table, hint, err)
}

// validateSchema verifies that all FTS5 triggers exist in sqlite_master.
// Returns an error if any trigger is missing (indicates schema corruption).
func validateSchema(sqlDB *sql.DB) error {
	triggers := []string{
		"memories_ai", "memories_au", "memories_ad",
		"user_prompts_ai", "user_prompts_au", "user_prompts_ad",
	}
	for _, trigger := range triggers {
		var name string
		err := sqlDB.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger,
		).Scan(&name)
		if err != nil {
			return fmt.Errorf("trigger %q missing or corrupted: %w", trigger, err)
		}
	}
	return nil
}
