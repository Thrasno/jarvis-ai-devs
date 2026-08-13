// This file puts the pre-apply backup in front of replay, and makes that
// ordering the only way to reach the applier.
//
// Replay is destructive by design: WriteInstructions discards a managed
// instruction file that carries no Jarvis sentinels and renders it fresh
// (agent/claude.go:350-356, agent/opencode.go:445-452). That behaviour is only
// defensible because the archive taken here holds the previous bytes, so the
// backup is a hard precondition rather than a courtesy. Run is therefore the
// entry point a command uses; Apply stays available for tests that drive the
// component order directly.
package sync

import (
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
)

// BackupSourceOperation labels every snapshot replay takes, so a recovering user
// can tell a sync archive from a reconcile or restore one in ~/.jarvis/backups.
const BackupSourceOperation = "sync"

// ErrNoBackup refuses a replay pass that was handed no way to take a backup.
// Failing closed matters more than convenience here: a nil seam would otherwise
// read as "no backup needed" and silently mutate.
var ErrNoBackup = errors.New("replay requires a backup before it may mutate anything")

// SnapshotCreator is the pre-apply backup seam, satisfied by
// lifecycle.BackupStore.CreateSnapshotOfTargets.
type SnapshotCreator func(sourceOperation string, targets []lifecycle.BackupTarget) (lifecycle.BackupManifest, error)

// RunInput is a replay pass with its safety machinery attached.
type RunInput struct {
	Plan   Plan
	Apply  ApplyInput
	Backup SnapshotCreator
}

// RunResult pairs the recovery point with the outcome it protects.
type RunResult struct {
	Backup lifecycle.BackupManifest
	Report Report
}

// BackupTargets projects the plan's tracked-path list onto backup targets.
//
// It reads Plan.Tracked and builds nothing of its own. That list has one
// producer and two consumers, the backup and the idempotency diff, so a path
// this run is responsible for cannot end up measured but unprotected.
func BackupTargets(plan Plan) []lifecycle.BackupTarget {
	targets := make([]lifecycle.BackupTarget, 0, len(plan.Tracked))
	for _, tracked := range plan.Tracked {
		targets = append(targets, lifecycle.BackupTarget{Path: tracked.Path})
	}
	return targets
}

// Run measures, protects, replays, then measures again.
//
// A backup failure returns before the applier is ever called, so the run mutates
// nothing at all and the cause is reported rather than downgraded to a warning.
// Partial protection is not on the menu: there is no per-agent or best-effort
// path through here. The same is true of the opening snapshot, which fails
// closed for the same reason: an unmeasurable path list cannot be reported on.
//
// The changed-path report is still measured after the fact rather than
// short-circuited before it, so an unchanged machine gets its files rewritten
// with identical bytes and what this guarantees is that the report says zero,
// truthfully. Every tracked path now carries the digest of the content replay
// would write, which is what a pre-apply comparison needs; consuming it here is
// the next step.
func Run(in RunInput) (RunResult, error) {
	if in.Backup == nil {
		return RunResult{}, ErrNoBackup
	}
	before, err := TakeSnapshot(in.Plan.Tracked)
	if err != nil {
		return RunResult{}, fmt.Errorf("measure %d tracked paths before replay: %w", len(in.Plan.Tracked), err)
	}
	manifest, err := in.Backup(BackupSourceOperation, BackupTargets(in.Plan))
	if err != nil {
		return RunResult{}, fmt.Errorf("back up %d tracked paths before replay: %w", len(in.Plan.Tracked), err)
	}
	result := RunResult{Backup: manifest, Report: Apply(in.Apply)}
	// Mode assertion is part of the mutation pass and runs before the closing
	// snapshot, so a mode a writer left behind is corrected rather than merely
	// reported as drift on every run.
	if err := EnforceModes(in.Plan.Tracked); err != nil {
		return result, err
	}
	after, err := TakeSnapshot(in.Plan.Tracked)
	if err != nil {
		return result, fmt.Errorf("measure %d tracked paths after replay: %w", len(in.Plan.Tracked), err)
	}
	result.Report = attributeChanges(result.Report, in.Plan.Tracked, Diff(before, after))
	return result, nil
}
