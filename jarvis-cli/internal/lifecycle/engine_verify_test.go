package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
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
