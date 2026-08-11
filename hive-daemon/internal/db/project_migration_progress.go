package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ProjectMigrationPhase names one step of the fold transaction. Discrete phases,
// never a percentage: the executor's steps have wildly different costs (a REINDEX
// against a table rebuild), so any percentage would be a number the daemon cannot
// honour and an operator would time their patience against.
type ProjectMigrationPhase string

const (
	ProjectMigrationPhaseBackup ProjectMigrationPhase = "backup"
	// ProjectMigrationPhaseRevalidate covers the second fingerprint check, the one
	// taken inside the transaction. The first is outside it and cannot exclude a
	// peer writing between the read and the BEGIN.
	ProjectMigrationPhaseRevalidate ProjectMigrationPhase = "revalidate"
	ProjectMigrationPhaseRegistry   ProjectMigrationPhase = "registry"
	// ProjectMigrationPhaseCompositeCursors covers both composite rekeys — the
	// cursor tables and the governance composites — because they are one
	// uninterruptible run of key coalescing with no operator-visible boundary
	// between them.
	ProjectMigrationPhaseCompositeCursors   ProjectMigrationPhase = "composite-cursors"
	ProjectMigrationPhaseSyncStateCoalesce  ProjectMigrationPhase = "sync-state-coalesce"
	ProjectMigrationPhasePropagationEnqueue ProjectMigrationPhase = "propagation-enqueue"
	ProjectMigrationPhaseRekey              ProjectMigrationPhase = "rekey"
	ProjectMigrationPhaseOwnershipRebuild   ProjectMigrationPhase = "ownership-rebuild"
	ProjectMigrationPhaseRebuildState       ProjectMigrationPhase = "rebuild-state"
	ProjectMigrationPhaseCommit             ProjectMigrationPhase = "commit"
)

