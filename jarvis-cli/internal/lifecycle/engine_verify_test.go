package lifecycle

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func TestEngineVerify_ClassifiesOwnedNonOwnedUnknownWithoutMutation(t *testing.T) {
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: false},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			NonOwnedChanges: []string{"custom note outside managed boundary"},
			UnknownChanges:  []string{"untracked external integration state"},
		},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})
	result, err := engine.Verify("claude")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Status != sddruntime.StatusFail {
		t.Fatalf("expected fail because owned drift exists, got %q", result.Status)
	}
	if !hasDriftClass(result.Report.Checks, sddruntime.DriftOwned) {
		t.Fatal("expected owned drift classification")
	}
	if !hasDriftClass(result.Report.Checks, sddruntime.DriftNonOwned) {
		t.Fatal("expected non-owned drift classification")
	}
	if !hasDriftClass(result.Report.Checks, sddruntime.DriftUnknown) {
		t.Fatal("expected unknown drift classification")
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("verify must be read-only; apply calls = %d", adapter.applyCalls)
	}
}

func TestEngineVerify_UsesRuntimeStoreModeContract(t *testing.T) {
	t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")

	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		}},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})
	result, err := engine.Verify("claude")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}

	readTargets := findCheck(result.Report.Checks, "invariant.store.read_targets")
	if readTargets == nil {
		t.Fatal("expected invariant.store.read_targets check")
	}
	if got := strings.TrimSpace(readTargets.Observed); got != "openspec" {
		t.Fatalf("observed store read targets = %q, want %q", got, "openspec")
	}

	writeTargets := findCheck(result.Report.Checks, "invariant.store.write_targets")
	if writeTargets == nil {
		t.Fatal("expected invariant.store.write_targets check")
	}
	if got := strings.TrimSpace(writeTargets.Observed); got != "openspec" {
		t.Fatalf("observed store write targets = %q, want %q", got, "openspec")
	}
}

func TestEngineDoctor_ReturnsReadOnlyPlan(t *testing.T) {
	adapter := &fakeProviderAdapter{
		name: "opencode",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: true, MarkersValid: true},
				"orchestrator": {Exists: false},
				"skills":       {Exists: true},
			},
		},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"opencode": adapter}, HomeDir: t.TempDir()})
	plan, err := engine.Doctor("opencode")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("doctor must generate remediation steps for owned drift")
	}
	if plan.ReadOnly != true {
		t.Fatal("doctor plan must be read-only")
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("doctor must not mutate state; apply calls = %d", adapter.applyCalls)
	}
}

func TestEngineDoctor_DoesNotBootstrapMissingLedger(t *testing.T) {
	home := t.TempDir()
	ledgerPath := filepath.Join(home, ".jarvis", "managed-state.json")
	adapter := &fakeProviderAdapter{
		name: "opencode",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: true, MarkersValid: true},
				"orchestrator": {Exists: false},
				"skills":       {Exists: true},
			},
		},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"opencode": adapter}, HomeDir: home})
	plan, err := engine.Doctor("opencode")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}

	if !plan.ReadOnly {
		t.Fatal("doctor plan must be marked read-only")
	}
	if len(plan.Steps) == 0 {
		t.Fatal("doctor must still report diagnosis without bootstrapping the ledger")
	}
	if _, err := os.Stat(ledgerPath); !os.IsNotExist(err) {
		t.Fatalf("doctor must not create lifecycle ledger at %s; stat err=%v", ledgerPath, err)
	}
	if _, err := os.Stat(filepath.Dir(ledgerPath)); !os.IsNotExist(err) {
		t.Fatalf("doctor must not create lifecycle state directory; stat err=%v", err)
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("doctor must not apply mutations; apply calls = %d", adapter.applyCalls)
	}
}

func TestEngineDoctor_DoesNotPersistMissingProviderSchema(t *testing.T) {
	home := t.TempDir()
	ledgerPath := filepath.Join(home, ".jarvis", "managed-state.json")
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ledger := []byte(`{"version":"v1","jarvis_version":"dev","contract_version":"2026.05"}`)
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: false},
			"skills":       {Exists: true},
		}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	plan, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}

	if !plan.ReadOnly {
		t.Fatal("doctor plan must be marked read-only")
	}
	if len(plan.Steps) == 0 {
		t.Fatal("doctor must still diagnose drift when ledger schema is missing")
	}
	after, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != string(ledger) {
		t.Fatalf("doctor must not persist provider schema defaults; before=%s after=%s", string(ledger), string(after))
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("doctor must not apply mutations; apply calls = %d", adapter.applyCalls)
	}
}

