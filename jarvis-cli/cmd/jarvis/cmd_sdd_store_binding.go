package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddspecsync"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

// resolveBoundProgressStore selects a writer only after the immutable binding
// for the request coordinates has been read or safely adopted. It never treats
// a binding, protocol, listing, or lock error as an absent binding.
func resolveBoundProgressStore(ctx context.Context, root, project, change string) (progressAdvancer, error) {
	if err := validateProgressBindingCoordinates(project, change); err != nil {
		return nil, err
	}
	workspace, openSpecRoot, err := progressBindingWorkspace(root, change)
	if err != nil {
		return nil, err
	}
	hc, err := hiveclient.NewFromEnv()
	if err != nil {
		return nil, fmt.Errorf("connect to hive-daemon: %w", err)
	}
	binding, err := resolveSddStoreBindingAt(ctx, hc, project, change, workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve SDD store binding: %w", err)
	}
	return progressStoreForBindingWithContext(ctx, binding.Mode, openSpecRoot)
}

// resolveSddStoreBindingAt is the shared binding resolver for status, continue,
// and progress mutations. The caller supplies an already selected workspace so
// binding coordinates never select an unrelated local change directory.
func resolveSddStoreBindingAt(ctx context.Context, hc *hiveclient.Client, project, change, workspace string) (sddbinding.Resolution, error) {
	if hc == nil {
		return sddbinding.Resolution{}, errors.New("Hive binding client is required")
	}
	hiveSource := sddstatus.NewHiveSource(hc, project)
	openSpecSource := sddstatus.NewOpenSpecSourceForProject(workspace, project)
	resolver := sddbinding.LegacyResolver{
		HiveBindings:      hc,
		HiveSource:        hiveSource,
		OpenSpecSource:    openSpecSource,
		OpenSpecChangeDir: filepath.Join(workspace, "openspec", "changes", change),
		ResolutionLockDir: workspace,
	}
	return resolver.ResolveAndAdopt(ctx, project, change, initialStoreSelection())
}

func validateProgressBindingCoordinates(project, change string) error {
	if project != strings.TrimSpace(project) || project != hiveclient.CanonicalProjectKey(project) || change != strings.TrimSpace(change) || !applyprogress.ValidID(project) || !applyprogress.ValidID(change) {
		return errors.New("SDD progress request project and change must be canonical non-empty identifiers")
	}
	if filepath.Base(change) != change || change == "." {
		return errors.New("SDD progress request change must be a single canonical coordinate")
	}
	return nil
}

// progressBindingWorkspace validates a supplied OpenSpec change root against
// the request coordinate. With no root, it retains the current workspace as a
// lock root so a pure Hive operation neither creates nor depends on OpenSpec.
func progressBindingWorkspace(root, change string) (workspace, openSpecRoot string, err error) {
	if root == "" {
		workingDir, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		workspace, err = canonicalSddWorkspaceDirectory(workingDir)
		if err != nil {
			return "", "", fmt.Errorf("validate workspace: %w", err)
		}
		return workspace, filepath.Join(workspace, "openspec", "changes", change), nil
	}
	if root != strings.TrimSpace(root) {
		return "", "", errors.New("SDD progress root must be canonical without surrounding whitespace")
	}

	openSpecRoot, err = canonicalSddWorkspaceDirectory(root)
	if err != nil {
		return "", "", fmt.Errorf("validate canonical OpenSpec change root: %w", err)
	}
	if root != openSpecRoot {
		return "", "", errors.New("SDD progress root must be an absolute canonical path without aliases")
	}
	workspace = filepath.Dir(filepath.Dir(filepath.Dir(openSpecRoot)))
	if openSpecRoot != filepath.Join(workspace, "openspec", "changes", change) {
		return "", "", errors.New("SDD progress root must be the canonical OpenSpec change root for the request change")
	}
	return workspace, openSpecRoot, nil
}

// progressStoreForBinding preserves the env-only factory seam for focused unit
// tests. Product command paths must use progressStoreForBindingWithContext.
func progressStoreForBinding(mode sddruntime.StoreMode, openSpecRoot string) (progressAdvancer, error) {
	return progressStoreForBindingWithFactory(mode, openSpecRoot, newProgressHive)
}

