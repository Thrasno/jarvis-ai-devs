package sddbinding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// ProvenancePreflight records a store chosen explicitly in the SDD session preflight.
const ProvenancePreflight = "initial:preflight"

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
// OpenSpec change directory that owns the local copy right before adopting it.
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
	if pending.Mode() != sddruntime.StoreModeHive && r.OpenSpecBindings == nil {
		if err := r.ensureBindChangeDirectory(change); err != nil {
			return Resolution{}, err
		}
	}
	return r.adopt(ctx, project, change, *pending, localStore)
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

// ensureBindChangeDirectory creates the missing OpenSpec change directory and
// revalidates the coordinate, so a symlink planted meanwhile is rejected
// before the local copy is written.
func (r LegacyResolver) ensureBindChangeDirectory(change string) error {
	if err := os.MkdirAll(r.OpenSpecChangeDir, 0o755); err != nil {
		return fmt.Errorf("create OpenSpec change directory: %w", err)
	}
	return r.validateBindChangeDirectory(change)
}
