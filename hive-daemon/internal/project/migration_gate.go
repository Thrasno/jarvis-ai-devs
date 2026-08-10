package project

import (
	"errors"
	"fmt"
)

const MigrationStateReady = "ready"
const MigrationStateBlocked = "migration-blocked"

const defaultMigrationBlockedReason = "migration status is unavailable"
const defaultMigrationContinuation = "hive project identity status"

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

type MigrationGate struct {
	status MigrationStatus
}

type MigrationBlockedError struct {
	Status MigrationStatus
}

func (e *MigrationBlockedError) Error() string {
	return fmt.Sprintf("migration-blocked: %s", e.Status.Reason)
}

func NewMigrationGate(status MigrationStatus) *MigrationGate {
	if status.State != MigrationStateReady {
		status.State = MigrationStateBlocked
		if status.Reason == "" {
			status.Reason = defaultMigrationBlockedReason
		}
		status.Continuation = defaultMigrationContinuation
	}
	return &MigrationGate{status: status}
}

func (g *MigrationGate) Status() MigrationStatus {
	return g.status
}

func (g *MigrationGate) Check() error {
	if g == nil || g.status.State == MigrationStateReady {
		return nil
	}
	return &MigrationBlockedError{Status: g.status}
}
