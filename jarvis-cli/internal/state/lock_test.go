package state

import (
	"os"
	"testing"
)

// The lock is what makes a read-modify-write of the manifest safe: the re-read
// and the write sit inside one critical section no second writer can enter, and
// that section has to end so the next writer can.
func TestWithLock_HoldsTheManifestExclusivelyAndReleasesItAfterwards(t *testing.T) {
	isolateHome(t)
	path, err := LockPath()
	if err != nil {
		t.Fatalf("LockPath: %v", err)
	}

	var second error
	if err := WithLock(func() error {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("lock file is absent while the lock is held: %v", statErr)
		}
		second = WithLock(func() error {
			t.Error("a second holder entered the critical section")
			return nil
		})
		return nil
	}); err != nil {
		t.Fatalf("WithLock: %v", err)
	}

	if second == nil {
		t.Fatal("a held lock must refuse the second holder instead of letting it write")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file survived the critical section: %v", err)
	}
}
