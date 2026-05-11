package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

func TestLifecycleError_ExposesStructuredEnvelope(t *testing.T) {
	err := NewLifecycleError("backup_failed", "settings", string(sddruntime.OwnershipJSONPath), "backup", "retry with writable backup dir", errors.New("disk full"))

	if err.Code != "backup_failed" || err.AssetID != "settings" || err.Scope != string(sddruntime.OwnershipJSONPath) || err.Stage != "backup" || err.NextAction == "" {
		t.Fatalf("unexpected lifecycle envelope fields: %#v", err)
	}
	if err.Unwrap() == nil {
		t.Fatal("expected unwrap error to be preserved")
	}
}

func TestEngineReconcile_RequiresBackupBeforeApplyAndRunsPostVerify(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: false},
			"skills":       {Exists: true},
		}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	_, err := engine.Reconcile("claude")
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if adapter.applyCalls == 0 {
		t.Fatal("expected reconcile to apply owned mutations")
	}
	if len(adapter.applyStages) == 0 || adapter.applyStages[0] != "backup-complete" {
		t.Fatalf("expected backup before apply, got stages=%v", adapter.applyStages)
	}
	if adapter.verifyCalls < 2 {
		t.Fatalf("expected post-verify gate to run, verify calls=%d", adapter.verifyCalls)
	}
}

func TestEngineReconcile_BlocksNonOwnedMutations(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name: "opencode",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		}, NonOwnedChanges: []string{"custom user section"}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"opencode": adapter}, HomeDir: home})

	result, err := engine.Reconcile("opencode")
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if len(result.SkippedNonOwned) == 0 {
		t.Fatal("expected explicit non-owned skips")
	}
	if adapter.applyNonOwnedCount != 0 {
		t.Fatalf("expected non-owned ops to be blocked, applied=%d", adapter.applyNonOwnedCount)
	}
}

func TestEngineReconcile_AppliesOnlyAutoSafeOwnedSteps(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: false},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			NonOwnedChanges: []string{"custom user section"},
			UnknownChanges:  []string{"unmapped runtime drift"},
		},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	result, err := engine.Reconcile("claude")
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if result.Applied != 1 {
		t.Fatalf("applied = %d, want 1", result.Applied)
	}
	if len(adapter.appliedAssets) != 1 || adapter.appliedAssets[0] != "instructions" {
		t.Fatalf("expected only auto-safe owned instructions to apply, got %v", adapter.appliedAssets)
	}
	if len(adapter.backupTargetPaths) != 1 || adapter.backupTargetPaths[0] != "instructions" {
		t.Fatalf("expected backup only for applied owned asset, got %v", adapter.backupTargetPaths)
	}
	if result.ManualRequired != 2 {
		t.Fatalf("manual-required count = %d, want 2", result.ManualRequired)
	}
	if len(result.SkippedNonOwned) == 0 {
		t.Fatal("expected non-owned notes to remain reported")
	}
	if adapter.applyNonOwnedCount != 0 {
		t.Fatalf("expected non-owned ops to be blocked, applied=%d", adapter.applyNonOwnedCount)
	}
}

func TestEngineReconcile_ReportsFailClassManualRequiredWithoutApplyOrBackup(t *testing.T) {
	home := t.TempDir()
	ledgerPath := filepath.Join(home, ".jarvis", "managed-state.json")
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ledger := `{"version":"v1","jarvis_version":"dev","contract_version":"2026.05","provider_schema_version":"v0"}`
	if err := os.WriteFile(ledgerPath, []byte(ledger), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	result, err := engine.Reconcile("claude")
	if err != nil {
		t.Fatalf("Reconcile must return reportable manual-required result, got error: %v", err)
	}

	if result.Applied != 0 {
		t.Fatalf("applied = %d, want 0", result.Applied)
	}
	if result.ManualRequired != 1 {
		t.Fatalf("manual-required count = %d, want 1", result.ManualRequired)
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("manual-only reconcile must not apply mutations; calls=%d", adapter.applyCalls)
	}
	if len(adapter.backupTargetPaths) != 0 {
		t.Fatalf("manual-only reconcile must not prepare backups; targets=%v", adapter.backupTargetPaths)
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "backups")); !os.IsNotExist(err) {
		t.Fatalf("manual-only reconcile must not create backup artifacts; stat err=%v", err)
	}
}

func TestEngineReconcileDryRun_UsesDoctorPlanWithoutApplyOrBackup(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: false},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			NonOwnedChanges: []string{"custom user section"},
		},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	dryRunPlan, err := engine.ReconcileDryRun("claude")
	if err != nil {
		t.Fatalf("ReconcileDryRun returned error: %v", err)
	}
	doctorPlan, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}

	if !reflect.DeepEqual(dryRunPlan, doctorPlan) {
		t.Fatalf("dry-run plan must match doctor-derived apply plan:\ndryRun=%#v\ndoctor=%#v", dryRunPlan, doctorPlan)
	}
	if !dryRunPlan.ReadOnly {
		t.Fatal("dry-run plan must be read-only preview metadata")
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("dry-run must not apply mutations; calls=%d", adapter.applyCalls)
	}
	if len(adapter.backupTargetPaths) != 0 {
		t.Fatalf("dry-run must not prepare backups; targets=%v", adapter.backupTargetPaths)
	}
	if _, err := os.Stat(filepath.Join(home, ".jarvis", "backups")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create backup artifacts; stat err=%v", err)
	}
}

func TestEngineRestore_BlocksUnsafeManifestPathBeforeWrite(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{name: "claude"}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	manifest := BackupManifest{
		SnapshotID:      "snap-1",
		SourceOperation: "reconcile",
		Entries: []BackupEntry{{
			Path:     "../../etc/passwd",
			Checksum: "abc",
		}},
	}
	if err := engine.backups.saveManifest(manifest); err != nil {
		t.Fatalf("saveManifest: %v", err)
	}

	_, err := engine.Restore("claude", "snap-1")
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
	var lerr *LifecycleError
	if !errors.As(err, &lerr) || lerr.Code != "restore_unsafe_path" {
		t.Fatalf("expected restore_unsafe_path lifecycle error, got %v", err)
	}
	if adapter.restoreWrites != 0 {
		t.Fatalf("restore must abort before writes, writes=%d", adapter.restoreWrites)
	}
}

func TestEngineRestore_RunsPostVerifyGate(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	managedFile := filepath.Join(home, ".claude", "sdd-orchestrator.md")
	if err := os.MkdirAll(filepath.Dir(managedFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(managedFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	manifest := BackupManifest{SnapshotID: "snap-verify", SourceOperation: "restore", ArchivePath: "", Entries: []BackupEntry{{Path: managedFile, Checksum: ""}}}
	storeManifest, err := engine.backups.CreateSnapshot("restore", []BackupTarget{{Path: managedFile}})
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := engine.backups.saveManifest(BackupManifest{SnapshotID: storeManifest.SnapshotID, SourceOperation: manifest.SourceOperation, ArchivePath: storeManifest.ArchivePath, Entries: storeManifest.Entries}); err != nil {
		t.Fatalf("saveManifest: %v", err)
	}

	if _, err := engine.Restore("claude", storeManifest.SnapshotID); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if adapter.verifyCalls == 0 {
		t.Fatal("expected restore to trigger post-verify")
	}
}
