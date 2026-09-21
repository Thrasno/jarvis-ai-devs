package sddbinding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

const (
	ProvenanceLegacyHiveProgress       = "legacy:hive-progress"
	ProvenanceLegacyOpenSpecProgress   = "legacy:openspec-progress"
	ProvenanceLegacyEquivalentProgress = "legacy:equivalent-progress"
)

var (
	ErrUnsupportedStoredBinding = errors.New("unsupported stored SDD binding")
	ErrBindingCopiesDiverged    = errors.New("SDD binding copies diverged")
	ErrLegacyProgressDiverged   = errors.New("legacy protected progress diverged")
	ErrPartialAdoption          = errors.New("partial SDD binding adoption")
	ErrInvalidAdoptionResponse  = errors.New("invalid SDD binding adoption response")
)

// HiveBindingStore is the bounded Hive binding API consumed by legacy
// resolution. The concrete hiveclient.Client satisfies this interface.
type HiveBindingStore interface {
	GetSDDStoreBinding(ctx context.Context, project, change string) (hiveclient.SDDStoreBinding, bool, error)
	AdoptSDDStoreBinding(ctx context.Context, project, change string, request hiveclient.SDDStoreBindingRequest) (hiveclient.SDDStoreBinding, bool, error)
}

// OpenSpecBindingStore isolates change-local binding persistence for tests and
// leaves the filesystem implementation as the default.
type OpenSpecBindingStore interface {
	ReadOpenSpec(changeDir string) (*Binding, error)
	AdoptOpenSpec(changeDir string, requested Binding) (Binding, bool, error)
}

type openSpecResolutionLocker interface {
	LockOpenSpec(changeDir string) (unlock func(), err error)
}

type lockedOpenSpecBindingStore struct{ root *changeRoot }

func (s lockedOpenSpecBindingStore) ReadOpenSpec(string) (*Binding, error) {
	doc, _, exists, err := readStateDocument(s.root)
	if err != nil || !exists {
		return nil, err
	}
	binding, _, err := bindingFromDocument(doc)
	return binding, err
}

func (s lockedOpenSpecBindingStore) AdoptOpenSpec(_ string, requested Binding) (Binding, bool, error) {
	if _, err := New(requested.Mode(), requested.Provenance()); err != nil {
		return Binding{}, false, err
	}
	return adoptOpenSpecLocked(s.root, requested)
}

// InitialSelection is consulted only when neither store has a persisted copy
// or protected progress. Existing bindings always take precedence.
type InitialSelection struct {
	Mode       sddruntime.StoreMode
	Provenance string
}

// Resolution is the authoritative store decision for one explicit change.
type Resolution struct {
	Mode       sddruntime.StoreMode
	Provenance string
	Persisted  bool
}

// LegacyResolver reconciles persisted copies first, then safely adopts an
// unbound legacy change from read-only protected-progress observations.
type LegacyResolver struct {
	HiveBindings      HiveBindingStore
	OpenSpecBindings  OpenSpecBindingStore
	HiveSource        sddstatus.ArtifactSource
	OpenSpecSource    sddstatus.ArtifactSource
	OpenSpecChangeDir string
}

// PartialAdoptionError reports that Hive accepted a hybrid binding but the
// OpenSpec mirror did not. The immutable first write is never rolled back;
// retrying exact adoption repairs the missing mirror.
type PartialAdoptionError struct {
	Binding Binding
	Cause   error
}

func (e *PartialAdoptionError) Error() string {
	return fmt.Sprintf("%v: Hive accepted %s/%q before OpenSpec failed: %v", ErrPartialAdoption, e.Binding.Mode(), e.Binding.Provenance(), e.Cause)
}

func (e *PartialAdoptionError) Unwrap() error { return errors.Join(ErrPartialAdoption, e.Cause) }

// ResolveAndAdopt returns or establishes one immutable binding. It never
// mutates protected progress and never changes an existing selection. The
// OpenSpec directory lock serializes the read-decide-adopt transaction across
// CLI processes that address the same physical change directory.
func (r LegacyResolver) ResolveAndAdopt(ctx context.Context, project, change string, initial InitialSelection) (Resolution, error) {
	if r.HiveBindings == nil {
		return Resolution{}, errors.New("Hive binding store is required")
	}
	project = hiveclient.CanonicalProjectKey(project)
	change = strings.TrimSpace(change)
	if project == "" || change == "" {
		return Resolution{}, fmt.Errorf("%w: project and change are required", ErrInvalidBinding)
	}
	if strings.TrimSpace(r.OpenSpecChangeDir) == "" {
		return Resolution{}, fmt.Errorf("%w: OpenSpec change directory is required", ErrInvalidBinding)
	}

	if r.OpenSpecBindings != nil {
		locker, ok := r.OpenSpecBindings.(openSpecResolutionLocker)
		if !ok {
			return Resolution{}, errors.New("injected OpenSpec binding store must provide the legacy resolution lock")
		}
		unlock, err := locker.LockOpenSpec(r.OpenSpecChangeDir)
		if err != nil {
			return Resolution{}, err
		}
		defer unlock()
		return r.resolveAndAdoptLocked(ctx, project, change, initial, r.OpenSpecBindings)
	}

	root, err := openChangeRoot(r.OpenSpecChangeDir)
	if err != nil {
		return Resolution{}, err
	}
	defer func() { _ = root.Close() }()
	unlock, err := root.lock()
	if err != nil {
		return Resolution{}, err
	}
	defer unlock()
	return r.resolveAndAdoptLocked(ctx, project, change, initial, lockedOpenSpecBindingStore{root: root})
}

