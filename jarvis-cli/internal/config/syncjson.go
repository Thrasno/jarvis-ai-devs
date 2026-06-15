package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type syncJSON struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AutoSync *bool  `json:"auto_sync,omitempty"`
}

// WriteSyncCredentials writes ~/.jarvis/sync.json with cloud credentials.
//
// The autoSync parameter controls the auto_sync field using a tri-state:
//   - nil: preserve the existing value from the current file (or omit if no file / field absent).
//   - &true: force-enable auto_sync regardless of any existing value.
//   - &false: force-disable auto_sync regardless of any existing value.
//
// Required auth fields (api_url, email, password) are always updated.
// Unknown fields in an existing file are ignored so that a newer-format sync.json
// does not block credential updates.
// If the file already exists and cannot be parsed as JSON, the call returns an error
// without writing a new file.
func WriteSyncCredentials(apiURL, email, password string, autoSync *bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".jarvis", "sync.json")

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create ~/.jarvis: %w", err)
	}

	var existingAutoSync *bool
	if existingData, err := os.ReadFile(path); err == nil {
		var existing syncJSON
		if decodeErr := json.Unmarshal(existingData, &existing); decodeErr != nil {
			return fmt.Errorf("parse existing sync.json: %w", decodeErr)
		}
		existingAutoSync = existing.AutoSync
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read existing sync.json: %w", err)
	}

	// Resolve final auto_sync value: explicit intent overrides the preserved value.
	resolved := existingAutoSync
	if autoSync != nil {
		v := *autoSync // copy to a fresh addressable bool — avoid aliasing the caller's pointer
		resolved = &v
	}

	payload := syncJSON{
		APIURL:   strings.TrimSpace(apiURL),
		Email:    strings.TrimSpace(email),
		Password: password,
		AutoSync: resolved,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sync.json: %w", err)
	}

	if err := atomicWriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write sync.json: %w", err)
	}
	return nil
}

// DeleteSyncCredentials removes ~/.jarvis/sync.json if it exists.
// The operation is idempotent and succeeds when the file is already missing.
func DeleteSyncCredentials() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".jarvis", "sync.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete sync.json: %w", err)
	}
	return nil
}
