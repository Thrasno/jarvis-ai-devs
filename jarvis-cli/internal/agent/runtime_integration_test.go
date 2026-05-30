package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
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

	if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}
	if err := installOptionalManagedArtifacts(a.ConfigDir()); err != nil {
		t.Fatalf("install optional managed artifacts: %v", err)
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

	if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
		t.Fatalf("InstallOrchestrator: %v", err)
	}
	if err := installOptionalManagedArtifacts(a.ConfigDir()); err != nil {
		t.Fatalf("install optional managed artifacts: %v", err)
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

func TestObserveRuntime_ParsesProviderQualifiedAssignmentsWithoutLowercasingModel(t *testing.T) {
	plan, err := sddruntime.Build("opencode")
	if err != nil {
		t.Fatalf("Build opencode: %v", err)
	}
	configDir := t.TempDir()
	orchestrator := `| Phase | Default Model | Reason |
|-------|---------------|--------|
| sdd-apply | OpenAI/GPT-5.1-Codex-Max | Implementation |`
	if err := os.WriteFile(filepath.Join(configDir, filepath.Base(plan.Paths.Orchestrator)), []byte(orchestrator), 0644); err != nil {
		t.Fatalf("write orchestrator: %v", err)
	}

	observed, err := observeRuntime(configDir, plan)
	if err != nil {
		t.Fatalf("observeRuntime: %v", err)
	}
	if got := observed.ModelAssignments["sdd-apply"]; got != "OpenAI/GPT-5.1-Codex-Max" {
		t.Fatalf("parsed assignment = %q, want case-preserving provider/model", got)
	}
}

func TestObserveRuntime_FallbackUsesConfiguredOpenCodeProviderQualifiedAssignments(t *testing.T) {
	previousLoad := loadAppConfig
	loadAppConfig = func() (*config.AppConfig, error) {
		cfg := &config.AppConfig{}
		cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{
			"sdd-apply": {OpenCode: "opus", Claude: "haiku"},
		}
		cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
			"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		}
		return cfg, nil
	}
	t.Cleanup(func() { loadAppConfig = previousLoad })

	plan, err := sddruntime.Build("opencode")
	if err != nil {
		t.Fatalf("Build opencode: %v", err)
	}
	observed, err := observeRuntime(t.TempDir(), plan)
	if err != nil {
		t.Fatalf("observeRuntime: %v", err)
	}

	if got := observed.ModelAssignments["sdd-apply"]; got != "openai/gpt-5.1-codex-max" {
		t.Fatalf("sdd-apply observed assignment = %q, want provider-qualified assignment", got)
	}
	if got := observed.ResolvedModelAssignments["sdd-apply"]; got != "openai/gpt-5.1-codex-max" {
		t.Fatalf("sdd-apply resolved assignment = %q, want provider-qualified assignment", got)
	}
}

func TestOpenCodeAgent_ObserveRuntime_ManifestUsesRequiredArtifactsOnly(t *testing.T) {
	home := t.TempDir()
	a := &OpenCodeAgent{home: home, templatesFS: testTemplatesFS}

	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
		t.Fatalf("WriteInstructions: %v", err)
	}
	if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
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
	if !observed.Manifest.Present {
		t.Fatalf("expected manifest present with required artifacts installed, got false (ids=%v)", observed.Manifest.ManagedArtifactIDs)
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
		if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
			t.Fatalf("InstallOrchestrator for %s: %v", a.Name(), err)
		}
		if err := installOptionalManagedArtifacts(a.ConfigDir()); err != nil {
			t.Fatalf("install optional managed artifacts for %s: %v", a.Name(), err)
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

	parityKeys := []string{
		"invariant.prompt.required_sources_order",
		"invariant.store.mode",
		"invariant.store.read_targets",
		"invariant.store.write_targets",
		"invariant.memory.artifact_topics_boundary",
		"invariant.memory.general_topics_boundary",
	}
	for _, key := range parityKeys {
		if checkStatusByKey(claudeReport.Checks, key) != checkStatusByKey(opencodeReport.Checks, key) {
			t.Fatalf("parity check %q status mismatch across adapters", key)
		}
	}
}

func TestAdapters_RuntimeObservation_ParityFailureIsSymmetricForLayerRoleDrift(t *testing.T) {
	base := sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            true,
			ContractVersion:    sddruntime.DefaultContract().Version,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath:        sddruntime.DefaultContract().RegistryPath,
		PromptSourceIDs:     []string{"layer2.persona", "layer1.behavior", "skill.sdd-orchestrator", "registry.skill-index", "protocol.hive"},
		StoreMode:           "hybrid",
		StoreReadFrom:       []string{"hive", "openspec"},
		StoreWriteTo:        []string{"hive", "openspec"},
		ArtifactTopics:      []string{"sdd/jarvis-agent-parity-vs-gentle/spec"},
		GeneralMemoryTopics: []string{"runtime/notes"},
		ModelAssignments: map[string]string{
			"default":      "sonnet",
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
		},
		Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}

	claudeReport := sddruntime.Verify("claude", base)
	opencodeReport := sddruntime.Verify("opencode", base)

	for _, report := range []struct {
		name string
		r    sddruntime.IntegrityReport
	}{
		{name: "claude", r: claudeReport},
		{name: "opencode", r: opencodeReport},
	} {
		if report.r.Status != sddruntime.StatusFail {
			t.Fatalf("%s expected blocked status for layer-role parity drift, got %q", report.name, report.r.Status)
		}
		check := checkStatusByKey(report.r.Checks, "invariant.prompt.required_sources_order")
		if check != sddruntime.StatusFail {
			t.Fatalf("%s expected required source ordering check fail, got %q", report.name, check)
		}
	}
}

