package httpapi

import (
	"database/sql"
	"errors"
	"testing"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// triggerPrimaryKeyViolation returns a *sqlite.Error with code
// SQLITE_CONSTRAINT_PRIMARYKEY (1555) by inserting a duplicate into a
// PRIMARY KEY column on an in-memory SQLite database.
// modernc.org/sqlite does not export a constructor for *sqlite.Error — the only
// way to obtain one with the correct code is via a real database operation.
func triggerPrimaryKeyViolation(t *testing.T) error {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer sqlDB.Close()

	if _, err := sqlDB.Exec(`CREATE TABLE t (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO t VALUES ('x')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err = sqlDB.Exec(`INSERT INTO t VALUES ('x')`)
	if err == nil {
		t.Fatal("expected PRIMARY KEY constraint error, got nil")
	}
	return err
}

// triggerUniqueConstraintViolation returns a *sqlite.Error with code
// SQLITE_CONSTRAINT_UNIQUE (2067) by inserting a duplicate into a UNIQUE
// (non-PK) column on an in-memory SQLite database.
func triggerUniqueConstraintViolation(t *testing.T) error {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer sqlDB.Close()

	if _, err := sqlDB.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO t (name) VALUES ('alice')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err = sqlDB.Exec(`INSERT INTO t (name) VALUES ('alice')`)
	if err == nil {
		t.Fatal("expected UNIQUE constraint error, got nil")
	}
	return err
}

func TestIsDuplicateKeyError_PrimaryKeyViolation_ReturnsTrue(t *testing.T) {
	err := triggerPrimaryKeyViolation(t)

	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("expected *sqlite.Error, got %T: %v", err, err)
	}
	if sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
		t.Fatalf("expected SQLITE_CONSTRAINT_PRIMARYKEY (%d), got %d", sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY, sqliteErr.Code())
	}

	if !isDuplicateKeyError(err) {
		t.Errorf("isDuplicateKeyError should return true for SQLITE_CONSTRAINT_PRIMARYKEY, got false")
	}
}

func TestIsDuplicateKeyError_UniqueConstraintViolation_ReturnsTrue(t *testing.T) {
	err := triggerUniqueConstraintViolation(t)

	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("expected *sqlite.Error, got %T: %v", err, err)
	}
	if sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		t.Fatalf("expected SQLITE_CONSTRAINT_UNIQUE (%d), got %d", sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqliteErr.Code())
	}

	if !isDuplicateKeyError(err) {
		t.Errorf("isDuplicateKeyError should return true for SQLITE_CONSTRAINT_UNIQUE, got false")
	}
}

func TestIsDuplicateKeyError_PlainUniqueStringError_ReturnsFalse(t *testing.T) {
	// An error whose message contains "unique" but is not a *sqlite.Error.
	// This is the key proof that the old heuristic is gone.
	err := errors.New("UNIQUE constraint failed: sessions.id")

	if isDuplicateKeyError(err) {
		t.Errorf("isDuplicateKeyError should return false for plain string error containing 'unique', got true")
	}
}

func TestIsDuplicateKeyError_PlainDuplicateStringError_ReturnsFalse(t *testing.T) {
	// An error whose message contains "duplicate" but is not a *sqlite.Error.
	err := errors.New("duplicate key value violates unique constraint")

	if isDuplicateKeyError(err) {
		t.Errorf("isDuplicateKeyError should return false for plain string error containing 'duplicate', got true")
	}
}

func TestIsDuplicateKeyError_NilError_ReturnsFalse(t *testing.T) {
	if isDuplicateKeyError(nil) {
		t.Error("isDuplicateKeyError should return false for nil error")
	}
}
