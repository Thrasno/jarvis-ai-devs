package governance

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

// pendingFoldFixture is the database the wizard exists for: one project written
// under two spellings that fold to one canonical key, with the gate parked in the
// pending state the startup preflight installs.
func pendingFoldFixture(t *testing.T) (string, *db.DB, *BackupStore, *project.MigrationGate) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, spelling := range []string{"Foo.Bar", "foo-bar"} {
		if _, err := store.RawDB().Exec(
			`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES (?, ?, ?, 'dev', 'test')`,
			"session-"+spelling, "sync-"+spelling, spelling); err != nil {
			t.Fatal(err)
		}
	}
	backups := NewSQLiteBackupStore(path, "", store.RawDB())
	preflight, err := db.ReadProjectMigrationPreflight(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.NeedsOperatorReview() {
		t.Fatalf("preflight = %+v, want a plan waiting for the operator", preflight)
	}
	gate := project.NewMigrationGate(project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		PlanFingerprint: preflight.Plan.Fingerprint,
	})
	return path, store, backups, gate
}

func approvedRequest(t *testing.T, runner *ProjectMigrationRunner) project.MigrationExecuteRequest {
	t.Helper()
	preflight, err := runner.MigrationPlan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return project.MigrationExecuteRequest{
		PlanFingerprint: preflight.Plan.Fingerprint,
		Confirmation:    db.ProjectMigrationConfirmation(preflight.Plan),
	}
}

func awaitTerminalRun(t *testing.T, runner *ProjectMigrationRunner) db.ProjectMigrationRun {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		run, found, err := runner.MigrationProgress(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if found && run.Outcome != db.ProjectMigrationRunRunning {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("fold never reached a terminal outcome")
	return db.ProjectMigrationRun{}
}

func sessionProjects(t *testing.T, store *db.DB) []string {
	t.Helper()
	rows, err := store.RawDB().Query(`SELECT project FROM sessions ORDER BY project`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var projects []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, value)
	}
	return projects
}

// TestExecutingAFoldOnAReadyGateIsRefused keeps the fold to the one state that
// asked for it. A ready gate means the preflight already settled everything, so a
// fold request there is a stale wizard, not an instruction.
func TestExecutingAFoldOnAReadyGateIsRefused(t *testing.T) {
	_, store, backups, _ := pendingFoldFixture(t)
	ready := project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady})
	runner := NewProjectMigrationRunner(store, backups, ready)
	err := runner.ExecuteMigration(context.Background(), project.MigrationExecuteRequest{
		PlanFingerprint: "whatever", Confirmation: "whatever",
	})
	if !errors.Is(err, ErrProjectMigrationNotPending) {
		t.Fatalf("execute = %v, want ErrProjectMigrationNotPending", err)
	}
}

// TestExecutingAFoldWithAStaleFingerprintIsRefusedDistinctly is what lets the
// TUI say "the plan you reviewed is out of date" instead of "error". The
// fingerprint is the operator's evidence that the plan on screen is the plan on
// disk, and a mismatch has its own recoverable meaning: re-read and review again.
func TestExecutingAFoldWithAStaleFingerprintIsRefusedDistinctly(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	request := approvedRequest(t, runner)
	request.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
	err := runner.ExecuteMigration(context.Background(), request)
	if !errors.Is(err, db.ErrProjectMigrationPlanStale) {
		t.Fatalf("execute = %v, want the stale-plan outcome", err)
	}
	if run, found, _ := runner.MigrationProgress(context.Background()); found {
		t.Fatalf("progress = %+v, want no run recorded for a refused request", run)
	}
}

