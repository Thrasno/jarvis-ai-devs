package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

func TestFileCompensationStoreRejectsPathsOutsideRenderedManagedOutputs(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{
		Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed"),
	}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}

	if err := store.Write("user/notes.md", []byte("replacement"), reconcile.Provenance{}); err == nil {
		t.Fatal("Write() error = nil, want rejection for user-owned path")
	}
	if _, err := store.Snapshot("../outside"); err == nil {
		t.Fatal("Snapshot() error = nil, want rejection for unsafe path")
	}
}

func TestFileCompensationStoreRejectsOpenCodeIdentityPathMismatch(t *testing.T) {
	for _, output := range []RenderedManagedOutput{
		{Identity: openCodeGlobalConfigIdentity, Location: "other/opencode.json"},
		{Identity: "other-managed-output", Location: openCodeGlobalConfigLocation},
	} {
		if _, err := NewFileCompensationStore(t.TempDir(), []RenderedManagedOutput{output}); err == nil {
			t.Fatalf("NewFileCompensationStore(%#v) error = nil, want OpenCode binding rejection", output)
		}
	}
}

func TestFileCompensationStoreRejectsManagedPathSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "user-owned")
	if err := os.WriteFile(outside, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "claude"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "claude", "CLAUDE.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md"}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if err := store.Write("claude/CLAUDE.md", []byte("managed"), reconcile.Provenance{}); err == nil {
		t.Fatal("Write() error = nil, want symlink rejection")
	}
	got, readErr := os.ReadFile(outside)
	if readErr != nil || string(got) != "preserve" {
		t.Fatalf("outside file = %q, %v; want preserved bytes", got, readErr)
	}
}

func TestFileCompensationStoreRejectsSymlinkRootBeforeAnyMutation(t *testing.T) {
	outside := t.TempDir()
	rootLink := filepath.Join(t.TempDir(), "managed-root")
	if err := os.Symlink(outside, rootLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := NewFileCompensationStore(rootLink, []RenderedManagedOutput{{
		Identity: "jarvis-instructions", Location: "claude/CLAUDE.md",
	}})
	if err == nil {
		t.Fatal("NewFileCompensationStore() error = nil, want symlink-root rejection")
	}
	if strings.Contains(err.Error(), rootLink) || strings.Contains(err.Error(), outside) {
		t.Fatalf("NewFileCompensationStore() error leaked filesystem path: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "claude", "CLAUDE.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside managed artifact = %v, want no mutation", statErr)
	}
}

func TestFileCompensationStoreRechecksRootBeforeMutation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{
		Identity: "jarvis-instructions", Location: "claude/CLAUDE.md",
	}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err = store.Write("claude/CLAUDE.md", []byte("managed"), reconcile.Provenance{})
	if err == nil {
		t.Fatal("Write() error = nil, want swapped-root rejection")
	}
	if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), outside) {
		t.Fatalf("Write() error leaked filesystem path: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "claude", "CLAUDE.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside managed artifact = %v, want no mutation", statErr)
	}
}

func TestFileCompensationStorePersistsOpenCodeProvenanceForFreshReconfigure(t *testing.T) {
	root := t.TempDir()
	location := openCodeGlobalConfigLocation
	content := []byte(`{"theme":"night","mcp":{"hive":{"type":"local"}}}`)
	provenance := reconcile.Provenance{
		Version:         "v1",
		ManagedIdentity: openCodeGlobalConfigIdentity,
		Location:        location,
		ManifestDigest:  managedOutputDigest(content),
	}
	outputs := []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}}
	store, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if err := store.Write(location, content, provenance); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	fresh, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("fresh NewFileCompensationStore() error = %v", err)
	}
	snapshot, err := fresh.Snapshot(location)
	if err != nil {
		t.Fatalf("fresh Snapshot() error = %v", err)
	}
	if !snapshot.Exists || string(snapshot.Bytes) != string(content) || snapshot.Provenance != provenance {
		t.Fatalf("fresh Snapshot() = %#v, want exact bytes and provenance", snapshot)
	}

}

