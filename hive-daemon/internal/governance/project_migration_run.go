package governance

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

var (
	// ErrProjectMigrationNotPending means there is nothing to approve: the gate is
	// serving, so the preflight already settled whatever there was.
	ErrProjectMigrationNotPending = errors.New("project migration is not waiting for an operator decision")
	// ErrProjectMigrationConfirmationMismatch means the echoed phrase is not the
	// one this daemon derives from the plan it is holding.
	ErrProjectMigrationConfirmationMismatch = errors.New("project migration confirmation phrase does not match the plan")
	// ErrProjectMigrationAlreadyRunning means a fold is in flight. It is not a
	// fault: the honest answer is "watch the progress you already have".
	ErrProjectMigrationAlreadyRunning = errors.New("project migration is already running")
)

// ProjectMigrationRunner owns the operator-approved identity fold: it transports
// the plan, validates the approval, runs the fold in the background, and keeps the
// progress readable.
//
// The fold runs detached from the request that asked for it because it rebuilds
// tables and reindexes: a request that waited for it would outlive any client
// timeout, and the operator would lose the only channel that could tell them what
// happened. Progress is therefore the contract, not a nicety.
type ProjectMigrationRunner struct {
	store   *db.DB
	backups *BackupStore
	gate    *project.MigrationGate

	mu   sync.Mutex
	live *db.ProjectMigrationRun

	// failpoint aborts the transaction at its last safe point. Test-only, and the
	// only way to exercise the rollback path without corrupting a real database.
	failpoint func() error
}

func NewProjectMigrationRunner(store *db.DB, backups *BackupStore, gate *project.MigrationGate) *ProjectMigrationRunner {
	return &ProjectMigrationRunner{store: store, backups: backups, gate: gate}
}

// MigrationPlan re-reads the preflight rather than serving whatever startup saw.
// The operator reviews what is on disk now, and the fingerprint they carry back is
// checked against a plan read the same way, so an approval can never apply to a
// plan the database no longer has.
func (r *ProjectMigrationRunner) MigrationPlan(ctx context.Context) (db.ProjectMigrationPreflight, error) {
	return db.ReadProjectMigrationPreflight(ctx, r.store)
}

// MigrationProgress answers from memory while a fold is in flight and from SQLite
// otherwise.
//
// The in-memory path is not a cache optimization, it is the only thing that works:
// the fold holds the single write transaction on the one pooled connection, so a
// progress read that queried the database would block until the fold finished and
// report nothing during the only window where it mattered.
func (r *ProjectMigrationRunner) MigrationProgress(ctx context.Context) (db.ProjectMigrationRun, bool, error) {
	r.mu.Lock()
	live := r.live
	r.mu.Unlock()
	if live != nil {
		return *live, true, nil
	}
	return r.store.LatestProjectMigrationRun(ctx)
}

