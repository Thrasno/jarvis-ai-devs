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

// ErrUnprotectedAgent refuses a run whose two halves describe different work.
// The backup covers Plan.Tracked while the mutation covers Apply.Targets, and
// nothing about those two fields forces them to belong to the same run: a
// caller that plans one agent's paths and applies another's still mutates, and
// the archive holds none of what was overwritten. That is the archive's entire
// purpose defeated, so the pair is refused rather than reconciled.
var ErrUnprotectedAgent = errors.New("the plan tracks no path for an agent this run would mutate, so the backup cannot protect it")

// SnapshotCreator is the pre-apply backup seam, satisfied by
// lifecycle.BackupStore.CreateSnapshotOfTargets.
type SnapshotCreator func(sourceOperation string, targets []lifecycle.BackupTarget) (lifecycle.BackupManifest, error)

// RunInput is a replay pass with its safety machinery attached.
type RunInput struct {
	Plan   Plan
	Apply  ApplyInput
	Backup SnapshotCreator
	// Bookkeeping is optional: nil records nothing, which is what a caller
	// driving the applier directly wants.
	Bookkeeping *Bookkeeping
}

// RunResult pairs the recovery point with the outcome it protects.
type RunResult struct {
	Backup        lifecycle.BackupManifest
	Report        Report
	Verified      bool
	AddedSkillIDs []string
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
// The opening measurement also decides whether there is anything to do at all.
// Every tracked path carries the digest of the content replay would write, so a
// machine already holding that content and mode is converged before the applier
// is ever called and the run skips it. Zero changed files is not the same
// promise as zero writes: the components rewrite unconditionally, so measuring
// after the fact still removes and recreates every managed file on an unchanged
// machine. A short-circuited run takes no backup either, because the backup
// exists to make the destructive instruction writer defensible and nothing on
// this path mutates anything.
func Run(in RunInput) (RunResult, error) {
	if in.Backup == nil {
		return RunResult{}, ErrNoBackup
	}
	if err := protectsEveryTarget(in.Plan, in.Apply.Targets); err != nil {
		return RunResult{}, err
	}
	before, err := TakeSnapshot(in.Plan.Tracked)
	if err != nil {
		return RunResult{}, fmt.Errorf("measure %d tracked paths before replay: %w", len(in.Plan.Tracked), err)
	}
	if before.Matches(in.Plan.Tracked) {
		result := RunResult{Report: convergedWithoutApplying(in.Apply.Targets)}
		if err := verifyApplied(before, in.Plan.Tracked, in.Apply.Targets); err != nil {
			return result, err
		}
		result.Verified = true
		added, err := in.Bookkeeping.record(false, result.Report.Converged())
		if err != nil {
			return result, err
		}
		result.AddedSkillIDs = added
		return result, nil
	}
	manifest, err := in.Backup(BackupSourceOperation, BackupTargets(in.Plan))
	if err != nil {
		return RunResult{}, fmt.Errorf("back up %d tracked paths before replay: %w", len(in.Plan.Tracked), err)
	}
	result := RunResult{Backup: manifest, Report: Apply(in.Apply)}
	// Mode assertion is part of the mutation pass and runs before the closing
	// snapshot, so a mode a writer left behind is corrected rather than merely
	// reported as drift on every run.
	//
	// Neither failure below records anything. That reverses an earlier decision,
	// which wrote the record on both paths on the grounds that an unmeasured diff
	// is not evidence that nothing changed. That premise still holds -- the
	// applier already ran, and the report says so -- but the record is not a log
	// of attempts: ManagedAssetDigest is a claim that this machine holds the asset
	// set the digest names, and a run that never asserted a mode or never measured
	// what it produced has no evidence for that claim. Advancing it anyway lets the
	// digest run ahead of the machine, and the next run reads it back as convergence.
	//
	// The rule is therefore temporal, not a state: the digest is written only
	// after a sufficient final measurement, and never marked pending, failed or
	// attempted. A second persisted state would have to be interpreted, migrated
	// and kept honest by every later reader; leaving the previous digest in place
	// needs none of that, and the next run re-measures from scratch anyway.
	if err := EnforceModes(in.Plan.Tracked); err != nil {
		return result, err
	}
	after, err := TakeSnapshot(in.Plan.Tracked)
	if err != nil {
		return result, fmt.Errorf("measure %d tracked paths after replay: %w", len(in.Plan.Tracked), err)
	}
	changed := Diff(before, after)
	result.Report = attributeChanges(result.Report, in.Plan.Tracked, changed)
	if err := verifyApplied(after, in.Plan.Tracked, in.Apply.Targets); err != nil {
		return result, err
	}
	result.Verified = true
	if !result.Report.Converged() {
		return result, nil
	}
	added, err := in.Bookkeeping.record(len(changed) > 0, true)
	if err != nil {
		return result, err
	}
	result.AddedSkillIDs = added
	return result, nil
}

// protectsEveryTarget checks that the plan and the applier describe the same
// run, before anything is backed up or mutated.
//
// The check is agent ownership, and it reuses what the planner already recorded:
// every tracked path carries the agent it belongs to, so an agent about to be
// mutated with no tracked path of its own is an agent this backup cannot cover.
// No second identity is introduced for the pairing -- a run token or a plan ID
// would be one more thing to keep in step with the very lists it claims to
// bind. An empty target list mutates nothing and is therefore covered trivially.
func protectsEveryTarget(plan Plan, targets []AgentTarget) error {
	protected := make(map[string]bool, len(plan.Tracked))
	for _, tracked := range plan.Tracked {
		protected[tracked.Agent] = true
	}
	for _, target := range targets {
		if !protected[target.ID] {
			return fmt.Errorf("agent %q: %w", target.ID, ErrUnprotectedAgent)
		}
	}
	return nil
}

// convergedWithoutApplying is the honest report of a run that found nothing to
// do: every agent is already in its desired state and no path changed. The
// claim is measured, not assumed — it comes from comparing the machine against
// the plan's own desired digests.
func convergedWithoutApplying(targets []AgentTarget) Report {
	report := Report{Agents: make([]AgentResult, 0, len(targets)), Changed: []string{}}
	for _, target := range targets {
		// Completed is empty rather than the whole order: this run skipped the
		// applier, so no component executed. Convergence here is measured against
		// the plan's desired digests, not claimed from work that was never done.
		report.Agents = append(report.Agents, AgentResult{
			Agent: target.ID, Converged: true, Changed: []string{}, Completed: []string{},
		})
	}
	return report
}
