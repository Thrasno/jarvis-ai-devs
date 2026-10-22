package sddbinding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestBindAdoptsUnboundChangeWithPreflightProvenance(t *testing.T) {
	for _, tt := range []struct {
		mode               sddruntime.StoreMode
		wantHiveAdoptions  int
		wantLocalAdoptions int
	}{
		{mode: sddruntime.StoreModeHive, wantHiveAdoptions: 1},
		{mode: sddruntime.StoreModeOpenSpec, wantLocalAdoptions: 1},
		{mode: sddruntime.StoreModeHybrid, wantHiveAdoptions: 1, wantLocalAdoptions: 1},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			hive := &fakeHiveBindingStore{}
			local := &fakeOpenSpecBindingStore{}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
			got, err := resolver.Bind(context.Background(), "project", "change", tt.mode)
			if err != nil {
				t.Fatal(err)
			}
			if got != (Resolution{Mode: tt.mode, Provenance: ProvenancePreflight, Persisted: true}) {
				t.Fatalf("resolution = %#v", got)
			}
			if len(hive.adopted) != tt.wantHiveAdoptions || len(local.adopted) != tt.wantLocalAdoptions {
				t.Fatalf("adoptions Hive=%d OpenSpec=%d", len(hive.adopted), len(local.adopted))
			}
		})
	}
}

func TestBindReplaysPersistedBindingWithTheSameMode(t *testing.T) {
	for _, tt := range []struct {
		name  string
		hive  *hiveclient.SDDStoreBinding
		local *Binding
		mode  sddruntime.StoreMode
	}{
		{name: "Hive from status default", hive: hiveBinding("hive", "initial:default:hive"), mode: sddruntime.StoreModeHive},
		{name: "OpenSpec preflight", local: bindingPointer(t, sddruntime.StoreModeOpenSpec, ProvenancePreflight), mode: sddruntime.StoreModeOpenSpec},
		{name: "hybrid preflight", hive: hiveBinding("hybrid", ProvenancePreflight), local: bindingPointer(t, sddruntime.StoreModeHybrid, ProvenancePreflight), mode: sddruntime.StoreModeHybrid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{binding: tt.hive, found: tt.hive != nil}
			local := &fakeOpenSpecBindingStore{binding: tt.local}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: &fakeLegacySource{}, OpenSpecSource: &fakeLegacySource{}, OpenSpecChangeDir: "change-dir"}
			got, err := resolver.Bind(context.Background(), "project", "change", tt.mode)
			if err != nil || got.Mode != tt.mode || !got.Persisted {
				t.Fatalf("resolution = %#v, error = %v", got, err)
			}
			if len(hive.adopted) != 0 || len(local.adopted) != 0 {
				t.Fatalf("replay adopted Hive=%d OpenSpec=%d", len(hive.adopted), len(local.adopted))
			}
		})
	}
}