func (r LegacyResolver) resolveAndAdoptLocked(ctx context.Context, project, change string, initial InitialSelection, localStore OpenSpecBindingStore) (Resolution, error) {
	hive, hiveFound, err := r.HiveBindings.GetSDDStoreBinding(ctx, project, change)
	if err != nil {
		return Resolution{}, err
	}
	if hiveFound {
		if err := validateStoredHiveBinding(hive, project, change); err != nil {
			return Resolution{}, err
		}
	}
	local, err := localStore.ReadOpenSpec(r.OpenSpecChangeDir)
	if err != nil {
		return Resolution{}, err
	}
	if local != nil && local.Mode() != sddruntime.StoreModeOpenSpec && local.Mode() != sddruntime.StoreModeHybrid {
		return Resolution{}, fmt.Errorf("%w: OpenSpec copy has mode %q", ErrUnsupportedStoredBinding, local.Mode())
	}

	switch {
	case hiveFound && local != nil:
		if hive.Mode != hiveclient.SDDStoreModeHybrid || local.Mode() != sddruntime.StoreModeHybrid || hive.Provenance != local.Provenance() {
			return Resolution{}, fmt.Errorf("%w: Hive %s/%q and OpenSpec %s/%q", ErrBindingCopiesDiverged, hive.Mode, hive.Provenance, local.Mode(), local.Provenance())
		}
		return persistedResolution(sddruntime.StoreModeHybrid, hive.Provenance), nil
	case hiveFound:
		if hive.Mode == hiveclient.SDDStoreModeHive {
			return persistedResolution(sddruntime.StoreModeHive, hive.Provenance), nil
		}
		if err := r.requireEquivalentProgress(ctx, change); err != nil {
			return Resolution{}, err
		}
		binding, err := New(sddruntime.StoreModeHybrid, hive.Provenance)
		if err != nil {
			return Resolution{}, err
		}
		return r.adopt(ctx, project, change, binding, localStore)
	case local != nil:
		if local.Mode() == sddruntime.StoreModeOpenSpec {
			return persistedResolution(sddruntime.StoreModeOpenSpec, local.Provenance()), nil
		}
		if err := r.requireEquivalentProgress(ctx, change); err != nil {
			return Resolution{}, err
		}
		return r.adopt(ctx, project, change, *local, localStore)
	}

	hiveProgress, openSpecProgress, err := r.observeProgress(ctx, change)
	if err != nil {
		return Resolution{}, err
	}
	if !legacyProgressValid(hiveProgress) || !legacyProgressValid(openSpecProgress) {
		return Resolution{}, ErrLegacyProgressDiverged
	}
	var selected InitialSelection
	switch {
	case !hiveProgress.Present && !openSpecProgress.Present:
		selected, err = validateInitialSelection(initial)
		if err != nil {
			return Resolution{}, err
		}
		if selected.Mode == sddruntime.StoreModeNone {
			return Resolution{Mode: selected.Mode, Provenance: selected.Provenance, Persisted: false}, nil
		}
	case hiveProgress.Present && !openSpecProgress.Present:
		selected = InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: ProvenanceLegacyHiveProgress}
	case !hiveProgress.Present && openSpecProgress.Present:
		selected = InitialSelection{Mode: sddruntime.StoreModeOpenSpec, Provenance: ProvenanceLegacyOpenSpecProgress}
	default:
		if !sddstatus.LegacyProgressEquivalent(hiveProgress, openSpecProgress) {
			return Resolution{}, ErrLegacyProgressDiverged
		}
		selected = InitialSelection{Mode: sddruntime.StoreModeHybrid, Provenance: ProvenanceLegacyEquivalentProgress}
	}
	binding, err := New(selected.Mode, selected.Provenance)
	if err != nil {
		return Resolution{}, err
	}
	return r.adopt(ctx, project, change, binding, localStore)
}

func (r LegacyResolver) observeProgress(ctx context.Context, change string) (sddstatus.LegacyProgressObservation, sddstatus.LegacyProgressObservation, error) {
	if r.HiveSource == nil || r.OpenSpecSource == nil {
		return sddstatus.LegacyProgressObservation{}, sddstatus.LegacyProgressObservation{}, errors.New("both legacy progress sources are required")
	}
	hive, err := sddstatus.ObserveLegacyProgress(ctx, r.HiveSource, change)
	if err != nil {
		return sddstatus.LegacyProgressObservation{}, sddstatus.LegacyProgressObservation{}, fmt.Errorf("inspect Hive protected progress: %w", err)
	}
	local, err := sddstatus.ObserveLegacyProgress(ctx, r.OpenSpecSource, change)
	if err != nil {
		return sddstatus.LegacyProgressObservation{}, sddstatus.LegacyProgressObservation{}, fmt.Errorf("inspect OpenSpec protected progress: %w", err)
	}
	return hive, local, nil
}