func TestEngineDoctor_ReturnsDeterministicStructuredDiagnosis(t *testing.T) {
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"skills":       {Exists: false},
				"instructions": {Exists: false},
				"orchestrator": {Exists: true},
			},
			NonOwnedChanges: []string{"custom user note"},
			UnknownChanges:  []string{"unexpected runtime file"},
		},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})
	first, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	second, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error on repeated run: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("doctor plan must be deterministic across repeated runs:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.Provider != "claude" {
		t.Fatalf("plan provider = %q, want claude", first.Provider)
	}
	if first.Status != sddruntime.StatusFail {
		t.Fatalf("plan status = %q, want %q", first.Status, sddruntime.StatusFail)
	}
	wantOrder := []string{"artifact.instructions.present", "artifact.skills.present", "drift.non_owned", "drift.unknown"}
	if len(first.Steps) != len(wantOrder) {
		t.Fatalf("steps len = %d, want %d: %#v", len(first.Steps), len(wantOrder), first.Steps)
	}
	for i, wantKey := range wantOrder {
		step := first.Steps[i]
		if step.CheckKey != wantKey {
			t.Fatalf("step[%d] check key = %q, want %q", i, step.CheckKey, wantKey)
		}
		if step.ReasonCode == "" || step.Class == "" || step.NextAction == "" {
			t.Fatalf("step[%d] missing structured fields: %#v", i, step)
		}
	}

	if first.Steps[0].ReasonCode != "managed_artifact_missing" || first.Steps[0].Class != "owned" || !first.Steps[0].SafeToAutoApply {
		t.Fatalf("owned missing artifact has wrong diagnosis: %#v", first.Steps[0])
	}
	if first.Steps[2].ReasonCode != "non_owned_drift" || first.Steps[2].Class != "manual-required" || first.Steps[2].SafeToAutoApply {
		t.Fatalf("non-owned drift must require manual action: %#v", first.Steps[2])
	}
	if first.Steps[3].ReasonCode != "unknown_drift" || first.Steps[3].Class != "manual-required" || first.Steps[3].SafeToAutoApply {
		t.Fatalf("unknown drift must require manual action: %#v", first.Steps[3])
	}
	if adapter.applyCalls != 0 {
		t.Fatalf("doctor must not mutate state; apply calls = %d", adapter.applyCalls)
	}
}

func TestEngineVerify_FailsOnIncompatibleProviderSchemaVersion(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".jarvis", "managed-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ledger := `{"version":"v1","jarvis_version":"dev","contract_version":"2026.05","provider_schema_version":"v0"}`
	if err := os.WriteFile(path, []byte(ledger), 0o644); err != nil {
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
	result, err := engine.Verify("claude")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Status != sddruntime.StatusFail {
		t.Fatalf("expected fail for incompatible provider schema, got %q", result.Status)
	}
	if !hasCheckKey(result.Report.Checks, "ledger.provider_schema_version") {
		t.Fatal("expected ledger.provider_schema_version failure check")
	}
}

func TestEngineVerify_NonCriticalDriftReturnsWarnWithActionableDiagnostics(t *testing.T) {
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: true, MarkersValid: true},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			NonOwnedChanges: []string{"custom alias outside managed boundary"},
		},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})
	result, err := engine.Verify("claude")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Status != sddruntime.StatusWarn {
		t.Fatalf("expected warn status for non-critical drift, got %q", result.Status)
	}

	check := findCheck(result.Report.Checks, "drift.non_owned")
	if check == nil {
		t.Fatal("expected drift.non_owned check")
	}
	if check.DriftClass != sddruntime.DriftNonOwned {
		t.Fatalf("expected non-owned drift class, got %q", check.DriftClass)
	}
	if !strings.Contains(check.Message, "outside managed boundaries") {
		t.Fatalf("expected actionable non-owned diagnostic message, got %q", check.Message)
	}
}

func TestEngineVerify_InvalidRuntimeStoreModeEnvReturnsError(t *testing.T) {
	t.Setenv("JARVIS_SDD_STORE_MODE", "memory")

	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		}},
	}

	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})
	_, err := engine.Verify("claude")
	if err == nil {
		t.Fatal("expected invalid runtime store mode error")
	}
	if !strings.Contains(err.Error(), "invalid store mode") {
		t.Fatalf("expected invalid store mode error, got %v", err)
	}
}

func hasDriftClass(checks []sddruntime.CheckResult, want sddruntime.DriftClass) bool {
	for _, check := range checks {
		if check.DriftClass == want {
			return true
		}
	}
	return false
}

func hasCheckKey(checks []sddruntime.CheckResult, key string) bool {
	for _, check := range checks {
		if check.Key == key {
			return true
		}
	}
	return false
}

func findCheck(checks []sddruntime.CheckResult, key string) *sddruntime.CheckResult {
	for i := range checks {
		if checks[i].Key == key {
			return &checks[i]
		}
	}
	return nil
}
