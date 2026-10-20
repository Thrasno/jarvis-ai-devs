package sddbinding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// ProvenancePreflight records a store chosen explicitly in the SDD session preflight.
const ProvenancePreflight = "initial:preflight"

// bindAdoptOpenSpec writes the local copy for Bind; tests replace it to
// inject a local commit failure.
var bindAdoptOpenSpec = func(store OpenSpecBindingStore, changeDir string, requested Binding) (Binding, bool, error) {
	return store.AdoptOpenSpec(changeDir, requested)
}

// Bind establishes the preflight store choice as the immutable binding of an
// unbound change. It shares the resolution lock and the read-decide path of
// ResolveAndAdopt, so a persisted binding, or the binding that protected
// progress already selects, always wins:
//
//   - the same mode is an idempotent replay that writes nothing and keeps the
//     existing provenance;
//   - a different mode returns *ConflictError and writes nothing.
//
// Only an unbound change without protected progress is adopted with
// provenance ProvenancePreflight. For openspec and hybrid, Bind creates the
// missing OpenSpec change directory only when the local copy is written
// (after Hive accepted the hybrid adoption) and removes the levels it created
// when that write fails, so a failed bind leaves the workspace as it was.
func (r LegacyResolver) Bind(ctx context.Context, project projectidentity.Key, change string, mode sddruntime.StoreMode) (Resolution, error) {
	requested, err := New(mode, ProvenancePreflight)
	if err != nil {
		return Resolution{}, err
	}
	project, change, err = r.validateResolutionRequest(ctx, project, change)
	if err != nil {
		return Resolution{}, err
	}
	if r.OpenSpecBindings == nil {
		if err := r.validateBindChangeDirectory(change); err != nil {
			return Resolution{}, err
		}
	}
	var resolution Resolution
	err = r.withResolutionLock(ctx, func(localStore OpenSpecBindingStore) error {
		var err error
		resolution, err = r.bindLocked(ctx, project, change, requested, localStore)
		var lost *hiveclient.SDDStoreBindingConflictError
		if errors.As(err, &lost) {
			// Another workspace adopted the change between the read and the
			// adoption. Decide once more against the binding that won.
			resolution, err = r.bindLocked(ctx, project, change, requested, localStore)
		}
		return err
	})
	return resolution, err
}

func (r LegacyResolver) bindLocked(ctx context.Context, project projectidentity.Key, change string, requested Binding, localStore OpenSpecBindingStore) (Resolution, error) {
	initial := InitialSelection{Mode: requested.Mode(), Provenance: requested.Provenance()}
	resolution, pending, err := r.decideLocked(ctx, project, change, initial, localStore)
	if err != nil {
		return Resolution{}, err
	}
	if pending == nil {
		if resolution.Mode != requested.Mode() {
			existing, err := New(resolution.Mode, resolution.Provenance)
			if err != nil {
				return Resolution{}, err
			}
			return Resolution{}, &ConflictError{Existing: existing, Requested: requested}
		}
		return resolution, nil
	}
	if pending.Mode() != requested.Mode() {
		return Resolution{}, &ConflictError{Existing: *pending, Requested: requested}
	}
	switch pending.Mode() {
	case sddruntime.StoreModeHive:
		if err := r.adoptHive(ctx, project, change, *pending); err != nil {
			return Resolution{}, err
		}
	case sddruntime.StoreModeOpenSpec:
		if err := r.bindOpenSpecCopy(change, *pending, localStore); err != nil {
			return Resolution{}, err
		}
	case sddruntime.StoreModeHybrid:
		if err := r.adoptHive(ctx, project, change, *pending); err != nil {
			return Resolution{}, err
		}
		if err := r.bindOpenSpecCopy(change, *pending, localStore); err != nil {
			return Resolution{}, &PartialAdoptionError{Binding: *pending, Cause: err}
		}
	default:
		return Resolution{}, fmt.Errorf("%w: mode %q", ErrInvalidBinding, pending.Mode())
	}
	return persistedResolution(pending.Mode(), pending.Provenance()), nil
}

// bindOpenSpecCopy writes the local copy, creating the missing OpenSpec
// change directory first. When the write fails, it removes exactly the
// directory levels it created.
func (r LegacyResolver) bindOpenSpecCopy(change string, requested Binding, localStore OpenSpecBindingStore) error {
	created, err := r.createBindChangeDirectory(change)
	if err == nil {
		var response Binding
		response, _, err = bindAdoptOpenSpec(localStore, r.OpenSpecChangeDir, requested)
		if err == nil && response != requested {
			err = fmt.Errorf("%w: OpenSpec response differs from requested immutable values", ErrInvalidAdoptionResponse)
		}
	}
	if err != nil {
		return errors.Join(err, removeCreatedDirectories(created))
	}
	return nil
}

// validateBindChangeDirectory requires the OpenSpec change directory to be the
// canonical coordinate inside the resolution workspace, because Bind may
// create it there.
func (r LegacyResolver) validateBindChangeDirectory(change string) error {
	if err := validateExistingChangeCoordinate(r.OpenSpecChangeDir, change); err != nil {
		return err
	}
	workspace := filepath.Dir(filepath.Dir(filepath.Dir(r.OpenSpecChangeDir)))
	if workspace != filepath.Clean(r.ResolutionLockDir) {
		return fmt.Errorf("%w: OpenSpec change directory is outside the resolution workspace", ErrInvalidBinding)
	}
	return nil
}

// createBindChangeDirectory creates the missing levels of the OpenSpec change
// directory below the workspace, top-down, and returns the ones this call
// created. It then revalidates the coordinate, so a symlink planted meanwhile
// is rejected before the local copy is written. An injected OpenSpec store
// owns its own directories.
func (r LegacyResolver) createBindChangeDirectory(change string) ([]string, error) {
	if r.OpenSpecBindings != nil {
		return nil, nil
	}
	workspace := filepath.Clean(r.ResolutionLockDir)
	var missing []string
	for dir := r.OpenSpecChangeDir; dir != workspace && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Lstat(dir); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect OpenSpec change directory: %w", err)
		}
		missing = append(missing, dir)
	}
	var created []string
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return created, fmt.Errorf("create OpenSpec change directory: %w", err)
		} else if err == nil {
			created = append(created, missing[i])
		}
	}
	return created, r.validateBindChangeDirectory(change)
}

// removeCreatedDirectories removes the directories createBindChangeDirectory
// created, deepest first. A directory that is no longer an empty directory
// holds content this call did not write, so it is kept.
func removeCreatedDirectories(created []string) error {
	var errs []error
	for i := len(created) - 1; i >= 0; i-- {
		info, err := os.Lstat(created[i])
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(created[i])
		if err != nil || len(entries) != 0 {
			continue
		}
		if err := os.Remove(created[i]); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove OpenSpec directory created by bind: %w", err))
		}
	}
	return errors.Join(errs...)
}
