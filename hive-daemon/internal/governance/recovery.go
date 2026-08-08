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
	return writePendingRestore(PendingRestorePath(dbPath), pendingRestore{RestoreRequest: req})
}

type pendingRestore struct {
	RestoreRequest
}

func writePendingRestore(pendingPath string, pending pendingRestore) error {
	data, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("encode pending restore: %w", err)
	}
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
	return executeScheduledRestore(ctx, dbPath, os.Remove)
}

func executeScheduledRestore(ctx context.Context, dbPath string, remove func(string) error) (bool, error) {
	pendingPath := PendingRestorePath(dbPath)
	completedPath := pendingPath + ".completed"
	if _, err := os.Stat(completedPath); err == nil {
		if err := remove(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, fmt.Errorf("clear completed pending restore: %w", err)
		}
		if err := remove(completedPath); err != nil {
			return true, fmt.Errorf("clear restore completion marker: %w", err)
		}
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect restore completion marker: %w", err)
	}
	data, err := os.ReadFile(pendingPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pending restore: %w", err)
	}
	var pending pendingRestore
	if err := json.Unmarshal(data, &pending); err != nil {
		return false, fmt.Errorf("decode pending restore: %w", err)
	}
	if _, err := NewBackupStore(dbPath, "").Restore(ctx, pending.RestoreRequest); err != nil {
		return false, err
	}
	if err := writePendingRestore(completedPath, pending); err != nil {
		return true, fmt.Errorf("record completed pending restore: %w", err)
	}
	if err := remove(pendingPath); err != nil {
		return true, fmt.Errorf("clear completed pending restore: %w", err)
	}
	if err := remove(completedPath); err != nil {
		return true, fmt.Errorf("clear restore completion marker: %w", err)
	}
	return true, nil
}