func TestFileCompensationStoreFailsClosedForAmbiguousOrCorruptOpenCodeProvenance(t *testing.T) {
	root := t.TempDir()
	location := openCodeGlobalConfigLocation
	content := []byte(`{"mcp":{"hive":{"token":"fixture-secret"}}}`)
	path := filepath.Join(root, filepath.FromSlash(location))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	outputs := []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}}
	store, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if _, err := store.Snapshot(location); err == nil || strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), path) {
		t.Fatalf("Snapshot() error = %v, want sanitized ambiguous-ownership rejection", err)
	}

	manifestPath := filepath.Join(root, ".jarvis", "metadata", "reconcile", "opencode-global-config.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatalf("MkdirAll(manifest) error = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(`{"version":"unknown","token":"fixture-secret"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	if _, err := store.Snapshot(location); err == nil || strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), manifestPath) {
		t.Fatalf("Snapshot() error = %v, want sanitized corrupt-manifest rejection", err)
	}
}

func TestFileCompensationStoreSnapshotsOpenCodePairStates(t *testing.T) {
	location := openCodeGlobalConfigLocation
	artifact := []byte(`{"theme":"night"}`)
	manifest := []byte(`{"token":"fixture-secret"}`)
	tests := []struct {
		name          string
		artifact      []byte
		artifactFound bool
		artifactErr   error
		manifest      []byte
		manifestFound bool
		manifestErr   error
		wantExists    bool
		wantErr       string
	}{
		{name: "both absent is clean", wantExists: false},
		{name: "artifact only is unmanaged", artifact: artifact, artifactFound: true, wantExists: true},
		{name: "manifest only fails closed", manifest: manifest, manifestFound: true, wantErr: "incomplete"},
		{name: "artifact read failure is sanitized", artifactErr: errors.New("read token=fixture-secret"), wantErr: "unavailable"},
		{name: "manifest read failure is sanitized", artifact: artifact, artifactFound: true, manifestErr: errors.New("read token=fixture-secret"), wantErr: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}})
			if err != nil {
				t.Fatalf("NewFileCompensationStore() error = %v", err)
			}
			artifactPath := filepath.Join(root, filepath.FromSlash(location))
			manifestPath := filepath.Join(root, ".jarvis", "metadata", "reconcile", "opencode-global-config.json")
			originalRead := readOpenCodePairFile
			t.Cleanup(func() { readOpenCodePairFile = originalRead })
			readOpenCodePairFile = func(path string) ([]byte, bool, error) {
				switch path {
				case artifactPath:
					return tt.artifact, tt.artifactFound, tt.artifactErr
				case manifestPath:
					return tt.manifest, tt.manifestFound, tt.manifestErr
				default:
					t.Fatalf("unexpected pair path %q", path)
					return nil, false, nil
				}
			}

			snapshot, err := store.Snapshot(location)
			if tt.wantErr == "" {
				if err != nil || snapshot.Exists != tt.wantExists || (tt.wantExists && string(snapshot.Bytes) != string(artifact)) {
					t.Fatalf("Snapshot() = (%#v, %v), want exists=%t", snapshot, err, tt.wantExists)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), root) {
				t.Fatalf("Snapshot() error = %v, want sanitized %q failure", err, tt.wantErr)
			}
		})
	}
}

func TestFileCompensationStoreRejectsOpenCodeManifestBindingMismatches(t *testing.T) {
	content := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	digest := managedOutputDigest(content)
	tests := []struct {
		name     string
		manifest string
	}{
		{name: "identity", manifest: `{"version":"v1","identity":"other","location":".config/opencode/opencode.json","digest":"` + digest + `","provenance":{"version":"v1","managed_identity":"opencode-global-config","location":".config/opencode/opencode.json","manifest_digest":"` + digest + `"}}`},
		{name: "path", manifest: `{"version":"v1","identity":"opencode-global-config","location":"other.json","digest":"` + digest + `","provenance":{"version":"v1","managed_identity":"opencode-global-config","location":"other.json","manifest_digest":"` + digest + `"}}`},
		{name: "digest", manifest: `{"version":"v1","identity":"opencode-global-config","location":".config/opencode/opencode.json","digest":"sha256:wrong","provenance":{"version":"v1","managed_identity":"opencode-global-config","location":".config/opencode/opencode.json","manifest_digest":"sha256:wrong"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			location := openCodeGlobalConfigLocation
			path := filepath.Join(root, filepath.FromSlash(location))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			manifestPath := filepath.Join(root, ".jarvis", "metadata", "reconcile", "opencode-global-config.json")
			if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
				t.Fatalf("MkdirAll(manifest) error = %v", err)
			}
			if err := os.WriteFile(manifestPath, []byte(tt.manifest), 0o600); err != nil {
				t.Fatalf("WriteFile(manifest) error = %v", err)
			}
			store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}})
			if err != nil {
				t.Fatalf("NewFileCompensationStore() error = %v", err)
			}
			if _, err := store.Snapshot(location); err == nil || strings.Contains(err.Error(), manifestPath) {
				t.Fatalf("Snapshot() error = %v, want sanitized mismatch rejection", err)
			}
		})
	}
}

