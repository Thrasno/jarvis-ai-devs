package lifecycle

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type EngineDeps struct {
	Adapters    map[string]ProviderAdapter
	HomeDir     string
	ProjectRoot string
}

type Engine struct {
	adapters    map[string]ProviderAdapter
	ledger      LedgerStore
	backups     BackupStore
	projectRoot string
}

func NewEngine(deps EngineDeps) *Engine {
	if deps.HomeDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			deps.HomeDir = home
		}
	}
	return &Engine{adapters: deps.Adapters, ledger: NewLedgerStore(deps.HomeDir), backups: NewBackupStore(deps.HomeDir), projectRoot: deps.ProjectRoot}
}

func (e *Engine) Verify(provider string) (VerifyResult, error) {
	return e.verify(provider, true)
}

func (e *Engine) verify(provider string, bootstrapLedger bool) (VerifyResult, error) {
	adapter, ok := e.adapters[provider]
	if !ok {
		return VerifyResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	var ledger Ledger
	var err error
	if bootstrapLedger {
		ledger, _, err = e.ledger.LoadOrBootstrap(provider)
	} else {
		ledger, _, err = e.ledger.LoadReadOnly(provider)
	}
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
	registryQuality := observed.RegistryQuality
	if e.projectRoot != "" {
		registryQuality = ObserveProjectRegistryQuality(e.projectRoot)
	}
	storeContract, err := sddruntime.ResolveRuntimeStoreContract(sddruntime.StoreModeHive)
	if err != nil {
		return VerifyResult{}, err
	}
	promptSourceIDs, err := sddruntime.DefaultPromptSourceIDs(provider, "orchestrator")
	if err != nil {
		return VerifyResult{}, err
	}
	report := sddruntime.Verify(provider, sddruntime.ObservedRuntime{
		Manifest:                   sddruntime.RuntimeManifestState{Present: true, ContractVersion: sddruntime.DefaultContract().Version, ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"}},
		RegistryPath:               sddruntime.DefaultContract().RegistryPath,
		PromptSourceIDs:            promptSourceIDs,
		StoreMode:                  string(storeContract.Mode),
		StoreReadFrom:              storeContract.ReadFrom,
		StoreWriteTo:               storeContract.WriteTo,
		ArtifactTopics:             []string{"sdd/runtime/verify"},
		GeneralMemoryTopics:        []string{"runtime/notes"},
		ModelAssignments:           sddruntime.DefaultContract().ModelAssignments,
		Artifacts:                  observed.Artifacts,
		NonOwnedChanges:            observed.NonOwnedChanges,
		UnknownChanges:             observed.UnknownChanges,
		RegistryQuality:            registryQuality,
		OpenCode:                   observed.OpenCode,
		ClaudeSDDSubagentHiveTools: observed.ClaudeSDDSubagentHiveTools,
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
	manualRequired := 0
	skippedNonOwned := []string{}
	for _, step := range plan.Steps {
		if !step.SafeToAutoApply || step.SafetyClass != "auto-safe" || step.AssetID == "" {
			manualRequired++
			if step.ReasonCode == "non_owned_drift" {
				skippedNonOwned = append(skippedNonOwned, step.NextAction)
			}
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
	if len(owned) == 0 {
		return ReconcileResult{Applied: 0, ManualRequired: manualRequired, SkippedNonOwned: skippedNonOwned}, nil
	}

	verifyResult, err := e.Verify(provider)
	if err != nil {
		return ReconcileResult{}, err
	}
	if verifyResult.Status == sddruntime.StatusFail {
		return ReconcileResult{}, NewLifecycleError("post_verify_failed", "", "logical", "post_verify", "run restore using latest snapshot", fmt.Errorf("status %s", verifyResult.Status))
	}
	if len(skippedNonOwned) == 0 {
		skippedNonOwned = verifyResult.Report.Notes
	}
	return ReconcileResult{Applied: len(owned), ManualRequired: manualRequired, SkippedNonOwned: skippedNonOwned}, nil
}

func (e *Engine) ReconcileDryRun(provider string) (DoctorPlan, error) {
	return e.Doctor(provider)
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
	if provider == "all" && mode == "all" {
		return e.uninstallAllProviders()
	}
	return e.uninstallProvider(provider, mode)
}

func (e *Engine) uninstallAllProviders() (UninstallResult, error) {
	providers := make([]string, 0, len(e.adapters))
	for provider := range e.adapters {
		providers = append(providers, provider)
	}
	sort.Strings(providers)

	var result UninstallResult
	for _, provider := range providers {
		providerResult, err := e.uninstallProvider(provider, "provider")
		if err != nil {
			return UninstallResult{}, err
		}
		result.Applied += providerResult.Applied
		if providerResult.VerifyStatus == sddruntime.StatusFail || result.VerifyStatus == "" {
			result.VerifyStatus = providerResult.VerifyStatus
		}
	}
	if err := e.ledger.remove(); err != nil {
		return UninstallResult{}, NewLifecycleError("ledger_cleanup_failed", "", "logical", "cleanup", "remove ledger manually and retry", err)
	}
	result.LedgerRemoved = true
	return result, nil
}

func (e *Engine) uninstallProvider(provider, mode string) (UninstallResult, error) {
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
		steps = append(steps, DoctorStep{AssetID: artifact.ID, Class: "owned", SafetyClass: "auto-safe", SafeToAutoApply: true, BackupNeeded: true})
	}
	return steps
}

func (e *Engine) Doctor(provider string) (DoctorPlan, error) {
	verifyResult, err := e.verify(provider, false)
	if err != nil {
		return DoctorPlan{}, err
	}
	plan := DoctorPlan{Provider: provider, Status: verifyResult.Status, ReadOnly: true, Steps: []DoctorStep{}}
	for _, check := range verifyResult.Report.Checks {
		if check.Status == sddruntime.StatusPass || check.DriftClass == sddruntime.DriftNone {
			continue
		}
		plan.Steps = append(plan.Steps, doctorStepFromCheck(check))
	}
	sort.SliceStable(plan.Steps, func(i, j int) bool {
		left := plan.Steps[i]
		right := plan.Steps[j]
		if left.CheckKey != right.CheckKey {
			return left.CheckKey < right.CheckKey
		}
		return left.AssetID < right.AssetID
	})
	return plan, nil
}

func doctorStepFromCheck(check sddruntime.CheckResult) DoctorStep {
	step := DoctorStep{
		CheckKey:    check.Key,
		Class:       "manual-required",
		SafetyClass: "manual-required",
		NextAction:  "review diagnosis and repair manually before rerunning doctor",
	}
	if strings.HasPrefix(check.Key, "registry.quality.") {
		step.ReasonCode = "registry_quality_warning"
		step.NextAction = "run jarvis skill-registry refresh from the project worktree and inspect any remaining registry warnings"
		return step
	}

	switch check.DriftClass {
	case sddruntime.DriftOwned:
		assetID, ok := managedArtifactAssetID(check.Key)
		if ok {
			step.AssetID = assetID
			step.ReasonCode = managedArtifactReasonCode(check)
			step.Class = "owned"
			step.SafetyClass = "auto-safe"
			step.SafeToAutoApply = true
			step.BackupNeeded = true
			step.NextAction = "restore managed artifact from Jarvis managed runtime state"
			return step
		}
		if isGeneratedAgentArtifactDrift(check.Key) {
			step.ReasonCode = generatedAgentArtifactReasonCode(check.Key)
			step.NextAction = "run jarvis init or supported reconfiguration to regenerate managed agent artifacts; preserve user-owned configuration through merge/no-clobber boundaries"
			return step
		}
		if check.Key == "ledger.provider_schema_version" {
			step.ReasonCode = "provider_schema_mismatch"
			step.NextAction = "run managed-state migration before reconcile"
			return step
		}
		step.ReasonCode = "manual_invariant_drift"
		step.NextAction = "inspect owned invariant drift and repair manually"
		return step
	case sddruntime.DriftNonOwned:
		step.ReasonCode = "non_owned_drift"
		step.SafetyClass = "non-owned"
		step.NextAction = "preserve user-owned changes and repair manually if needed"
		return step
	case sddruntime.DriftUnknown:
		step.ReasonCode = "unknown_drift"
		step.NextAction = "inspect unknown runtime drift before applying managed repairs"
		return step
	default:
		step.ReasonCode = "unknown_drift"
		return step
	}
}

func managedArtifactAssetID(checkKey string) (string, bool) {
	const prefix = "artifact."
	const suffix = ".present"
	if !strings.HasPrefix(checkKey, prefix) || !strings.HasSuffix(checkKey, suffix) {
		return "", false
	}
	assetID := strings.TrimSuffix(strings.TrimPrefix(checkKey, prefix), suffix)
	if assetID == "" {
		return "", false
	}
	return assetID, true
}

func managedArtifactReasonCode(check sddruntime.CheckResult) string {
	if check.Observed == "missing" {
		return "managed_artifact_missing"
	}
	return "managed_artifact_boundary_invalid"
}

func isGeneratedAgentArtifactDrift(checkKey string) bool {
	switch checkKey {
	case "invariant.opencode.sdd_hive_grants",
		"invariant.opencode.plugin_hive",
		"invariant.claude.sdd_hive_tools":
		return true
	default:
		return false
	}
}

func generatedAgentArtifactReasonCode(checkKey string) string {
	if checkKey == "invariant.opencode.sdd_hive_grants" {
		return "generated_sdd_hive_grants_outdated"
	}
	if checkKey == "invariant.claude.sdd_hive_tools" {
		return "generated_claude_sdd_hive_tools_outdated"
	}
	return "generated_opencode_artifact_outdated"
}