func progressStoreForBindingWithContext(ctx context.Context, mode sddruntime.StoreMode, openSpecRoot string) (progressAdvancer, error) {
	return progressStoreForBindingWithFactory(mode, openSpecRoot, func() (progressAdvancer, error) {
		return newProgressHiveWithContext(ctx)
	})
}

func progressStoreForBindingWithFactory(mode sddruntime.StoreMode, openSpecRoot string, newHive func() (progressAdvancer, error)) (progressAdvancer, error) {
	switch mode {
	case sddruntime.StoreModeHive:
		return newHive()
	case sddruntime.StoreModeOpenSpec:
		return newProgressOpenSpec(openSpecRoot), nil
	case sddruntime.StoreModeHybrid:
		hive, err := newHive()
		if err != nil {
			return nil, err
		}
		return sddprogress.Hybrid{Root: openSpecRoot, OpenSpec: newProgressOpenSpec(openSpecRoot), Hive: hive}, nil
	case sddruntime.StoreModeNone:
		return nil, errors.New("SDD progress store is disabled")
	default:
		return nil, fmt.Errorf("unsupported SDD progress store binding %q", mode)
	}
}

type boundSddArchiveCoordinates struct {
	workspace string
	root      string
	project   string
	change    string
}

// runBoundSddArchive resolves the immutable binding before choosing archive
// behavior. Hive archive is a logical, idempotent close; OpenSpec remains the
// sole filesystem mover.
func runBoundSddArchive(ctx context.Context, root, destination, projectFlag, changeFlag string, syncOptions archiveSpecSyncOptions) error {
	coordinates, err := resolveBoundSddArchiveCoordinates(root, projectFlag, changeFlag)
	if err != nil {
		return err
	}

	hc, err := hiveclient.NewFromEnv()
	if err != nil {
		return fmt.Errorf("connect to hive-daemon: %w", err)
	}
	binding, err := resolveSddStoreBindingAt(ctx, hc, coordinates.project, coordinates.change, coordinates.workspace)
	if err != nil {
		return fmt.Errorf("resolve SDD store binding: %w", err)
	}

	hiveSource := sddstatus.NewHiveSource(hc, coordinates.project)
	openSpecSource := sddstatus.NewOpenSpecSourceForProject(coordinates.workspace, coordinates.project)
	switch binding.Mode {
	case sddruntime.StoreModeHive:
		status, contents, err := boundArchiveStatus(ctx, hiveSource, coordinates, sddruntime.StoreModeHive)
		if err != nil {
			return err
		}
		return validateHiveArchiveStatus(status, contents)
	case sddruntime.StoreModeOpenSpec:
		if err := requireOpenSpecArchiveCoordinates(coordinates.root, destination); err != nil {
			return err
		}
		return archiveOpenSpecWithBinding(ctx, openSpecSource, coordinates, destination, sddruntime.StoreModeOpenSpec, syncOptions)
	case sddruntime.StoreModeHybrid:
		if err := requireOpenSpecArchiveCoordinates(coordinates.root, destination); err != nil {
			return err
		}
		return archiveHybridWithBinding(ctx, hiveSource, openSpecSource, coordinates, destination, syncOptions)
	case sddruntime.StoreModeNone:
		return errors.New("sdd archive is blocked because the resolved store binding is none")
	default:
		return fmt.Errorf("unsupported SDD archive store binding %q", binding.Mode)
	}
}

