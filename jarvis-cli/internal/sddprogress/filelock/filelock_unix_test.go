//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func probeExclusiveLock(path string) int {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return probeFailed
	}
	defer file.Close()

	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	switch {
	case err == nil:
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		return probeAcquired
	case errors.Is(err, unix.EWOULDBLOCK):
		return probeContended
	default:
		return probeFailed
	}
}