func TestObserveRuntime_ParsesRenderedOrchestratorAssignments(t *testing.T) {
	tests := []struct {
		name             string
		orchestratorBody string
		wantApplyModel   string
	}{
		{
			name: "reads explicit assignment from table",
			orchestratorBody: `# test
| Phase | Default Model | Reason |
|-------|---------------|--------|
| default | sonnet | baseline |
| orchestrator | opus | coordination |
| sdd-apply | haiku | implementation |
`,
			wantApplyModel: "haiku",
		},
		{
			name:             "falls back to contract when table is missing",
			orchestratorBody: "# test\nNo model table",
			wantApplyModel:   "sonnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			a := &OpenCodeAgent{home: home, templatesFS: testTemplatesFS}

			if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}

			if err := a.InstallOrchestrator([]byte(tt.orchestratorBody)); err != nil {
				t.Fatalf("InstallOrchestrator: %v", err)
			}
			if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
				t.Fatalf("WriteInstructions: %v", err)
			}
			if err := installOptionalManagedArtifacts(a.ConfigDir()); err != nil {
				t.Fatalf("install optional managed artifacts: %v", err)
			}
			skillsFS := fstest.MapFS{"_shared/SKILL.md": {Data: []byte("# shared")}}
			if err := a.InstallSkills(skillsFS, nil); err != nil {
				t.Fatalf("InstallSkills: %v", err)
			}

			observed, err := a.ObserveRuntime()
			if err != nil {
				t.Fatalf("ObserveRuntime: %v", err)
			}

			if got := observed.ModelAssignments["sdd-apply"]; got != tt.wantApplyModel {
				t.Fatalf("sdd-apply model = %q, want %q", got, tt.wantApplyModel)
			}
		})
	}
}