func legacyProgressValid(observation sddstatus.LegacyProgressObservation) bool {
	return !observation.Present || sddstatus.LegacyProgressEquivalent(observation, observation)
}

func (r LegacyResolver) requireEquivalentProgress(ctx context.Context, change string) error {
	hive, local, err := r.observeProgress(ctx, change)
	if err != nil {
		return err
	}
	if !sddstatus.LegacyProgressEquivalent(hive, local) {
		return ErrLegacyProgressDiverged
	}
	return nil
}

func (r LegacyResolver) adopt(ctx context.Context, project, change string, requested Binding, localStore OpenSpecBindingStore) (Resolution, error) {
	switch requested.Mode() {
	case sddruntime.StoreModeHive:
		if err := r.adoptHive(ctx, project, change, requested); err != nil {
			return Resolution{}, err
		}
	case sddruntime.StoreModeOpenSpec:
		if err := r.adoptOpenSpec(localStore, requested); err != nil {
			return Resolution{}, err
		}
	case sddruntime.StoreModeHybrid:
		if err := r.adoptHive(ctx, project, change, requested); err != nil {
			return Resolution{}, err
		}
		if err := r.adoptOpenSpec(localStore, requested); err != nil {
			return Resolution{}, &PartialAdoptionError{Binding: requested, Cause: err}
		}
	default:
		return Resolution{}, fmt.Errorf("%w: mode %q", ErrInvalidBinding, requested.Mode())
	}
	return persistedResolution(requested.Mode(), requested.Provenance()), nil
}

func (r LegacyResolver) adoptHive(ctx context.Context, project, change string, requested Binding) error {
	response, _, err := r.HiveBindings.AdoptSDDStoreBinding(ctx, project, change, hiveclient.SDDStoreBindingRequest{
		Mode:       hiveclient.SDDStoreMode(requested.Mode()),
		Provenance: requested.Provenance(),
	})
	if err != nil {
		return err
	}
	if err := validateStoredHiveBinding(response, project, change); err != nil || response.SchemaVersion != bindingSchemaVersion || string(response.Mode) != string(requested.Mode()) || response.Provenance != requested.Provenance() {
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidAdoptionResponse, err)
		}
		return fmt.Errorf("%w: Hive response differs from requested immutable values", ErrInvalidAdoptionResponse)
	}
	return nil
}

func (r LegacyResolver) adoptOpenSpec(localStore OpenSpecBindingStore, requested Binding) error {
	response, _, err := localStore.AdoptOpenSpec(r.OpenSpecChangeDir, requested)
	if err != nil {
		return err
	}
	if response != requested {
		return fmt.Errorf("%w: OpenSpec response differs from requested immutable values", ErrInvalidAdoptionResponse)
	}
	return nil
}

func validateStoredHiveBinding(binding hiveclient.SDDStoreBinding, project, change string) error {
	if binding.Project != project || binding.Change != change {
		return fmt.Errorf("%w: Hive coordinates do not match request", ErrUnsupportedStoredBinding)
	}
	if binding.SchemaVersion != bindingSchemaVersion {
		return fmt.Errorf("%w: Hive schema %q", ErrUnsupportedStoredBinding, binding.SchemaVersion)
	}
	if binding.Mode != hiveclient.SDDStoreModeHive && binding.Mode != hiveclient.SDDStoreModeHybrid {
		return fmt.Errorf("%w: Hive mode %q", ErrUnsupportedStoredBinding, binding.Mode)
	}
	if binding.Provenance == "" || binding.Provenance != strings.TrimSpace(binding.Provenance) {
		return fmt.Errorf("%w: Hive provenance is blank or untrimmed", ErrUnsupportedStoredBinding)
	}
	if binding.CreatedAt.IsZero() || binding.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("%w: Hive timestamp is invalid", ErrUnsupportedStoredBinding)
	}
	return nil
}

func validateInitialSelection(initial InitialSelection) (InitialSelection, error) {
	mode, err := sddruntime.ResolveStoreMode(string(initial.Mode))
	if err != nil {
		return InitialSelection{}, fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	provenance := strings.TrimSpace(initial.Provenance)
	if provenance == "" {
		return InitialSelection{}, fmt.Errorf("%w: provenance is empty", ErrInvalidBinding)
	}
	return InitialSelection{Mode: mode, Provenance: provenance}, nil
}

func persistedResolution(mode sddruntime.StoreMode, provenance string) Resolution {
	return Resolution{Mode: mode, Provenance: provenance, Persisted: true}
}
