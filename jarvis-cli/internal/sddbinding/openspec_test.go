package sddbinding

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"gopkg.in/yaml.v3"
)

func TestAdoptOpenSpecCreatesAndPreservesState(t *testing.T) {
	want := mustBinding(t, sddruntime.StoreModeOpenSpec, "initial selection")

	for _, tt := range []struct {
		name     string
		state    string
		wantMode sddruntime.StoreMode
		wantKeep string
	}{
		{
			name:     "absent state",
			wantMode: sddruntime.StoreModeOpenSpec,
		},
		{
			name: "existing unrelated mapping",
			state: `# existing state
phase:
  name: apply
  nested:
    keep: true
list:
  - unchanged
`,
			wantMode: sddruntime.StoreModeOpenSpec,
			wantKeep: "unchanged",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changeDir := t.TempDir()
			if tt.state != "" {
				writeState(t, changeDir, tt.state)
			}

			got, created, err := AdoptOpenSpec(changeDir, want)
			if err != nil {
				t.Fatalf("AdoptOpenSpec() error = %v", err)
			}
			if !created || got != want {
				t.Fatalf("AdoptOpenSpec() = %#v, created=%t; want %#v, true", got, created, want)
			}

			persisted, err := ReadOpenSpec(changeDir)
			if err != nil || persisted == nil || *persisted != want {
				t.Fatalf("ReadOpenSpec() = %#v, %v; want %#v, nil", persisted, err, want)
			}
			data := readState(t, changeDir)
			if !bytes.Contains(data, []byte("schema_version: 1")) {
				t.Fatalf("state omitted binding schema version: %q", data)
			}
			if tt.state != "" && !bytes.Contains(data, []byte("# existing state")) {
				t.Fatalf("state lost existing comment: %q", data)
			}
			var decoded map[string]any
			if err := yaml.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if gotPhase, ok := decoded["phase"].(map[string]any); tt.wantKeep != "" && (!ok || gotPhase["nested"].(map[string]any)["keep"] != true) {
				t.Fatalf("unrelated nested mapping was not preserved: %#v", decoded)
			}
			if tt.wantKeep != "" && decoded["list"].([]any)[0] != tt.wantKeep {
				t.Fatalf("unrelated list was not preserved: %#v", decoded)
			}
		})
	}
}

func TestAdoptOpenSpecExactReplayDoesNotRewriteBytes(t *testing.T) {
	changeDir := t.TempDir()
	want := mustBinding(t, sddruntime.StoreModeHybrid, "operator selected hybrid")
	writeState(t, changeDir, `# preserve this exact layout
other: {value: 1}
jarvis_sdd_binding:
  schema_version: 1
  mode: hybrid
  provenance: operator selected hybrid
`)
	before := readState(t, changeDir)

	got, created, err := AdoptOpenSpec(changeDir, want)
	if err != nil {
		t.Fatalf("AdoptOpenSpec() error = %v", err)
	}
	if created || got != want {
		t.Fatalf("AdoptOpenSpec() = %#v, created=%t; want %#v, false", got, created, want)
	}
	if after := readState(t, changeDir); !bytes.Equal(after, before) {
		t.Fatalf("exact replay rewrote state:\n got %q\nwant %q", after, before)
	}
}

func TestAdoptOpenSpecConflictsLeaveBytesUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name     string
		existing Binding
		incoming Binding
	}{
		{
			name:     "different mode",
			existing: mustBinding(t, sddruntime.StoreModeOpenSpec, "initial selection"),
			incoming: mustBinding(t, sddruntime.StoreModeHive, "initial selection"),
		},
		{
			name:     "different provenance",
			existing: mustBinding(t, sddruntime.StoreModeOpenSpec, "initial selection"),
			incoming: mustBinding(t, sddruntime.StoreModeOpenSpec, "administrative override"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changeDir := t.TempDir()
			writeState(t, changeDir, stateWithBinding(tt.existing))
			before := snapshotDirectory(t, changeDir)

			_, _, err := AdoptOpenSpec(changeDir, tt.incoming)
			if !errors.Is(err, ErrBindingConflict) {
				t.Fatalf("AdoptOpenSpec() error = %v, want binding conflict", err)
			}
			var conflict *ConflictError
			if !errors.As(err, &conflict) || conflict.Existing != tt.existing || conflict.Requested != tt.incoming {
				t.Fatalf("AdoptOpenSpec() conflict = %#v, want existing/requested bindings", conflict)
			}
			assertDirectoryUnchanged(t, changeDir, before)
		})
	}
}

