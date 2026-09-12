package filelock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

func TestAcquireWaitsForReleaseAndReturnsBusyAfterTimeout(t *testing.T) {
	previousTimeout := fileLockTimeout
	fileLockTimeout = 75 * time.Millisecond
	t.Cleanup(func() { fileLockTimeout = previousTimeout })
	path := filepath.Join(t.TempDir(), "apply-progress.lock")
	unlock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	released := make(chan struct{})
	go func(unlock func() error) {
		<-release
		_ = unlock()
		close(released)
	}(unlock)
	close(release)
	waited, err := Acquire(path)
	<-released
	if err != nil {
		t.Fatalf("Acquire() after release: %v", err)
	}
	if err := waited(); err != nil {
		t.Fatal(err)
	}

	unlock, err = Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlock() }()
	start := time.Now()
	_, err = Acquire(path)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire() while held error = %v, want ErrBusy", err)
	}
	if time.Since(start) < fileLockRetryInitialDelay {
		t.Fatal("Acquire() returned busy without waiting")
	}
}

func TestAcquireUsesCappedExponentialBackoff(t *testing.T) {
	previousTimeout := fileLockTimeout
	previousInitialDelay := fileLockRetryInitialDelay
	previousMaximumDelay := fileLockRetryMaximumDelay
	previousNow := fileLockNow
	previousSleep := fileLockSleep
	t.Cleanup(func() {
		fileLockTimeout = previousTimeout
		fileLockRetryInitialDelay = previousInitialDelay
		fileLockRetryMaximumDelay = previousMaximumDelay
		fileLockNow = previousNow
		fileLockSleep = previousSleep
	})

	fileLockTimeout = 75 * time.Millisecond
	fileLockRetryInitialDelay = 10 * time.Millisecond
	fileLockRetryMaximumDelay = 25 * time.Millisecond
	now := time.Unix(0, 0)
	fileLockNow = func() time.Time { return now }
	var delays []time.Duration
	fileLockSleep = func(delay time.Duration) {
		delays = append(delays, delay)
		now = now.Add(delay)
	}

	path := filepath.Join(t.TempDir(), "apply-progress.lock")
	unlock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlock() }()

	_, err = Acquire(path)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire() while held error = %v, want ErrBusy", err)
	}
	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond, 20 * time.Millisecond}
	if len(delays) != len(want) {
		t.Fatalf("retry delays = %v, want %v", delays, want)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("retry delay %d = %v, want %v", i, delays[i], want[i])
		}
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
