package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PendingRestorePath is deliberately adjacent to the database so the daemon
// lifecycle owner can restore it before opening SQLite on the next process.
func PendingRestorePath(dbPath string) string {
	return dbPath + ".restore-pending"
}

// ScheduleRestore validates and durably records a restore request. It never
// replaces a live database; ExecuteScheduledRestore owns that operation before
// SQLite is opened by a fresh daemon process.
func ScheduleRestore(dbPath string, req RestoreRequest) error {
	store := NewBackupStore(dbPath, "")
	if _, err := store.PlanRestore(context.Background(), req); err != nil {
		return err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode pending restore: %w", err)
	}
	pendingPath := PendingRestorePath(dbPath)
	temp, err := os.CreateTemp(filepath.Dir(pendingPath), ".restore-pending-*")
	if err != nil {
		return fmt.Errorf("create pending restore: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("secure pending restore: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write pending restore: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync pending restore: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close pending restore: %w", err)
	}
	if err := os.Rename(tempPath, pendingPath); err != nil {
		return fmt.Errorf("activate pending restore: %w", err)
	}
	return nil
}

// ExecuteScheduledRestore restores a previously validated backup before the
// daemon opens SQLite. A failure retains the request and leaves the live DB
// untouched unless BackupStore.Restore has completed its atomic replacement.
func ExecuteScheduledRestore(ctx context.Context, dbPath string) (bool, error) {
	pendingPath := PendingRestorePath(dbPath)
	data, err := os.ReadFile(pendingPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pending restore: %w", err)
	}
	var req RestoreRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return false, fmt.Errorf("decode pending restore: %w", err)
	}
	if _, err := NewBackupStore(dbPath, "").Restore(ctx, req); err != nil {
		return false, err
	}
	if err := os.Remove(pendingPath); err != nil {
		return false, fmt.Errorf("clear completed pending restore: %w", err)
	}
	return true, nil
}
