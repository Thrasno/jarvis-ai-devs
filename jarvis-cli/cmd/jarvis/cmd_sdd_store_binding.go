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