func TestNewRejectsUnpersistableBindings(t *testing.T) {
	for _, tt := range []struct {
		name       string
		mode       sddruntime.StoreMode
		provenance string
	}{
		{name: "none", mode: sddruntime.StoreModeNone, provenance: "selection"},
		{name: "empty mode", provenance: "selection"},
		{name: "invalid mode", mode: "remote", provenance: "selection"},
		{name: "empty provenance", mode: sddruntime.StoreModeHive},
		{name: "blank provenance", mode: sddruntime.StoreModeHive, provenance: " \t "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.mode, tt.provenance); !errors.Is(err, ErrInvalidBinding) {
				t.Fatalf("New(%q, %q) error = %v, want invalid binding", tt.mode, tt.provenance, err)
			}
		})
	}
}

func TestAdoptOpenSpecFailsClosedForInvalidExistingState(t *testing.T) {
	want := mustBinding(t, sddruntime.StoreModeOpenSpec, "initial selection")
	for _, tt := range []struct {
		name  string
		state string
	}{
		{name: "malformed yaml", state: "key: [unterminated\n"},
		{name: "non mapping root", state: "- item\n"},
		{name: "binding scalar", state: "jarvis_sdd_binding: invalid\n"},
		{name: "binding missing version", state: "jarvis_sdd_binding:\n  mode: openspec\n  provenance: initial selection\n"},
		{name: "binding unknown field", state: "jarvis_sdd_binding:\n  schema_version: 1\n  mode: openspec\n  provenance: initial selection\n  extra: nope\n"},
		{name: "binding invalid mode", state: "jarvis_sdd_binding:\n  schema_version: 1\n  mode: none\n  provenance: initial selection\n"},
		{name: "binding empty provenance", state: "jarvis_sdd_binding:\n  schema_version: 1\n  mode: openspec\n  provenance: ''\n"},
		{name: "unsupported schema", state: "jarvis_sdd_binding:\n  schema_version: 2\n  mode: openspec\n  provenance: initial selection\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changeDir := t.TempDir()
			writeState(t, changeDir, tt.state)
			before := snapshotDirectory(t, changeDir)

			_, _, err := AdoptOpenSpec(changeDir, want)
			if err == nil {
				t.Fatal("AdoptOpenSpec() accepted invalid existing state")
			}
			assertDirectoryUnchanged(t, changeDir, before)
		})
	}
}

