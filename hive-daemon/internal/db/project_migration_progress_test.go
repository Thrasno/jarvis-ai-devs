package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openProgressTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// TestProjectMigrationPhasesMatchTheExecutorOrder pins the phase vocabulary to
// what the transaction really does. A progress view that names a phase the
// executor never enters, or orders them differently from the code, tells an
// operator a story about a fold that is not happening.
func TestProjectMigrationPhasesMatchTheExecutorOrder(t *testing.T) {
	want := []ProjectMigrationPhase{
		ProjectMigrationPhaseBackup,
		ProjectMigrationPhaseRevalidate,
		ProjectMigrationPhaseRegistry,
		ProjectMigrationPhaseCompositeCursors,
		ProjectMigrationPhaseSyncStateCoalesce,
		ProjectMigrationPhasePropagationEnqueue,
		ProjectMigrationPhaseRekey,
		ProjectMigrationPhaseOwnershipRebuild,
		ProjectMigrationPhaseRebuildState,
		ProjectMigrationPhaseCommit,
	}
	got := ProjectMigrationPhases()
	if len(got) != len(want) {
		t.Fatalf("phases = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phase %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestProjectMigrationRunSurvivesReopeningTheDatabase is the durability
// requirement: the caller that started the fold may disconnect, and the daemon
// itself is one process per MCP client session, so a terminal result that only
// lived in memory would be unreadable exactly when the operator comes back for it.
func TestProjectMigrationRunSurvivesReopeningTheDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Truncate(time.Second)
	finished := started.Add(2 * time.Second)
	run := ProjectMigrationRun{
		PlanFingerprint: "fingerprint-1",
		Outcome:         ProjectMigrationRunSucceeded,
		Phase:           ProjectMigrationPhaseCommit,
		StartedAt:       started,
		FinishedAt:      finished,
		BackupID:        "backup-1",
		Summary: ProjectMigrationSummary{
			Ran: true, RowsRekeyed: 7, SessionsRequeued: 2, PromptsRequeued: 3, ReprojectsEnqueued: 4,
			SyncPositionsReset: []string{"foo-bar"},
		},
	}
	if err := database.SaveProjectMigrationRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, found, err := reopened.LatestProjectMigrationRun(context.Background())
	if err != nil || !found {
		t.Fatalf("latest run = %v, %v, want the persisted run", found, err)
	}
	if got.Outcome != ProjectMigrationRunSucceeded || got.Phase != ProjectMigrationPhaseCommit {
		t.Fatalf("outcome/phase = %q/%q, want succeeded/commit", got.Outcome, got.Phase)
	}
	if got.PlanFingerprint != "fingerprint-1" || got.BackupID != "backup-1" {
		t.Fatalf("fingerprint/backup = %q/%q, want fingerprint-1/backup-1", got.PlanFingerprint, got.BackupID)
	}
	if !got.StartedAt.Equal(started) || !got.FinishedAt.Equal(finished) {
		t.Fatalf("timestamps = %s/%s, want %s/%s", got.StartedAt, got.FinishedAt, started, finished)
	}
	if got.Summary.RowsRekeyed != 7 || got.Summary.SessionsRequeued != 2 ||
		got.Summary.PromptsRequeued != 3 || got.Summary.ReprojectsEnqueued != 4 {
		t.Fatalf("summary = %+v, want every counter round-tripped", got.Summary)
	}
	if len(got.Summary.SyncPositionsReset) != 1 || got.Summary.SyncPositionsReset[0] != "foo-bar" {
		t.Fatalf("sync positions reset = %v, want [foo-bar]", got.Summary.SyncPositionsReset)
	}
}

// TestSavingProjectMigrationRunReplacesThePreviousOne keeps the progress record
// a single latest-run row. The TUI polls "what is the fold doing now", and a
// history table would make that question ambiguous without buying anything.
func TestSavingProjectMigrationRunReplacesThePreviousOne(t *testing.T) {
	database := openProgressTestDB(t)
	ctx := context.Background()
	if err := database.SaveProjectMigrationRun(ctx, ProjectMigrationRun{
		PlanFingerprint: "fingerprint-1", Outcome: ProjectMigrationRunRunning,
		Phase: ProjectMigrationPhaseBackup, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveProjectMigrationRun(ctx, ProjectMigrationRun{
		PlanFingerprint: "fingerprint-1", Outcome: ProjectMigrationRunFailed,
		Phase: ProjectMigrationPhaseRekey, StartedAt: time.Now().UTC(),
		FinishedAt: time.Now().UTC(), Reason: ProjectMigrationReasonFault, Retryable: false,
	}); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := database.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_migration_runs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("stored rows = %d, want exactly the latest run", rows)
	}
	got, _, err := database.LatestProjectMigrationRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != ProjectMigrationRunFailed || got.Reason != ProjectMigrationReasonFault {
		t.Fatalf("latest run = %+v, want the failed terminal record", got)
	}
}

// TestNoProjectMigrationRunIsNotAnError separates "never asked for a fold" from
// a failure. A TUI that cannot tell those apart shows an alarming empty result
// screen on a database that is simply fine.
func TestNoProjectMigrationRunIsNotAnError(t *testing.T) {
	_, found, err := openProgressTestDB(t).LatestProjectMigrationRun(context.Background())
	if err != nil || found {
		t.Fatalf("latest run = %v, %v, want absent and no error", found, err)
	}
}

// TestInterruptedProjectMigrationRunIsAdoptedAsFailed closes the one gap a
// persisted running row leaves: the daemon can die mid-fold, and the row it left
// behind would otherwise claim forever that a fold is in flight. The transaction
// rolled back with the process, so the honest terminal state is a retryable
// failure that names the interruption.
func TestInterruptedProjectMigrationRunIsAdoptedAsFailed(t *testing.T) {
	database := openProgressTestDB(t)
	ctx := context.Background()
	if err := database.SaveProjectMigrationRun(ctx, ProjectMigrationRun{
		PlanFingerprint: "fingerprint-1", Outcome: ProjectMigrationRunRunning,
		Phase: ProjectMigrationPhaseRekey, StartedAt: time.Now().UTC(), BackupID: "backup-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.FailInterruptedProjectMigrationRuns(ctx); err != nil {
		t.Fatalf("adopt interrupted run: %v", err)
	}
	got, found, err := database.LatestProjectMigrationRun(ctx)
	if err != nil || !found {
		t.Fatalf("latest run = %v, %v", found, err)
	}
	if got.Outcome != ProjectMigrationRunFailed {
		t.Fatalf("outcome = %q, want failed", got.Outcome)
	}
	if got.Reason != ProjectMigrationReasonInterrupted {
		t.Fatalf("reason = %q, want %q", got.Reason, ProjectMigrationReasonInterrupted)
	}
	if !got.Retryable {
		t.Fatal("retryable = false; an interrupted fold rolled back and can be run again")
	}
	if got.FinishedAt.IsZero() {
		t.Fatal("finished_at = zero; an adopted run is terminal")
	}
	if got.BackupID != "backup-1" {
		t.Fatalf("backup id = %q, want the rollback point kept", got.BackupID)
	}
}

// TestAdoptingInterruptedRunsLeavesTerminalRunsAlone keeps the adoption from
// rewriting a result the operator may already have read.
func TestAdoptingInterruptedRunsLeavesTerminalRunsAlone(t *testing.T) {
	database := openProgressTestDB(t)
	ctx := context.Background()
	if err := database.SaveProjectMigrationRun(ctx, ProjectMigrationRun{
		PlanFingerprint: "fingerprint-1", Outcome: ProjectMigrationRunSucceeded,
		Phase: ProjectMigrationPhaseCommit, StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.FailInterruptedProjectMigrationRuns(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := database.LatestProjectMigrationRun(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != ProjectMigrationRunSucceeded || got.Reason != "" {
		t.Fatalf("run = %+v, want the successful record untouched", got)
	}
}

// TestProjectMigrationConfirmationIsDerivedFromThePlan locks the phrase to the
// plan. It must change when the plan changes — otherwise a phrase the operator
// copied from an older review would authorize a fold they never saw — and it must
// state how many projects are being unified so the phrase is not a magic word.
func TestProjectMigrationConfirmationIsDerivedFromThePlan(t *testing.T) {
	first := BuildProjectMigrationPlan([]ProjectStateRecord{
		{Table: ProjectStateSessions, Project: "Foo.Bar", Identity: "a", StableID: "a"},
		{Table: ProjectStateSessions, Project: "foo-bar", Identity: "b", StableID: "b"},
	})
	phrase := ProjectMigrationConfirmation(first)
	if phrase == "" {
		t.Fatal("confirmation = empty")
	}
	if want := "NORMALIZE 1 PROJECT " + first.Fingerprint[:projectMigrationConfirmationDigits]; phrase != want {
		t.Fatalf("confirmation = %q, want %q", phrase, want)
	}
	second := BuildProjectMigrationPlan([]ProjectStateRecord{
		{Table: ProjectStateSessions, Project: "Foo.Bar", Identity: "a", StableID: "a"},
		{Table: ProjectStateSessions, Project: "foo-bar", Identity: "b", StableID: "b"},
		{Table: ProjectStateMemories, Project: "Baz.Qux", Identity: "c", StableID: "c"},
		{Table: ProjectStateMemories, Project: "baz-qux", Identity: "d", StableID: "d"},
	})
	if other := ProjectMigrationConfirmation(second); other == phrase {
		t.Fatalf("confirmation = %q for a different plan, want a phrase bound to the plan", other)
	}
}

// TestProjectMigrationConfirmationReadsAsNormalization keeps the phrase in the
// product's own words. The operator has to READ this before typing it, and every
// other surface calls this normalization — the menu entry, the continuation
// string. "FOLD" is our internal word for the operation, and "1 PROJECTS" reads
// like a bug in the very screen asking for a destructive-looking confirmation.
func TestProjectMigrationConfirmationReadsAsNormalization(t *testing.T) {
	for _, tt := range []struct {
		name    string
		records []ProjectStateRecord
		want    string
	}{
		{
			name: "one project is singular",
			records: []ProjectStateRecord{
				{Table: ProjectStateSessions, Project: "Foo.Bar", Identity: "a", StableID: "a"},
				{Table: ProjectStateSessions, Project: "foo-bar", Identity: "b", StableID: "b"},
			},
			want: "NORMALIZE 1 PROJECT ",
		},
		{
			name: "several projects are plural",
			records: []ProjectStateRecord{
				{Table: ProjectStateSessions, Project: "Foo.Bar", Identity: "a", StableID: "a"},
				{Table: ProjectStateSessions, Project: "foo-bar", Identity: "b", StableID: "b"},
				{Table: ProjectStateMemories, Project: "Baz.Qux", Identity: "c", StableID: "c"},
				{Table: ProjectStateMemories, Project: "baz-qux", Identity: "d", StableID: "d"},
				{Table: ProjectStateSyncState, Project: "Zed.Zed", Identity: "canonical-project", Value: "1", StableID: "e"},
				{Table: ProjectStateSyncState, Project: "zed-zed", Identity: "canonical-project", Value: "1", StableID: "f"},
			},
			want: "NORMALIZE 3 PROJECTS ",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan := BuildProjectMigrationPlan(tt.records)
			want := tt.want + plan.Fingerprint[:projectMigrationConfirmationDigits]
			if got := ProjectMigrationConfirmation(plan); got != want {
				t.Fatalf("confirmation = %q, want %q", got, want)
			}
		})
	}
}
