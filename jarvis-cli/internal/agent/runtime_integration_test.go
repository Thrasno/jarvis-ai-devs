package agent

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

func TestAgentRuntimePlan_UsesCanonicalBuilder(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{name: "claude plan", agent: &ClaudeAgent{home: t.TempDir()}, want: "claude"},
		{name: "opencode plan", agent: &OpenCodeAgent{home: t.TempDir()}, want: "opencode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := tt.agent.RuntimePlan()
			if err != nil {
				t.Fatalf("RuntimePlan returned error: %v", err)
			}
			if plan.Agent != tt.want {
				t.Fatalf("plan.Agent = %q, want %q", plan.Agent, tt.want)
			}
			if plan.Contract.RegistryPath != sddruntime.DefaultRegistryPath {
				t.Fatalf("registry path mismatch: got %q want %q", plan.Contract.RegistryPath, sddruntime.DefaultRegistryPath)
			}
		})
	}
}

func TestClaudeAgent_ObserveRuntime_ProducesVerifierInput(t *testing.T) {
	home := t.TempDir()
	a := &ClaudeAgent{home: home, templatesFS: testTemplatesFS}

	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	orchestratorFS := fstest.MapFS{
		"embed/orchestrator/sdd-orchestrator.md": {Data: []byte("# orchestrator")},
	}
	if err := a.InstallOrchestrator(orchestratorFS); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}

	skillsFS := fstest.MapFS{"_shared/SKILL.md": {Data: []byte("# shared")}}
	if err := a.InstallSkills(skillsFS, nil); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	observed, err := a.ObserveRuntime()
	if err != nil {
		t.Fatalf("ObserveRuntime: %v", err)
	}

	report := sddruntime.Verify(a.Name(), observed)
	if report.Status != sddruntime.StatusPass {
		t.Fatalf("expected verifier pass, got %q", report.Status)
	}
}

func TestOpenCodeAgent_ObserveRuntime_ProducesVerifierInput(t *testing.T) {
	home := t.TempDir()
	a := &OpenCodeAgent{home: home, templatesFS: testTemplatesFS}

	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}

	orchestratorFS := fstest.MapFS{
		"embed/orchestrator/sdd-orchestrator.md": {Data: []byte("# orchestrator")},
	}
	if err := a.InstallOrchestrator(orchestratorFS); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}

	skillsFS := fstest.MapFS{"_shared/SKILL.md": {Data: []byte("# shared")}}
	if err := a.InstallSkills(skillsFS, nil); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	observed, err := a.ObserveRuntime()
	if err != nil {
		t.Fatalf("ObserveRuntime: %v", err)
	}

	report := sddruntime.Verify(a.Name(), observed)
	if report.Status != sddruntime.StatusPass {
		t.Fatalf("expected verifier pass, got %q", report.Status)
	}
}

func TestAdapters_RuntimeObservation_EquivalentContractSemantics(t *testing.T) {
	claudeHome := t.TempDir()
	opencodeHome := t.TempDir()

	claude := &ClaudeAgent{home: claudeHome, templatesFS: testTemplatesFS}
	opencode := &OpenCodeAgent{home: opencodeHome, templatesFS: testTemplatesFS}

	for _, a := range []Agent{claude, opencode} {
		if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
			t.Fatalf("mkdir config dir for %s: %v", a.Name(), err)
		}
		if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
			t.Fatalf("WriteInstructions for %s: %v", a.Name(), err)
		}
		orchestratorFS := fstest.MapFS{"embed/orchestrator/sdd-orchestrator.md": {Data: []byte("# orchestrator")}}
		if err := a.InstallOrchestrator(orchestratorFS); err != nil {
			t.Fatalf("InstallOrchestrator for %s: %v", a.Name(), err)
		}
		skillsFS := fstest.MapFS{"_shared/SKILL.md": {Data: []byte("# shared")}}
		if err := a.InstallSkills(skillsFS, nil); err != nil {
			t.Fatalf("InstallSkills for %s: %v", a.Name(), err)
		}
	}

	claudeObserved, err := claude.ObserveRuntime()
	if err != nil {
		t.Fatalf("ObserveRuntime claude: %v", err)
	}
	opencodeObserved, err := opencode.ObserveRuntime()
	if err != nil {
		t.Fatalf("ObserveRuntime opencode: %v", err)
	}

	claudeReport := sddruntime.Verify("claude", claudeObserved)
	opencodeReport := sddruntime.Verify("opencode", opencodeObserved)

	if claudeReport.Status != sddruntime.StatusPass || opencodeReport.Status != sddruntime.StatusPass {
		t.Fatalf("expected both pass, got claude=%q opencode=%q", claudeReport.Status, opencodeReport.Status)
	}

	if checkStatusByKey(claudeReport.Checks, "invariant.registry_path") != checkStatusByKey(opencodeReport.Checks, "invariant.registry_path") {
		t.Fatalf("registry invariant status mismatch across adapters")
	}
	if checkStatusByKey(claudeReport.Checks, "invariant.model.orchestrator") != checkStatusByKey(opencodeReport.Checks, "invariant.model.orchestrator") {
		t.Fatalf("orchestrator model invariant status mismatch across adapters")
	}
	if checkStatusByKey(claudeReport.Checks, "artifact.orchestrator.present") != checkStatusByKey(opencodeReport.Checks, "artifact.orchestrator.present") {
		t.Fatalf("orchestrator artifact status mismatch across adapters")
	}
}

func checkStatusByKey(checks []sddruntime.CheckResult, key string) sddruntime.IntegrityStatus {
	for _, check := range checks {
		if check.Key == key {
			return check.Status
		}
	}
	return ""
}