func TestFileCompensationStoreRestoresOpenCodeBytesAndProvenanceAsAPair(t *testing.T) {
	root := t.TempDir()
	location := openCodeGlobalConfigLocation
	outputs := []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}}
	store, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	prior := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	priorProvenance := reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: location, ManifestDigest: managedOutputDigest(prior)}
	if err := store.Write(location, prior, priorProvenance); err != nil {
		t.Fatalf("Write(prior) error = %v", err)
	}
	snapshot, err := store.Snapshot(location)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	updated := []byte(`{"mcp":{"hive":{"type":"remote"}}}`)
	updatedProvenance := reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: location, ManifestDigest: managedOutputDigest(updated)}
	if err := store.Write(location, updated, updatedProvenance); err != nil {
		t.Fatalf("Write(updated) error = %v", err)
	}
	if err := store.Write(location, snapshot.Bytes, snapshot.Provenance); err != nil {
		t.Fatalf("Write(restore) error = %v", err)
	}

	restored, err := NewFileCompensationStore(root, outputs)
	if err != nil || string(restoredSnapshotBytes(t, restored, location)) != string(prior) {
		t.Fatalf("fresh restored Snapshot() error = %v, want prior bytes", err)
	}
	got, err := restored.Snapshot(location)
	if err != nil || got.Provenance != priorProvenance {
		t.Fatalf("fresh restored Snapshot() = %#v, %v; want prior provenance", got, err)
	}
}

func TestFileCompensationStoreCompensatesOpenCodeBytesAndProvenance(t *testing.T) {
	root := t.TempDir()
	location := openCodeGlobalConfigLocation
	outputs := []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}}
	store, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	prior := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	priorProvenance := reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: location, ManifestDigest: managedOutputDigest(prior)}
	if err := store.Write(location, prior, priorProvenance); err != nil {
		t.Fatalf("Write(prior) error = %v", err)
	}
	updated := []byte(`{"mcp":{"hive":{"type":"remote"}}}`)
	plan := reconcile.BuildPlan(reconcile.Inventory{Artifacts: []reconcile.Artifact{{Identity: openCodeGlobalConfigIdentity, Location: location, Bytes: prior, Provenance: &priorProvenance}}}, reconcile.DesiredState{
		Manifest:  reconcile.Manifest{Version: "v1", Artifacts: map[string]reconcile.ManifestEntry{openCodeGlobalConfigIdentity: {Location: location, Digest: managedOutputDigest(updated)}}},
		Artifacts: []reconcile.DesiredArtifact{{Identity: openCodeGlobalConfigIdentity, Location: location, Bytes: updated}},
	})
	if plan.Blocked() {
		t.Fatalf("BuildPlan() = %#v, want writable plan", plan)
	}
	_, err = reconcile.ApplyWithCompensation(&failAfterWriteStore{FileCompensationStore: store}, nil, plan)
	if err == nil {
		t.Fatal("ApplyWithCompensation() error = nil, want compensated Store failure")
	}
	restored, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("fresh NewFileCompensationStore() error = %v", err)
	}
	got, err := restored.Snapshot(location)
	if err != nil || string(got.Bytes) != string(prior) || got.Provenance != priorProvenance {
		t.Fatalf("fresh Snapshot() = %#v, %v; want compensated prior pair", got, err)
	}
}

