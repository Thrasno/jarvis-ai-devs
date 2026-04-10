package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

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
    confidence      TEXT NOT NULL DEFAULT '',
    impact_score    INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_topic_key
ON memories(project, topic_key)
WHERE topic_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project);
CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at DESC);

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
    UPDATE memories_fts SET title=new.title, content=new.content, tags=new.tags
    WHERE rowid=new.id;
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    DELETE FROM memories_fts WHERE rowid=old.id;
END;
`

// DB wraps an SQLite connection with schema validation.
type DB struct {
	sqlDB *sql.DB
}

// Open opens (or creates) a SQLite database at dsn, initializes the schema,
// and validates that all required triggers exist. Use ":memory:" for tests.
func Open(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
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

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.sqlDB.Close()
}

func initSchema(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec(schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	return nil
}

// validateSchema verifies that all FTS5 triggers exist in sqlite_master.
// Returns an error if any trigger is missing (indicates schema corruption).
func validateSchema(sqlDB *sql.DB) error {
	triggers := []string{"memories_ai", "memories_au", "memories_ad"}
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