func TestReadOpenSpecTreatsMissingBindingAsUnboundAndRejectsUnsafePath(t *testing.T) {
	changeDir := t.TempDir()
	binding, err := ReadOpenSpec(changeDir)
	if err != nil || binding != nil {
		t.Fatalf("ReadOpenSpec(missing state) = %#v, %v; want nil, nil", binding, err)
	}
	writeState(t, changeDir, "unrelated: value\n")
	binding, err = ReadOpenSpec(changeDir)
	if err != nil || binding != nil {
		t.Fatalf("ReadOpenSpec(missing binding) = %#v, %v; want nil, nil", binding, err)
	}

	target := filepath.Join(t.TempDir(), "target.yaml")
	if err := os.WriteFile(target, []byte("unrelated: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(changeDir, "state.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(changeDir, "state.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOpenSpec(changeDir); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("ReadOpenSpec(symlink) error = %v, want unsafe path", err)
	}
}

func mustBinding(t *testing.T, mode sddruntime.StoreMode, provenance string) Binding {
	t.Helper()
	binding, err := New(mode, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func writeState(t *testing.T, changeDir, state string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(changeDir, "state.yaml"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readState(t *testing.T, changeDir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(changeDir, "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stateWithBinding(binding Binding) string {
	return "unrelated:\n  nested: preserved\njarvis_sdd_binding:\n  schema_version: 1\n  mode: " + string(binding.Mode()) + "\n  provenance: " + binding.Provenance() + "\n"
}

func TestAdoptOpenSpecRejectsYAMLAmbiguityWithoutMutation(t *testing.T) {
	want := mustBinding(t, sddruntime.StoreModeOpenSpec, "initial selection")
	for _, tt := range []struct{ name, state string }{
		{"inherited binding", "defaults: &defaults\n  jarvis_sdd_binding:\n    schema_version: 1\n    mode: hive\n    provenance: inherited\n<<: *defaults\n"},
		{"duplicate authority", "jarvis_sdd_binding:\n  schema_version: 1\n  mode: hive\n  provenance: first\njarvis_sdd_binding:\n  schema_version: 1\n  mode: openspec\n  provenance: second\n"},
		{"duplicate unrelated key", "unrelated:\n  value: first\n  value: second\n"},
		{"equivalent numeric keys", "1: first\n0x1: second\n"},
		{"equivalent signed zero keys", "0.0: first\n-0.0: second\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changeDir := t.TempDir()
			writeState(t, changeDir, tt.state)
			before := snapshotDirectory(t, changeDir)
			if _, _, err := AdoptOpenSpec(changeDir, want); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("AdoptOpenSpec() error = %v, want invalid state", err)
			}
			assertDirectoryUnchanged(t, changeDir, before)
		})
	}
}

func TestAdoptOpenSpecExactReplayLeavesDirectoryUnchanged(t *testing.T) {
	changeDir := t.TempDir()
	want := mustBinding(t, sddruntime.StoreModeHive, "selected")
	writeState(t, changeDir, stateWithBinding(want))
	before := snapshotDirectory(t, changeDir)
	if _, created, err := AdoptOpenSpec(changeDir, want); err != nil || created {
		t.Fatalf("AdoptOpenSpec() = created=%t, err=%v; want false, nil", created, err)
	}
	assertDirectoryUnchanged(t, changeDir, before)
}

func TestAdoptOpenSpecPreservesExistingPermissionAndUsesPrivateDefault(t *testing.T) {
	want := mustBinding(t, sddruntime.StoreModeHive, "selected")
	for _, tt := range []struct {
		name  string
		state string
		mode  os.FileMode
		want  os.FileMode
	}{
		{"existing writable state", "unrelated: value\n", 0o640, 0o640},
		{"existing read-only state", "unrelated: value\n", 0o444, 0o444},
		{"new writable state", "", 0, 0o600},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changeDir := t.TempDir()
			if tt.state != "" {
				writeState(t, changeDir, tt.state)
				if err := os.Chmod(filepath.Join(changeDir, stateFileName), tt.mode); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := AdoptOpenSpec(changeDir, want); err != nil {
				t.Fatal(err)
			}
			assertStatePermission(t, filepath.Join(changeDir, stateFileName), tt.want)
		})
	}
}

func TestAdoptOpenSpecConcurrentRequestsCreateOneBinding(t *testing.T) {
	changeDir := t.TempDir()
	want := mustBinding(t, sddruntime.StoreModeHybrid, "concurrent selection")
	const callers = 12
	start := make(chan struct{})
	results := make(chan struct {
		created bool
		err     error
	}, callers)
	var group sync.WaitGroup
	for range callers {
		group.Go(func() {
			<-start
			_, created, err := AdoptOpenSpec(changeDir, want)
			results <- struct {
				created bool
				err     error
			}{created, err}
		})
	}
	close(start)
	group.Wait()
	close(results)
	created := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent AdoptOpenSpec() error = %v", result.err)
		}
		if result.created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created = %d, want exactly one creator", created)
	}
	binding, err := ReadOpenSpec(changeDir)
	if err != nil || binding == nil || *binding != want {
		t.Fatalf("ReadOpenSpec() = %#v, %v; want %#v, nil", binding, err, want)
	}
}

func TestAdoptOpenSpecSucceedsAfterCommitDurabilityFailure(t *testing.T) {
	oldSync := syncChangeRoot
	syncChangeRoot = func(int) error { return errors.New("directory sync interrupted") }
	t.Cleanup(func() { syncChangeRoot = oldSync })
	changeDir := t.TempDir()
	want := mustBinding(t, sddruntime.StoreModeOpenSpec, "selected")
	got, created, err := AdoptOpenSpec(changeDir, want)
	if err != nil || !created || got != want {
		t.Fatalf("AdoptOpenSpec() = %#v, %t, %v; want %#v, true, nil", got, created, err, want)
	}
	if binding, err := ReadOpenSpec(changeDir); err != nil || binding == nil || *binding != want {
		t.Fatalf("committed binding = %#v, %v; want %#v, nil", binding, err, want)
	}
}

func snapshotDirectory(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot[rel] = "directory|" + info.Mode().String()
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[rel] = string(data) + "|" + info.Mode().String()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertDirectoryUnchanged(t *testing.T, root string, want map[string]string) {
	t.Helper()
	if got := snapshotDirectory(t, root); !reflect.DeepEqual(got, want) {
		t.Fatalf("change directory mutated:\n got %#v\nwant %#v", got, want)
	}
}
