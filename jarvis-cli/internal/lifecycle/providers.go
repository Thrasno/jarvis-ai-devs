package lifecycle

import (
	"os"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

type ProviderAdapter interface {
	Name() string
	Observe() (ObservedProviderState, error)
	Apply([]DoctorStep) error
	BackupTargets([]DoctorStep) ([]BackupTarget, error)
	Restore(BackupManifest) (int, error)
}

type fakeProviderAdapter struct {
	name       string
	observed   ObservedProviderState
	observeErr error
	applyErr   error
	restoreErr error
	applyCalls int
	applyStages []string
	verifyCalls int
	applyNonOwnedCount int
	restoreWrites int
	appliedAssets []string
	backupTargetPaths []string
}

func (f *fakeProviderAdapter) Name() string {
	return f.name
}

func (f *fakeProviderAdapter) Observe() (ObservedProviderState, error) {
	f.verifyCalls++
	if f.observeErr != nil {
		return ObservedProviderState{}, f.observeErr
	}
	return f.observed, nil
}

func (f *fakeProviderAdapter) Apply(steps []DoctorStep) error {
	f.applyCalls++
	if f.applyErr != nil {
		return f.applyErr
	}
	for _, step := range steps {
		f.appliedAssets = append(f.appliedAssets, step.AssetID)
		if step.SafetyClass == "non-owned" {
			f.applyNonOwnedCount++
		}
	}
	if f.observed.Artifacts == nil {
		f.observed.Artifacts = map[string]sddruntime.ObservedArtifact{}
	}
	f.observed.Artifacts["instructions"] = sddruntime.ObservedArtifact{Exists: true, MarkersValid: true}
	f.observed.Artifacts["orchestrator"] = sddruntime.ObservedArtifact{Exists: true}
	f.observed.Artifacts["skills"] = sddruntime.ObservedArtifact{Exists: true}
	f.observed.NonOwnedChanges = nil
	f.observed.UnknownChanges = nil
	return nil
}

func (f *fakeProviderAdapter) BackupTargets(steps []DoctorStep) ([]BackupTarget, error) {
	targets := make([]BackupTarget, 0, len(steps))
	for _, step := range steps {
		tmp, err := os.CreateTemp("", "jarvis-lifecycle-backup-*")
		if err != nil {
			return nil, err
		}
		if err := tmp.Close(); err != nil {
			return nil, err
		}
		targets = append(targets, BackupTarget{Path: tmp.Name()})
		f.backupTargetPaths = append(f.backupTargetPaths, step.AssetID)
	}
	return targets, nil
}

func (f *fakeProviderAdapter) Restore(manifest BackupManifest) (int, error) {
	if f.restoreErr != nil {
		return 0, f.restoreErr
	}
	f.restoreWrites += len(manifest.Entries)
	return len(manifest.Entries), nil
}
