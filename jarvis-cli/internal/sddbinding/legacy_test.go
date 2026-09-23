package sddbinding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestResolveExistingReadsOnlyPersistedCopies(t *testing.T) {
	unavailable := errors.New("store unavailable")
	for _, tt := range []struct {
		name    string
		hive    *hiveclient.SDDStoreBinding
		local   *Binding
		getErr  error
		readErr error
		want    error
		mode    sddruntime.StoreMode
	}{
		{name: "absent", want: ErrUnsupportedStoredBinding},
		{name: "Hive", hive: hiveBinding("hive", "selected"), mode: sddruntime.StoreModeHive},
		{name: "OpenSpec", local: bindingPointer(t, sddruntime.StoreModeOpenSpec, "selected"), mode: sddruntime.StoreModeOpenSpec},
		{name: "hybrid", hive: hiveBinding("hybrid", "selected"), local: bindingPointer(t, sddruntime.StoreModeHybrid, "selected"), mode: sddruntime.StoreModeHybrid},
		{name: "missing local hybrid", hive: hiveBinding("hybrid", "selected"), want: ErrBindingCopiesDiverged},
		{name: "missing Hive hybrid", local: bindingPointer(t, sddruntime.StoreModeHybrid, "selected"), want: ErrBindingCopiesDiverged},
		{name: "diverged", hive: hiveBinding("hybrid", "one"), local: bindingPointer(t, sddruntime.StoreModeHybrid, "two"), want: ErrBindingCopiesDiverged},
		{name: "malformed Hive", hive: hiveBinding("hive", " "), want: ErrUnsupportedStoredBinding},
		{name: "invalid local mode", local: bindingPointer(t, sddruntime.StoreModeHive, "selected"), want: ErrUnsupportedStoredBinding},
		{name: "Hive unavailable", getErr: unavailable, want: unavailable},
		{name: "OpenSpec unavailable", readErr: unavailable, want: unavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{binding: tt.hive, found: tt.hive != nil, getErr: tt.getErr}
			local := &fakeOpenSpecBindingStore{binding: tt.local, readErr: tt.readErr}
			hs, ls := &fakeLegacySource{}, &fakeLegacySource{}
			r := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: hs, OpenSpecSource: ls, OpenSpecChangeDir: filepath.Join(t.TempDir(), "openspec", "changes", "change")}
			got, err := r.ResolveExisting(context.Background(), "project", "change")
			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("error = %v, want %v", err, tt.want)
				}
			} else if err != nil || got.Mode != tt.mode || got.Provenance != "selected" || !got.Persisted {
				t.Fatalf("resolution = %#v, error = %v", got, err)
			}
			if len(hive.adopted) != 0 || len(local.adopted) != 0 || hs.calls != 0 || ls.calls != 0 {
				t.Fatalf("read-only resolver mutated or inspected progress")
			}
		})
	}
}

func TestResolveExistingRejectsUnboundPathsBeforeReads(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, tt := range []struct{ name, path string }{
		{"wrong change", filepath.Join(root, "openspec", "changes", "other")},
		{"wrong location", filepath.Join(root, "other", "changes", "change")},
		{"relative", filepath.Join("openspec", "changes", "change")},
		{"unclean", filepath.Join(root, "openspec", "changes") + "/../changes/change"},
		{"symlink ancestor", filepath.Join(alias, "openspec", "changes", "change")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{}
			local := &fakeOpenSpecBindingStore{}
			r := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, OpenSpecChangeDir: tt.path}
			_, err := r.ResolveExisting(context.Background(), "project", "change")
			if !errors.Is(err, ErrInvalidBinding) || hive.gets != 0 || local.reads != 0 {
				t.Fatalf("error = %v; Hive reads = %d; local reads = %d", err, hive.gets, local.reads)
			}
		})
	}
}

