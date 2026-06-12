package tui

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type setupAgentStub struct {
	name                  string
	mergeErrAt            int
	installSkillsErr      error
	installOrchErr        error
	writeInstructionsErr  error
	outputStyleErr        error
	observeRuntime        sddruntime.ObservedRuntime
	observeRuntimeErr     error
	runtimePlan           sddruntime.RuntimePlan
	runtimePlanErr        error
	installedOrchestrator string
	observeCalls          int

	mergeCalls int
}

func (a *setupAgentStub) Name() string                  { return a.name }
func (a *setupAgentStub) IsInstalled() bool             { return true }
func (a *setupAgentStub) ConfigDir() string             { return "/tmp/" + a.name }
func (a *setupAgentStub) SupportsOutputStyles() bool    { return true }
func (a *setupAgentStub) ClearOutputStyle(string) error { return nil }
func (a *setupAgentStub) RuntimePlan() (sddruntime.RuntimePlan, error) {
	if a.runtimePlanErr != nil {
		return sddruntime.RuntimePlan{}, a.runtimePlanErr
	}
	return a.runtimePlan, nil
}
func (a *setupAgentStub) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	a.observeCalls++
	if a.observeRuntimeErr != nil {
		return sddruntime.ObservedRuntime{}, a.observeRuntimeErr
	}
	return a.observeRuntime, nil
}

type setupConfigAwareAgentStub struct {
	*setupAgentStub
	observeRuntimeWithConfig func(*config.AppConfig) (sddruntime.ObservedRuntime, error)
}

func (a *setupConfigAwareAgentStub) ObserveRuntimeWithConfig(cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
	if a.observeRuntimeWithConfig != nil {
		a.observeCalls++
		return a.observeRuntimeWithConfig(cfg)
	}
	return a.ObserveRuntime()
}

func (a *setupAgentStub) MergeConfig(entry agent.MCPEntry) error {
	a.mergeCalls++
	if a.mergeErrAt > 0 && a.mergeCalls == a.mergeErrAt {
		return errors.New("merge failed: " + entry.Name)
	}
	return nil
}

func (a *setupAgentStub) WriteInstructions(string, string, []config.SkillInfo) error {
	return a.writeInstructionsErr
}

func (a *setupAgentStub) InstallSkills(fs.FS, []string) error {
	return a.installSkillsErr
}

func (a *setupAgentStub) InstallOrchestrator(content []byte) error {
	a.installedOrchestrator = string(content)
	return a.installOrchErr
}

func (a *setupAgentStub) InstallPromptHook(fs.FS) error {
	return nil
}

func (a *setupAgentStub) InstallSessionHooks(fs.FS) error { return nil }

func (a *setupAgentStub) WriteOutputStyle(*persona.Preset) error {
	return a.outputStyleErr
}

func TestConfigureWizardAgent_ErrorPropagation(t *testing.T) {
	tests := []struct {
		name    string
		agent   *setupAgentStub
		wantErr string
	}{
		{name: "hive merge fails", agent: &setupAgentStub{name: "claude", mergeErrAt: 1}, wantErr: "hive MCP config"},
		{name: "context7 merge fails", agent: &setupAgentStub{name: "claude", mergeErrAt: 2}, wantErr: "context7 MCP config"},
		{name: "install skills fails", agent: &setupAgentStub{name: "claude", installSkillsErr: errors.New("skills fail")}, wantErr: "install skills"},
		{name: "install orchestrator fails", agent: &setupAgentStub{name: "claude", installOrchErr: errors.New("orchestrator fail")}, wantErr: "install orchestrator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := configureWizardAgent(tt.agent, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil)
			if err == nil {
				t.Fatalf("configureWizardAgent expected error containing %q", tt.wantErr)
			}
			if got := err.Error(); got == "" || !strings.Contains(got, tt.wantErr) {
				t.Fatalf("error = %q, want contains %q", got, tt.wantErr)
			}
		})
	}
}

