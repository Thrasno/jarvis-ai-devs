package db

import (
	"testing"
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
