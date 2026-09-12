// Go exposes no aggregate "unix" build tag, so keep this explicit list aligned
// with the GOOS values supported by golang.org/x/sys/unix.
//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryLockFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrBusy
		}
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