// TestExecutingAFoldWithTheWrongConfirmationIsRefused pins the phrase as a real
// guard rather than decoration. The daemon derives it and compares against its
// own value, so a client that invents or replays a phrase cannot fold anything.
func TestExecutingAFoldWithTheWrongConfirmationIsRefused(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	// Empty, a well-formed phrase carrying somebody else's fingerprint, the wrong
	// noun agreement, and the right words in the wrong case: each is close enough
	// to pass a sloppy comparison, and none of them is the derived phrase.
	for _, confirmation := range []string{
		"", "NORMALIZE 1 PROJECT deadbeef", "NORMALIZE 1 PROJECTS deadbeef", "normalize 1 project",
	} {
		request := approvedRequest(t, runner)
		request.Confirmation = confirmation
		if err := runner.ExecuteMigration(context.Background(), request); !errors.Is(err, ErrProjectMigrationConfirmationMismatch) {
			t.Fatalf("execute with %q = %v, want ErrProjectMigrationConfirmationMismatch", confirmation, err)
		}
	}
}

// TestASecondConcurrentFoldIsRefusedAsAlreadyRunning distinguishes "your click
// arrived twice" from "the fold failed". Both would otherwise surface as one
// generic error and send the operator looking for a rollback that is not needed.
func TestASecondConcurrentFoldIsRefusedAsAlreadyRunning(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	release := make(chan struct{})
	runner.failpoint = func() error {
		<-release
		return nil
	}
	request := approvedRequest(t, runner)
	if err := runner.ExecuteMigration(context.Background(), request); err != nil {
		t.Fatalf("first execute = %v, want accepted", err)
	}
	if err := runner.ExecuteMigration(context.Background(), request); !errors.Is(err, ErrProjectMigrationAlreadyRunning) {
		close(release)
		t.Fatalf("second execute = %v, want ErrProjectMigrationAlreadyRunning", err)
	}
	close(release)
	awaitTerminalRun(t, runner)
}

// TestProgressIsReadableWhileTheFoldRunsAndAfterTheCallerReconnects is the whole
// point of running the fold in the background: the HTTP request returns
// immediately, so the only way the operator learns anything is by polling.
//
// It also proves the live view does not depend on the caller's connection or on
// touching SQLite: the migration holds the single write transaction while this
// reads, so a progress read that queried the database would block until the fold
// finished and report nothing while it mattered.
func TestProgressIsReadableWhileTheFoldRunsAndAfterTheCallerReconnects(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	reached := make(chan struct{})
	release := make(chan struct{})
	runner.failpoint = func() error {
		close(reached)
		<-release
		return nil
	}
	if err := runner.ExecuteMigration(context.Background(), approvedRequest(t, runner)); err != nil {
		t.Fatalf("execute = %v", err)
	}
	select {
	case <-reached:
	case <-time.After(30 * time.Second):
		t.Fatal("fold never reached the failpoint")
	}
	running, found, err := runner.MigrationProgress(context.Background())
	if err != nil || !found {
		t.Fatalf("progress = %v, %v, want a live run", found, err)
	}
	if running.Outcome != db.ProjectMigrationRunRunning {
		t.Fatalf("outcome = %q, want running", running.Outcome)
	}
	if running.Phase == "" {
		t.Fatal("phase = empty; a progress view with no phase says nothing")
	}
	if running.StartedAt.IsZero() {
		t.Fatal("started_at = zero")
	}
	close(release)

	final := awaitTerminalRun(t, runner)
	if final.Outcome != db.ProjectMigrationRunSucceeded {
		t.Fatalf("outcome = %q reason=%q detail=%q, want succeeded", final.Outcome, final.Reason, final.Detail)
	}
	if final.FinishedAt.IsZero() {
		t.Fatal("finished_at = zero on a terminal run")
	}
}