func TestConfigureWizardAgents_AggregatesResults(t *testing.T) {
	tests := []struct {
		name           string
		agents         []agent.Agent
		resolved       *persona.ResolvedPreset
		wantLen        int
		wantConfigured bool
		wantErrSubstr  string
	}{
		{
			name: "stops on first setup failure",
			agents: []agent.Agent{
				&setupAgentStub{name: "claude", mergeErrAt: 1},
				&setupAgentStub{name: "opencode"},
			},
			resolved:       nil,
			wantLen:        1,
			wantConfigured: false,
			wantErrSubstr:  "hive MCP config",
		},
		{
			name: "returns configured results when no preset to apply",
			agents: []agent.Agent{
				&setupAgentStub{name: "claude"},
				&setupAgentStub{name: "opencode"},
			},
			resolved:       nil,
			wantLen:        2,
			wantConfigured: true,
		},
		{
			name: "pipeline error is attached to last result",
			agents: []agent.Agent{
				&setupAgentStub{name: "claude"},
				&setupAgentStub{name: "opencode", writeInstructionsErr: errors.New("instruction fail")},
			},
			resolved:       &persona.ResolvedPreset{Slug: "neutra", Source: persona.PresetSourceBuiltin, Preset: &persona.Preset{Name: "neutra", DisplayName: "Neutra", Description: "x", Tone: persona.Tone{Formality: "neutral", Directness: "direct", Humor: "none", Language: "en-us"}, CommunicationStyle: persona.CommunicationStyle{Verbosity: "concise"}, CharacteristicPhrases: persona.CharacteristicPhrases{Greetings: []string{"Hi"}, Confirmations: []string{"OK"}}}},
			wantLen:        2,
			wantConfigured: true,
			wantErrSubstr:  "apply preset pipeline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := configureWizardAgents(tt.agents, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, tt.resolved, wizardPresetApplyContext{}, testSkillsFS, nil)
			if len(results) != tt.wantLen {
				t.Fatalf("len(results) = %d, want %d", len(results), tt.wantLen)
			}

			last := results[len(results)-1]
			if last.State.Configured != tt.wantConfigured {
				t.Fatalf("last configured = %v, want %v", last.State.Configured, tt.wantConfigured)
			}
			if tt.wantErrSubstr == "" {
				if last.Err != nil {
					t.Fatalf("unexpected last error: %v", last.Err)
				}
				return
			}
			if last.Err == nil {
				t.Fatalf("expected last error containing %q", tt.wantErrSubstr)
			}
			if got := last.Err.Error(); !strings.Contains(got, tt.wantErrSubstr) {
				t.Fatalf("last error = %q, want contains %q", got, tt.wantErrSubstr)
			}
		})
	}
}

func TestConfigureWizardAgents_RuntimeVerification(t *testing.T) {
	contract := sddruntime.DefaultContract()
	assignments := make(map[string]string, len(contract.ModelAssignments))
	for k, v := range contract.ModelAssignments {
		assignments[k] = v
	}

	passObserved := sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            true,
			ContractVersion:    sddruntime.DefaultContractVersion,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath: sddruntime.DefaultRegistryPath,
		PromptSourceIDs: []string{
			"layer1.behavior",
			"layer2.persona",
			"skill.sdd-orchestrator",
			"registry.skill-index",
			"protocol.hive",
		},
		StoreMode:           "hybrid",
		StoreReadFrom:       []string{"hive", "openspec"},
		StoreWriteTo:        []string{"hive", "openspec"},
		ArtifactTopics:      []string{"sdd/runtime/verify"},
		GeneralMemoryTopics: []string{"runtime/notes"},
		ModelAssignments:    assignments,
		Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
		// OpenCode field must be populated for the opencode agent so that
		// verifier checks do not fire spurious failures.
		OpenCode: compliantOpenCodeObservedForTUI(),
	}

	tests := []struct {
		name           string
		agent          *setupAgentStub
		wantConfigured bool
		wantErrSubstr  string
	}{
		{
			name: "fails setup when owned drift is detected",
			agent: &setupAgentStub{
				name: "claude",
				observeRuntime: sddruntime.ObservedRuntime{
					Manifest: sddruntime.RuntimeManifestState{Present: false},
				},
			},
			wantConfigured: false,
			wantErrSubstr:  "checks=manifest.present",
		},
		{
			name: "keeps setup successful for pass report",
			agent: &setupAgentStub{
				name:           "opencode",
				observeRuntime: passObserved,
			},
			wantConfigured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := configureWizardAgents([]agent.Agent{tt.agent}, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil)
			if len(results) != 1 {
				t.Fatalf("len(results) = %d, want 1", len(results))
			}

			got := results[0]
			if got.State.Configured != tt.wantConfigured {
				t.Fatalf("configured = %v, want %v", got.State.Configured, tt.wantConfigured)
			}
			if tt.wantErrSubstr == "" {
				if got.Err != nil {
					t.Fatalf("unexpected error: %v", got.Err)
				}
				if tt.agent.observeCalls == 0 {
					t.Fatalf("observe runtime was not called")
				}
				return
			}
			if got.Err == nil {
				t.Fatalf("expected error containing %q", tt.wantErrSubstr)
			}
			if !strings.Contains(got.Err.Error(), tt.wantErrSubstr) {
				t.Fatalf("error = %q, want contains %q", got.Err.Error(), tt.wantErrSubstr)
			}
		})
	}
}