// ProjectMigrationPhases returns the phases in the order ExecuteProjectMigration
// really runs them, so a progress view can lay out the whole sequence before the
// first one starts instead of growing a list as it goes.
func ProjectMigrationPhases() []ProjectMigrationPhase {
	return []ProjectMigrationPhase{
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
}

// ProjectMigrationRunOutcome is the terminal-or-not state of one approved fold.
type ProjectMigrationRunOutcome string

const (
	// ProjectMigrationRunNone is never persisted. It is the outcome a reader
	// projects when no fold was ever requested, so "nothing happened" cannot be
	// mistaken for "it failed and told us nothing".
	ProjectMigrationRunNone      ProjectMigrationRunOutcome = "no-run"
	ProjectMigrationRunRunning   ProjectMigrationRunOutcome = "running"
	ProjectMigrationRunSucceeded ProjectMigrationRunOutcome = "succeeded"
	ProjectMigrationRunFailed    ProjectMigrationRunOutcome = "failed"
)

// Machine-readable failure reasons. The point of the vocabulary is that each one
// implies a different next step, so a caller can act without parsing prose:
// contention means try again, interrupted means the fold rolled back with the
// process, fault means look at the detail and consider the rollback point.
const (
	ProjectMigrationReasonContention  = "contention"
	ProjectMigrationReasonInterrupted = "interrupted"
	ProjectMigrationReasonFault       = "fault"
)

// ProjectMigrationRun is the durable record of one approved fold.
//
// It is persisted rather than held in memory because both ends of the interaction
// are disposable: the caller is an HTTP client that may disconnect the moment it
// posted, and the daemon itself is one process per MCP client session. A result
// that lived only in the process that produced it would be unreadable exactly when
// the operator came back for it.
type ProjectMigrationRun struct {
	PlanFingerprint string
	Outcome         ProjectMigrationRunOutcome
	Phase           ProjectMigrationPhase
	StartedAt       time.Time
	FinishedAt      time.Time
	// Reason is one of the machine-readable constants above; Detail is the raw
	// error text, which is for a human to read and never for a caller to branch on.
	Reason    string
	Detail    string
	Retryable bool
	BackupID  string
	Summary   ProjectMigrationSummary
}

// projectMigrationRunRowID pins the progress record to a single row.
//
// Only the latest fold is ever answered — the TUI asks "what is happening now, or
// what happened last" — and a history table would have to answer "which run?"
// without anything asking the question. A CHECK-constrained single row makes the
// upsert trivial and the read unambiguous.
const projectMigrationRunRowID = 1

const projectMigrationRunsSchema = `CREATE TABLE IF NOT EXISTS project_migration_runs (
	id               INTEGER PRIMARY KEY CHECK (id = 1),
	plan_fingerprint TEXT NOT NULL,
	outcome          TEXT NOT NULL,
	phase            TEXT NOT NULL DEFAULT '',
	started_at       DATETIME NOT NULL,
	finished_at      DATETIME,
	reason           TEXT NOT NULL DEFAULT '',
	detail           TEXT NOT NULL DEFAULT '',
	retryable        INTEGER NOT NULL DEFAULT 0,
	backup_id        TEXT NOT NULL DEFAULT '',
	summary_json     TEXT NOT NULL DEFAULT '{}'
)`

// SaveProjectMigrationRun records the fold's current or final state, replacing
// whatever the previous run left behind.
func (d *DB) SaveProjectMigrationRun(ctx context.Context, run ProjectMigrationRun) error {
	summary, err := json.Marshal(run.Summary)
	if err != nil {
		return fmt.Errorf("encode project migration summary: %w", err)
	}
	_, err = d.sqlDB.ExecContext(ctx, `INSERT INTO project_migration_runs
	(id, plan_fingerprint, outcome, phase, started_at, finished_at, reason, detail, retryable, backup_id, summary_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		plan_fingerprint = excluded.plan_fingerprint,
		outcome = excluded.outcome,
		phase = excluded.phase,
		started_at = excluded.started_at,
		finished_at = excluded.finished_at,
		reason = excluded.reason,
		detail = excluded.detail,
		retryable = excluded.retryable,
		backup_id = excluded.backup_id,
		summary_json = excluded.summary_json`,
		projectMigrationRunRowID, run.PlanFingerprint, string(run.Outcome), string(run.Phase),
		formatMigrationRunTime(run.StartedAt), nullableMigrationRunTime(run.FinishedAt),
		run.Reason, run.Detail, run.Retryable, run.BackupID, string(summary))
	if err != nil {
		return fmt.Errorf("save project migration run: %w", err)
	}
	return nil
}

// LatestProjectMigrationRun reports the persisted fold record, or false when no
// fold was ever requested on this database.
func (d *DB) LatestProjectMigrationRun(ctx context.Context) (ProjectMigrationRun, bool, error) {
	var (
		run        ProjectMigrationRun
		outcome    string
		phase      string
		startedAt  string
		finishedAt sql.NullString
		summary    string
	)
	err := d.sqlDB.QueryRowContext(ctx, `SELECT plan_fingerprint, outcome, phase,
	CAST(started_at AS TEXT), CAST(finished_at AS TEXT), reason, detail, retryable, backup_id, summary_json
	FROM project_migration_runs WHERE id = ?`, projectMigrationRunRowID).Scan(
		&run.PlanFingerprint, &outcome, &phase, &startedAt, &finishedAt,
		&run.Reason, &run.Detail, &run.Retryable, &run.BackupID, &summary)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectMigrationRun{}, false, nil
	}
	if err != nil {
		return ProjectMigrationRun{}, false, fmt.Errorf("read project migration run: %w", err)
	}
	run.Outcome = ProjectMigrationRunOutcome(outcome)
	run.Phase = ProjectMigrationPhase(phase)
	run.StartedAt = parseMigrationRunTime(startedAt)
	if finishedAt.Valid {
		run.FinishedAt = parseMigrationRunTime(finishedAt.String)
	}
	if err := json.Unmarshal([]byte(summary), &run.Summary); err != nil {
		return ProjectMigrationRun{}, false, fmt.Errorf("decode project migration summary: %w", err)
	}
	return run, true, nil
}

