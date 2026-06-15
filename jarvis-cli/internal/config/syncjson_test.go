package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteSyncCredentials_CreatesFile(t *testing.T) {
	tmpHome := isolateHome(t)

	if err := WriteSyncCredentials("https://hivemem.dev", "user@example.com", "s3cr3t", nil); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	path := filepath.Join(tmpHome, ".jarvis", "sync.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sync.json: %v", err)
	}
	if !strings.Contains(string(data), `"email":"user@example.com"`) {
		t.Fatalf("expected written email, got: %s", string(data))
	}
	if strings.Contains(string(data), "auto_sync") {
		t.Fatalf("expected new credentials to keep auto_sync omitted/disabled, got: %s", string(data))
	}
}

func TestWriteSyncCredentials_PreservesPasswordWhitespace(t *testing.T) {
	tmpHome := isolateHome(t)

	if err := WriteSyncCredentials("https://hivemem.dev", "user@example.com", " s3cr3t ", nil); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".jarvis", "sync.json"))
	if err != nil {
		t.Fatalf("read sync.json: %v", err)
	}
	if !strings.Contains(string(data), `"password":" s3cr3t "`) {
		t.Fatalf("expected password preserved with whitespace, got: %s", string(data))
	}
}

func TestWriteSyncCredentials_PreservesAutoSync(t *testing.T) {
	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":false}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass", nil); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":false`) {
		t.Fatalf("expected auto_sync preserved, got: %s", body)
	}
	if !strings.Contains(body, `"email":"new@example.com"`) {
		t.Fatalf("expected updated credentials, got: %s", body)
	}
}

func TestWriteSyncCredentials_ReturnsParseErrorForEmptyExistingFile(t *testing.T) {
	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(jarvisDir, "sync.json")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}

	err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass", nil)
	if err == nil {
		t.Fatal("expected parse error for empty existing sync.json")
	}
	if !strings.Contains(err.Error(), "parse existing sync.json") {
		t.Fatalf("expected parse error, got: %v", err)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read sync.json after failure: %v", readErr)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty sync.json preserved after parse failure, got: %q", string(data))
	}
}

func TestWriteSyncCredentials_LeavesPreviousFileWhenUpdateFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows chmod does not make the destination directory reliably read-only")
	}

	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(jarvisDir, "sync.json")
	original := `{"api_url":"https://hivemem.dev","email":"old@example.com","password":"old"}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	// Make destination directory read-only so atomic rename/write fails.
	if err := os.Chmod(jarvisDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(jarvisDir, 0755) })

	err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass", nil)
	if err == nil {
		t.Fatal("expected write failure when destination directory is read-only")
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read sync.json after failure: %v", readErr)
	}
	if string(data) != original {
		t.Fatalf("expected original sync.json content preserved on failure, got: %s", string(data))
	}
}

func TestWriteSyncCredentials_ForceEnableWritesTrue(t *testing.T) {
	tmpHome := isolateHome(t)

	enable := true
	if err := WriteSyncCredentials("https://hivemem.dev", "user@example.com", "s3cr3t", &enable); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".jarvis", "sync.json"))
	if err != nil {
		t.Fatalf("read sync.json: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync:true written, got: %s", body)
	}
	if !strings.Contains(body, `"email":"user@example.com"`) {
		t.Fatalf("expected email written, got: %s", body)
	}
}

func TestWriteSyncCredentials_ForceEnableOverridesExistingFalse(t *testing.T) {
	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":false}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	enable := true
	if err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass", &enable); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync overridden to true, got: %s", body)
	}
	if !strings.Contains(body, `"email":"new@example.com"`) {
		t.Fatalf("expected updated email, got: %s", body)
	}
}

func TestWriteSyncCredentials_ForceDisableWritesFalse(t *testing.T) {
	tmpHome := isolateHome(t)

	disable := false
	if err := WriteSyncCredentials("https://hivemem.dev", "user@example.com", "s3cr3t", &disable); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".jarvis", "sync.json"))
	if err != nil {
		t.Fatalf("read sync.json: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":false`) {
		t.Fatalf("expected auto_sync:false written, got: %s", body)
	}
}

func TestWriteSyncCredentials_PreserveNoFileOmitsField(t *testing.T) {
	tmpHome := isolateHome(t)

	if err := WriteSyncCredentials("https://hivemem.dev", "user@example.com", "s3cr3t", nil); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpHome, ".jarvis", "sync.json"))
	if err != nil {
		t.Fatalf("read sync.json: %v", err)
	}
	if strings.Contains(string(data), "auto_sync") {
		t.Fatalf("expected auto_sync field absent when nil and no existing file, got: %s", string(data))
	}
}

func TestDeleteSyncCredentials_RemovesExistingFile(t *testing.T) {
	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(jarvisDir, "sync.json")
	if err := os.WriteFile(path, []byte(`{"email":"old@example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := DeleteSyncCredentials(); err != nil {
		t.Fatalf("DeleteSyncCredentials: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected sync.json deleted, stat err: %v", err)
	}
}

func TestWriteSyncCredentials_ToleratesUnknownFieldsInExistingFile(t *testing.T) {
	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":true,"refresh_token":"tok123","future_field":42}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass", nil); err != nil {
		t.Fatalf("WriteSyncCredentials should tolerate unknown fields, got: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"email":"new@example.com"`) {
		t.Fatalf("expected updated credentials, got: %s", body)
	}
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync preserved, got: %s", body)
	}
}

func TestDeleteSyncCredentials_IdempotentWhenMissing(t *testing.T) {
	isolateHome(t)

	if err := DeleteSyncCredentials(); err != nil {
		t.Fatalf("DeleteSyncCredentials missing file should not fail: %v", err)
	}
}

func TestWriteSyncCredentials_NilPreservesExistingAutoSyncTrue(t *testing.T) {
	tmpHome := isolateHome(t)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":true}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass", nil); err != nil {
		t.Fatalf("WriteSyncCredentials: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync preserved as true when nil passed, got: %s", body)
	}
	if !strings.Contains(body, `"email":"new@example.com"`) {
		t.Fatalf("expected updated credentials, got: %s", body)
	}
}