func TestLegacyBindingCanceledContextAvoidsReadsAndAdoptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hive := &fakeHiveBindingStore{}
	local := &fakeOpenSpecBindingStore{}
	hiveSource := &fakeLegacySource{}
	openSpecSource := &fakeLegacySource{}
	resolver := LegacyResolver{
		HiveBindings:      hive,
		OpenSpecBindings:  local,
		HiveSource:        hiveSource,
		OpenSpecSource:    openSpecSource,
		OpenSpecChangeDir: "change-dir",
	}

	_, err := resolver.ResolveAndAdopt(ctx, "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveAndAdopt() error = %v, want context.Canceled", err)
	}
	if hive.gets != 0 || local.reads != 0 || len(hive.adopted) != 0 || len(local.adopted) != 0 || hiveSource.calls != 0 || openSpecSource.calls != 0 {
		t.Fatalf("canceled resolution performed effects: Hive gets=%d adopts=%d, OpenSpec reads=%d adopts=%d, sources=%d/%d", hive.gets, len(hive.adopted), local.reads, len(local.adopted), hiveSource.calls, openSpecSource.calls)
	}
}

func TestLegacyBindingCancellationWhileResolutionLockContendedHasNoLateEffects(t *testing.T) {
	resolutionDir := t.TempDir()
	changeDir := filepath.Join(resolutionDir, "openspec", "changes", "change")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	holder, err := openChangeRoot(resolutionDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := holder.Close(); err != nil {
			t.Errorf("close resolution lock holder: %v", err)
		}
	})

	holderAcquired := make(chan struct{})
	holderErr := make(chan error, 1)
	releaseHolder := make(chan struct{})
	holderReleased := make(chan struct{})
	go func() {
		unlock, err := holder.lock()
		if err != nil {
			holderErr <- err
			return
		}
		close(holderAcquired)
		<-releaseHolder
		unlock()
		close(holderReleased)
	}()
	select {
	case <-holderAcquired:
	case err := <-holderErr:
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	releaseAndWait := func() {
		releaseOnce.Do(func() {
			close(releaseHolder)
			<-holderReleased
		})
	}
	t.Cleanup(releaseAndWait)

	hive := &fakeHiveBindingStore{}
	hiveSource := observationSource(sddstatus.LegacyProgressObservation{})
	openSpecSource := observationSource(sddstatus.LegacyProgressObservation{})
	resolver := LegacyResolver{
		HiveBindings:      hive,
		HiveSource:        hiveSource,
		OpenSpecSource:    openSpecSource,
		OpenSpecChangeDir: changeDir,
		ResolutionLockDir: resolutionDir,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err = resolver.ResolveAndAdopt(ctx, "project", "change", InitialSelection{Mode: sddruntime.StoreModeOpenSpec, Provenance: "initial"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ResolveAndAdopt() error = %v, want context.DeadlineExceeded", err)
	}
	assertNoContentionEffects(t, hive, hiveSource, openSpecSource, changeDir)

	releaseAndWait()
	assertNoContentionEffects(t, hive, hiveSource, openSpecSource, changeDir)
}

func assertNoContentionEffects(t *testing.T, hive *fakeHiveBindingStore, hiveSource, openSpecSource *fakeLegacySource, changeDir string) {
	t.Helper()
	if hive.gets != 0 || len(hive.adopted) != 0 || hiveSource.calls != 0 || openSpecSource.calls != 0 {
		t.Fatalf("deadline resolution performed effects: Hive gets=%d adopts=%d, sources=%d/%d", hive.gets, len(hive.adopted), hiveSource.calls, openSpecSource.calls)
	}
	if _, err := os.Stat(filepath.Join(changeDir, stateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deadline resolution wrote state.yaml: %v", err)
	}
}

func TestLegacyBindingUsesValidPersistedCopiesWithoutInspection(t *testing.T) {
	for _, tt := range []struct {
		name       string
		hive       *hiveclient.SDDStoreBinding
		openspec   *Binding
		wantMode   sddruntime.StoreMode
		provenance string
	}{
		{name: "Hive copy", hive: hiveBinding("hive", "selected:hive"), wantMode: sddruntime.StoreModeHive, provenance: "selected:hive"},
		{name: "OpenSpec copy", openspec: bindingPointer(t, sddruntime.StoreModeOpenSpec, "selected:openspec"), wantMode: sddruntime.StoreModeOpenSpec, provenance: "selected:openspec"},
		{name: "matching hybrid copies", hive: hiveBinding("hybrid", "selected:hybrid"), openspec: bindingPointer(t, sddruntime.StoreModeHybrid, "selected:hybrid"), wantMode: sddruntime.StoreModeHybrid, provenance: "selected:hybrid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{binding: tt.hive, found: tt.hive != nil}
			local := &fakeOpenSpecBindingStore{binding: tt.openspec}
			hiveSource := &fakeLegacySource{err: errors.New("must not inspect Hive")}
			openSpecSource := &fakeLegacySource{err: errors.New("must not inspect OpenSpec")}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: hiveSource, OpenSpecSource: openSpecSource, OpenSpecChangeDir: "change-dir"}

			got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeNone, Provenance: "ignored"})
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tt.wantMode || got.Provenance != tt.provenance || !got.Persisted {
				t.Fatalf("resolution = %#v", got)
			}
			if hiveSource.calls != 0 || openSpecSource.calls != 0 || len(hive.adopted) != 0 || len(local.adopted) != 0 {
				t.Fatalf("unexpected inspection/adoption: hive source=%d OpenSpec source=%d hive adopts=%d OpenSpec adopts=%d", hiveSource.calls, openSpecSource.calls, len(hive.adopted), len(local.adopted))
			}
		})
	}
}

