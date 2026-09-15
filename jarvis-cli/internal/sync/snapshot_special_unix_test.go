//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sync

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTakeSnapshotRejectsFIFOWithoutReadingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := TakeSnapshot([]TrackedPath{{Path: path, Mode: ManagedFileMode}})
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("TakeSnapshot error = %v, want a special-file rejection", err)
		}
	case <-time.After(100 * time.Millisecond):
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat fifo: %v", err)
		}
		t.Fatalf("TakeSnapshot blocked reading FIFO with mode %v", info.Mode())
	}
}
