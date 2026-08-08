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
		if blocked.Status.State != project.MigrationStateBlocked || blocked.Status.Reason == "" || blocked.Status.Continuation == "" {
			t.Fatalf("blocked status = %#v, want structured fail-closed status", blocked.Status)
		}
	}
}
