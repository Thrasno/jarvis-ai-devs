package project_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

// TestMigrationGatePendingOperatorReviewKeepsItsOwnStateAndTUIContinuation
// separates "waiting for a decision nobody has made yet" from "the migration
// tried and failed". Both close the gate, but only the failure state carries a
// CLI status command; the pending state must send the operator to the wizard
// that can actually make the decision.
func TestMigrationGatePendingOperatorReviewKeepsItsOwnStateAndTUIContinuation(t *testing.T) {
	gate := project.NewMigrationGate(project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		Reason:          "two spellings of one project",
		PlanFingerprint: "fingerprint-1",
	})
	status := gate.Status()
	if status.State != project.MigrationStatePendingOperatorReview {
		t.Fatalf("state = %q, want the pending-operator state preserved", status.State)
	}
	if status.Continuation != project.MigrationPendingOperatorContinuation {
		t.Fatalf("continuation = %q, want %q", status.Continuation, project.MigrationPendingOperatorContinuation)
	}
	if !strings.Contains(status.Continuation, "jarvis hive") || !strings.Contains(status.Continuation, "Project normalization") {
		t.Fatalf("continuation = %q, want the TUI entry point and its screen", status.Continuation)
	}
	if status.Reason != "two spellings of one project" {
		t.Fatalf("reason = %q, want the caller's reason preserved", status.Reason)
	}
	if status.PlanFingerprint != "fingerprint-1" {
		t.Fatalf("plan fingerprint = %q, want it preserved for the resolution guard", status.PlanFingerprint)
	}
	if status.BackupID != "" {
		t.Fatalf("backup id = %q, want empty; the pending state never mutated anything", status.BackupID)
	}
	var blocked *project.MigrationBlockedError
	if err := gate.Check(); !errors.As(err, &blocked) {
		t.Fatalf("Check() = %v, want the gate closed", err)
	}
	if !gate.Blocking() {
		t.Fatal("Blocking() = false, want a pending gate to report itself closed")
	}
	if blocked.Status.State != project.MigrationStatePendingOperatorReview {
		t.Fatalf("blocked error status = %#v, want the pending state on the wire", blocked.Status)
	}
	if !strings.Contains(blocked.Error(), project.MigrationStatePendingOperatorReview) {
		t.Fatalf("error = %q, want it to name the pending state rather than a generic block", blocked.Error())
	}
}

// TestMigrationGatePendingOperatorReviewSuppliesADefaultReason keeps the pending
// state self-describing: an empty reason on the wire tells the operator nothing.
func TestMigrationGatePendingOperatorReviewSuppliesADefaultReason(t *testing.T) {
	status := project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStatePendingOperatorReview}).Status()
	if status.Reason == "" {
		t.Fatal("reason = empty, want a default that explains what is being waited on")
	}
}

// TestMigrationGateStillFailsClosedForUnknownStatesAfterPendingWasAdded proves
// the new state did not turn the default branch into an escape hatch: anything
// this package does not recognize is still the plain failure block with the CLI
// continuation.
func TestMigrationGateStillFailsClosedForUnknownStatesAfterPendingWasAdded(t *testing.T) {
	for _, state := range []string{"", "unknown", "migration-pending", "MIGRATION-PENDING-OPERATOR-REVIEW"} {
		status := project.NewMigrationGate(project.MigrationStatus{State: state}).Status()
		if status.State != project.MigrationStateBlocked {
			t.Fatalf("state %q = %q, want it folded into %q", state, status.State, project.MigrationStateBlocked)
		}
		if status.Continuation != "hive project identity status" {
			t.Fatalf("state %q continuation = %q, want the failure continuation", state, status.Continuation)
		}
	}
}