func TestLegacyBindingRejectsInvalidPersistedCopiesBeforeInspection(t *testing.T) {
	readFailure := errors.New("binding service unavailable")
	for _, tt := range []struct {
		name     string
		hive     *hiveclient.SDDStoreBinding
		hiveErr  error
		openspec *Binding
		want     error
	}{
		{name: "Hive read failure", hiveErr: readFailure, want: readFailure},
		{name: "future Hive schema", hive: hiveBindingWith("2", "hive", "selected"), want: ErrUnsupportedStoredBinding},
		{name: "misplaced Hive mode", hive: hiveBindingWith("1", "openspec", "selected"), want: ErrUnsupportedStoredBinding},
		{name: "misplaced OpenSpec mode", openspec: bindingPointer(t, sddruntime.StoreModeHive, "selected"), want: ErrUnsupportedStoredBinding},
		{name: "two single-store copies", hive: hiveBinding("hive", "selected"), openspec: bindingPointer(t, sddruntime.StoreModeOpenSpec, "selected"), want: ErrBindingCopiesDiverged},
		{name: "hybrid provenance mismatch", hive: hiveBinding("hybrid", "one"), openspec: bindingPointer(t, sddruntime.StoreModeHybrid, "two"), want: ErrBindingCopiesDiverged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{binding: tt.hive, found: tt.hive != nil, getErr: tt.hiveErr}
			local := &fakeOpenSpecBindingStore{binding: tt.openspec}
			hiveSource := &fakeLegacySource{}
			openSpecSource := &fakeLegacySource{}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: hiveSource, OpenSpecSource: openSpecSource, OpenSpecChangeDir: "change-dir"}

			_, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if hiveSource.calls != 0 || openSpecSource.calls != 0 || len(hive.adopted) != 0 || len(local.adopted) != 0 {
				t.Fatalf("invalid binding caused inspection or adoption")
			}
		})
	}
}