// TestAnApprovedFoldUnifiesTheSpellingsAndOpensTheGateWithoutARestart is the
// success path end to end. The gate has to open in place: hive-daemon is spawned
// by an MCP client, so telling the operator to restart the daemon after approving
// their own fold means telling them to restart their editor session.
func TestAnApprovedFoldUnifiesTheSpellingsAndOpensTheGateWithoutARestart(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	if err := runner.ExecuteMigration(context.Background(), approvedRequest(t, runner)); err != nil {
		t.Fatalf("execute = %v", err)
	}
	run := awaitTerminalRun(t, runner)
	if run.Outcome != db.ProjectMigrationRunSucceeded {
		t.Fatalf("outcome = %q reason=%q detail=%q, want succeeded", run.Outcome, run.Reason, run.Detail)
	}
	if projects := sessionProjects(t, store); len(projects) != 2 || projects[0] != "foo-bar" || projects[1] != "foo-bar" {
		t.Fatalf("session projects = %v, want both folded onto the canonical key", projects)
	}
	if run.Summary.RowsRekeyed == 0 {
		t.Fatalf("summary = %+v, want the rekey counter the executor produced", run.Summary)
	}
	if run.Phase != db.ProjectMigrationPhaseCommit {
		t.Fatalf("phase = %q, want the last phase the transaction ran", run.Phase)
	}
	if run.BackupID == "" {
		t.Fatal("backup id = empty; the mandatory pre-mutation archive must be reportable as a rollback point")
	}
	if _, err := backups.ValidateArchive(context.Background(), run.BackupID); err != nil {
		t.Fatalf("validate reported backup: %v", err)
	}
	if err := gate.Check(); err != nil {
		t.Fatalf("gate = %v, want ready in place after a successful fold", err)
	}
	if gate.Status().State != project.MigrationStateReady {
		t.Fatalf("gate state = %q, want ready", gate.Status().State)
	}
	if err := store.CreateSession("post-fold", "foo-bar", "", "dev", "test"); err != nil {
		t.Fatalf("write after a successful fold: %v", err)
	}
}

// TestAFailedFoldLeavesTheDatabaseUnmutatedAndTheGateClosed keeps the failure
// honest: the transaction rolls back, so nothing about the operator's projects
// changed, the gate must not have opened, and the archive has to still be there.
func TestAFailedFoldLeavesTheDatabaseUnmutatedAndTheGateClosed(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	runner.failpoint = func() error { return errors.New("injected executor fault") }
	if err := runner.ExecuteMigration(context.Background(), approvedRequest(t, runner)); err != nil {
		t.Fatalf("execute = %v", err)
	}
	run := awaitTerminalRun(t, runner)
	if run.Outcome != db.ProjectMigrationRunFailed {
		t.Fatalf("outcome = %q, want failed", run.Outcome)
	}
	if run.Reason != db.ProjectMigrationReasonFault {
		t.Fatalf("reason = %q, want %q", run.Reason, db.ProjectMigrationReasonFault)
	}
	if run.Retryable {
		t.Fatal("retryable = true; an injected fault is not contention")
	}
	if run.Detail == "" {
		t.Fatal("detail = empty; the operator needs to know what failed")
	}
	if run.BackupID == "" {
		t.Fatal("backup id = empty; a failed fold must still offer its rollback point")
	}
	if _, err := backups.ValidateArchive(context.Background(), run.BackupID); err != nil {
		t.Fatalf("validate retained backup: %v", err)
	}
	if projects := sessionProjects(t, store); len(projects) != 2 || projects[0] != "Foo.Bar" || projects[1] != "foo-bar" {
		t.Fatalf("session projects = %v, want both spellings exactly as the operator wrote them", projects)
	}
	if gate.Check() == nil {
		t.Fatal("gate = ready after a failed fold, want it still closed")
	}
	if gate.Status().State != project.MigrationStatePendingOperatorReview {
		t.Fatalf("gate state = %q, want the pending state preserved", gate.Status().State)
	}
	// Every write the transaction owed must have rolled back with it, including
	// the canonical registry population — the first statement inside the
	// transaction, and therefore the one that proves the rollback reached the
	// beginning rather than only the end.
	var registered int
	if err := store.RawDB().QueryRow(`SELECT COUNT(*) FROM project_identities`).Scan(&registered); err != nil || registered != 0 {
		t.Fatalf("project_identities rows = %d, %v; want the rollback to have undone the registry population", registered, err)
	}
}

