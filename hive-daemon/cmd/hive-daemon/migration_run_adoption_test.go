package main

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

// TestStartupSettlesAFoldInterruptedByTheDaemonStopping covers the wiring, not the
// storage: the adoption itself is tested in internal/db, and what this pins is that
// startup actually performs it. Without the call, a daemon killed mid-fold would
// serve a progress record claiming forever that a fold is still running, and the
// operator would wait for a result that can never arrive.
func TestStartupSettlesAFoldInterruptedByTheDaemonStopping(t *testing.T) {
	_, store := openStartupTestDB(t)
	ctx := context.Background()
	if err := store.SaveProjectMigrationRun(ctx, db.ProjectMigrationRun{
		PlanFingerprint: "fingerprint-1",
		Outcome:         db.ProjectMigrationRunRunning,
		Phase:           db.ProjectMigrationPhaseRekey,
		StartedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	adoptInterruptedMigrationRuns(ctx, store)

	run, found, err := store.LatestProjectMigrationRun(ctx)
	if err != nil || !found {
		t.Fatalf("latest run = %v, %v", found, err)
	}
	if run.Outcome != db.ProjectMigrationRunFailed || run.Reason != db.ProjectMigrationReasonInterrupted {
		t.Fatalf("run = %+v, want a failed record naming the interruption", run)
	}
	if !run.Retryable {
		t.Fatal("retryable = false; the interrupted fold rolled back and can be approved again")
	}
}
