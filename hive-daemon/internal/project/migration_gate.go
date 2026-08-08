package project

import "fmt"

const MigrationStateReady = "ready"
const MigrationStateBlocked = "migration-blocked"

// MigrationStatus is the boundary-neutral contract every Hive access surface
// can use to fail closed while migration governance is unresolved.
type MigrationStatus struct {
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Continuation string `json:"continuation,omitempty"`
	BackupID     string `json:"backup_id,omitempty"`
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
	return &MigrationGate{status: status}
}

func (g *MigrationGate) Status() MigrationStatus {
	return g.status
}

func (g *MigrationGate) Check() error {
	if g.status.State == MigrationStateBlocked {
		return &MigrationBlockedError{Status: g.status}
	}
	return nil
}
