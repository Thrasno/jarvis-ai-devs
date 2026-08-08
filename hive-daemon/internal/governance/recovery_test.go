package governance

import (
	"context"
	"errors"
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

func TestScheduledRestoreReportsReplayableRequestWhenCompletionCannotBeRecorded(t *testing.T) {
	dbPath := scheduledRestoreFixture(t)

	markerFailure := errors.New("no space left on device")
	restored, err := executeScheduledRestore(context.Background(), dbPath, restoreIO{
		write: func(string, pendingRestore) error { return markerFailure },
	})
	if !restored {
		t.Fatal("restore did not replace the live database")
	}
	if !errors.Is(err, ErrPendingRestoreReplayable) || !errors.Is(err, markerFailure) {
		t.Fatalf("error = %v, want a replayable pending restore wrapping %v", err, markerFailure)
	}
	if _, statErr := os.Stat(PendingRestorePath(dbPath)); statErr != nil {
		t.Fatalf("pending request was cleared despite the failure: %v", statErr)
	}
}

func TestScheduledRestoreCleanupFailureNeverRepeatsCompletedRestore(t *testing.T) {
	dbPath := scheduledRestoreFixture(t)

	removeFailure := errors.New("marker removal failed")
	if restored, err := executeScheduledRestore(context.Background(), dbPath, restoreIO{remove: func(string) error { return removeFailure }}); !restored || !errors.Is(err, removeFailure) {
		t.Fatalf("first restore = %v, %v; want completed restore with cleanup failure", restored, err)
	}
	if err := os.WriteFile(dbPath, []byte("valid-post-recovery-write"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The next start only clears markers: it restores nothing, so it must not
	// report a restore either.
	if restored, err := ExecuteScheduledRestore(context.Background(), dbPath); err != nil || restored {
		t.Fatalf("restart cleanup = %v, %v; want marker cleanup without a reported restore", restored, err)
	}
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "valid-post-recovery-write" {
		t.Fatalf("restart database = %q, want post-recovery write preserved", got)
	}
	if restored, err := ExecuteScheduledRestore(context.Background(), dbPath); err != nil || restored {
		t.Fatalf("second restart = %v, %v; want no pending recovery", restored, err)
	}
}

// scheduledRestoreFixture stages a live database whose current bytes differ from
// a validated backup, with that backup already scheduled for restore.
func scheduledRestoreFixture(t *testing.T) string {
	t.Helper()
	dbDir := filepath.Join(t.TempDir(), "live")
	if err := os.Mkdir(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "memory.db")
	if err := os.WriteFile(dbPath, []byte("before-migration"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err := NewBackupStore(dbPath, "").Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("blocked-state"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ScheduleRestore(dbPath, RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)}); err != nil {
		t.Fatal(err)
	}
	return dbPath
}
