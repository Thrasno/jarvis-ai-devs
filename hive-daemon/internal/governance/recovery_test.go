package governance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScheduledRestoreRestoresBytesOnlyBeforeDaemonReopensDatabase(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "live")
	if err := os.Mkdir(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("before-migration"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := NewBackupStore(dbPath, "")
	backup, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("blocked-migration-state"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ScheduleRestore(dbPath, RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)}); err != nil {
		t.Fatalf("ScheduleRestore: %v", err)
	}
	restored, err := ExecuteScheduledRestore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("ExecuteScheduledRestore: %v", err)
	}
	if !restored {
		t.Fatal("restore was not executed")
	}
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before-migration" {
		t.Fatalf("restored bytes = %q, want original backup", got)
	}
	if _, err := os.Stat(PendingRestorePath(dbPath)); !os.IsNotExist(err) {
		t.Fatalf("pending restore was not cleared after success: %v", err)
	}
}

func TestScheduledRestoreFailureLeavesLiveBytesAndPendingRecovery(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "live")
	if err := os.Mkdir(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := NewBackupStore(dbPath, "")
	backup, err := backups.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("live-state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ScheduleRestore(dbPath, RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(backup.ArchivePath); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteScheduledRestore(context.Background(), dbPath); err == nil {
		t.Fatal("expected restore failure")
	}
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "live-state" {
		t.Fatalf("live bytes changed after failed restore: %q", got)
	}
	if _, err := os.Stat(PendingRestorePath(dbPath)); err != nil {
		t.Fatalf("pending recovery was removed after failure: %v", err)
	}
}
