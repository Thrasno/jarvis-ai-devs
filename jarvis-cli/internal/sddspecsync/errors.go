package sddspecsync

import (
	"errors"
	"fmt"
	"strings"
)

// Merge and plan error kinds. Every merge failure is a *MergeError whose Kind
// is one of these sentinels, so callers can use errors.Is.
var (
	ErrInvalidDelta          = errors.New("unsupported delta spec structure")
	ErrInvalidMainSpec       = errors.New("main spec cannot be merged safely")
	ErrNoRequirementsSection = errors.New("main spec has no unique requirements section")
	ErrDuplicateRequirement  = errors.New("requirement already exists")
	ErrRequirementNotFound   = errors.New("requirement not found")
	ErrRemovalEvidence       = errors.New("REMOVED requirement lacks valid Reason/Migration evidence")
	ErrRenameEvidence        = errors.New("RENAMED requirement lacks valid Old name/New name evidence")
	ErrNoDeltaSpecs          = errors.New("change has no delta specs")
)

// Apply error kinds. Every apply failure is an *ApplyError.
var (
	ErrInvalidPlan      = errors.New("spec sync plan is not internally consistent")
	ErrStale            = errors.New("main spec changed since the plan was built")
	ErrWriteFailed      = errors.New("main spec write failed")
	ErrRecoveryRequired = errors.New("spec sync rollback could not be verified")
	ErrUnsafePath       = errors.New("unsafe spec path")
)

// Stable failure codes shared with the sdd-archive contract.
const (
	CodeConflictRecoveryRequired = "spec_sync_conflict_recovery_required"
	CodeRecoveryRequired         = "spec_sync_recovery_required"
	CodeWriteFailed              = "spec_sync_write_failed"
)

// MergeError describes why a delta cannot be merged into a main spec.
type MergeError struct {
	Kind        error
	Capability  string
	Section     string
	Requirement string
	Detail      string
}

func (e *MergeError) Error() string {
	var b strings.Builder
	b.WriteString("spec sync")
	if e.Capability != "" {
		fmt.Fprintf(&b, " capability %q", e.Capability)
	}
	if e.Section != "" {
		fmt.Fprintf(&b, " %s", e.Section)
	}
	if e.Requirement != "" {
		fmt.Fprintf(&b, " requirement %q", e.Requirement)
	}
	fmt.Fprintf(&b, ": %v", e.Kind)
	if e.Detail != "" {
		fmt.Fprintf(&b, ": %s", e.Detail)
	}
	return b.String()
}

func (e *MergeError) Unwrap() error { return e.Kind }

// RecoveryTarget is a main spec whose rollback could not be verified, with the
// digest it must be restored to (AbsentDigest means it must not exist).
type RecoveryTarget struct {
	Path           string
	ExpectedDigest string
}

// ApplyError describes a failed Apply. When Recovery is non-empty the working
// tree is not back at its pre-sync state and needs manual recovery.
type ApplyError struct {
	Kind     error
	Path     string
	Cause    error
	Recovery []RecoveryTarget
}

func (e *ApplyError) Error() string {
	msg := fmt.Sprintf("spec sync %s: %v", e.Path, e.Kind)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	if len(e.Recovery) > 0 {
		paths := make([]string, 0, len(e.Recovery))
		for _, target := range e.Recovery {
			paths = append(paths, target.Path+" (expected "+target.ExpectedDigest+")")
		}
		msg += "; " + ErrRecoveryRequired.Error() + ": " + strings.Join(paths, ", ")
	}
	return msg
}

func (e *ApplyError) Unwrap() []error {
	errs := []error{e.Kind}
	if e.Cause != nil {
		errs = append(errs, e.Cause)
	}
	if len(e.Recovery) > 0 {
		errs = append(errs, ErrRecoveryRequired)
	}
	return errs
}

// Code maps the failure to the sdd-archive return code.
func (e *ApplyError) Code() string {
	switch {
	case len(e.Recovery) > 0:
		return CodeRecoveryRequired
	case errors.Is(e.Kind, ErrStale):
		return CodeConflictRecoveryRequired
	default:
		return CodeWriteFailed
	}
}
