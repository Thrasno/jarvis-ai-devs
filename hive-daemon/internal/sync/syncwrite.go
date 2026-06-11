package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigUpdate holds the four fields that can be updated via the config API.
type ConfigUpdate struct {
	APIURL   string
	Email    string
	Password string
	AutoSync bool
}

// WriteFileConfig marshals u into a syncFileConfig and writes it atomically
// to the path returned by configFilePath (test-swappable). The parent
// directory is created with mode 0700 if it does not exist.
// File mode is 0600 on Unix; permission enforcement is skipped on Windows,
// consistent with loadFromFile behaviour.
func WriteFileConfig(u ConfigUpdate) error {
	path, err := configFilePath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	fc := syncFileConfig{
		APIURL:   u.APIURL,
		Email:    u.Email,
		Password: u.Password,
		AutoSync: u.AutoSync,
	}

	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := atomicWriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// atomicWriteFile writes data to path atomically by writing to a sibling temp
// file and renaming. perm is applied on Unix; skipped on Windows.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hive-sync-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Ensure temp file is removed on any failure path.
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Apply permissions before rename so the final file always has 0600.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, perm); err != nil {
			return fmt.Errorf("chmod temp file: %w", err)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp to config: %w", err)
	}
	cleanup = false
	return nil
}
