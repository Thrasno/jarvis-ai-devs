package governance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
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
	backup, err := store.CreateTemporaryMigrationBackup(context.Background(), "plan-fingerprint")
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

func TestProjectMigrationReusesUnexpiredBackupForSamePlan(t *testing.T) {
	database, path := migrationFixture(t)
	backups := NewSQLiteBackupStore(path, filepath.Join(t.TempDir(), "backups"), database.RawDB())
	fail := errors.New("forced migration failure")

	for attempt := 0; attempt < 3; attempt++ {
		plan, err := db.ReadProjectMigrationPlan(context.Background(), database)
		if err != nil {
			t.Fatal(err)
		}
		if err := executeProjectMigrationWithBackup(context.Background(), database, plan, backups, func() error { return fail }); !errors.Is(err, fail) {
			t.Fatalf("attempt %d error = %v, want failpoint", attempt, err)
		}
	}

	retained, err := backups.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 1 {
		t.Fatalf("backups after repeated blocked migrations = %d, want a single reused backup", len(retained))
	}
}

func TestProjectMigrationCreatesFreshBackupForDifferentPlan(t *testing.T) {
	database, path := migrationFixture(t)
	backups := NewSQLiteBackupStore(path, filepath.Join(t.TempDir(), "backups"), database.RawDB())
	fail := errors.New("forced migration failure")

	plan, err := db.ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if err := executeProjectMigrationWithBackup(context.Background(), database, plan, backups, func() error { return fail }); !errors.Is(err, fail) {
		t.Fatalf("first attempt error = %v, want failpoint", err)
	}

	if _, err := database.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES ('s2', 'session-2', ' Other.Project ', 'dev', 'test')`); err != nil {
		t.Fatal(err)
	}
	changed, err := db.ReadProjectMigrationPlan(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Fingerprint == plan.Fingerprint {
		t.Fatal("plan fingerprint did not change; fixture cannot prove a fresh backup")
	}
	if err := executeProjectMigrationWithBackup(context.Background(), database, changed, backups, func() error { return fail }); !errors.Is(err, fail) {
		t.Fatalf("second attempt error = %v, want failpoint", err)
	}

	retained, err := backups.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(retained) != 2 {
		t.Fatalf("backups for two distinct plans = %d, want one backup per plan", len(retained))
	}
}

// TestUnusableMigrationBackupIsReportedBeforeRecopyingTheDatabase covers a
// corrupt archive: it can never be reused, so every daemon start pays for a
// fresh full copy of the database. That is the disk cost reuse exists to avoid,
// and skipping it silently leaves nobody able to see why.
func TestUnusableMigrationBackupIsReportedBeforeRecopyingTheDatabase(t *testing.T) {
	database, path := migrationFixture(t)
	backups := NewSQLiteBackupStore(path, filepath.Join(t.TempDir(), "backups"), database.RawDB())
	corrupt, err := backups.CreateTemporaryMigrationBackup(context.Background(), "plan-fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corrupt.ArchivePath, []byte("truncated"), 0o600); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	restore := captureDaemonLog(t, &log)
	fresh, err := backups.EnsureTemporaryMigrationBackup(context.Background(), "plan-fingerprint")
	restore()

	if err != nil {
		t.Fatalf("EnsureTemporaryMigrationBackup = %v, want a fresh copy instead of a corrupt rollback", err)
	}
	if fresh.ID == corrupt.ID {
		t.Fatal("corrupt archive was reused as a rollback point")
	}
	if !strings.Contains(log.String(), corrupt.ID) {
		t.Fatalf("log = %q, want the rejected backup %q named", log.String(), corrupt.ID)
	}
	if !strings.Contains(log.String(), "checksum") {
		t.Fatalf("log = %q, want the rejection reason reported", log.String())
	}
}

// captureDaemonLog redirects the daemon's stderr logger for one assertion and
// returns the restore step.
func captureDaemonLog(t *testing.T, into *bytes.Buffer) func() {
	t.Helper()
	previous := logger.Log.Writer()
	logger.Log.SetOutput(into)
	return func() { logger.Log.SetOutput(previous) }
}

// migrationFixture returns an open database whose project spellings require the
// canonical identity migration, plus its on-disk path.
func migrationFixture(t *testing.T) (*db.DB, string) {
	t.Helper()
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
	return database, path
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
