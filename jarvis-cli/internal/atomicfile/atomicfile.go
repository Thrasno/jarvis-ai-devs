// Package atomicfile writes files atomically and durably.
//
// A write lands in a temp file in the destination directory, is fsynced, then
// renamed over the destination, and finally the parent directory is fsynced.
// A reader therefore observes either the previous content or the complete new
// content, never a partial write.
package atomicfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// yamlFileMode is the owner-only mode used for Jarvis YAML stores under ~/.jarvis.
const yamlFileMode os.FileMode = 0600

// dirMode is the mode used when creating a missing parent directory.
const dirMode os.FileMode = 0755

// WriteYAML creates the parent directory if needed and atomically writes data
// with owner-only permissions.
func WriteYAML(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return Write(path, data, yamlFileMode)
}

// Write atomically writes data to path with the given mode. The parent
// directory must already exist.
func Write(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := bytes.NewReader(data).WriteTo(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent dir: %w", err)
	}
	defer func() {
		_ = d.Close()
	}()
	if runtime.GOOS != "windows" {
		if err := d.Sync(); err != nil {
			return fmt.Errorf("fsync parent dir: %w", err)
		}
	}

	cleanup = false
	return nil
}
