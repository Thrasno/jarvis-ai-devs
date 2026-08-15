package state

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The in-process half of the guarantee: while the lock is held the lock file is
// on disk and a second WithLock in this process is refused, and the file is gone
// once the critical section ends. Cross-process exclusion, which is what the
// lock actually exists for, is proven separately by
// TestWithLock_ExcludesASecondProcess; a refusal inside one process would also
// hold for a lock built from an in-memory mutex, and that lock would protect
// nothing.
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

// lockHelperEnv promotes the helper test below into the second process of the
// cross-process test. Every other run of the suite skips it.
const lockHelperEnv = "JARVIS_STATE_LOCK_HELPER"

// The two outcomes the second process can report, printed rather than encoded in
// an exit status so the `go test` harness cannot be mistaken for the answer.
const (
	lockHelperAcquired = "JARVIS-LOCK-ACQUIRED"
	lockHelperRefused  = "JARVIS-LOCK-REFUSED"
)

// The lock exists to exclude a second *process*: two short-lived jarvis commands
// finishing at once would each write a manifest the other has already replaced.
// Nothing in this process can demonstrate that, so this re-executes the test
// binary and has the child take the lock for real — once while this process
// holds it, once after it has let go.
func TestWithLock_ExcludesASecondProcess(t *testing.T) {
	isolateHome(t)

	var whileHeld string
	if err := WithLock(func() error {
		whileHeld = secondProcessTakesTheLock(t)
		return nil
	}); err != nil {
		t.Fatalf("WithLock: %v", err)
	}

	if whileHeld != lockHelperRefused {
		t.Fatalf("the second process reported %q; a held lock must refuse it", whileHeld)
	}
	if released := secondProcessTakesTheLock(t); released != lockHelperAcquired {
		t.Fatalf("the second process reported %q after the critical section ended; the lock must be available again", released)
	}
}

// secondProcessTakesTheLock runs the helper below in a separate process against
// the same temporary home, which the child inherits through the environment, and
// returns which outcome it reported.
func secondProcessTakesTheLock(t *testing.T) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess_TakesTheManifestLock$")
	cmd.Env = append(os.Environ(), lockHelperEnv+"=1")
	out, err := cmd.CombinedOutput()
	switch {
	case strings.Contains(string(out), lockHelperRefused):
		return lockHelperRefused
	case strings.Contains(string(out), lockHelperAcquired):
		return lockHelperAcquired
	}
	t.Fatalf("the second process reported neither outcome (%v):\n%s", err, out)
	return ""
}

// TestHelperProcess_TakesTheManifestLock is the second process of the test
// above rather than a test of its own, which is why it skips unless it was
// re-executed with the marker set.
func TestHelperProcess_TakesTheManifestLock(t *testing.T) {
	if os.Getenv(lockHelperEnv) != "1" {
		t.Skip("second-process entry point for TestWithLock_ExcludesASecondProcess")
	}
	if err := WithLock(func() error { return nil }); err != nil {
		fmt.Println(lockHelperRefused, err)
		return
	}
	fmt.Println(lockHelperAcquired)
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