func TestVerifyConfiguredAgentRuntime_NilConfigFallsBackToObservedRuntime(t *testing.T) {
	assignments, err := sddruntime.DefaultAssignmentsForPlatform(sddruntime.PlatformClaude)
	if err != nil {
		t.Fatalf("resolve default assignments: %v", err)
	}
	a := &setupAgentStub{
		name:           "claude",
		observeRuntime: passingRuntimeObservation(t, "claude", assignments, nil),
	}

	if err := verifyConfiguredAgentRuntime(a, nil); err != nil {
		t.Fatalf("verifyConfiguredAgentRuntime with nil config returned error: %v", err)
	}
	if a.observeCalls != 1 {
		t.Fatalf("observe calls = %d, want 1", a.observeCalls)
	}
}

func TestConfigureWizardAgents_RuntimeVerificationUsesPendingConfigForOpenCodeDefault(t *testing.T) {
	pendingCfg := &config.AppConfig{}
	pendingCfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
	}

	pendingAssignments, err := sddruntime.ResolveAssignmentsForPlatform(sddruntime.PlatformOpenCode, pendingCfg)
	if err != nil {
		t.Fatalf("resolve pending assignments: %v", err)
	}
	staleAssignments, err := sddruntime.DefaultAssignmentsForPlatform(sddruntime.PlatformOpenCode)
	if err != nil {
		t.Fatalf("resolve stale assignments: %v", err)
	}

	staleObserved := passingRuntimeObservation(t, "opencode", pendingAssignments, staleAssignments)
	a := &setupConfigAwareAgentStub{
		setupAgentStub: &setupAgentStub{
			name:           "opencode",
			observeRuntime: staleObserved,
		},
	}
	a.observeRuntimeWithConfig = func(cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
		resolved, err := sddruntime.ResolveAssignmentsForPlatform(sddruntime.PlatformOpenCode, cfg)
		if err != nil {
			return sddruntime.ObservedRuntime{}, err
		}
		observed := staleObserved
		observed.ResolvedModelAssignments = resolved
		return observed, nil
	}

	results := configureWizardAgents([]agent.Agent{a}, pendingCfg, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected verification error with pending config: %v", results[0].Err)
	}
	if !results[0].State.Configured {
		t.Fatalf("configured = false, want true")
	}
	if !strings.Contains(a.installedOrchestrator, "| default | openai/gpt-5.1-codex-max |") {
		t.Fatalf("rendered orchestrator did not use pending default assignment:\n%s", a.installedOrchestrator)
	}
	if a.observeCalls == 0 {
		t.Fatalf("observe runtime was not called")
	}
}

func passingRuntimeObservation(t *testing.T, agentName string, modelAssignments, resolvedAssignments map[string]string) sddruntime.ObservedRuntime {
	t.Helper()
	promptIDs, err := sddruntime.DefaultPromptSourceIDs(agentName, "orchestrator")
	if err != nil {
		t.Fatalf("default prompt source ids: %v", err)
	}
	storeContract, err := sddruntime.ResolveRuntimeStoreContract(sddruntime.StoreModeHive)
	if err != nil {
		t.Fatalf("resolve store contract: %v", err)
	}
	contract := sddruntime.DefaultContract()

	r := sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            true,
			ContractVersion:    contract.Version,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath:             contract.RegistryPath,
		PromptSourceIDs:          promptIDs,
		StoreMode:                string(storeContract.Mode),
		StoreReadFrom:            storeContract.ReadFrom,
		StoreWriteTo:             storeContract.WriteTo,
		ArtifactTopics:           []string{"sdd/runtime/verify"},
		GeneralMemoryTopics:      []string{"runtime/notes"},
		ModelAssignments:         modelAssignments,
		ResolvedModelAssignments: resolvedAssignments,
		Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}
	if agentName == "opencode" {
		r.OpenCode = compliantOpenCodeObservedForTUI()
	}
	return r
}

// compliantOpenCodeObservedForTUI returns an ObservedOpenCodeConfig with all
// invariant fields set to their canonical passing values. Used by test helpers
// in this package that build ObservedRuntime for the opencode agent without
// going through the full wizard flow.
func compliantOpenCodeObservedForTUI() sddruntime.ObservedOpenCodeConfig {
	subagents := []string{
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
		"sdd-init", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent",
	}
	return sddruntime.ObservedOpenCodeConfig{
		ParseSucceeded:     true,
		ShareMode:          "disabled",
		DefaultAgent:       "sdd-orchestrator",
		OrchestratorMode:   "primary",
		OrchestratorModel:  "legacy=opus",
		OrchestratorPrompt: "{file:./sdd-orchestrator.md}",
		AgentNames:         append([]string{"sdd-orchestrator"}, subagents...),
		HiddenSubagents:    subagents,
		TaskAllows:         subagents,
		TaskWildcardDeny:   true,
		BashWildcardAllow:  true,
		ReadSecretDenies:   true,
		MCPHivePresent:     true,
		MCPContext7Present: true,
		PluginHiveExists:   true,
	}
}
