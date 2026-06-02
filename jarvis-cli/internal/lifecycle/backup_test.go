package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArchiveChecksumKeyNormalizesCrossPlatformPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "unix absolute root preserved", path: "/home/dev/project/file.txt", want: "/home/dev/project/file.txt"},
		{name: "unix archive relative becomes rooted", path: "home/dev/project/file.txt", want: "/home/dev/project/file.txt"},
		{name: "windows drive path keeps drive root", path: `C:\Users\dev\notes.txt`, want: "C:/Users/dev/notes.txt"},
		{name: "unc path keeps double root", path: `\\server\share\dir\file.bin`, want: "//server/share/dir/file.bin"},
		{name: "duplicate separators and dot segments are cleaned", path: `/home//dev/./project///file.txt`, want: "/home/dev/project/file.txt"},
		{name: "parent segment cleanup does not corrupt drive root", path: `C:\Users\dev\..\ops\notes.txt`, want: "C:/Users/ops/notes.txt"},
		{name: "parent segment cleanup does not corrupt unc root", path: `\\server\share\dir\..\file.bin`, want: "//server/share/file.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := archiveChecksumKey(tt.path); got != tt.want {
				t.Fatalf("archiveChecksumKey(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestArchivePathRoundtripPreservesRootSemantics(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		archiveName string
		restored    string
	}{
		{name: "unix tar names keep existing no-leading-slash compatibility", path: "/home/dev/project/file.txt", archiveName: "home/dev/project/file.txt", restored: "/home/dev/project/file.txt"},
		{name: "windows drive tar names are not prefixed as unix absolute", path: `C:\Users\dev\notes.txt`, archiveName: "C:/Users/dev/notes.txt", restored: "C:/Users/dev/notes.txt"},
		{name: "unc tar names preserve double-root semantics", path: `\\server\share\dir\file.bin`, archiveName: "//server/share/dir/file.bin", restored: "//server/share/dir/file.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := archivePathName(tt.path); got != tt.archiveName {
				t.Fatalf("archivePathName(%q) = %q, want %q", tt.path, got, tt.archiveName)
			}
			if strings.HasPrefix(tt.archiveName, "/C:") {
				t.Fatalf("archive name must not introduce unix root before drive letter: %q", tt.archiveName)
			}
			if got := restoreArchivePath(tt.archiveName); got != tt.restored {
				t.Fatalf("restoreArchivePath(%q) = %q, want %q", tt.archiveName, got, tt.restored)
			}
		})
	}
}

func TestValidateSnapshotNormalizesManifestAndArchivePaths(t *testing.T) {
	tests := []struct {
		name         string
		manifestPath string
		archiveName  string
		content      string
	}{
		{name: "windows manifest backslashes match drive archive slashes", manifestPath: `C:\Users\dev\notes.txt`, archiveName: "C:/Users/dev/notes.txt", content: "windows notes"},
		{name: "unc manifest backslashes match unc archive slashes", manifestPath: `\\server\share\dir\file.bin`, archiveName: "//server/share/dir/file.bin", content: "unc content"},
		{name: "unix manifest absolute matches relative tar name", manifestPath: "/home/dev/project/file.txt", archiveName: "home/dev/project/file.txt", content: "unix content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archivePath := writeTestArchive(t, tt.archiveName, []byte(tt.content))
			manifest := BackupManifest{
				SnapshotID:  "snap-normalized",
				ArchivePath: archivePath,
				Entries:     []BackupEntry{{Path: tt.manifestPath, Checksum: checksumHex([]byte(tt.content))}},
			}

			store := NewBackupStore(t.TempDir())
			if err := store.ValidateSnapshot(manifest); err != nil {
				t.Fatalf("ValidateSnapshot returned error: %v", err)
			}
		})
	}
}

func TestValidateSnapshotAcceptsBoundedLegacyArchiveNames(t *testing.T) {
	archivePath := writeTestArchive(t, "server/share/dir/file.bin", []byte("legacy unc content"))
	manifest := BackupManifest{
		SnapshotID:  "snap-legacy-unc",
		ArchivePath: archivePath,
		Entries:     []BackupEntry{{Path: "//server/share/dir/file.bin", Checksum: checksumHex([]byte("legacy unc content"))}},
	}

	store := NewBackupStore(t.TempDir())
	if err := store.ValidateSnapshot(manifest); err != nil {
		t.Fatalf("ValidateSnapshot returned error for bounded legacy name: %v", err)
	}

	unsafeKeys := legacyArchiveChecksumKeys("../server/share/dir/file.bin")
	if len(unsafeKeys) != 0 {
		t.Fatalf("legacyArchiveChecksumKeys accepted unsafe traversal path: %#v", unsafeKeys)
	}
	for _, key := range legacyArchiveChecksumKeys(`C:\Users\dev\notes.txt`) {
		if strings.HasPrefix(key, "/C:") {
			t.Fatalf("legacyArchiveChecksumKeys must not create ambiguous drive-root key: %#v", key)
		}
	}
}

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
		if runtime.GOOS == "windows" && isWindowsSymlinkPrivilegeError(err) {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
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

func isWindowsSymlinkPrivilegeError(err error) bool {
	if err == nil {
		return false
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return strings.Contains(linkErr.Err.Error(), "privilege")
	}
	return strings.Contains(err.Error(), "privilege")
}

func writeTestArchive(t *testing.T, name string, content []byte) string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "snapshot.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return archivePath
}

func checksumHex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