func TestFileCompensationStoreRestoresPairWhenManifestDeleteFails(t *testing.T) {
	root := t.TempDir()
	location := openCodeGlobalConfigLocation
	content := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	provenance := reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: location, ManifestDigest: managedOutputDigest(content)}
	outputs := []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}}
	store, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	if err := store.Write(location, content, provenance); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	originalRemove := removeOpenCodePairFile
	t.Cleanup(func() { removeOpenCodePairFile = originalRemove })
	calls := 0
	removeOpenCodePairFile = func(path string) error {
		calls++
		if calls == 2 {
			return errors.New("remove token=fixture-secret")
		}
		return originalRemove(path)
	}
	if err := store.Delete(location); err == nil || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatalf("Delete() error = %v, want sanitized pair failure", err)
	}
	restored, err := NewFileCompensationStore(root, outputs)
	if err != nil {
		t.Fatalf("fresh NewFileCompensationStore() error = %v", err)
	}
	got, err := restored.Snapshot(location)
	if err != nil || string(got.Bytes) != string(content) || got.Provenance != provenance {
		t.Fatalf("fresh Snapshot() = %#v, %v; want restored pair", got, err)
	}
}

func TestFileCompensationStoreRestoresPriorPairAfterOpenCodePublishFailures(t *testing.T) {
	for _, tt := range []struct {
		name      string
		failPath  func(string, string) bool
		component string
	}{
		{name: "artifact", failPath: func(path, artifactPath string) bool { return path == artifactPath }, component: "artifact"},
		{name: "manifest", failPath: func(path, artifactPath string) bool { return path != artifactPath }, component: "manifest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, store, location, prior, priorProvenance := newOpenCodeStoreWithPriorPair(t)
			artifactPath := filepath.Join(root, filepath.FromSlash(location))
			originalWrite := writeOpenCodePairFile
			t.Cleanup(func() { writeOpenCodePairFile = originalWrite })
			failedPublish := false
			writeOpenCodePairFile = func(path string, content []byte) error {
				if !failedPublish && tt.failPath(path, artifactPath) && string(content) != string(prior) {
					failedPublish = true
					return errors.New("publish token=fixture-secret")
				}
				return originalWrite(path, content)
			}

			updated := []byte(`{"mcp":{"hive":{"type":"remote"}}}`)
			err := store.Write(location, updated, openCodeTestProvenance(location, updated))
			if err == nil || strings.Contains(err.Error(), "fixture-secret") || !strings.Contains(err.Error(), "write failed") {
				t.Fatalf("Write() error = %v, want sanitized %s publish failure", err, tt.component)
			}
			assertOpenCodePair(t, root, location, prior, priorProvenance)
			assertNoOpenCodePairTemps(t, root)
		})
	}
}

func TestFileCompensationStoreFailsClosedWhenOpenCodePairRestorationIsIncomplete(t *testing.T) {
	t.Run("restore write", func(t *testing.T) {
		root, store, location, prior, _ := newOpenCodeStoreWithPriorPair(t)
		artifactPath := filepath.Join(root, filepath.FromSlash(location))
		originalWrite := writeOpenCodePairFile
		t.Cleanup(func() { writeOpenCodePairFile = originalWrite })
		failedPublish := false
		writeOpenCodePairFile = func(path string, content []byte) error {
			if path != artifactPath && !failedPublish && string(content) != string(prior) {
				failedPublish = true
				return errors.New("publish token=fixture-secret")
			}
			if failedPublish && path == artifactPath && string(content) == string(prior) {
				return errors.New("restore token=fixture-secret")
			}
			return originalWrite(path, content)
		}

		updated := []byte(`{"mcp":{"hive":{"type":"remote"}}}`)
		err := store.Write(location, updated, openCodeTestProvenance(location, updated))
		if err == nil || strings.Contains(err.Error(), "fixture-secret") || !strings.Contains(err.Error(), "artifact") || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("Write() error = %v, want sanitized artifact restoration evidence", err)
		}
		assertNoOpenCodePairTemps(t, root)
	})

	t.Run("restore delete", func(t *testing.T) {
		root := t.TempDir()
		location := openCodeGlobalConfigLocation
		store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}})
		if err != nil {
			t.Fatalf("NewFileCompensationStore() error = %v", err)
		}
		originalWrite, originalRemove := writeOpenCodePairFile, removeOpenCodePairFile
		t.Cleanup(func() {
			writeOpenCodePairFile = originalWrite
			removeOpenCodePairFile = originalRemove
		})
		writeOpenCodePairFile = func(path string, content []byte) error {
			if strings.HasSuffix(path, "opencode-global-config.json") {
				return errors.New("publish token=fixture-secret")
			}
			return originalWrite(path, content)
		}
		removeOpenCodePairFile = func(path string) error {
			if strings.HasSuffix(path, "opencode.json") {
				return errors.New("restore token=fixture-secret")
			}
			return originalRemove(path)
		}

		content := []byte(`{"mcp":{"hive":{"type":"remote"}}}`)
		err = store.Write(location, content, openCodeTestProvenance(location, content))
		if err == nil || strings.Contains(err.Error(), "fixture-secret") || !strings.Contains(err.Error(), "artifact") || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("Write() error = %v, want sanitized artifact restoration evidence", err)
		}
		assertNoOpenCodePairTemps(t, root)
	})
}

