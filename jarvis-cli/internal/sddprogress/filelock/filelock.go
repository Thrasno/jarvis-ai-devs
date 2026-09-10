package filelock

import (
	"fmt"
	"os"
	"sync"
)

// Acquire obtains an exclusive lock for path and returns a function that releases it.
func Acquire(path string) (func() error, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lockfile %q: %w", path, err)
	}

	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock %q: %w", path, err)
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
