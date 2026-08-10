package project_test

import (
	"errors"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

func TestMigrationGateFailsClosedForEmptyAndUnknownStatus(t *testing.T) {
	for _, status := range []project.MigrationStatus{{}, {State: "unknown"}} {
		gate := project.NewMigrationGate(status)
		err := gate.Check()
		var blocked *project.MigrationBlockedError
		if !errors.As(err, &blocked) {
			t.Fatalf("Check() error = %v, want MigrationBlockedError", err)
		}
		if blocked.Status.State != project.MigrationStateBlocked || blocked.Status.Reason == "" || blocked.Status.Continuation != "hive project identity status" {
			t.Fatalf("blocked status = %#v, want structured fail-closed status", blocked.Status)
		}
	}
}

func TestMigrationGateReplacesNonExecutableContinuation(t *testing.T) {
	gate := project.NewMigrationGate(project.MigrationStatus{
		State:        project.MigrationStateBlocked,
		Continuation: "hive project identity resolve then retry",
	})
	if got := gate.Status().Continuation; got != "hive project identity status" {
		t.Fatalf("Continuation = %q, want one executable status command", got)
	}
}
