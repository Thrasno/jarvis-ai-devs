package config

import (
	"bytes"
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
// If the file already exists, optional supported settings (currently auto_sync)
// are preserved while required auth fields are updated.
func WriteSyncCredentials(apiURL, email, password string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".jarvis", "sync.json")

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create ~/.jarvis: %w", err)
	}

	var autoSync *bool
	if existingData, err := os.ReadFile(path); err == nil {
		var existing syncJSON
		dec := json.NewDecoder(bytes.NewReader(existingData))
		dec.DisallowUnknownFields()
		if decodeErr := dec.Decode(&existing); decodeErr != nil {
			return fmt.Errorf("parse existing sync.json: %w", decodeErr)
		}
		autoSync = existing.AutoSync
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read existing sync.json: %w", err)
	}

	payload := syncJSON{
		APIURL:   strings.TrimSpace(apiURL),
		Email:    strings.TrimSpace(email),
		Password: strings.TrimSpace(password),
		AutoSync: autoSync,
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