func TestLegacyBindingAdoptsFromProtectedProgressMatrix(t *testing.T) {
	canonical := legacyBindingSnapshot(t, "change")
	divergentCanonical := legacyBindingSnapshot(t, "other-change")
	for _, tt := range []struct {
		name               string
		hive, openspec     sddstatus.LegacyProgressObservation
		initial            InitialSelection
		wantMode           sddruntime.StoreMode
		wantProvenance     string
		wantPersisted      bool
		wantHiveAdoptions  int
		wantLocalAdoptions int
		wantErr            error
	}{
		{name: "neither uses initial Hive", initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial:default"}, wantMode: sddruntime.StoreModeHive, wantProvenance: "initial:default", wantPersisted: true, wantHiveAdoptions: 1},
		{name: "neither uses initial OpenSpec", initial: InitialSelection{Mode: sddruntime.StoreModeOpenSpec, Provenance: " initial:environment "}, wantMode: sddruntime.StoreModeOpenSpec, wantProvenance: "initial:environment", wantPersisted: true, wantLocalAdoptions: 1},
		{name: "neither uses initial hybrid", initial: InitialSelection{Mode: sddruntime.StoreModeHybrid, Provenance: "initial:hybrid"}, wantMode: sddruntime.StoreModeHybrid, wantProvenance: "initial:hybrid", wantPersisted: true, wantHiveAdoptions: 1, wantLocalAdoptions: 1},
		{name: "neither none remains unpersisted", initial: InitialSelection{Mode: sddruntime.StoreModeNone, Provenance: "initial:none"}, wantMode: sddruntime.StoreModeNone, wantProvenance: "initial:none"},
		{name: "Hive progress wins", hive: legacyProgress("hive"), initial: InitialSelection{Mode: sddruntime.StoreModeOpenSpec, Provenance: "ignored"}, wantMode: sddruntime.StoreModeHive, wantProvenance: ProvenanceLegacyHiveProgress, wantPersisted: true, wantHiveAdoptions: 1},
		{name: "OpenSpec progress wins", openspec: legacyProgress("openspec"), initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantMode: sddruntime.StoreModeOpenSpec, wantProvenance: ProvenanceLegacyOpenSpecProgress, wantPersisted: true, wantLocalAdoptions: 1},
		{name: "equivalent progress becomes hybrid", hive: legacyProgress("same"), openspec: legacyProgress("same"), initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantMode: sddruntime.StoreModeHybrid, wantProvenance: ProvenanceLegacyEquivalentProgress, wantPersisted: true, wantHiveAdoptions: 1, wantLocalAdoptions: 1},
		{name: "equivalent canonical v2 becomes hybrid", hive: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: canonical}, openspec: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: canonical}, initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantMode: sddruntime.StoreModeHybrid, wantProvenance: ProvenanceLegacyEquivalentProgress, wantPersisted: true, wantHiveAdoptions: 1, wantLocalAdoptions: 1},
		{name: "divergent progress blocks", hive: legacyProgress("one"), openspec: legacyProgress("two"), initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
		{name: "divergent canonical v2 blocks", hive: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: canonical}, openspec: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: divergentCanonical}, initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
		{name: "one blocked Hive progress cannot select authority", hive: blockedProgress(), initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
		{name: "one blocked OpenSpec progress cannot select authority", openspec: blockedProgress(), initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
		{name: "one blank progress cannot select authority", hive: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: " \t"}, initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
		{name: "one noncanonical JSON progress cannot select authority", openspec: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: `{"schema":"jarvis.sdd-apply-progress/v2"}`}, initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
		{name: "blocked progress cannot prove hybrid", hive: blockedProgress(), openspec: blockedProgress(), initial: InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}, wantErr: ErrLegacyProgressDiverged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{}
			local := &fakeOpenSpecBindingStore{}
			resolver := LegacyResolver{
				HiveBindings: hive, OpenSpecBindings: local,
				HiveSource: observationSource(tt.hive), OpenSpecSource: observationSource(tt.openspec),
				OpenSpecChangeDir: "change-dir",
			}
			got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", tt.initial)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if tt.wantErr == nil && (got.Mode != tt.wantMode || got.Provenance != tt.wantProvenance || got.Persisted != tt.wantPersisted) {
				t.Fatalf("resolution = %#v", got)
			}
			if len(hive.adopted) != tt.wantHiveAdoptions || len(local.adopted) != tt.wantLocalAdoptions {
				t.Fatalf("adoptions Hive=%d OpenSpec=%d", len(hive.adopted), len(local.adopted))
			}
		})
	}
}

func TestLegacyBindingRepairsOneMissingHybridMirrorOnlyAfterEquality(t *testing.T) {
	for _, tt := range []struct {
		name               string
		hiveBinding        *hiveclient.SDDStoreBinding
		openSpecBinding    *Binding
		hive, openspec     sddstatus.LegacyProgressObservation
		wantHiveAdoptions  int
		wantLocalAdoptions int
		wantErr            error
	}{
		{name: "repair OpenSpec mirror", hiveBinding: hiveBinding("hybrid", "selected"), hive: legacyProgress("same"), openspec: legacyProgress("same"), wantHiveAdoptions: 1, wantLocalAdoptions: 1},
		{name: "repair Hive mirror", openSpecBinding: bindingPointer(t, sddruntime.StoreModeHybrid, "selected"), hive: legacyProgress("same"), openspec: legacyProgress("same"), wantHiveAdoptions: 1, wantLocalAdoptions: 1},
		{name: "missing progress mirror blocks repair", hiveBinding: hiveBinding("hybrid", "selected"), hive: legacyProgress("same"), wantErr: ErrLegacyProgressDiverged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hive := &fakeHiveBindingStore{binding: tt.hiveBinding, found: tt.hiveBinding != nil}
			local := &fakeOpenSpecBindingStore{binding: tt.openSpecBinding}
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(tt.hive), OpenSpecSource: observationSource(tt.openspec), OpenSpecChangeDir: "change-dir"}
			got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeNone, Provenance: "ignored"})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v", err)
				}
			} else if err != nil || got.Mode != sddruntime.StoreModeHybrid || got.Provenance != "selected" || !got.Persisted {
				t.Fatalf("resolution=%#v error=%v", got, err)
			}
			if len(hive.adopted) != tt.wantHiveAdoptions || len(local.adopted) != tt.wantLocalAdoptions {
				t.Fatalf("adoptions Hive=%d OpenSpec=%d", len(hive.adopted), len(local.adopted))
			}
		})
	}
}