func TestFileCompensationStoreFailsClosedWhenOpenCodePriorPairCannotBeRead(t *testing.T) {
	for _, component := range []string{"artifact", "manifest"} {
		t.Run(component, func(t *testing.T) {
			root, store, location, prior, _ := newOpenCodeStoreWithPriorPair(t)
			artifactPath := filepath.Join(root, filepath.FromSlash(location))
			originalRead := readOpenCodePairFile
			t.Cleanup(func() { readOpenCodePairFile = originalRead })
			readOpenCodePairFile = func(path string) ([]byte, bool, error) {
				if (component == "artifact") == (path == artifactPath) {
					return nil, false, errors.New("read token=fixture-secret")
				}
				return originalRead(path)
			}

			updated := []byte(`{"mcp":{"hive":{"type":"remote"}}}`)
			err := store.Write(location, updated, openCodeTestProvenance(location, updated))
			if err == nil || strings.Contains(err.Error(), "fixture-secret") || !strings.Contains(err.Error(), component) || !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("Write() error = %v, want sanitized %s read evidence", err, component)
			}
			readOpenCodePairFile = originalRead
			assertOpenCodePair(t, root, location, prior, openCodeTestProvenance(location, prior))
		})
	}
}

func newOpenCodeStoreWithPriorPair(t *testing.T) (string, *FileCompensationStore, string, []byte, reconcile.Provenance) {
	t.Helper()
	root := t.TempDir()
	location := openCodeGlobalConfigLocation
	prior := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	store, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}})
	if err != nil {
		t.Fatalf("NewFileCompensationStore() error = %v", err)
	}
	provenance := openCodeTestProvenance(location, prior)
	if err := store.Write(location, prior, provenance); err != nil {
		t.Fatalf("Write(prior) error = %v", err)
	}
	return root, store, location, prior, provenance
}

func openCodeTestProvenance(location string, content []byte) reconcile.Provenance {
	return reconcile.Provenance{Version: "v1", ManagedIdentity: openCodeGlobalConfigIdentity, Location: location, ManifestDigest: managedOutputDigest(content)}
}

func assertOpenCodePair(t *testing.T, root, location string, content []byte, provenance reconcile.Provenance) {
	t.Helper()
	fresh, err := NewFileCompensationStore(root, []RenderedManagedOutput{{Identity: openCodeGlobalConfigIdentity, Location: location}})
	if err != nil {
		t.Fatalf("fresh NewFileCompensationStore() error = %v", err)
	}
	snapshot, err := fresh.Snapshot(location)
	if err != nil || !snapshot.Exists || string(snapshot.Bytes) != string(content) || snapshot.Provenance != provenance {
		t.Fatalf("fresh Snapshot() = %#v, %v; want coherent prior pair", snapshot, err)
	}
}