func TestObserveRuntime_FallbackIgnoresStaleLegacyContractAssignments(t *testing.T) {
	tests := []struct {
		name      string
		agent     string
		wantPhase string
		wantModel string
	}{
		{name: "opencode fallback derives from platform defaults", agent: "opencode", wantPhase: "sdd-apply", wantModel: "sonnet"},
		{name: "claude fallback derives from platform defaults", agent: "claude", wantPhase: "orchestrator", wantModel: "opus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			configDir := home

			if err := os.MkdirAll(configDir, 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}

			plan, err := runtimePlanFor(tt.agent)
			if err != nil {
				t.Fatalf("runtimePlanFor returned error: %v", err)
			}
			plan.Contract.ModelAssignments = map[string]string{
				"sdd-apply":    "stale-legacy-value",
				"orchestrator": "stale-legacy-value",
			}

			if err := os.WriteFile(configDir+"/"+"sdd-orchestrator.md", []byte("# no table"), 0644); err != nil {
				t.Fatalf("write orchestrator artifact: %v", err)
			}
			if err := os.WriteFile(configDir+"/"+"AGENTS.md", []byte("<!-- jarvis:layer1:start -->\nX\n<!-- jarvis:layer1:end -->"), 0644); err != nil {
				t.Fatalf("write instructions artifact: %v", err)
			}
			if err := os.MkdirAll(configDir+"/skills", 0755); err != nil {
				t.Fatalf("mkdir skills dir: %v", err)
			}
			if err := installOptionalManagedArtifacts(configDir); err != nil {
				t.Fatalf("install optional managed artifacts: %v", err)
			}

			observed, err := observeRuntime(configDir, plan)
			if err != nil {
				t.Fatalf("observeRuntime returned error: %v", err)
			}

			if got := observed.ModelAssignments[tt.wantPhase]; got != tt.wantModel {
				t.Fatalf("phase %q model = %q, want %q", tt.wantPhase, got, tt.wantModel)
			}
		})
	}
}

func TestObserveRuntime_UsesConfiguredStoreModeContract(t *testing.T) {
	tests := []struct {
		name      string
		storeMode string
		wantRead  []string
		wantWrite []string
	}{
		{name: "hive", storeMode: "hive", wantRead: []string{"hive"}, wantWrite: []string{"hive"}},
		{name: "openspec", storeMode: "openspec", wantRead: []string{"openspec"}, wantWrite: []string{"openspec"}},
		{name: "hybrid", storeMode: "hybrid", wantRead: []string{"hive", "openspec"}, wantWrite: []string{"hive", "openspec"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JARVIS_SDD_STORE_MODE", tt.storeMode)

			home := t.TempDir()
			a := &OpenCodeAgent{home: home, templatesFS: testTemplatesFS}
			if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
				t.Fatalf("mkdir config dir: %v", err)
			}
			if err := a.WriteInstructions("# Layer1", "# Layer2", nil); err != nil {
				t.Fatalf("WriteInstructions: %v", err)
			}
			if err := a.InstallOrchestrator([]byte("# orchestrator")); err != nil {
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

			if observed.StoreMode != tt.storeMode {
				t.Fatalf("StoreMode = %q, want %q", observed.StoreMode, tt.storeMode)
			}
			if got := strings.Join(observed.StoreReadFrom, ","); got != strings.Join(tt.wantRead, ",") {
				t.Fatalf("StoreReadFrom = %v, want %v", observed.StoreReadFrom, tt.wantRead)
			}
			if got := strings.Join(observed.StoreWriteTo, ","); got != strings.Join(tt.wantWrite, ",") {
				t.Fatalf("StoreWriteTo = %v, want %v", observed.StoreWriteTo, tt.wantWrite)
			}
		})
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

func installOptionalManagedArtifacts(configDir string) error {
	if err := os.WriteFile(configDir+"/settings.json", []byte(`{"statusLine":{"type":"command","command":"echo ok"}}`), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(configDir+"/opencode.json", []byte(`{"model":"sonnet"}`), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(configDir+"/output-styles", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(configDir+"/hive-hooks", 0755); err != nil {
		return err
	}
	return nil
}