func TestLegacyBindingHybridAdoptionIsBackendFirstAndRetryable(t *testing.T) {
	order := []string{}
	hive := &fakeHiveBindingStore{order: &order}
	local := &fakeOpenSpecBindingStore{order: &order, adoptErr: errors.New("local commit failed")}
	resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}

	_, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHybrid, Provenance: "initial"})
	var partial *PartialAdoptionError
	if !errors.As(err, &partial) || !errors.Is(err, ErrPartialAdoption) {
		t.Fatalf("error = %#v, want partial adoption", err)
	}
	if strings.Join(order, ",") != "hive,openspec" || partial.Binding.Mode() != sddruntime.StoreModeHybrid || partial.Binding.Provenance() != "initial" {
		t.Fatalf("order=%v partial=%#v", order, partial)
	}

	local.adoptErr = nil
	order = nil
	got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"})
	if err != nil || got.Mode != sddruntime.StoreModeHybrid || len(hive.adopted) != 2 || len(local.adopted) != 2 {
		t.Fatalf("retry resolution=%#v error=%v Hive adopts=%d OpenSpec adopts=%d", got, err, len(hive.adopted), len(local.adopted))
	}
}

func TestLegacyBindingResolvesHiveCopyWithoutOpenSpecChangeDirectory(t *testing.T) {
	root := t.TempDir()
	resolver := LegacyResolver{
		HiveBindings:      &fakeHiveBindingStore{binding: hiveBinding("hive", "selected"), found: true},
		OpenSpecChangeDir: filepath.Join(root, "openspec", "changes", "change"),
		ResolutionLockDir: root,
	}
	got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeOpenSpec, Provenance: "ignored"})
	if err != nil || got.Mode != sddruntime.StoreModeHive || got.Provenance != "selected" || !got.Persisted {
		t.Fatalf("resolution=%#v error=%v", got, err)
	}
}

func TestLegacyBindingUsesProjectLockWithConcreteOpenSpecStore(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "openspec", "changes", "change")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := LegacyResolver{
		HiveBindings:      &fakeHiveBindingStore{},
		HiveSource:        observationSource(sddstatus.LegacyProgressObservation{}),
		OpenSpecSource:    observationSource(sddstatus.LegacyProgressObservation{}),
		OpenSpecChangeDir: changeDir,
		ResolutionLockDir: root,
	}
	got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeOpenSpec, Provenance: "initial"})
	if err != nil || got.Mode != sddruntime.StoreModeOpenSpec || !got.Persisted {
		t.Fatalf("resolution=%#v error=%v", got, err)
	}
	persisted, err := ReadOpenSpec(changeDir)
	if err != nil || persisted == nil || persisted.Mode() != sddruntime.StoreModeOpenSpec || persisted.Provenance() != "initial" {
		t.Fatalf("persisted=%#v error=%v", persisted, err)
	}
}

func TestLegacyBindingRejectsPhysicalResolutionLockAlias(t *testing.T) {
	changeDir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "change-alias")
	if err := os.Symlink(changeDir, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{binding: hiveBinding("hive", "selected"), found: true}, OpenSpecChangeDir: changeDir, ResolutionLockDir: alias}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("physical alias lock error = %v", err)
	}
}

func TestLegacyBindingRejectsDanglingOpenSpecChangeSymlink(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "dangling-change")
	if err := os.Symlink(filepath.Join(root, "missing-target"), changeDir); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{binding: hiveBinding("hive", "selected"), found: true}, OpenSpecChangeDir: changeDir, ResolutionLockDir: root}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("dangling change symlink error = %v", err)
	}
}

