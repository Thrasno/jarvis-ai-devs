package lifecycle

import (
	"fmt"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

type EngineDeps struct {
	Adapters map[string]ProviderAdapter
	HomeDir  string
}

type Engine struct {
	adapters map[string]ProviderAdapter
	ledger   LedgerStore
	backups  BackupStore
}

func NewEngine(deps EngineDeps) *Engine {
	return &Engine{adapters: deps.Adapters, ledger: NewLedgerStore(deps.HomeDir), backups: NewBackupStore(deps.HomeDir)}
}

func (e *Engine) Verify(provider string) (VerifyResult, error) {
	adapter, ok := e.adapters[provider]
	if !ok {
		return VerifyResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	ledger, _, err := e.ledger.LoadOrBootstrap(provider)
	if err != nil {
		return VerifyResult{}, err
	}
	if ledger.ProviderSchemaVersion != providerSchemaFor(provider) {
		report := sddruntime.NewIntegrityReport(provider, sddruntime.DefaultContract())
		report.AddCheck(sddruntime.CheckResult{
			Key:        "ledger.provider_schema_version",
			Status:     sddruntime.StatusFail,
			DriftClass: sddruntime.DriftOwned,
			Expected:   providerSchemaFor(provider),
			Observed:   ledger.ProviderSchemaVersion,
			Message:    "provider schema version is incompatible; run managed-state migration before verify/reconcile",
		})
		return VerifyResult{Status: sddruntime.StatusFail, Report: report}, nil
	}
	observed, err := adapter.Observe()
	if err != nil {
		return VerifyResult{}, err
	}
	report := sddruntime.Verify(provider, sddruntime.ObservedRuntime{
		Manifest:         sddruntime.RuntimeManifestState{Present: true, ContractVersion: sddruntime.DefaultContract().Version, ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"}},
		RegistryPath:     sddruntime.DefaultContract().RegistryPath,
		ModelAssignments: sddruntime.DefaultContract().ModelAssignments,
		Artifacts:        observed.Artifacts,
		NonOwnedChanges:  observed.NonOwnedChanges,
		UnknownChanges:   observed.UnknownChanges,
	})
	return VerifyResult{Status: report.Status, Report: report}, nil
}

func (e *Engine) Reconcile(provider string) (ReconcileResult, error) {
	adapter, ok := e.adapters[provider]
	if !ok {
		return ReconcileResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	plan, err := e.Doctor(provider)
	if err != nil {
		return ReconcileResult{}, err
	}

	owned := make([]DoctorStep, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.SafetyClass == "non-owned" {
			continue
		}
		owned = append(owned, step)
	}

	if len(owned) > 0 {
		targets, err := adapter.BackupTargets(owned)
		if err != nil {
			return ReconcileResult{}, NewLifecycleError("backup_targets_failed", "", "logical", "backup", "review provider backup target resolution", err)
		}
		if _, err := e.backups.CreateSnapshot("reconcile", targets); err != nil {
			return ReconcileResult{}, NewLifecycleError("backup_failed", "", "logical", "backup", "retry reconcile after backup issue is solved", err)
		}
		if f, ok := adapter.(*fakeProviderAdapter); ok {
			f.applyStages = append(f.applyStages, "backup-complete")
		}
		if err := adapter.Apply(owned); err != nil {
			return ReconcileResult{}, NewLifecycleError("apply_failed", "", "logical", "apply", "restore last snapshot and retry", err)
		}
	}

	verifyResult, err := e.Verify(provider)
	if err != nil {
		return ReconcileResult{}, err
	}
	if verifyResult.Status == sddruntime.StatusFail {
		return ReconcileResult{}, NewLifecycleError("post_verify_failed", "", "logical", "post_verify", "run restore using latest snapshot", fmt.Errorf("status %s", verifyResult.Status))
	}
	return ReconcileResult{Applied: len(owned), SkippedNonOwned: verifyResult.Report.Notes}, nil
}

func (e *Engine) Restore(provider, snapshotID string) (RestoreResult, error) {
	adapter, ok := e.adapters[provider]
	if !ok {
		return RestoreResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	manifest, err := e.backups.loadManifest(snapshotID)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := e.backups.ValidateManifest(manifest); err != nil {
		return RestoreResult{}, NewLifecycleError("restore_unsafe_path", "", "logical", "restore", "remove unsafe manifest entries and retry", err)
	}
	if err := e.backups.ValidateSnapshot(manifest); err != nil {
		return RestoreResult{}, NewLifecycleError("restore_checksum_mismatch", "", "logical", "restore", "use an intact snapshot archive", err)
	}
	restored, err := adapter.Restore(manifest)
	if err != nil {
		return RestoreResult{}, NewLifecycleError("restore_failed", "", "logical", "restore", "retry restore with valid snapshot", err)
	}
	verifyResult, err := e.Verify(provider)
	if err != nil {
		return RestoreResult{}, err
	}
	if verifyResult.Status == sddruntime.StatusFail {
		return RestoreResult{}, NewLifecycleError("post_verify_failed", "", "logical", "post_verify", "run restore using another valid snapshot", fmt.Errorf("status %s", verifyResult.Status))
	}
	return RestoreResult{Restored: restored}, nil
}

func (e *Engine) Backup(provider, sourceOperation string) (string, error) {
	adapter, ok := e.adapters[provider]
	if !ok {
		return "", fmt.Errorf("unsupported provider %q", provider)
	}
	steps := managedUninstallSteps()
	targets, err := adapter.BackupTargets(steps)
	if err != nil {
		return "", NewLifecycleError("backup_targets_failed", "", "logical", "backup", "review provider backup target resolution", err)
	}
	manifest, err := e.backups.CreateSnapshot(sourceOperation, targets)
	if err != nil {
		return "", NewLifecycleError("backup_failed", "", "logical", "backup", "retry backup after storage issue is solved", err)
	}
	return manifest.SnapshotID, nil
}

func (e *Engine) Uninstall(provider, mode string) (UninstallResult, error) {
	adapter, ok := e.adapters[provider]
	if !ok {
		return UninstallResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	if mode == "soft" || mode == "purge" {
		return UninstallResult{}, NewLifecycleError("unsupported_uninstall_mode", "", "logical", "validate", "use supported mode: provider or all", nil)
	}
	if mode == "" {
		mode = "provider"
	}

	steps := managedUninstallSteps()
	targets, err := adapter.BackupTargets(steps)
	if err != nil {
		return UninstallResult{}, NewLifecycleError("backup_targets_failed", "", "logical", "backup", "review provider backup target resolution", err)
	}
	if _, err := e.backups.CreateSnapshot("uninstall", targets); err != nil {
		return UninstallResult{}, NewLifecycleError("backup_failed", "", "logical", "backup", "retry uninstall after backup issue is solved", err)
	}
	if f, ok := adapter.(*fakeProviderAdapter); ok {
		f.applyStages = append(f.applyStages, "backup-complete")
	}
	if err := adapter.Apply(steps); err != nil {
		return UninstallResult{}, NewLifecycleError("uninstall_apply_failed", "", "logical", "apply", "restore last snapshot and retry", err)
	}
	verifyResult, err := e.Verify(provider)
	if err != nil {
		return UninstallResult{}, err
	}
	if verifyResult.Status == sddruntime.StatusFail {
		return UninstallResult{}, NewLifecycleError("post_verify_failed", "", "logical", "post_verify", "run restore using latest snapshot", fmt.Errorf("status %s", verifyResult.Status))
	}

	ledgerRemoved := false
	if mode == "all" {
		if err := e.ledger.remove(); err != nil {
			return UninstallResult{}, NewLifecycleError("ledger_cleanup_failed", "", "logical", "cleanup", "remove ledger manually and retry", err)
		}
		ledgerRemoved = true
	}

	return UninstallResult{Applied: len(steps), VerifyStatus: verifyResult.Status, LedgerRemoved: ledgerRemoved}, nil
}

func managedUninstallSteps() []DoctorStep {
	contract := sddruntime.DefaultContract()
	steps := make([]DoctorStep, 0, len(contract.ManagedArtifacts))
	for _, artifact := range contract.ManagedArtifacts {
		steps = append(steps, DoctorStep{AssetID: artifact.ID, SafetyClass: "auto-safe", BackupNeeded: true})
	}
	return steps
}

func (e *Engine) Doctor(provider string) (DoctorPlan, error) {
	verifyResult, err := e.Verify(provider)
	if err != nil {
		return DoctorPlan{}, err
	}
	plan := DoctorPlan{ReadOnly: true, Steps: []DoctorStep{}}
	for _, check := range verifyResult.Report.Checks {
		if check.DriftClass != sddruntime.DriftOwned || check.Status == sddruntime.StatusPass {
			continue
		}
		plan.Steps = append(plan.Steps, DoctorStep{AssetID: check.Key, SafetyClass: "auto-safe", BackupNeeded: true})
	}
	return plan, nil
}