// TestContentionIsReportedAsRetryableRatherThanAsAFault keeps the one failure an
// operator must NOT react to out of the alarm path. hive-daemon runs one process
// per MCP session, so a peer can re-plan the same database underneath this fold;
// nothing is broken and the answer is "try again", not "restore a backup".
func TestContentionIsReportedAsRetryableRatherThanAsAFault(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	runner.failpoint = func() error { return db.ErrProjectMigrationPlanStale }
	if err := runner.ExecuteMigration(context.Background(), approvedRequest(t, runner)); err != nil {
		t.Fatalf("execute = %v", err)
	}
	run := awaitTerminalRun(t, runner)
	if run.Outcome != db.ProjectMigrationRunFailed {
		t.Fatalf("outcome = %q, want failed", run.Outcome)
	}
	if run.Reason != db.ProjectMigrationReasonContention {
		t.Fatalf("reason = %q, want %q", run.Reason, db.ProjectMigrationReasonContention)
	}
	if !run.Retryable {
		t.Fatal("retryable = false; contention is exactly the failure worth retrying")
	}
	if gate.Check() == nil {
		t.Fatal("gate = ready after a fold that never committed")
	}
}

// TestAFoldThatResetsASyncPositionSaysSoInTheResult surfaces the merge that
// looks like data loss and is not. The operator sees the project re-pull its full
// window on the next sync; without this line in the result they have no way to
// know the fold did it on purpose.
func TestAFoldThatResetsASyncPositionSaysSoInTheResult(t *testing.T) {
	_, store, backups, gate := pendingFoldFixture(t)
	if _, err := store.RawDB().Exec(
		`INSERT INTO sync_state (project, last_sync_at, last_attempt_at) VALUES ('Foo.Bar', NULL, '2026-03-02 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RawDB().Exec(
		`INSERT INTO sync_state (project, last_sync_at, last_attempt_at) VALUES ('foo-bar', '2026-02-01 00:00:00', '2026-02-02 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	// The plan changed with the new rows, so the gate has to describe what the
	// operator would actually be approving.
	preflight, err := db.ReadProjectMigrationPreflight(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	gate.Adopt(project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		PlanFingerprint: preflight.Plan.Fingerprint,
	})
	runner := NewProjectMigrationRunner(store, backups, gate)
	if err := runner.ExecuteMigration(context.Background(), approvedRequest(t, runner)); err != nil {
		t.Fatalf("execute = %v", err)
	}
	run := awaitTerminalRun(t, runner)
	if run.Outcome != db.ProjectMigrationRunSucceeded {
		t.Fatalf("outcome = %q reason=%q detail=%q, want succeeded", run.Outcome, run.Reason, run.Detail)
	}
	if len(run.Summary.SyncPositionsReset) != 1 || run.Summary.SyncPositionsReset[0] != "foo-bar" {
		t.Fatalf("sync positions reset = %v, want [foo-bar]", run.Summary.SyncPositionsReset)
	}
}

// TestATerminalRunIsReadableAfterTheDaemonRestarts covers the shape of this
// product: one daemon process per MCP client session, so the process that ran the
// fold is routinely gone by the time the operator looks at the result.
func TestATerminalRunIsReadableAfterTheDaemonRestarts(t *testing.T) {
	path, store, backups, gate := pendingFoldFixture(t)
	runner := NewProjectMigrationRunner(store, backups, gate)
	if err := runner.ExecuteMigration(context.Background(), approvedRequest(t, runner)); err != nil {
		t.Fatalf("execute = %v", err)
	}
	before := awaitTerminalRun(t, runner)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	if err := restarted.FailInterruptedProjectMigrationRuns(context.Background()); err != nil {
		t.Fatalf("adopt interrupted runs on restart: %v", err)
	}
	after := NewProjectMigrationRunner(restarted, NewSQLiteBackupStore(path, "", restarted.RawDB()),
		project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady}))
	run, found, err := after.MigrationProgress(context.Background())
	if err != nil || !found {
		t.Fatalf("progress after restart = %v, %v, want the persisted result", found, err)
	}
	if run.Outcome != before.Outcome || run.Summary.RowsRekeyed != before.Summary.RowsRekeyed {
		t.Fatalf("restarted run = %+v, want the same result as %+v", run, before)
	}
	if run.BackupID != before.BackupID {
		t.Fatalf("backup id = %q, want %q", run.BackupID, before.BackupID)
	}
}
