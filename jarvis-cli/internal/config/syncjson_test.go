package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSyncCredentials_CreatesFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if err := WriteSyncCredentials("https://hivemem.dev", "user@example.com", "s3cr3t"); err != nil {
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
}

func TestWriteSyncCredentials_PreservesAutoSync(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":false}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := WriteSyncCredentials("https://hivemem.dev", "new@example.com", "newpass"); err != nil {
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
