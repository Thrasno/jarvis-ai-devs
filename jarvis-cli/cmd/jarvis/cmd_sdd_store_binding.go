package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
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
	openSpecSource := sddstatus.NewOpenSpecSource(workspace)
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
func runBoundSddArchive(ctx context.Context, root, destination, projectFlag, changeFlag string) error {
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
	openSpecSource := sddstatus.NewOpenSpecSource(coordinates.workspace)
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
		return archiveOpenSpecWithBinding(ctx, openSpecSource, coordinates, destination, sddruntime.StoreModeOpenSpec)
	case sddruntime.StoreModeHybrid:
		if err := requireOpenSpecArchiveCoordinates(coordinates.root, destination); err != nil {
			return err
		}
		return archiveHybridWithBinding(ctx, hiveSource, openSpecSource, coordinates, destination)
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

func archiveOpenSpecWithBinding(ctx context.Context, source sddstatus.ArtifactSource, coordinates boundSddArchiveCoordinates, destination string, mode sddruntime.StoreMode) error {
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
	return (sddprogress.OpenSpec{Root: coordinates.root}).ArchiveWithLifecycleValidation(destination, validate)
}

func archiveHybridWithBinding(ctx context.Context, hiveSource *sddstatus.HiveSource, openSpecSource *sddstatus.OpenSpecSource, coordinates boundSddArchiveCoordinates, destination string) error {
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
	return (sddprogress.OpenSpec{Root: coordinates.root}).ArchiveWithLifecycleValidation(destination, validate)
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
