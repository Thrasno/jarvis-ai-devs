// This file guards ~/.jarvis/state.yaml against concurrent writers. Updating
// one field still means reading the manifest, changing it and writing it back,
// and that is only safe inside a critical section: without one, two processes
// finishing at once each write a manifest the other has already replaced.
package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockPath returns the lock file guarding the desired-state manifest.
func LockPath() (string, error) {
	path, err := Path()
	return path + ".lock", err
}

// WithLock runs fn while holding the manifest lock, and always releases it. A
// busy lock fails immediately: jarvis writes the manifest from short-lived
// non-interactive commands, so a lock held right now means another jarvis
// process is mid-write, and saying so beats blocking silently.
func WithLock(fn func() error) error {
	path, err := LockPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	// Exclusive creation is atomic, so two processes cannot both hold the lock.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return fmt.Errorf("%s is locked by another jarvis process; remove %s if none is running", stateFileName, path)
	}
	if err != nil {
		return fmt.Errorf("lock %s: %w", stateFileName, err)
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(path)
	}()
	return fn()
}