func TestLegacyBindingRejectsDanglingOpenSpecAncestorSymlink(t *testing.T) {
	root := t.TempDir()
	openSpec := filepath.Join(root, "openspec")
	if err := os.Symlink(filepath.Join(root, "missing-target"), openSpec); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	changeDir := filepath.Join(openSpec, "changes", "change")
	resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{binding: hiveBinding("hive", "selected"), found: true}, OpenSpecChangeDir: changeDir, ResolutionLockDir: root}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("dangling ancestor symlink error = %v", err)
	}
}

func TestLegacyBindingRejectsResolutionLockAtChangeDirectory(t *testing.T) {
	changeDir := t.TempDir()
	resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{}, OpenSpecChangeDir: changeDir, ResolutionLockDir: changeDir}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("same-directory lock error = %v", err)
	}
}

func TestLegacyBindingRejectsMissingOpenSpecChangeDirectory(t *testing.T) {
	resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{}, OpenSpecBindings: &fakeOpenSpecBindingStore{}, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{})}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("missing change directory error = %v", err)
	}
}

func TestLegacyBindingRejectsInvalidInitialSelectionAndAdoptionResponse(t *testing.T) {
	for _, initial := range []InitialSelection{{Mode: "memory", Provenance: "initial"}, {Mode: sddruntime.StoreModeHive, Provenance: " \t"}} {
		resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{}, OpenSpecBindings: &fakeOpenSpecBindingStore{}, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
		if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", initial); !errors.Is(err, ErrInvalidBinding) {
			t.Fatalf("initial %#v error = %v", initial, err)
		}
	}

	for _, malformed := range []*hiveclient.SDDStoreBinding{
		hiveBinding("hive", "other"),
		func() *hiveclient.SDDStoreBinding {
			binding := hiveBinding("hive", "initial")
			binding.Change = " change "
			return binding
		}(),
	} {
		hive := &fakeHiveBindingStore{adoptResponse: malformed}
		resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: &fakeOpenSpecBindingStore{}, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
		if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"}); !errors.Is(err, ErrInvalidAdoptionResponse) {
			t.Fatalf("malformed adoption %#v error = %v", malformed, err)
		}
	}
}

func TestLegacyBindingSerializesConflictingInitialSelections(t *testing.T) {
	hive := &concurrentHiveBindingStore{}
	local := newConcurrentOpenSpecBindingStore()
	resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: emptyLegacySource{}, OpenSpecSource: emptyLegacySource{}, OpenSpecChangeDir: "change-dir"}
	start := make(chan struct{})
	type outcome struct {
		resolution Resolution
		err        error
	}
	outcomes := make(chan outcome, 2)
	for _, initial := range []InitialSelection{{Mode: sddruntime.StoreModeHive, Provenance: "initial:hive"}, {Mode: sddruntime.StoreModeOpenSpec, Provenance: "initial:openspec"}} {
		go func(initial InitialSelection) {
			<-start
			resolution, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", initial)
			outcomes <- outcome{resolution: resolution, err: err}
		}(initial)
	}
	close(start)
	first, second := <-outcomes, <-outcomes
	if first.err != nil || second.err != nil {
		t.Fatalf("errors = %v, %v", first.err, second.err)
	}
	if first.resolution.Mode != second.resolution.Mode || first.resolution.Provenance != second.resolution.Provenance {
		t.Fatalf("concurrent resolutions diverged: %#v and %#v", first.resolution, second.resolution)
	}
}

func TestLegacyBindingPropagatesProgressAndLocalBindingReadFailures(t *testing.T) {
	progressFailure := errors.New("progress unavailable")
	resolver := LegacyResolver{HiveBindings: &fakeHiveBindingStore{}, OpenSpecBindings: &fakeOpenSpecBindingStore{}, HiveSource: &fakeLegacySource{err: progressFailure}, OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"}); !errors.Is(err, progressFailure) {
		t.Fatalf("progress error = %v", err)
	}

	readFailure := errors.New("invalid local state")
	resolver = LegacyResolver{HiveBindings: &fakeHiveBindingStore{}, OpenSpecBindings: &fakeOpenSpecBindingStore{readErr: readFailure}, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
	if _, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "initial"}); !errors.Is(err, readFailure) {
		t.Fatalf("OpenSpec binding read error = %v", err)
	}
}

