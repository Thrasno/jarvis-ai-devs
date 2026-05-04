package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrBootstrapLedger_CreatesLedgerWhenMissing(t *testing.T) {
	home := t.TempDir()
	store := NewLedgerStore(home)

	ledger, created, err := store.LoadOrBootstrap("claude")
	if err != nil {
		t.Fatalf("LoadOrBootstrap returned error: %v", err)
	}
	if !created {
		t.Fatal("expected bootstrap to create ledger for legacy install")
	}
	if ledger.ProviderSchemaVersion == "" {
		t.Fatal("provider schema version must be initialized")
	}
}

func TestLoadOrBootstrapLedger_MigratesLegacyMissingProviderSchema(t *testing.T) {
	home := t.TempDir()
	store := NewLedgerStore(home)
	legacyPath := filepath.Join(home, ".jarvis", "managed-state.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	legacy := `{"version":"v1","jarvis_version":"dev","contract_version":"2026.05"}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ledger, created, err := store.LoadOrBootstrap("claude")
	if err != nil {
		t.Fatalf("LoadOrBootstrap returned error: %v", err)
	}
	if created {
		t.Fatal("expected existing legacy ledger to migrate in-place")
	}
	if ledger.ProviderSchemaVersion == "" {
		t.Fatal("migration must populate provider_schema_version")
	}
}

func TestLoadOrBootstrapLedger_RejectsIncompatibleVersion(t *testing.T) {
	home := t.TempDir()
	store := NewLedgerStore(home)
	path := filepath.Join(home, ".jarvis", "managed-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	incompatible := `{"version":"v0","jarvis_version":"dev","contract_version":"2026.05","provider_schema_version":"v1"}`
	if err := os.WriteFile(path, []byte(incompatible), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := store.LoadOrBootstrap("claude")
	if err == nil {
		t.Fatal("expected incompatible version error")
	}
}
