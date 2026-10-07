package lifecycle

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func compliantOpenCodeConfigForResidueTest() sddruntime.ObservedOpenCodeConfig {
	cfg := fakeCompliantOpenCodeConfig()
	return cfg
}

// TestEngineDoctor_ReportsLegacy4RResidueInformationally_Claude asserts that
// when the Claude adapter observes retired review-* agent files, doctor
// reports one informational step that never auto-applies and points to the
// wizard reset, without failing doctor.
func TestEngineDoctor_ReportsLegacy4RResidueInformationally_Claude(t *testing.T) {
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: true, MarkersValid: true},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			ClaudeLegacy4RResidue: []string{"review-risk.md", "review-readability.md"},
		},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})

	plan, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if plan.Status == sddruntime.StatusFail {
		t.Fatalf("legacy 4R residue must never fail doctor, got status=%q", plan.Status)
	}

	step := findStep(plan.Steps, "invariant.claude.legacy_4r_residue")
	if step == nil {
		t.Fatalf("expected doctor step for legacy 4R residue in %#v", plan.Steps)
	}
	if step.SafeToAutoApply {
		t.Fatalf("legacy 4R residue step must never be auto-safe, got %+v", *step)
	}
	if step.Class != "informational" || step.SafetyClass != "informational" {
		t.Fatalf("expected informational class/safety_class, got %+v", *step)
	}
	if step.ReasonCode != "legacy_4r_residue" {
		t.Fatalf("expected reason_code legacy_4r_residue, got %q", step.ReasonCode)
	}
	if step.NextAction == "" {
		t.Fatal("expected next_action pointing at the wizard reset")
	}
}

// TestEngineDoctor_ReportsLegacy4RResidueInformationally_OpenCode mirrors the
// Claude case for the OpenCode adapter, covering partial residue (agent entry
// only, no task allow).
func TestEngineDoctor_ReportsLegacy4RResidueInformationally_OpenCode(t *testing.T) {
	oc := compliantOpenCodeConfigForResidueTest()
	oc.AgentNames = append(oc.AgentNames, "review-risk")
	oc.HiddenSubagents = append(oc.HiddenSubagents, "review-risk")

	adapter := &fakeProviderAdapter{
		name: "opencode",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: true, MarkersValid: true},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			OpenCode: oc,
		},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"opencode": adapter}, HomeDir: t.TempDir()})

	plan, err := engine.Doctor("opencode")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if plan.Status == sddruntime.StatusFail {
		t.Fatalf("legacy 4R residue must never fail doctor, got status=%q", plan.Status)
	}

	step := findStep(plan.Steps, "invariant.opencode.legacy_4r_residue")
	if step == nil {
		t.Fatalf("expected doctor step for legacy 4R residue in %#v", plan.Steps)
	}
	if step.SafeToAutoApply || step.Class != "informational" || step.SafetyClass != "informational" {
		t.Fatalf("expected non-applicable informational step, got %+v", *step)
	}
}

// TestEngineDoctor_NoLegacy4RResidueStepWhenClean asserts that a clean
// installation produces no legacy 4R residue doctor step at all.
func TestEngineDoctor_NoLegacy4RResidueStepWhenClean(t *testing.T) {
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: t.TempDir()})

	plan, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if step := findStep(plan.Steps, "invariant.claude.legacy_4r_residue"); step != nil {
		t.Fatalf("expected no legacy 4R residue step when clean, got %+v", *step)
	}
}

// TestEngineReconcile_NeverDeletesLegacy4RResidue asserts that reconcile
// treats legacy 4R residue as manual-required only, never applies/deletes
// anything for it, and does not fail.
func TestEngineReconcile_NeverDeletesLegacy4RResidue(t *testing.T) {
	home := t.TempDir()
	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{
			Artifacts: map[string]sddruntime.ObservedArtifact{
				"instructions": {Exists: true, MarkersValid: true},
				"orchestrator": {Exists: true},
				"skills":       {Exists: true},
			},
			ClaudeLegacy4RResidue: []string{"review-risk.md", "review-readability.md", "review-reliability.md", "review-resilience.md"},
		},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	result, err := engine.Reconcile("claude")
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if result.Applied != 0 {
		t.Fatalf("expected no applied mutations for legacy 4R residue, applied=%d assets=%v", result.Applied, adapter.appliedAssets)
	}
	if len(adapter.appliedAssets) != 0 {
		t.Fatalf("expected reconcile never to touch legacy 4R residue files, applied assets=%v", adapter.appliedAssets)
	}
	if result.ManualRequired == 0 {
		t.Fatal("expected legacy 4R residue to remain manual-required, never silently dropped")
	}
}