func assertNoOpenCodePairTemps(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".jarvis-") {
			t.Fatalf("temporary pair file remained: %s", entry.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
}

type failAfterWriteStore struct {
	*FileCompensationStore
	failed bool
}

func (s *failAfterWriteStore) Write(location string, content []byte, provenance reconcile.Provenance) error {
	if err := s.FileCompensationStore.Write(location, content, provenance); err != nil {
		return err
	}
	if s.failed {
		return nil
	}
	s.failed = true
	return errors.New("write token=fixture-secret")
}

func restoredSnapshotBytes(t *testing.T, store *FileCompensationStore, location string) []byte {
	t.Helper()
	snapshot, err := store.Snapshot(location)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	return snapshot.Bytes
}

func TestProductionExecutorPersistsRecoveryEvidenceForFreshReload(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, "state", "recovery.json")
	executor := ProductionExecutor{reconcile: func(request ReconcileInstallRequest, _ NativeMCPReplacer) (ReconcileInstallResult, error) {
		evidence, err := reconcile.NewFileRecoveryEvidenceStore(request.EvidencePath)
		if err != nil {
			return ReconcileInstallResult{}, err
		}
		if err := evidence.PersistDegradedRecovery(reconcile.RecoveryEvidence{
			FailedTarget: "claude/token=super-secret",
		}); err != nil {
			return ReconcileInstallResult{}, err
		}
		return ReconcileInstallResult{}, errors.New("Store persistence token=super-secret failed")
	}}

	_, err := executor.Execute(ProductionReconcileInput{
		Root: root, EvidencePath: evidencePath,
		RenderedOutputs: []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed")}},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want degraded reconciliation failure")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), evidencePath) {
		t.Fatalf("Execute() error leaked evidence failure detail: %v", err)
	}

	fresh, loadErr := reconcile.NewFileRecoveryEvidenceStore(evidencePath)
	if loadErr != nil {
		t.Fatalf("fresh NewFileRecoveryEvidenceStore() error = %v", loadErr)
	}
	got, loadErr := fresh.LoadDegradedRecovery()
	if loadErr != nil {
		t.Fatalf("fresh LoadDegradedRecovery() error = %v", loadErr)
	}
	if got.FailedTarget != "claude/<redacted>" || got.RecoveryAction == "" {
		t.Fatalf("fresh evidence = %#v, want sanitized durable recovery evidence", got)
	}
}

func TestProductionExecutorSanitizesFilesystemAndNativeAdapterFailures(t *testing.T) {
	tests := []struct {
		name      string
		executor  ProductionExecutor
		input     ProductionReconcileInput
		forbidden []string
	}{
		{
			name: "filesystem persistence", executor: ProductionExecutor{reconcile: func(ReconcileInstallRequest, NativeMCPReplacer) (ReconcileInstallResult, error) {
				return ReconcileInstallResult{}, errors.New("write /private/config token=super-secret")
			}},
			forbidden: []string{"/private/config", "super-secret"},
		},
		{
			name: "native command", executor: ProductionExecutor{Native: &compositionNativeMCP{err: errors.New("command claude mcp add token=super-secret /private/config")}},
			input:     ProductionReconcileInput{SelectedAgents: []string{"claude"}, ClaudeMCPs: []NativeMCPDefinition{nativeMCPDefinition("hive", "desired-secret")}},
			forbidden: []string{"command claude", "/private/config", "super-secret", "desired-secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			input.Root = t.TempDir()
			input.EvidencePath = filepath.Join(input.Root, "state", "recovery.json")
			input.RenderedOutputs = []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed")}}

			_, err := tt.executor.Execute(input)
			if err == nil {
				t.Fatal("Execute() error = nil, want sanitized adapter failure")
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("Execute() error leaked %q: %v", forbidden, err)
				}
			}
		})
	}
}

func TestProductionExecutorSurfacesSafeStructuredNativeFailure(t *testing.T) {
	executor := ProductionExecutor{reconcile: func(ReconcileInstallRequest, NativeMCPReplacer) (ReconcileInstallResult, error) {
		return ReconcileInstallResult{Native: NativeMCPResult{
			Phase: NativeMCPInspected, TargetName: "hive", FixedLocation: "claude --scope user",
			ErrorCategory: "wrong-scope", ErrorCode: "project-config", Guidance: nativeMCPFixForwardGuidance,
		}}, errors.New("Scope: Project config token=super-secret C:\\Users\\teammate")
	}}
	root := t.TempDir()
	_, err := executor.Execute(ProductionReconcileInput{
		Root: root, EvidencePath: filepath.Join(root, "state", "recovery.json"),
		RenderedOutputs: []RenderedManagedOutput{{Identity: "jarvis-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("managed")}},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want structured native failure")
	}
	for _, want := range []string{"phase=inspected", "target=hive", "error=wrong-scope/project-config"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Execute() error = %v, want %q", err, want)
		}
	}
	for _, forbidden := range []string{"super-secret", `C:\Users`, "Scope: Project config"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("Execute() error leaked %q: %v", forbidden, err)
		}
	}
}

