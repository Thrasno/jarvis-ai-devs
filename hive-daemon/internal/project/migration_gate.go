package project

import (
	"errors"
	"fmt"
	"sync"
)

const MigrationStateReady = "ready"
const MigrationStateBlocked = "migration-blocked"

// MigrationStatePendingOperatorReview is the closed-but-intact state: the
// preflight found project identities it cannot fold on its own authority, and it
// stopped before writing anything at all.
//
// It is deliberately not MigrationStateBlocked. That state means a migration was
// attempted and failed, so the database may be anything the executor left behind
// and only the narrow recovery surface is safe. This state means nothing was
// attempted: the database is exactly as the operator left it, so read-only
// surfaces stay reachable and what is missing is a human decision, not a repair.
const MigrationStatePendingOperatorReview = "migration-pending-operator-review"

const defaultMigrationBlockedReason = "migration status is unavailable"
const defaultMigrationContinuation = "hive project identity status"

// MigrationPendingOperatorContinuation names the only surface that can actually
// make this decision. The CLI status command can report the ambiguity but cannot
// resolve it, so pointing there leaves the operator with a dead end; the wizard
// lives in the Hive TUI.
const MigrationPendingOperatorContinuation = "jarvis hive → Project normalization"

const defaultMigrationPendingOperatorReason = "project identities are ambiguous and need an explicit operator decision"

var ErrIdentityResolutionStale = errors.New("project identity resolution is stale or unrelated")

// IdentityResolutionRequest authorizes one explicit identity choice. The guard
// is PlanFingerprint: the preflight-conflict path never mutates the database, so
// it never creates a rollback archive and BackupID cannot prove anything there.
// BackupID stays on the wire for compatibility with older clients.
type IdentityResolutionRequest struct {
	SourceProject   string `json:"source_project"`
	TargetProject   string `json:"target_project"`
	BackupID        string `json:"backup_id,omitempty"`
	PlanFingerprint string `json:"plan_fingerprint"`
	Confirmation    string `json:"confirmation"`
}

func IdentityResolutionConfirmation(source, target string) string {
	return "RESOLVE project identity " + source + " INTO " + target
}

// MigrationExecuteRequest authorizes the one fold the operator reviewed.
//
// Both fields are echoes, not instructions. PlanFingerprint is the plan the
// operator saw, so a database that moved underneath them refuses the request
// instead of folding something else; Confirmation is compared against the phrase
// the daemon derives from that same plan, never trusted as the source of truth.
type MigrationExecuteRequest struct {
	PlanFingerprint string `json:"plan_fingerprint"`
	Confirmation    string `json:"confirmation"`
}

// MigrationStatus is the boundary-neutral contract every Hive access surface
// can use to fail closed while migration governance is unresolved.
// BackupID is reported only for an archive taken for this exact plan
// fingerprint that still passes its own checksum and has not passed its
// retention, so a rollback can never restore an unrelated older database. It
// need not have been taken during this run: a blocked migration is re-attempted
// on every daemon start and reuses the archive it already took. PlanFingerprint
// identifies the exact plan the operator was shown and is the resolution guard.
type MigrationStatus struct {
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
	Continuation    string `json:"continuation,omitempty"`
	BackupID        string `json:"backup_id,omitempty"`
	PlanFingerprint string `json:"plan_fingerprint,omitempty"`
}

// MigrationGate is shared by every access surface — HTTP and MCP hold the same
// pointer — so its status is guarded: it is now written after construction, by the
// operator's own fold succeeding, while requests are being served.
type MigrationGate struct {
	mu     sync.RWMutex
	status MigrationStatus
}

type MigrationBlockedError struct {
	Status MigrationStatus
}

func (e *MigrationBlockedError) Error() string {
	return fmt.Sprintf("%s: %s", e.Status.State, e.Status.Reason)
}

// NewMigrationGate normalizes a status into exactly one of the three states this
// package recognizes. Every unknown state still folds into MigrationStateBlocked
// with its own reason and continuation, so a status this package does not
// understand can never come up permissive or under-described.
func NewMigrationGate(status MigrationStatus) *MigrationGate {
	return &MigrationGate{status: normalizeMigrationStatus(status)}
}

// Adopt replaces the gate's status in place, through the same normalization
// NewMigrationGate applies.
//
// It exists because the operator's approved fold has to open the gate without a
// restart. hive-daemon is spawned by an MCP client, so "restart the daemon" means
// "restart your editor session" — an absurd thing to ask of someone who just
// approved the repair themselves. Every surface holds this one pointer, so
// flipping it here is what makes Hive usable again everywhere at once.
func (g *MigrationGate) Adopt(status MigrationStatus) {
	if g == nil {
		return
	}
	normalized := normalizeMigrationStatus(status)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.status = normalized
}

func normalizeMigrationStatus(status MigrationStatus) MigrationStatus {
	switch status.State {
	case MigrationStateReady:
	case MigrationStatePendingOperatorReview:
		if status.Reason == "" {
			status.Reason = defaultMigrationPendingOperatorReason
		}
		status.Continuation = MigrationPendingOperatorContinuation
	default:
		status.State = MigrationStateBlocked
		if status.Reason == "" {
			status.Reason = defaultMigrationBlockedReason
		}
		status.Continuation = defaultMigrationContinuation
	}
	return status
}

func (g *MigrationGate) Status() MigrationStatus {
	if g == nil {
		return MigrationStatus{State: MigrationStateReady}
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.status
}

// Blocking reports a gate that is not serving, whichever closed state it is in.
// Callers that must treat "waiting for a decision" and "the migration failed"
// alike — the recovery handlers, the request gate — ask this instead of comparing
// against one state and silently letting the other through.
func (g *MigrationGate) Blocking() bool {
	return g.Check() != nil
}

func (g *MigrationGate) Check() error {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.status.State == MigrationStateReady {
		return nil
	}
	return &MigrationBlockedError{Status: g.status}
}