func TestBindRejectsDifferentBindingWithoutWriting(t *testing.T) {
	for _, tt := range []struct {
		name           string
		hive           *hiveclient.SDDStoreBinding
		local          *Binding
		hiveProgress   sddstatus.LegacyProgressObservation
		openProgress   sddstatus.LegacyProgressObservation
		requested      sddruntime.StoreMode
		wantExisting   sddruntime.StoreMode
		wantProvenance string
	}{
		{name: "persisted Hive", hive: hiveBinding("hive", "initial:default:hive"), requested: sddruntime.StoreModeHybrid, wantExisting: sddruntime.StoreModeHive, wantProvenance: "initial:default:hive"},
		{name: "persisted OpenSpec", local: bindingPointer(t, sddruntime.StoreModeOpenSpec, "selected"), requested: sddruntime.StoreModeHive, wantExisting: sddruntime.StoreModeOpenSpec, wantProvenance: "selected"},
		{name: "persisted hybrid", hive: hiveBinding("hybrid", "selected"), local: bindingPointer(t, sddruntime.StoreModeHybrid, "selected"), requested: sddruntime.StoreModeOpenSpec, wantExisting: sddruntime.StoreModeHybrid, wantProvenance: "selected"},
		{name: "hybrid with missing mirror", hive: hiveBinding("hybrid", "selected"), requested: sddruntime.StoreModeHive, wantExisting: sddruntime.StoreModeHybrid, wantProvenance: "selected"},
		{name: "Hive progress infers Hive", hiveProgress: legacyProgress("hive"), requested: sddruntime.StoreModeHybrid, wantExisting: sddruntime.StoreModeHive, wantProvenance: ProvenanceLegacyHiveProgress},
		{name: "OpenSpec progress infers OpenSpec", openProgress: legacyProgress("openspec"), requested: sddruntime.StoreModeHive, wantExisting: sddruntime.StoreModeOpenSpec, wantProvenance: ProvenanceLegacyOpenSpecProgress},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{binding: tt.hive, found: tt.hive != nil}
			local := &fakeOpenSpecBindingStore{binding: tt.local}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(tt.hiveProgress), OpenSpecSource: observationSource(tt.openProgress), OpenSpecChangeDir: "change-dir"}
			_, err := resolver.Bind(context.Background(), "project", "change", tt.requested)
			var conflict *ConflictError
			if !errors.As(err, &conflict) || !errors.Is(err, ErrBindingConflict) {
				t.Fatalf("error = %v, want ConflictError", err)
			}
			if conflict.Existing.Mode() != tt.wantExisting || conflict.Existing.Provenance() != tt.wantProvenance || conflict.Requested.Mode() != tt.requested || conflict.Requested.Provenance() != ProvenancePreflight {
				t.Fatalf("conflict = existing %s/%q requested %s/%q", conflict.Existing.Mode(), conflict.Existing.Provenance(), conflict.Requested.Mode(), conflict.Requested.Provenance())
			}
			if len(hive.adopted) != 0 || len(local.adopted) != 0 {
				t.Fatalf("conflict adopted Hive=%d OpenSpec=%d", len(hive.adopted), len(local.adopted))
			}
		})
	}
}

func TestBindKeepsLegacyInferenceThatMatchesTheRequestedMode(t *testing.T) {
	hive := &fakeHiveBindingStore{}
	local := &fakeOpenSpecBindingStore{}
	resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(legacyProgress("hive")), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
	got, err := resolver.Bind(context.Background(), "project", "change", sddruntime.StoreModeHive)
	if err != nil || got != (Resolution{Mode: sddruntime.StoreModeHive, Provenance: ProvenanceLegacyHiveProgress, Persisted: true}) {
		t.Fatalf("resolution = %#v, error = %v", got, err)
	}
	if len(hive.adopted) != 1 || len(local.adopted) != 0 {
		t.Fatalf("adoptions Hive=%d OpenSpec=%d", len(hive.adopted), len(local.adopted))
	}
}

func TestBindRejectsUnpersistableModeBeforeReads(t *testing.T) {
	for _, mode := range []sddruntime.StoreMode{sddruntime.StoreModeNone, "memory", ""} {
		t.Run(string(mode), func(t *testing.T) {
			hive := &fakeHiveBindingStore{}
			local := &fakeOpenSpecBindingStore{}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: &fakeLegacySource{}, OpenSpecSource: &fakeLegacySource{}, OpenSpecChangeDir: "change-dir"}
			if _, err := resolver.Bind(context.Background(), "project", "change", mode); !errors.Is(err, ErrInvalidBinding) {
				t.Fatalf("error = %v, want ErrInvalidBinding", err)
			}
			if hive.gets != 0 || local.reads != 0 || len(hive.adopted) != 0 || len(local.adopted) != 0 {
				t.Fatalf("invalid mode touched stores: gets=%d reads=%d", hive.gets, local.reads)
			}
		})
	}
}

func TestBindCreatesOpenSpecChangeDirectoryOnlyForFileBackedModes(t *testing.T) {
	for _, tt := range []struct {
		mode    sddruntime.StoreMode
		wantDir bool
	}{
		{mode: sddruntime.StoreModeHive},
		{mode: sddruntime.StoreModeOpenSpec, wantDir: true},
		{mode: sddruntime.StoreModeHybrid, wantDir: true},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			root := t.TempDir()
			changeDir := filepath.Join(root, "openspec", "changes", "change")
			resolver := LegacyResolver{
				HiveBindings:      &fakeHiveBindingStore{},
				HiveSource:        observationSource(sddstatus.LegacyProgressObservation{}),
				OpenSpecSource:    observationSource(sddstatus.LegacyProgressObservation{}),
				OpenSpecChangeDir: changeDir,
				ResolutionLockDir: root,
			}
			if _, err := resolver.Bind(context.Background(), "project", "change", tt.mode); err != nil {
				t.Fatal(err)
			}
			persisted, err := ReadOpenSpec(changeDir)
			if !tt.wantDir {
				if _, statErr := os.Stat(filepath.Join(root, "openspec")); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("hive bind created openspec/: %v", statErr)
				}
				return
			}
			if err != nil || persisted == nil || persisted.Mode() != tt.mode || persisted.Provenance() != ProvenancePreflight {
				t.Fatalf("persisted = %#v, error = %v", persisted, err)
			}
		})
	}
}