func TestProductionExecutorBuildsManagedStorePlanAndSkipsNativeWithoutClaude(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, "state", "recovery.json")
	input := ProductionReconcileInput{
		SelectedAgents: []string{"opencode"},
		Root:           root,
		EvidencePath:   evidencePath,
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: "opencode-mcp", Location: "opencode/opencode.json", Bytes: []byte(`{"mcp":{}}`),
		}},
	}

	native := &compositionNativeMCP{}
	result, err := (ProductionExecutor{Native: native}).Execute(input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Native.Phase != NativeMCPSkipped {
		t.Fatalf("native phase = %q, want %q", result.Native.Phase, NativeMCPSkipped)
	}
	if native.calls != 0 {
		t.Fatalf("native calls = %d, want deterministic no-agent skip", native.calls)
	}
	got, err := os.ReadFile(filepath.Join(root, "opencode", "opencode.json"))
	if err != nil || string(got) != `{"mcp":{}}` {
		t.Fatalf("managed OpenCode output = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Dir(evidencePath)); err != nil {
		t.Fatalf("evidence parent was not created: %v", err)
	}
}

func TestProductionPlanUsesClaudeUserScopeAndKeepsOpenCodeOutOfNativeManager(t *testing.T) {
	input := ProductionReconcileInput{
		SelectedAgents: []string{"claude", "opencode"},
		Root:           t.TempDir(),
		EvidencePath:   filepath.Join(t.TempDir(), "recovery.json"),
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: "opencode-mcp", Location: "opencode/opencode.json", Bytes: []byte(`{"mcp":{"hive":{}}}`),
		}},
		ClaudeMCPs: []NativeMCPDefinition{nativeMCPDefinition("hive", "secret")},
	}

	request, err := BuildProductionReconcileRequest(input)
	if err != nil {
		t.Fatalf("BuildProductionReconcileRequest() error = %v", err)
	}
	if len(request.DesiredMCPs) != 1 || request.DesiredMCPs[0].Scope != nativeMCPUserScope {
		t.Fatalf("native definitions = %#v, want one Claude user-scope definition", request.DesiredMCPs)
	}
	if len(request.StorePlan.Operations) != 1 || request.StorePlan.Operations[0].Location != "opencode/opencode.json" {
		t.Fatalf("Store plan = %#v, want independent OpenCode JSON output", request.StorePlan)
	}
}

func TestProductionExecutorPreservesUnprovenancedManagedLocation(t *testing.T) {
	root := t.TempDir()
	location := "claude/CLAUDE.md"
	path := filepath.Join(root, filepath.FromSlash(location))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("user-owned"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := (ProductionExecutor{}).Execute(ProductionReconcileInput{
		Root: root, EvidencePath: filepath.Join(root, "recovery.json"),
		RenderedOutputs: []RenderedManagedOutput{{
			Identity: "jarvis-instructions", Location: location, Bytes: []byte("managed"),
			Existing: &reconcile.Artifact{Identity: "jarvis-instructions", Location: location, Bytes: []byte("user-owned")},
		}},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want unprovenanced collision failure")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "user-owned" {
		t.Fatalf("user-owned file = %q, %v; want preserved bytes", got, readErr)
	}
}

func TestProductionPlanRejectsClaudeDefinitionsOutsideUserScope(t *testing.T) {
	_, err := BuildProductionReconcileRequest(ProductionReconcileInput{
		SelectedAgents: []string{"claude"}, Root: t.TempDir(), EvidencePath: filepath.Join(t.TempDir(), "recovery.json"),
		ClaudeMCPs: []NativeMCPDefinition{{Identity: "hive", Scope: "project", AddArgs: []string{"mcp", "add", "--scope", "project", "hive"}}},
	})
	if err == nil {
		t.Fatal("BuildProductionReconcileRequest() error = nil, want non-user Claude scope rejection")
	}
}
