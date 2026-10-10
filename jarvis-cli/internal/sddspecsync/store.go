package sddspecsync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/atomicfile"
)

// newSpecMode is the mode of main specs created by Apply. Existing specs keep
// their mode.
const newSpecMode os.FileMode = 0o644

// OSStore is a Store rooted at a project directory. Paths are slash-separated
// and relative to Root; symlinks and non-regular files are refused.
type OSStore struct {
	Root string
}

func (s OSStore) ReadFile(name string) ([]byte, bool, error) {
	full, err := s.resolve(name)
	if err != nil {
		return nil, false, err
	}
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, name)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, false, err
	}
	if data == nil {
		data = []byte{}
	}
	return data, true, nil
}

func (s OSStore) WriteFile(name string, data []byte) error {
	full, err := s.resolve(name)
	if err != nil {
		return err
	}
	mode := newSpecMode
	if info, err := os.Lstat(full); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, name)
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if _, err := s.resolve(name); err != nil {
		return err
	}
	return atomicfile.Write(full, data, mode)
}

func (s OSStore) Remove(name string) error {
	full, err := s.resolve(name)
	if err != nil {
		return err
	}
	return atomicfile.Remove(full)
}

// resolve maps name below Root and rejects escapes and symlinked components.
func (s OSStore) resolve(name string) (string, error) {
	if !fs.ValidPath(name) || name == "." {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	current := s.Root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: %s is a symlink", ErrUnsafePath, name)
		}
	}
	return filepath.Join(s.Root, filepath.FromSlash(name)), nil
}
