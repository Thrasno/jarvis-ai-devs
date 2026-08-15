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

// Update is itself a read-modify-write, so it has to run inside the same
// critical section the other writers take. Without it, a `jarvis sync`
// bookkeeping write and a concurrent `jarvis persona set` are not mutually
// exclusive and the later save silently discards the earlier one.
func TestUpdate_RunsInsideTheManifestLock(t *testing.T) {
	isolateHome(t)
	seed := New()
	seed.Persona = "recorded"
	if err := Save(seed); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	var updateErr error
	if err := WithLock(func() error {
		updateErr = Update(func(st *State) {
			st.Persona = "written-by-a-second-writer"
		})
		return nil
	}); err != nil {
		t.Fatalf("WithLock: %v", err)
	}

	if updateErr == nil {
		t.Fatal("Update wrote the manifest while another writer held the lock")
	}

	after, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if after.Persona != "recorded" {
		t.Fatalf("persona = %q; the locked-out writer must change nothing", after.Persona)
	}
}

// The lock is released once Update returns, so an ordinary sequential caller is
// never blocked by its own earlier write.
func TestUpdate_ReleasesTheLockForTheNextWriter(t *testing.T) {
	isolateHome(t)

	if err := Update(func(st *State) { st.Persona = "first" }); err != nil {
		t.Fatalf("first Update: %v", err)
	}
	if err := Update(func(st *State) { st.Skills = []string{"go-testing"} }); err != nil {
		t.Fatalf("second Update: %v", err)
	}

	after, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if after.Persona != "first" || len(after.Skills) != 1 {
		t.Fatalf("manifest = %+v; both sequential updates must be recorded", after)
	}
}
