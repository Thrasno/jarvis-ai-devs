//go:build !windows

package sddbinding

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type changeRoot struct{ fd int }

type stateRecord struct {
	data   []byte
	mode   os.FileMode
	exists bool
	dev    uint64
	ino    uint64
}

func init() {
	// Directory sync is best-effort after rename: rename is the commit point, so
	// a sync failure cannot be reported as no mutation.
	syncChangeRoot = func(fd int) error { return unix.Fsync(fd) }
}

func openChangeRoot(changeDir string) (*changeRoot, error) {
	if strings.TrimSpace(changeDir) == "" {
		return nil, fmt.Errorf("%w: empty change directory", ErrUnsafePath)
	}
	path, err := filepath.Abs(filepath.Clean(changeDir))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve change directory", ErrUnsafePath)
	}
	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: open filesystem root", ErrUnsafePath)
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, fmt.Errorf("%w: change directory component", ErrUnsafePath)
		}
		fd = next
	}
	return &changeRoot{fd: fd}, nil
}

func (r *changeRoot) Close() error { return unix.Close(r.fd) }

func (r *changeRoot) samePhysicalDirectory(other *changeRoot) (bool, error) {
	var left, right unix.Stat_t
	if err := unix.Fstat(r.fd, &left); err != nil {
		return false, fmt.Errorf("%w: resolution lock directory identity", ErrUnsafePath)
	}
	if err := unix.Fstat(other.fd, &right); err != nil {
		return false, fmt.Errorf("%w: OpenSpec change directory identity", ErrUnsafePath)
	}
	return left.Dev == right.Dev && left.Ino == right.Ino, nil
}

func (r *changeRoot) lock() (func(), error) {
	if err := unix.Flock(r.fd, unix.LOCK_EX); err != nil {
		return nil, fmt.Errorf("lock OpenSpec binding directory: %w", err)
	}
	return func() { _ = unix.Flock(r.fd, unix.LOCK_UN) }, nil
}

func (r *changeRoot) readState() (stateRecord, error) {
	var expected unix.Stat_t
	if err := unix.Fstatat(r.fd, stateFileName, &expected, unix.AT_SYMLINK_NOFOLLOW); err == unix.ENOENT {
		return stateRecord{}, nil
	} else if err != nil || expected.Mode&unix.S_IFMT != unix.S_IFREG {
		return stateRecord{}, fmt.Errorf("%w: state.yaml", ErrUnsafePath)
	}
	fd, err := unix.Openat(r.fd, stateFileName, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return stateRecord{}, fmt.Errorf("%w: state.yaml", ErrUnsafePath)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || uint64(stat.Dev) != uint64(expected.Dev) || stat.Ino != expected.Ino {
		_ = unix.Close(fd)
		return stateRecord{}, fmt.Errorf("%w: state.yaml", ErrUnsafePath)
	}
	file := os.NewFile(uintptr(fd), stateFileName)
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return stateRecord{}, fmt.Errorf("read OpenSpec binding state: %w", err)
	}
	return stateRecord{data: data, mode: os.FileMode(stat.Mode).Perm(), exists: true, dev: uint64(stat.Dev), ino: stat.Ino}, nil
}

func (r *changeRoot) replaceState(data []byte, prior stateRecord) error {
	var current unix.Stat_t
	err := unix.Fstatat(r.fd, stateFileName, &current, unix.AT_SYMLINK_NOFOLLOW)
	if prior.exists {
		if err != nil || current.Mode&unix.S_IFMT != unix.S_IFREG || uint64(current.Dev) != prior.dev || current.Ino != prior.ino {
			return fmt.Errorf("%w: state.yaml changed before commit", ErrUnsafePath)
		}
	} else if err != unix.ENOENT {
		return fmt.Errorf("%w: state.yaml appeared before commit", ErrUnsafePath)
	}
	mode := prior.mode
	if !prior.exists {
		mode = 0o600
	}
	name, file, err := r.createTemp(mode)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Unlinkat(r.fd, name, 0) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write OpenSpec binding stage: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync OpenSpec binding stage: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close OpenSpec binding stage: %w", err)
	}
	if err := unix.Renameat(r.fd, name, r.fd, stateFileName); err != nil {
		return fmt.Errorf("replace OpenSpec binding state: %w", err)
	}
	_ = syncChangeRoot(r.fd)
	return nil
}

func (r *changeRoot) createTemp(mode os.FileMode) (string, *os.File, error) {
	for range 10 {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", nil, fmt.Errorf("create OpenSpec binding stage: %w", err)
		}
		name := ".state.yaml.sdd-binding-" + hex.EncodeToString(suffix[:])
		fd, err := unix.Openat(r.fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode))
		if err == unix.EEXIST {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create OpenSpec binding stage: %w", err)
		}
		if err := unix.Fchmod(fd, uint32(mode)); err != nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(r.fd, name, 0)
			return "", nil, fmt.Errorf("set OpenSpec binding mode: %w", err)
		}
		return name, os.NewFile(uintptr(fd), name), nil
	}
	return "", nil, fmt.Errorf("create OpenSpec binding stage: exhausted unique names")
}