// ExecuteMigration validates the approval and hands the fold to a background
// goroutine, returning as soon as the run is durably marked as started.
func (r *ProjectMigrationRunner) ExecuteMigration(ctx context.Context, req project.MigrationExecuteRequest) error {
	if r.gate == nil || r.gate.Status().State != project.MigrationStatePendingOperatorReview {
		return ErrProjectMigrationNotPending
	}
	preflight, err := r.MigrationPlan(ctx)
	if err != nil {
		return err
	}
	if !preflight.NeedsOperatorReview() {
		// The database resolved itself since the gate was installed — a peer daemon
		// folded it, or the rows went away. There is nothing left to approve.
		//
		// The gate is deliberately NOT opened here. Only a fold this runner actually
		// executed may open it; letting a refused request flip it would make the
		// closed gate defeatable by any caller posting to this route, and the
		// existing retry route already re-runs the startup preflight for a gate that
		// has genuinely gone stale.
		return ErrProjectMigrationNotPending
	}
	if req.PlanFingerprint == "" || req.PlanFingerprint != preflight.Plan.Fingerprint {
		return db.ErrProjectMigrationPlanStale
	}
	// Derived, never accepted: the comparison is against this daemon's own value
	// for the plan it just read. The TUI normalizes whitespace locally and sends
	// the canonical phrase, so an exact comparison is the right one.
	if req.Confirmation != db.ProjectMigrationConfirmation(preflight.Plan) {
		return ErrProjectMigrationConfirmationMismatch
	}
	if !preflight.Plan.Executable || len(preflight.Plan.Conflicts) != 0 {
		return db.ErrProjectMigrationPlanUnsafe
	}

	run := db.ProjectMigrationRun{
		PlanFingerprint: preflight.Plan.Fingerprint,
		Outcome:         db.ProjectMigrationRunRunning,
		Phase:           db.ProjectMigrationPhaseBackup,
		StartedAt:       time.Now().UTC(),
	}
	r.mu.Lock()
	if r.live != nil {
		r.mu.Unlock()
		return ErrProjectMigrationAlreadyRunning
	}
	r.live = &run
	r.mu.Unlock()

	// Persisted before the fold starts and while the connection is still free: the
	// transaction takes the only one, so this is the last chance to record that a
	// fold began at all. A process that dies mid-fold leaves this row behind, and
	// FailInterruptedProjectMigrationRuns adopts it on the next start.
	if err := r.store.SaveProjectMigrationRun(ctx, run); err != nil {
		r.mu.Lock()
		r.live = nil
		r.mu.Unlock()
		return err
	}

	// Detached from the request context on purpose: the caller is expected to
	// disconnect immediately, and cancelling a fold because the operator's poll
	// loop moved on would abort a transaction that was about to commit.
	go r.fold(context.WithoutCancel(ctx), preflight.Plan)
	return nil
}

func (r *ProjectMigrationRunner) fold(ctx context.Context, plan db.ProjectMigrationPlan) {
	r.store.SetProjectMigrationPhaseObserver(r.observePhase)
	defer r.store.SetProjectMigrationPhaseObserver(nil)

	err := executeProjectMigrationWithBackup(ctx, r.store, plan, r.backups, r.failpoint)

	run := r.snapshotLiveRun()
	run.FinishedAt = time.Now().UTC()
	run.BackupID = r.rollbackPointForPlan(ctx, plan.Fingerprint)
	if err == nil {
		run.Outcome = db.ProjectMigrationRunSucceeded
		run.Phase = db.ProjectMigrationPhaseCommit
		run.Summary = r.store.LastProjectMigrationSummary()
		// The fold is what the gate was waiting for, and the executor's own
		// post-commit checks already proved nothing is left to normalize.
		r.gate.Adopt(project.MigrationStatus{State: project.MigrationStateReady})
	} else {
		run.Outcome = db.ProjectMigrationRunFailed
		run.Detail = err.Error()
		// Contention is the one failure an operator must not react to: a peer
		// daemon re-planned the same database, nothing committed, and the answer is
		// to try again rather than to reach for the rollback.
		if db.IsProjectMigrationContention(err) {
			run.Reason = db.ProjectMigrationReasonContention
			run.Retryable = true
		} else {
			run.Reason = db.ProjectMigrationReasonFault
		}
	}

	// The terminal record is persisted before the live view is dropped, so a poll
	// can never observe the gap where neither answers.
	if saveErr := r.store.SaveProjectMigrationRun(ctx, run); saveErr != nil {
		logger.Log.Printf("could not persist project migration result: %v", saveErr)
	}
	r.mu.Lock()
	r.live = nil
	r.mu.Unlock()
}

func (r *ProjectMigrationRunner) observePhase(phase db.ProjectMigrationPhase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live == nil {
		return
	}
	updated := *r.live
	updated.Phase = phase
	r.live = &updated
}

func (r *ProjectMigrationRunner) snapshotLiveRun() db.ProjectMigrationRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live == nil {
		return db.ProjectMigrationRun{}
	}
	return *r.live
}

// rollbackPointForPlan reports the archive that can undo this exact plan. It is
// looked up rather than remembered because the executor takes or reuses the
// archive itself, and an unrelated older backup must never be offered as this
// fold's rollback.
func (r *ProjectMigrationRunner) rollbackPointForPlan(ctx context.Context, planFingerprint string) string {
	if r.backups == nil {
		return ""
	}
	backup, found, err := r.backups.MigrationBackupForPlan(ctx, planFingerprint)
	if err != nil || !found {
		return ""
	}
	return backup.ID
}