// FailInterruptedProjectMigrationRuns adopts a running record left behind by a
// process that died mid-fold.
//
// The transaction rolled back with the process, so nothing was applied, but the
// row still claims a fold is in flight — and nothing would ever contradict it.
// Turning it into a retryable failure is the only honest reading, and it is why
// the daemon calls this once on startup rather than leaving the ambiguity for the
// operator to interpret.
func (d *DB) FailInterruptedProjectMigrationRuns(ctx context.Context) error {
	_, err := d.sqlDB.ExecContext(ctx, `UPDATE project_migration_runs
	SET outcome = ?, reason = ?, detail = ?, retryable = 1, finished_at = ?
	WHERE outcome = ?`,
		string(ProjectMigrationRunFailed), ProjectMigrationReasonInterrupted,
		"the daemon stopped before the fold committed; the transaction rolled back and nothing was applied",
		formatMigrationRunTime(time.Now().UTC()), string(ProjectMigrationRunRunning))
	if err != nil {
		return fmt.Errorf("adopt interrupted project migration run: %w", err)
	}
	return nil
}

func formatMigrationRunTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableMigrationRunTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return formatMigrationRunTime(value)
}

func parseMigrationRunTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

// projectMigrationConfirmationDigits is how much of the fingerprint the phrase
// carries. Enough that a phrase copied from a plan the operator no longer has on
// screen will not match the current one; short enough to retype by hand, which is
// the whole point of a confirmation phrase.
const projectMigrationConfirmationDigits = 8

// ProjectMigrationConfirmation derives the phrase that authorizes this exact
// plan.
//
// It is derived, never accepted as input: the daemon computes it from the plan it
// is about to execute and compares the operator's echo against its own value. A
// phrase supplied as the source of truth would authorize nothing — a client could
// send whatever it liked.
//
// The wording is the product's own: every other surface calls this normalization,
// including the menu entry the operator arrived from, so the phrase they have to
// read and retype says the same thing rather than exposing "fold" — an internal
// word for the operation. The noun agrees with the count, because a mismatch reads
// like a bug in the very screen asking for a destructive-looking confirmation.
//
// The count is the plan's own action count, i.e. the canonical keys this fold
// unifies, so the phrase states the size of what is being approved rather than
// being a magic word. The fingerprint prefix binds it to the plan: change the
// database and the phrase the operator was shown stops matching.
func ProjectMigrationConfirmation(plan ProjectMigrationPlan) string {
	fingerprint := plan.Fingerprint
	if len(fingerprint) > projectMigrationConfirmationDigits {
		fingerprint = fingerprint[:projectMigrationConfirmationDigits]
	}
	projects := len(plan.Actions)
	noun := "PROJECTS"
	if projects == 1 {
		noun = "PROJECT"
	}
	return "NORMALIZE " + strconv.Itoa(projects) + " " + noun + " " + fingerprint
}

// SetProjectMigrationPhaseObserver installs a live phase reporter for the next
// fold.
//
// It lives on the handle rather than in ExecuteProjectMigration's signature for
// the same reason LastProjectMigrationSummary does: the executor's parameters are
// load-bearing at every call site, and this is observability — a caller that
// installs nothing loses a progress view, not correctness.
//
// The observer must not touch this database. The fold holds the single write
// transaction on the one pooled connection, so a phase report that issued a query
// would deadlock against the transaction reporting it.
func (d *DB) SetProjectMigrationPhaseObserver(observe func(ProjectMigrationPhase)) {
	d.migrationPhaseMu.Lock()
	defer d.migrationPhaseMu.Unlock()
	d.migrationPhase = observe
}

func (d *DB) reportProjectMigrationPhase(phase ProjectMigrationPhase) {
	d.migrationPhaseMu.Lock()
	observe := d.migrationPhase
	d.migrationPhaseMu.Unlock()
	if observe != nil {
		observe(phase)
	}
}