func resolveBoundSddArchiveCoordinates(root, projectFlag, changeFlag string) (boundSddArchiveCoordinates, error) {
	var coordinates boundSddArchiveCoordinates
	if root != strings.TrimSpace(root) {
		return coordinates, errors.New("SDD archive root must be canonical without surrounding whitespace")
	}

	if root != "" {
		canonicalRoot, err := canonicalSddWorkspaceDirectory(root)
		if err != nil {
			return coordinates, fmt.Errorf("validate canonical OpenSpec change root: %w", err)
		}
		if root != canonicalRoot {
			return coordinates, errors.New("SDD archive root must be an absolute canonical path without aliases")
		}
		workspace := filepath.Dir(filepath.Dir(filepath.Dir(canonicalRoot)))
		change := filepath.Base(canonicalRoot)
		if canonicalRoot != filepath.Join(workspace, "openspec", "changes", change) {
			return coordinates, errors.New("sdd archive requires a canonical OpenSpec change root")
		}
		if changeFlag != "" {
			if err := validateSddArchiveCoordinate(changeFlag, "change"); err != nil {
				return coordinates, err
			}
			if changeFlag != change {
				return coordinates, errors.New("SDD archive --change must match the canonical OpenSpec root coordinate")
			}
		}
		coordinates.workspace, coordinates.root, coordinates.change = workspace, canonicalRoot, change
	} else {
		if err := validateSddArchiveCoordinate(changeFlag, "change"); err != nil {
			return coordinates, err
		}
		workingDir, err := os.Getwd()
		if err != nil {
			return coordinates, err
		}
		workspace, err := canonicalSddWorkspaceDirectory(workingDir)
		if err != nil {
			return coordinates, fmt.Errorf("validate workspace: %w", err)
		}
		coordinates.workspace, coordinates.change = workspace, changeFlag
	}

	if projectFlag != "" {
		if err := validateSddArchiveCoordinate(projectFlag, "project"); err != nil {
			return coordinates, err
		}
		coordinates.project = projectFlag
	} else {
		project, err := resolveSddProject("", coordinates.workspace)
		if err != nil {
			return coordinates, err
		}
		coordinates.project = project
	}
	if err := validateSddArchiveCoordinate(coordinates.project, "project"); err != nil {
		return boundSddArchiveCoordinates{}, err
	}
	return coordinates, nil
}

func validateSddArchiveCoordinate(value, name string) error {
	if value == "" || value != strings.TrimSpace(value) || !applyprogress.ValidID(value) {
		return fmt.Errorf("SDD archive %s must be a canonical non-empty identifier", name)
	}
	if name == "project" && value != hiveclient.CanonicalProjectKey(value) {
		return errors.New("SDD archive project must use the canonical Hive project key")
	}
	if name == "change" && (filepath.Base(value) != value || value == ".") {
		return errors.New("SDD archive change must be a single canonical coordinate")
	}
	return nil
}

func requireOpenSpecArchiveCoordinates(root, destination string) error {
	if root == "" || destination == "" {
		return errors.New("sdd archive requires --root and --destination for OpenSpec and hybrid bindings")
	}
	return nil
}

type boundArchiveView struct {
	artifacts map[string]sddstatus.ArtifactState
	contents  map[string]string
	status    *sddstatus.ChangeStatus
}

func captureBoundArchiveView(ctx context.Context, source sddstatus.ArtifactSource, coordinates boundSddArchiveCoordinates, mode sddruntime.StoreMode) (boundArchiveView, error) {
	artifacts, contents, err := source.FetchArtifacts(ctx, coordinates.change)
	if err != nil {
		return boundArchiveView{}, fmt.Errorf("fetch %s archive artifacts: %w", mode, err)
	}
	status := sddstatus.ComputeStatus(coordinates.change, string(mode), sddstatus.Input{
		Artifacts:        artifacts,
		Contents:         contents,
		AllowedEditRoots: []string{coordinates.workspace},
	})
	status.ChangeRoot = coordinates.root
	return boundArchiveView{artifacts: artifacts, contents: contents, status: status}, nil
}

func boundArchiveStatus(ctx context.Context, source sddstatus.ArtifactSource, coordinates boundSddArchiveCoordinates, mode sddruntime.StoreMode) (*sddstatus.ChangeStatus, map[string]string, error) {
	view, err := captureBoundArchiveView(ctx, source, coordinates, mode)
	if err != nil {
		return nil, nil, err
	}
	return view.status, view.contents, nil
}

func validateHiveArchiveStatus(status *sddstatus.ChangeStatus, contents map[string]string) error {
	if status.Artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactDone ||
		status.Artifacts[sddstatus.ArtifactVerifyReport] != sddstatus.ArtifactDone ||
		status.Artifacts[sddstatus.ArtifactArchiveReport] != sddstatus.ArtifactDone ||
		strings.TrimSpace(contents[sddstatus.ArtifactVerifyReport]) == "" ||
		strings.TrimSpace(contents[sddstatus.ArtifactArchiveReport]) == "" ||
		(status.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepReady && status.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepAllDone) {
		return errors.New("sdd archive blocked until Hive progress, verification, and archive report are complete")
	}
	return nil
}

// archiveSpecSyncOptions selects how archive treats the OpenSpec spec sync.
type archiveSpecSyncOptions struct {
	planOnly           bool
	confirmDestructive bool
	out                io.Writer
}