func TestLegacyBindingPropagatesFirstWriteConflictWithoutLocalMutation(t *testing.T) {
	conflict := &hiveclient.SDDStoreBindingConflictError{}
	hive := &fakeHiveBindingStore{adoptErr: conflict}
	local := &fakeOpenSpecBindingStore{}
	resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecSource: observationSource(sddstatus.LegacyProgressObservation{}), OpenSpecChangeDir: "change-dir"}
	_, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHybrid, Provenance: "initial"})
	if !errors.As(err, &conflict) || len(local.adopted) != 0 {
		t.Fatalf("error=%#v local adoptions=%d", err, len(local.adopted))
	}
}

type concurrentHiveBindingStore struct {
	mu      sync.Mutex
	binding *hiveclient.SDDStoreBinding
}

func (f *concurrentHiveBindingStore) GetSDDStoreBinding(context.Context, string, string) (hiveclient.SDDStoreBinding, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.binding == nil {
		return hiveclient.SDDStoreBinding{}, false, nil
	}
	return *f.binding, true, nil
}

func (f *concurrentHiveBindingStore) AdoptSDDStoreBinding(_ context.Context, project, change string, request hiveclient.SDDStoreBindingRequest) (hiveclient.SDDStoreBinding, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.binding != nil {
		if f.binding.Mode != request.Mode || f.binding.Provenance != request.Provenance {
			return hiveclient.SDDStoreBinding{}, false, &hiveclient.SDDStoreBindingConflictError{Existing: *f.binding, Requested: hiveclient.SDDStoreBinding{Project: hiveclient.CanonicalProjectKey(project), Change: change, SchemaVersion: "1", Mode: request.Mode, Provenance: request.Provenance}}
		}
		return *f.binding, false, nil
	}
	binding := hiveclient.SDDStoreBinding{Project: hiveclient.CanonicalProjectKey(project), Change: change, SchemaVersion: "1", Mode: request.Mode, Provenance: request.Provenance, CreatedAt: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)}
	f.binding = &binding
	return binding, true, nil
}

type concurrentOpenSpecBindingStore struct {
	resolution sync.Mutex
	mu         sync.Mutex
	locked     atomic.Bool
	reads      atomic.Int32
	bothRead   chan struct{}
	closeOnce  sync.Once
	binding    *Binding
}

func newConcurrentOpenSpecBindingStore() *concurrentOpenSpecBindingStore {
	return &concurrentOpenSpecBindingStore{bothRead: make(chan struct{})}
}

func (f *concurrentOpenSpecBindingStore) LockOpenSpec(ctx context.Context, _ string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.resolution.Lock()
	if err := ctx.Err(); err != nil {
		f.resolution.Unlock()
		return nil, err
	}
	f.locked.Store(true)
	return func() {
		f.locked.Store(false)
		f.resolution.Unlock()
	}, nil
}

func (f *concurrentOpenSpecBindingStore) ReadOpenSpec(string) (*Binding, error) {
	if !f.locked.Load() {
		if f.reads.Add(1) == 2 {
			f.closeOnce.Do(func() { close(f.bothRead) })
		}
		<-f.bothRead
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.binding == nil {
		return nil, nil
	}
	copy := *f.binding
	return &copy, nil
}

func (f *concurrentOpenSpecBindingStore) AdoptOpenSpec(_ string, requested Binding) (Binding, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.binding != nil {
		if *f.binding != requested {
			return Binding{}, false, &ConflictError{Existing: *f.binding, Requested: requested}
		}
		return requested, false, nil
	}
	f.binding = &requested
	return requested, true, nil
}

type emptyLegacySource struct{}

func (emptyLegacySource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	return map[string]sddstatus.ArtifactState{}, map[string]string{}, nil
}

func (emptyLegacySource) ListChanges(context.Context) ([]string, error) { return nil, nil }

type fakeHiveBindingStore struct {
	binding       *hiveclient.SDDStoreBinding
	gets          int
	found         bool
	getErr        error
	adoptErr      error
	adoptResponse *hiveclient.SDDStoreBinding
	adopted       []hiveclient.SDDStoreBindingRequest
	order         *[]string
}

func (f *fakeHiveBindingStore) GetSDDStoreBinding(context.Context, string, string) (hiveclient.SDDStoreBinding, bool, error) {
	f.gets++
	if f.getErr != nil {
		return hiveclient.SDDStoreBinding{}, false, f.getErr
	}
	if f.binding == nil {
		return hiveclient.SDDStoreBinding{}, f.found, nil
	}
	return *f.binding, f.found, nil
}

func (f *fakeHiveBindingStore) AdoptSDDStoreBinding(_ context.Context, project, change string, request hiveclient.SDDStoreBindingRequest) (hiveclient.SDDStoreBinding, bool, error) {
	f.adopted = append(f.adopted, request)
	if f.order != nil {
		*f.order = append(*f.order, "hive")
	}
	if f.adoptErr != nil {
		return hiveclient.SDDStoreBinding{}, false, f.adoptErr
	}
	if f.adoptResponse != nil {
		return *f.adoptResponse, true, nil
	}
	binding := hiveclient.SDDStoreBinding{Project: hiveclient.CanonicalProjectKey(project), Change: change, SchemaVersion: "1", Mode: request.Mode, Provenance: request.Provenance, CreatedAt: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)}
	f.binding, f.found = &binding, true
	return binding, true, nil
}