func TestBindDoesNotCreateOpenSpecDirectoryOnConflict(t *testing.T) {
	root := t.TempDir()
	resolver := LegacyResolver{
		HiveBindings:      &fakeHiveBindingStore{binding: hiveBinding("hive", "initial:default:hive"), found: true},
		HiveSource:        observationSource(sddstatus.LegacyProgressObservation{}),
		OpenSpecSource:    observationSource(sddstatus.LegacyProgressObservation{}),
		OpenSpecChangeDir: filepath.Join(root, "openspec", "changes", "change"),
		ResolutionLockDir: root,
	}
	if _, err := resolver.Bind(context.Background(), "project", "change", sddruntime.StoreModeHybrid); !errors.Is(err, ErrBindingConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, "openspec")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting bind created openspec/: %v", err)
	}
}

func TestBindRejectsChangeDirectoryOutsideTheResolutionWorkspace(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(other, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, tt := range []struct{ name, changeDir string }{
		{name: "another workspace", changeDir: filepath.Join(other, "openspec", "changes", "change")},
		{name: "wrong change", changeDir: filepath.Join(root, "openspec", "changes", "other")},
		{name: "symlinked workspace", changeDir: filepath.Join(alias, "openspec", "changes", "change")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{}
			resolver := LegacyResolver{HiveBindings: hive, HiveSource: &fakeLegacySource{}, OpenSpecSource: &fakeLegacySource{}, OpenSpecChangeDir: tt.changeDir, ResolutionLockDir: root}
			if _, err := resolver.Bind(context.Background(), "project", "change", sddruntime.StoreModeOpenSpec); !errors.Is(err, ErrInvalidBinding) {
				t.Fatalf("error = %v, want ErrInvalidBinding", err)
			}
			if hive.gets != 0 {
				t.Fatalf("invalid coordinates reached Hive")
			}
		})
	}
}

