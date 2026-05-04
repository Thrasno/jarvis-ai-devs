package agent

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/lifecycle"
)

func TestLifecycleAdapter_BackupTargetsResolveProviderPaths(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	targets, err := adapter.BackupTargets([]lifecycle.DoctorStep{{AssetID: "orchestrator"}, {AssetID: "skills"}})
	if err != nil {
		t.Fatalf("BackupTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Path == "" || targets[1].Path == "" {
		t.Fatal("backup targets must contain resolved absolute paths")
	}
}

func TestLifecycleAdapter_ApplyCreatesManagedArtifacts(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	if err := adapter.Apply([]lifecycle.DoctorStep{{AssetID: "orchestrator"}, {AssetID: "skills"}}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")); err != nil {
		t.Fatalf("expected orchestrator artifact to exist after apply: %v", err)
	}
	if stat, err := os.Stat(filepath.Join(a.ConfigDir(), "skills")); err != nil || !stat.IsDir() {
		t.Fatalf("expected skills directory after apply, statErr=%v isDir=%v", err, err == nil && stat.IsDir())
	}
}

func TestLifecycleAdapter_RestoreWritesFilesFromSnapshotArchive(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	target := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := writeSingleFileArchive(archive, target, []byte("restored-content")); err != nil {
		t.Fatalf("writeSingleFileArchive: %v", err)
	}

	manifest := lifecycle.BackupManifest{
		SnapshotID:  "snap-1",
		ArchivePath: archive,
		Entries:     []lifecycle.BackupEntry{{Path: target}},
	}

	restored, err := adapter.Restore(manifest)
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored != 1 {
		t.Fatalf("expected 1 restored entry, got %d", restored)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(raw) != "restored-content" {
		t.Fatalf("unexpected restored content: %q", string(raw))
	}
}

func TestLifecycleAdapter_RestoreRejectsPathOutsideAllowedRoots(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := writeSingleFileArchive(archive, "/etc/passwd", []byte("nope")); err != nil {
		t.Fatalf("writeSingleFileArchive: %v", err)
	}

	manifest := lifecycle.BackupManifest{
		SnapshotID:  "snap-escape",
		ArchivePath: archive,
		Entries:     []lifecycle.BackupEntry{{Path: "/etc/passwd"}},
	}

	_, err := adapter.Restore(manifest)
	if err == nil {
		t.Fatal("expected restore to reject path outside allowed roots")
	}
}

func writeSingleFileArchive(archivePath, absolutePath string, content []byte) (err error) {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	gz := gzip.NewWriter(f)
	defer func() {
		if cerr := gz.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	tw := tar.NewWriter(gz)
	defer func() {
		if cerr := tw.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	name := absolutePath
	if len(name) > 0 && name[0] == filepath.Separator {
		name = name[1:]
	}
	hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	return nil
}