// openArchiveStore is the OpenSpec archive mover; tests replace it to inject
// move failures.
var openArchiveStore = func(root string) sddprogress.OpenSpec { return sddprogress.OpenSpec{Root: root} }

type archiveOutput struct {
	Outcome     string          `json:"outcome"`
	Code        string          `json:"code,omitempty"`
	Detail      string          `json:"detail,omitempty"`
	Recovery    string          `json:"recovery,omitempty"`
	Destination string          `json:"destination,omitempty"`
	SpecSync    *specSyncOutput `json:"spec_sync,omitempty"`
}

type specSyncOutput struct {
	Destructive     bool                     `json:"destructive"`
	Synced          bool                     `json:"synced"`
	Reverted        bool                     `json:"reverted,omitempty"`
	Targets         []specSyncTargetOutput   `json:"targets"`
	Removals        []specSyncRemovalOutput  `json:"removals,omitempty"`
	RecoveryTargets []specSyncRecoveryOutput `json:"recovery_targets,omitempty"`
}

type specSyncTargetOutput struct {
	Capability      string                 `json:"capability"`
	Path            string                 `json:"path"`
	DeltaPath       string                 `json:"delta_path"`
	Action          string                 `json:"action"`
	BeforeDigest    string                 `json:"before_digest"`
	AfterDigest     string                 `json:"after_digest"`
	Added           []string               `json:"added,omitempty"`
	Modified        []string               `json:"modified,omitempty"`
	Removed         []string               `json:"removed,omitempty"`
	Renamed         []specSyncRenameOutput `json:"renamed,omitempty"`
	IgnoredSections []string               `json:"ignored_sections,omitempty"`
}

type specSyncRenameOutput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type specSyncRemovalOutput struct {
	Capability   string   `json:"capability"`
	Requirements []string `json:"requirements"`
}

type specSyncRecoveryOutput struct {
	Path           string `json:"path"`
	ExpectedDigest string `json:"expected_digest"`
}

func archiveOpenSpecWithBinding(ctx context.Context, source sddstatus.ArtifactSource, coordinates boundSddArchiveCoordinates, destination string, mode sddruntime.StoreMode, syncOptions archiveSpecSyncOptions) error {
	validate := func() error {
		status, _, err := boundArchiveStatus(ctx, source, coordinates, mode)
		if err != nil {
			return err
		}
		return validateSddArchiveStatus(status, destination)
	}
	if err := validate(); err != nil {
		return err
	}
	return runArchiveWithSpecSync(coordinates, destination, validate, syncOptions)
}

func archiveHybridWithBinding(ctx context.Context, hiveSource *sddstatus.HiveSource, openSpecSource *sddstatus.OpenSpecSource, coordinates boundSddArchiveCoordinates, destination string, syncOptions archiveSpecSyncOptions) error {
	validate := func() error {
		hive, err := captureBoundArchiveView(ctx, hiveSource, coordinates, sddruntime.StoreModeHive)
		if err != nil {
			return err
		}
		openSpec, err := captureBoundArchiveView(ctx, openSpecSource, coordinates, sddruntime.StoreModeOpenSpec)
		if err != nil {
			return err
		}
		return validateHybridArchiveViews(hive, openSpec, destination)
	}
	if err := validate(); err != nil {
		return err
	}
	return runArchiveWithSpecSync(coordinates, destination, validate, syncOptions)
}

const (
	archiveOutcomeArchived = "archived"
	archiveOutcomePlanned  = "planned"
	archiveOutcomeBlocked  = "blocked"

	// codeArchiveRecoveryRequired reports an archive move that failed or could
	// not be made durable after the spec sync was applied.
	codeArchiveRecoveryRequired = "archive_recovery_required"

	archiveSpecsRoot = "openspec/specs"
)