func TestBindLeavesWorkspaceUntouchedWhenHiveAdoptionFails(t *testing.T) {
	for _, tt := range []struct {
		name     string
		existing []string
	}{
		{name: "no openspec directory"},
		{name: "existing openspec directory", existing: []string{"openspec"}},
		{name: "existing changes directory", existing: []string{"openspec", "openspec/changes", "openspec/changes/other"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range tt.existing {
				if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			before := workspaceTree(t, root)
			adoptFailure := errors.New("daemon rejected adoption")
			resolver := LegacyResolver{
				HiveBindings:      &fakeHiveBindingStore{adoptErr: adoptFailure},
				HiveSource:        observationSource(sddstatus.LegacyProgressObservation{}),
				OpenSpecSource:    observationSource(sddstatus.LegacyProgressObservation{}),
				OpenSpecChangeDir: filepath.Join(root, "openspec", "changes", "change"),
				ResolutionLockDir: root,
			}
			if _, err := resolver.Bind(context.Background(), "project", "change", sddruntime.StoreModeHybrid); !errors.Is(err, adoptFailure) {
				t.Fatalf("error = %v, want the Hive adoption failure", err)
			}
			if after := workspaceTree(t, root); after != before {
				t.Fatalf("failed bind changed the workspace:\n%s\nwant:\n%s", after, before)
			}
		})
	}
}

func TestBindRemovesOnlyCreatedDirectoriesWhenOpenSpecAdoptionFails(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := workspaceTree(t, root)
	localFailure := errors.New("local commit failed")
	resolver := LegacyResolver{
		HiveBindings:      &fakeHiveBindingStore{},
		HiveSource:        observationSource(sddstatus.LegacyProgressObservation{}),
		OpenSpecSource:    observationSource(sddstatus.LegacyProgressObservation{}),
		OpenSpecChangeDir: filepath.Join(root, "openspec", "changes", "change"),
		ResolutionLockDir: root,
	}
	original := bindAdoptOpenSpec
	bindAdoptOpenSpec = func(OpenSpecBindingStore, string, Binding) (Binding, bool, error) {
		return Binding{}, false, localFailure
	}
	t.Cleanup(func() { bindAdoptOpenSpec = original })

	if _, err := resolver.Bind(context.Background(), "project", "change", sddruntime.StoreModeOpenSpec); !errors.Is(err, localFailure) {
		t.Fatalf("error = %v, want the OpenSpec adoption failure", err)
	}
	if after := workspaceTree(t, root); after != before {
		t.Fatalf("failed bind changed the workspace:\n%s\nwant:\n%s", after, before)
	}
}

// racingHiveBindingStore models another workspace adopting the same change
// between this resolver's read and its adoption.
type racingHiveBindingStore struct {
	competing hiveclient.SDDStoreBinding
	won       bool
	adopted   []hiveclient.SDDStoreBindingRequest
}

func (f *racingHiveBindingStore) GetSDDStoreBinding(context.Context, string, string) (hiveclient.SDDStoreBinding, bool, error) {
	if !f.won {
		return hiveclient.SDDStoreBinding{}, false, nil
	}
	return f.competing, true, nil
}

func (f *racingHiveBindingStore) AdoptSDDStoreBinding(_ context.Context, project, change string, request hiveclient.SDDStoreBindingRequest) (hiveclient.SDDStoreBinding, bool, error) {
	f.adopted = append(f.adopted, request)
	if !f.won {
		f.won = true
		return hiveclient.SDDStoreBinding{}, false, &hiveclient.SDDStoreBindingConflictError{Existing: f.competing, Requested: hiveclient.SDDStoreBinding{Project: project, Change: change, SchemaVersion: "1", Mode: request.Mode, Provenance: request.Provenance}}
	}
	return f.competing, false, nil
}

func TestBindResolvesAgainstTheBindingThatWonARace(t *testing.T) {
	for _, tt := range []struct {
		name           string
		competing      *hiveclient.SDDStoreBinding
		requested      sddruntime.StoreMode
		wantConflict   bool
		wantProvenance string
		wantHive       int
		wantLocal      int
	}{
		{name: "different mode", competing: hiveBinding("hive", "initial:default:hive"), requested: sddruntime.StoreModeHybrid, wantConflict: true, wantHive: 1},
		{name: "same Hive mode", competing: hiveBinding("hive", "initial:default:hive"), requested: sddruntime.StoreModeHive, wantProvenance: "initial:default:hive", wantHive: 1},
		// The mirror repair replays the winner's Hive copy, as ResolveAndAdopt does.
		{name: "same hybrid mode repairs the local mirror", competing: hiveBinding("hybrid", "initial:environment:JARVIS_SDD_STORE_MODE"), requested: sddruntime.StoreModeHybrid, wantProvenance: "initial:environment:JARVIS_SDD_STORE_MODE", wantHive: 2, wantLocal: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &racingHiveBindingStore{competing: *tt.competing}
			local := &fakeOpenSpecBindingStore{}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
			got, err := resolver.Bind(context.Background(), "project", "change", tt.requested)
			if tt.wantConflict {
				var conflict *ConflictError
				if !errors.As(err, &conflict) || conflict.Existing.Mode() != sddruntime.StoreMode(tt.competing.Mode) || conflict.Existing.Provenance() != tt.competing.Provenance {
					t.Fatalf("error = %v, want ConflictError naming the winning binding", err)
				}
			} else if err != nil || got != (Resolution{Mode: tt.requested, Provenance: tt.wantProvenance, Persisted: true}) {
				t.Fatalf("resolution = %#v, error = %v", got, err)
			}
			if len(hive.adopted) != tt.wantHive || len(local.adopted) != tt.wantLocal {
				t.Fatalf("adoptions Hive=%d OpenSpec=%d, want %d/%d", len(hive.adopted), len(local.adopted), tt.wantHive, tt.wantLocal)
			}
		})
	}
}

// workspaceTree lists every path under root with its type, so a test can
// assert a failed bind left the workspace exactly as it was.
func workspaceTree(t *testing.T, root string) string {
	t.Helper()
	var tree []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		tree = append(tree, rel+" "+entry.Type().String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(tree, "\n")
}
