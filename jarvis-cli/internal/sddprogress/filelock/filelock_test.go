package filelock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const probePathEnv = "JARVIS_FILELOCK_PROBE_PATH"

const (
	probeAcquired = iota
	probeContended
	probeFailed
)

func TestAcquireMutualExclusion(t *testing.T) {
	if path := os.Getenv(probePathEnv); path != "" {
		os.Exit(probeExclusiveLock(path))
	}

	path := filepath.Join(t.TempDir(), "apply-progress.lock")
	unlock, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if got := runProbe(t, path); got != probeContended {
		t.Fatalf("probe while held exit code = %d, want %d", got, probeContended)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock() error = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("second unlock() error = %v", err)
	}
	if got := runProbe(t, path); got != probeAcquired {
		t.Fatalf("probe after unlock exit code = %d, want %d", got, probeAcquired)
	}
}

func TestAcquireReusesResidualLockfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apply-progress.lock")
	if err := os.WriteFile(path, []byte("residual lockfile"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	unlock, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() with residual file error = %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat() residual lockfile error = %v", err)
	}
}

func runProbe(t *testing.T, path string) int {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestAcquireMutualExclusion$")
	cmd.Env = append(os.Environ(), probePathEnv+"="+path)
	err := cmd.Run()
	if err == nil {
		return probeAcquired
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	t.Fatalf("run probe: %v", err)
	return probeFailed
}
