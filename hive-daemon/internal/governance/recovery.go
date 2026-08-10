package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// ErrPendingRestoreReplayable reports that a restore replaced the live database
// but its request could not be durably cleared, so the next daemon start would
// replay it. Serving in that state would silently discard every write made in
// between, so the daemon must stop instead of surviving this error.
var ErrPendingRestoreReplayable = errors.New("pending restore request may be replayed on the next start")

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
	// The rename itself must reach the disk: without it a power loss can lose a
	// request the operator was already told had been accepted.
	return syncDir(filepath.Dir(pendingPath))
}

func syncDir(path string) error {
	return syncDirWith(path, (*os.File).Sync)
}

func syncDirWith(path string, syncFile func(*os.File) error) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open pending restore dir: %w", err)
	}
	if err := syncFile(dir); err != nil && !directoryFsyncUnsupported(err) {
		_ = dir.Close()
		return fmt.Errorf("sync pending restore dir: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close pending restore dir: %w", err)
	}
	return nil
}

// directoryFsyncUnsupported reports the failures that mean "this platform or
// filesystem does not flush directory handles", as opposed to a real I/O
// failure. Flushing the directory only hardens the rename that already
// committed the request, so on those platforms the operation succeeded and must
// be reported as such: a false failure makes the operator believe no restore is
// pending while the next daemon start silently applies it and discards the
// session's writes.
//
// Windows returns ERROR_ACCESS_DENIED from FlushFileBuffers on a directory
// handle, which fs.ErrPermission matches (it also matches EPERM). Container and
// network filesystems answer ENOTSUP, EOPNOTSUPP or ENOSYS, which
// errors.ErrUnsupported matches, or EINVAL, which nothing wraps: errors.Is on a
// bare syscall.EINVAL is the only way to reach it, since fs.ErrInvalid is a
// distinct sentinel os.File raises for a nil or closed handle and never for an
// errno. Do not read syscall.EINVAL as the redundant spelling of an idiomatic
// wrapper and delete it; removing it silently reintroduces the replayed
// restore, and TestPendingRestoreAcceptedWhereDirectoryFsyncIsUnsupported is
// what catches that. Treat this list as the set that has come up, not as
// exhaustive. A directory the process genuinely may not read already failed at
// os.Open above, so tolerating a permission error here cannot hide that case.
//
// EIO and ENOSPC are deliberately not tolerated: they signal device trouble
// that threatens the request file itself, not just the rename's durability.
func directoryFsyncUnsupported(err error) bool {
	return errors.Is(err, errors.ErrUnsupported) ||
		errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, syscall.EINVAL)
}

// ExecuteScheduledRestore restores a previously validated backup before the
// daemon opens SQLite. A failure retains the request and leaves the live DB
// untouched unless BackupStore.Restore has completed its atomic replacement.
func ExecuteScheduledRestore(ctx context.Context, dbPath string) (bool, error) {
	return executeScheduledRestore(ctx, dbPath, restoreIO{})
}

// restoreIO isolates the filesystem effects that run after the live database has
// already been replaced, so tests can exercise their failure paths.
type restoreIO struct {
	remove func(string) error
	write  func(string, pendingRestore) error
}

func (r restoreIO) removeFile(path string) error {
	if r.remove == nil {
		return os.Remove(path)
	}
	return r.remove(path)
}

func (r restoreIO) writePending(path string, pending pendingRestore) error {
	if r.write == nil {
		return writePendingRestore(path, pending)
	}
	return r.write(path, pending)
}

func executeScheduledRestore(ctx context.Context, dbPath string, r restoreIO) (bool, error) {
	pendingPath := PendingRestorePath(dbPath)
	completedPath := pendingPath + ".completed"
	if _, err := os.Stat(completedPath); err == nil {
		// A completion marker means an earlier start already restored these
		// bytes; this start only clears markers and restores nothing.
		if err := r.removeFile(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("clear completed pending restore: %w", err)
		}
		if err := r.removeFile(completedPath); err != nil {
			return false, fmt.Errorf("clear restore completion marker: %w", err)
		}
		return false, nil
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
	if err := r.writePending(completedPath, pending); err != nil {
		// The request is still on disk with nothing recording that it already
		// ran, so the next start would restore these bytes again.
		return true, fmt.Errorf("%w: record completed pending restore: %w", ErrPendingRestoreReplayable, err)
	}
	if err := r.removeFile(pendingPath); err != nil {
		return true, fmt.Errorf("clear completed pending restore: %w", err)
	}
	if err := r.removeFile(completedPath); err != nil {
		return true, fmt.Errorf("clear restore completion marker: %w", err)
	}
	return true, nil
}
