// Package sddbinding persists immutable SDD artifact-store selections.
package sddbinding

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

var (
	// ErrInvalidBinding identifies a binding that cannot be persisted safely.
	ErrInvalidBinding = errors.New("invalid SDD store binding")
	// ErrBindingConflict identifies a requested binding that differs from the persisted one.
	ErrBindingConflict = errors.New("SDD store binding conflict")
	// ErrInvalidState identifies a state document that cannot establish binding authority.
	ErrInvalidState = errors.New("invalid OpenSpec binding state")
	// ErrUnsafePath identifies a change directory or state path unsafe for mutation.
	ErrUnsafePath = errors.New("unsafe OpenSpec binding path")
)

// Binding is a validated, immutable artifact-store decision and its provenance.
type Binding struct {
	mode       sddruntime.StoreMode
	provenance string
}

// New validates a persistable artifact-store binding.
func New(mode sddruntime.StoreMode, provenance string) (Binding, error) {
	resolved, err := sddruntime.ResolveStoreMode(string(mode))
	if err != nil || resolved == sddruntime.StoreModeNone {
		return Binding{}, fmt.Errorf("%w: mode %q", ErrInvalidBinding, mode)
	}
	provenance = strings.TrimSpace(provenance)
	if provenance == "" {
		return Binding{}, fmt.Errorf("%w: provenance is empty", ErrInvalidBinding)
	}
	return Binding{mode: resolved, provenance: provenance}, nil
}

// Mode returns the validated store mode selected for this change.
func (b Binding) Mode() sddruntime.StoreMode { return b.mode }

// Provenance returns the immutable reason or source that selected this binding.
func (b Binding) Provenance() string { return b.provenance }

// ConflictError retains the two immutable decisions that cannot be reconciled automatically.
type ConflictError struct {
	Existing  Binding
	Requested Binding
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%v: existing %s/%q differs from requested %s/%q", ErrBindingConflict, e.Existing.mode, e.Existing.provenance, e.Requested.mode, e.Requested.provenance)
}

func (e *ConflictError) Unwrap() error { return ErrBindingConflict }
