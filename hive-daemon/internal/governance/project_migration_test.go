package governance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

func TestExecuteProjectMigrationWithBackupCreatesRetainedTemporaryBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session', ' Foo.Bar ', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RawDB().Exec(`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES ('memory', ' Foo.Bar ', 'title', 'content', 's')`); err != nil {
		t.Fatal(err)
	}
	plan, err := db.ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	backups := NewSQLiteBackupStore(path, filepath.Join(t.TempDir(), "backups"), database.RawDB())
	if err := ExecuteProjectMigrationWithBackup(context.Background(), database, plan, backups); err != nil {
		t.Fatalf("ExecuteProjectMigrationWithBackup() error = %v", err)
	}
	created, err := backups.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || !created[0].Temporary || created[0].SourceOperation != ProjectMigrationBackupOperation || !created[0].RetainUntil.After(created[0].CreatedAt) {
		t.Fatalf("migration backup = %+v, want retained temporary migration metadata", created)
	}
}

func TestTemporaryMigrationBackupPrunesOnlyAfterRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := os.WriteFile(path, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewBackupStore(path, filepath.Join(t.TempDir(), "backups"))
	createdAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return createdAt }
	backup, err := store.CreateTemporaryMigrationBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PruneExpiredTemporaryMigrationBackups(context.Background(), backup.RetainUntil.Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if retained, err := store.List(context.Background()); err != nil || len(retained) != 1 {
		t.Fatalf("backups before retention = %v, %v; want one retained backup", retained, err)
	}
	if err := store.PruneExpiredTemporaryMigrationBackups(context.Background(), backup.RetainUntil); err != nil {
		t.Fatal(err)
	}
	if retained, err := store.List(context.Background()); err != nil || len(retained) != 0 {
		t.Fatalf("backups at retention = %v, %v; want no retained backup", retained, err)
	}
}

func TestProjectMigrationBackupSurvivesFailedExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s', 'session', ' Foo.Bar ', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RawDB().Exec(`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES ('memory', ' Foo.Bar ', 'title', 'content', 's')`); err != nil {
		t.Fatal(err)
	}
	plan, err := db.ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	backups := NewSQLiteBackupStore(path, filepath.Join(t.TempDir(), "backups"), database.RawDB())
	fail := errors.New("forced migration failure")
	if err := executeProjectMigrationWithBackup(context.Background(), database, plan, backups, func() error { return fail }); !errors.Is(err, fail) {
		t.Fatalf("executeProjectMigrationWithBackup() error = %v, want failpoint", err)
	}
	if retained, err := backups.List(context.Background()); err != nil || len(retained) != 1 {
		t.Fatalf("backups after failed migration = %v, %v; want retained backup", retained, err)
	}
}