type fakeOpenSpecBindingStore struct {
	resolution sync.Mutex
	binding    *Binding
	reads      int
	readErr    error
	adoptErr   error
	adopted    []Binding
	order      *[]string
}

func (f *fakeOpenSpecBindingStore) LockOpenSpec(ctx context.Context, _ string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.resolution.Lock()
	if err := ctx.Err(); err != nil {
		f.resolution.Unlock()
		return nil, err
	}
	return f.resolution.Unlock, nil
}

func (f *fakeOpenSpecBindingStore) ReadOpenSpec(string) (*Binding, error) {
	f.reads++
	return f.binding, f.readErr
}

func (f *fakeOpenSpecBindingStore) AdoptOpenSpec(_ string, requested Binding) (Binding, bool, error) {
	f.adopted = append(f.adopted, requested)
	if f.order != nil {
		*f.order = append(*f.order, "openspec")
	}
	if f.adoptErr != nil {
		return Binding{}, false, f.adoptErr
	}
	if f.binding != nil && *f.binding != requested {
		return Binding{}, false, &ConflictError{Existing: *f.binding, Requested: requested}
	}
	created := f.binding == nil
	f.binding = &requested
	return requested, created, nil
}

type fakeLegacySource struct {
	observation sddstatus.LegacyProgressObservation
	err         error
	calls       int
}

func (f *fakeLegacySource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	artifacts := map[string]sddstatus.ArtifactState{}
	contents := map[string]string{}
	if f.observation.Present {
		artifacts[sddstatus.ArtifactApplyProgress] = f.observation.State
		if f.observation.Content != "" {
			contents[sddstatus.ArtifactApplyProgress] = f.observation.Content
		}
	}
	return artifacts, contents, nil
}

func (*fakeLegacySource) ListChanges(context.Context) ([]string, error) { return nil, nil }

func observationSource(observation sddstatus.LegacyProgressObservation) *fakeLegacySource {
	return &fakeLegacySource{observation: observation}
}

func legacyBindingSnapshot(t *testing.T, change string) string {
	t.Helper()
	_, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema:             applyprogress.SnapshotSchema,
		Project:            "project",
		Change:             change,
		Generation:         1,
		Revision:           1,
		TaskManifestSHA256: strings.Repeat("a", 64),
		Status:             applyprogress.StatusPartial,
		Coverage:           []applyprogress.Coverage{},
		Batches:            []applyprogress.BatchRef{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func legacyProgress(content string) sddstatus.LegacyProgressObservation {
	return sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: content}
}

func blockedProgress() sddstatus.LegacyProgressObservation {
	return sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactBlockedInvalid, Content: "blocked"}
}

func bindingPointer(t *testing.T, mode sddruntime.StoreMode, provenance string) *Binding {
	t.Helper()
	binding, err := New(mode, provenance)
	if err != nil {
		t.Fatal(err)
	}
	return &binding
}

func hiveBinding(mode, provenance string) *hiveclient.SDDStoreBinding {
	return hiveBindingWith("1", mode, provenance)
}

func hiveBindingWith(schema, mode, provenance string) *hiveclient.SDDStoreBinding {
	return &hiveclient.SDDStoreBinding{
		Project:       "project",
		Change:        "change",
		SchemaVersion: schema,
		Mode:          hiveclient.SDDStoreMode(mode),
		Provenance:    provenance,
		CreatedAt:     time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
	}
}
