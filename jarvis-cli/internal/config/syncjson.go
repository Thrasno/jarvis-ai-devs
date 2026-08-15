package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/atomicfile"
)

type syncJSON struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AutoSync *bool  `json:"auto_sync,omitempty"`
}

// SyncCredentialsComplete reports whether the given sync.json contents carry the
// credentials a login actually produces, rather than merely being valid JSON.
//
// It lives next to the writer on purpose: the answer is "does this match what
// WriteSyncCredentials emits", so it decodes into that same struct instead of a
// second, drifting copy of the field names. api_url, email and password are the
// three fields the writer always emits and the three hive-daemon refuses the
// file without; auto_sync is optional at both ends. Unknown fields are ignored
// so a newer-format file is not mistaken for a broken one, which mirrors the
// tolerance WriteSyncCredentials already grants an existing file.
//
// api_url and email are compared after trimming because the writer trims them,
// so blanks could never have come from a real login. The password is compared
// verbatim, because the writer deliberately preserves its whitespace.
func SyncCredentialsComplete(contents []byte) bool {
	var credentials syncJSON
	if err := json.Unmarshal(contents, &credentials); err != nil {
		return false
	}
	return strings.TrimSpace(credentials.APIURL) != "" &&
		strings.TrimSpace(credentials.Email) != "" &&
		credentials.Password != ""
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

	if err := atomicfile.Write(path, data, 0600); err != nil {
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
