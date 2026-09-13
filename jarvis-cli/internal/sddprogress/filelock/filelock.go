package filelock

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// ErrBusy identifies a lock that could not be acquired before the bounded wait elapsed.
var ErrBusy = errors.New("file lock busy")

// BusyError retains the contended path while allowing callers to classify ErrBusy.
type BusyError struct{ Path string }

func (e *BusyError) Error() string { return fmt.Sprintf("lock %q: %v", e.Path, ErrBusy) }
func (e *BusyError) Unwrap() error { return ErrBusy }

var (
	fileLockRetryInitialDelay = 10 * time.Millisecond
	fileLockRetryMaximumDelay = 250 * time.Millisecond
	fileLockTimeout           = 5 * time.Second
	fileLockNow               = time.Now
	fileLockSleep             = time.Sleep
)

func fileLockRetryDelay(retry int, remaining time.Duration) time.Duration {
	delay := fileLockRetryInitialDelay
	if fileLockRetryMaximumDelay > 0 && delay > fileLockRetryMaximumDelay {
		delay = fileLockRetryMaximumDelay
	}
	for i := 0; i < retry && delay < fileLockRetryMaximumDelay; i++ {
		if delay > fileLockRetryMaximumDelay/2 {
			delay = fileLockRetryMaximumDelay
		} else {
			delay *= 2
		}
	}
	if delay > remaining {
		return remaining
	}
	return delay
}

// Acquire obtains an exclusive lock for path and returns a function that releases it.
// It uses nonblocking platform primitives so a stale or active lock cannot block forever.
func Acquire(path string) (func() error, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lockfile %q: %w", path, err)
	}

	deadline := fileLockNow().Add(fileLockTimeout)
	for retry := 0; ; retry++ {
		err = tryLockFile(file)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrBusy) {
			_ = file.Close()
			return nil, fmt.Errorf("lock %q: %w", path, err)
		}
		remaining := deadline.Sub(fileLockNow())
		if remaining <= 0 {
			_ = file.Close()
			return nil, &BusyError{Path: path}
		}
		fileLockSleep(fileLockRetryDelay(retry, remaining))
	}

	var once sync.Once
	var unlockErr error
	return func() error {
		once.Do(func() {
			unlockErr = unlockFile(file)
			if err := file.Close(); unlockErr == nil {
				unlockErr = err
			}
		})
		return unlockErr
	}, nil
}