var specSyncRecovery = map[string]string{
	sddspecsync.CodeConfirmationRequired:     "The spec sync removes the listed requirements. Show the removals to the user once and rerun the same command with --confirm-destructive only after explicit confirmation. Nothing was written or moved.",
	sddspecsync.CodeMergeBlocked:             "Fix the delta spec (or main spec) named in detail, then rerun archive. Nothing was written or moved; never hand-edit main specs.",
	sddspecsync.CodePlanFailed:               "Spec sync could not read the delta or main specs. Fix the cause in detail, then rerun archive. Nothing was written or moved.",
	sddspecsync.CodeConflictRecoveryRequired: "A main spec changed while archive was syncing; nothing was overwritten and the change was not moved. Review the concurrent edit, then rerun archive.",
	sddspecsync.CodeWriteFailed:              "A main spec write failed and every written spec was restored; the change was not moved. Fix the cause in detail, then rerun archive.",
	sddspecsync.CodeRecoveryRequired:         "Main specs could not be verifiably restored. Restore each recovery_targets path to its expected_digest (absent means the file must not exist) without overwriting other edits, then rerun archive.",
}

// archiveSpecSync merges the change's delta specs into openspec/specs as the
// archive pre-move step, so the merge and the topology rename share the
// archive lock. It records the JSON result for runArchiveWithSpecSync.
type archiveSpecSync struct {
	workspace  string
	changeRoot string
	options    archiveSpecSyncOptions
	ran        bool
	moveFailed bool
	revertErr  error
	output     archiveOutput
}

// errArchivePlanOnly stops a --plan archive after the plan is recorded.
var errArchivePlanOnly = errors.New("sdd archive plan only")

func runArchiveWithSpecSync(coordinates boundSddArchiveCoordinates, destination string, validate func() error, options archiveSpecSyncOptions) error {
	sync := &archiveSpecSync{
		workspace:  coordinates.workspace,
		changeRoot: "openspec/changes/" + coordinates.change,
		options:    options,
	}
	err := openArchiveStore(coordinates.root).ArchiveWithPreMoveStep(destination, validate, sync.step)
	return sync.finish(err, destination)
}

func (a *archiveSpecSync) step() (func() error, error) {
	a.ran = true
	plan, err := sddspecsync.BuildPlan(os.DirFS(a.workspace), a.changeRoot, archiveSpecsRoot)
	if err != nil {
		code := sddspecsync.CodePlanFailed
		var mergeErr *sddspecsync.MergeError
		if errors.As(err, &mergeErr) {
			code = mergeErr.Code()
		}
		a.block(code, err)
		return nil, err
	}
	a.output.SpecSync = specSyncPlanOutput(plan)
	if a.options.planOnly {
		return nil, errArchivePlanOnly
	}
	if plan.Destructive() && !a.options.confirmDestructive {
		err := errors.New("spec sync removes requirements and needs --confirm-destructive")
		a.block(sddspecsync.CodeConfirmationRequired, err)
		return nil, err
	}
	store := sddspecsync.OSStore{Root: a.workspace}
	if _, err := sddspecsync.Apply(plan, store); err != nil {
		a.blockApply(err)
		return nil, err
	}
	a.output.SpecSync.Synced = true
	return func() error {
		a.moveFailed = true
		a.revertErr = sddspecsync.Revert(plan, store)
		return a.revertErr
	}, nil
}

func (a *archiveSpecSync) block(code string, err error) {
	a.output.Outcome = archiveOutcomeBlocked
	a.output.Code = code
	a.output.Detail = err.Error()
	a.output.Recovery = specSyncRecovery[code]
}

// blockApply records an Apply or Revert failure with its recovery targets.
func (a *archiveSpecSync) blockApply(err error) {
	code := sddspecsync.CodeWriteFailed
	var applyErr *sddspecsync.ApplyError
	if errors.As(err, &applyErr) {
		code = applyErr.Code()
		for _, target := range applyErr.Recovery {
			a.output.SpecSync.RecoveryTargets = append(a.output.SpecSync.RecoveryTargets, specSyncRecoveryOutput{Path: target.Path, ExpectedDigest: target.ExpectedDigest})
		}
	}
	a.block(code, err)
}

