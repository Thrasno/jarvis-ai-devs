package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupStore_CreateSnapshotWritesManifestWithChecksums(t *testing.T) {
	home := t.TempDir()
	store := NewBackupStore(home)

	root := filepath.Join(home, ".claude")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(root, "settings.json")
	if err := os.WriteFile(target, []byte(`{"foo":"bar"}`), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	manifest, err := store.CreateSnapshot("reconcile", []BackupTarget{{Path: target}})
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}
	if manifest.SnapshotID == "" || manifest.ArchivePath == "" {
		t.Fatalf("expected snapshot metadata populated: %#v", manifest)
	}
	if manifest.CreatedAt == "" {
		t.Fatalf("expected created_at metadata populated: %#v", manifest)
	}
	if manifest.JarvisVersion == "" || manifest.ContractVersion == "" || manifest.ProviderSchemaVersion == "" {
		t.Fatalf("expected version metadata populated: %#v", manifest)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].Checksum == "" {
		t.Fatalf("expected per-entry checksum in manifest: %#v", manifest.Entries)
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "backups", manifest.SnapshotID+".manifest.json")); err != nil {
		t.Fatalf("expected persisted manifest: %v", err)
	}
	if info, err := os.Stat(manifest.ArchivePath); err != nil {
		t.Fatalf("expected compressed snapshot archive: %v", err)
	} else if info.Size() == 0 {
		t.Fatal("expected non-empty compressed snapshot archive")
	}
}

func TestBackupStore_CreateSnapshotSetsSourceOperationAndArchivePath(t *testing.T) {
	home := t.TempDir()
	store := NewBackupStore(home)

	root := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(root, "config.toml")
	if err := os.WriteFile(target, []byte("key = \"value\""), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	manifest, err := store.CreateSnapshot("backup", []BackupTarget{{Path: target}})
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}
	if manifest.SourceOperation != "backup" {
		t.Fatalf("source operation mismatch: got %q", manifest.SourceOperation)
	}
	wantArchive := filepath.Join(home, ".jarvis", "backups", manifest.SnapshotID+".tar.gz")
	if manifest.ArchivePath != wantArchive {
		t.Fatalf("archive path mismatch: got %q want %q", manifest.ArchivePath, wantArchive)
	}
}

func TestBackupStore_RestoreRejectsEscapingPath(t *testing.T) {
	home := t.TempDir()
	store := NewBackupStore(home)

	manifest := BackupManifest{
		SnapshotID:      "snap-unsafe",
		SourceOperation: "restore",
		Entries:         []BackupEntry{{Path: "../../etc/shadow", Checksum: "abc"}},
	}
	err := store.ValidateManifest(manifest)
	if err == nil {
		t.Fatal("expected unsafe path validation error")
	}
}

func TestBackupStore_ValidateSnapshotDetectsChecksumMismatch(t *testing.T) {
	home := t.TempDir()
	store := NewBackupStore(home)

	root := filepath.Join(home, ".claude")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(root, "settings.json")
	if err := os.WriteFile(target, []byte(`{"before":"value"}`), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	manifest, err := store.CreateSnapshot("restore", []BackupTarget{{Path: target}})
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}
	manifest.Entries[0].Checksum = "deadbeef"

	if err := store.ValidateSnapshot(manifest); err == nil {
		t.Fatal("expected checksum mismatch for tampered manifest")
	}
}

func TestBackupStore_CreateSnapshotFailsWhenTargetMissing(t *testing.T) {
	home := t.TempDir()
	store := NewBackupStore(home)

	missing := filepath.Join(home, ".claude", "missing.json")
	_, err := store.CreateSnapshot("backup", []BackupTarget{{Path: missing}})
	if err == nil {
		t.Fatal("expected error when backup target is missing")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected actionable missing-target error, got: %v", err)
	}
}

func TestBackupStore_ValidateManifestRejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	store := NewBackupStore(home)

	root := filepath.Join(home, ".claude")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	symlinked := filepath.Join(root, "evil")
	if err := os.Symlink("/etc", symlinked); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	manifest := BackupManifest{
		SnapshotID:      "snap-symlink",
		SourceOperation: "restore",
		Entries:         []BackupEntry{{Path: filepath.Join(symlinked, "passwd"), Checksum: "abc"}},
	}

	err := store.ValidateManifest(manifest)
	if err == nil {
		t.Fatal("expected symlink escape validation error")
	}
}
