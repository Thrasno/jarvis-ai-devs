package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

func TestPendingRestoreIsAppliedBeforeMigrationIsReplanned(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "live", "memory.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	backups := governance.NewSQLiteBackupStore(dbPath, "", store.RawDB())
	backup, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	blocked := runStartupMigrationWith(context.Background(), store, func(context.Context, db.ProjectMigrationPlan) error {
		return errors.New("identity conflict")
	})
	if blocked.Status().State != project.MigrationStateBlocked {
		t.Fatalf("initial migration state = %q, want blocked", blocked.Status().State)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := governance.ScheduleRestore(dbPath, governance.RestoreRequest{BackupID: backup.ID, Confirmation: governance.RestoreConfirmation(backup.ID)}); err != nil {
		t.Fatal(err)
	}
	if restored, err := governance.ExecuteScheduledRestore(context.Background(), dbPath); err != nil || !restored {
		t.Fatalf("ExecuteScheduledRestore = %v, %v; want true, nil", restored, err)
	}

	reopened, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replanned := runStartupMigrationWith(context.Background(), reopened, func(context.Context, db.ProjectMigrationPlan) error {
		return nil
	})
	if replanned.Status().State != project.MigrationStateReady {
		t.Fatalf("replanned migration state = %q, want ready", replanned.Status().State)
	}
}