// finish prints the JSON result once spec sync was reached and maps the
// archive error. Earlier gate failures keep their historical plain errors.
func (a *archiveSpecSync) finish(archiveErr error, destination string) error {
	if !a.ran {
		return archiveErr
	}
	switch {
	case errors.Is(archiveErr, errArchivePlanOnly):
		a.output.Outcome = archiveOutcomePlanned
		archiveErr = nil
	case archiveErr == nil:
		a.output.Outcome = archiveOutcomeArchived
		a.output.Destination = destination
	case a.output.Code != "":
		// The step itself blocked; its code and recovery are already recorded.
	case a.moveFailed && a.revertErr != nil:
		a.blockApply(a.revertErr)
		a.output.Detail = archiveErr.Error()
	case a.moveFailed:
		a.output.SpecSync.Reverted = true
		a.block(codeArchiveRecoveryRequired, archiveErr)
		a.output.Recovery = "The archive move failed after spec sync; main specs were restored to their pre-sync bytes and the change was not moved. Fix the move error in detail, then rerun archive."
	default:
		a.block(codeArchiveRecoveryRequired, archiveErr)
		a.output.Destination = destination
		a.output.Recovery = "Main specs were synced and the change was moved, but the move could not be made durable. Verify the destination topology and the main spec digests in spec_sync before continuing."
	}
	encoder := json.NewEncoder(a.options.out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(a.output); err != nil {
		return errors.Join(archiveErr, fmt.Errorf("write archive result: %w", err))
	}
	if archiveErr != nil {
		return fmt.Errorf("sdd archive blocked: %s: %w", a.output.Code, archiveErr)
	}
	return nil
}

func specSyncPlanOutput(plan sddspecsync.Plan) *specSyncOutput {
	output := &specSyncOutput{Destructive: plan.Destructive(), Targets: []specSyncTargetOutput{}}
	for _, t := range plan.Targets {
		action := "updated"
		if !t.Existed {
			action = "created"
		}
		target := specSyncTargetOutput{
			Capability:      t.Capability,
			Path:            t.MainPath,
			DeltaPath:       t.DeltaPath,
			Action:          action,
			BeforeDigest:    t.BeforeDigest,
			AfterDigest:     t.AfterDigest,
			Added:           t.Changes.Added,
			Modified:        t.Changes.Modified,
			Removed:         t.Changes.Removed,
			IgnoredSections: t.Changes.IgnoredSections,
		}
		for _, r := range t.Changes.Renamed {
			target.Renamed = append(target.Renamed, specSyncRenameOutput{From: r.From, To: r.To})
		}
		output.Targets = append(output.Targets, target)
		if len(t.Changes.Removed) > 0 {
			output.Removals = append(output.Removals, specSyncRemovalOutput{Capability: t.Capability, Requirements: t.Changes.Removed})
		}
	}
	return output
}

func validateHybridArchiveViews(hive, openSpec boundArchiveView, destination string) error {
	if err := validateArchiveReportsMatch(hive.status, hive.contents, openSpec.status, openSpec.contents); err != nil {
		return err
	}
	if err := validateHiveArchiveStatus(hive.status, hive.contents); err != nil {
		return err
	}
	if err := validateSddArchiveStatus(openSpec.status, destination); err != nil {
		return err
	}
	if !sddstatus.LegacyProgressEquivalent(archiveProgressObservation(hive), archiveProgressObservation(openSpec)) {
		return errors.New("sdd archive blocked: Hive and OpenSpec protected progress are not equivalent")
	}
	return nil
}

func archiveProgressObservation(view boundArchiveView) sddstatus.LegacyProgressObservation {
	state, present := view.artifacts[sddstatus.ArtifactApplyProgress]
	return sddstatus.LegacyProgressObservation{
		Present: present,
		State:   state,
		Content: view.contents[sddstatus.ArtifactApplyProgress],
	}
}

func validateArchiveReportsMatch(hiveStatus *sddstatus.ChangeStatus, hiveContents map[string]string, openSpecStatus *sddstatus.ChangeStatus, openSpecContents map[string]string) error {
	for _, candidate := range []struct {
		name     string
		status   *sddstatus.ChangeStatus
		contents map[string]string
	}{
		{name: "Hive", status: hiveStatus, contents: hiveContents},
		{name: "OpenSpec", status: openSpecStatus, contents: openSpecContents},
	} {
		if candidate.status.Artifacts[sddstatus.ArtifactArchiveReport] != sddstatus.ArtifactDone || strings.TrimSpace(candidate.contents[sddstatus.ArtifactArchiveReport]) == "" {
			return fmt.Errorf("sdd archive blocked: %s archive-report must be done and non-empty", candidate.name)
		}
	}
	if hiveContents[sddstatus.ArtifactArchiveReport] != openSpecContents[sddstatus.ArtifactArchiveReport] {
		return errors.New("sdd archive blocked: Hive and OpenSpec archive-report contents differ")
	}
	return nil
}
