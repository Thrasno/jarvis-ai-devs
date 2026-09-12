//go:build windows

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func probeExclusiveLock(path string) int {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return probeFailed
	}
	defer file.Close()

	overlapped := windows.Overlapped{}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		^uint32(0),
		^uint32(0),
		&overlapped,
	)
	switch {
	case err == nil:
		_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, ^uint32(0), ^uint32(0), &overlapped)
		return probeAcquired
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return probeContended
	default:
		return probeFailed
	}
}
