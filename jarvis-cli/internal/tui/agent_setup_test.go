package tui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/projectregistry"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type setupAgentStub struct {
	name                    string
	mergeErrAt              int
	installSkillsErr        error
	installOrchErr          error
	writeInstructionsErr    error
	outputStyleErr          error
	observeRuntime          sddruntime.ObservedRuntime
	observeRuntimeErr       error
	runtimePlan             sddruntime.RuntimePlan
	runtimePlanErr          error
	registryAutomationErr   error
	installedOrchestrator   string
	layer2                  string
	v2OutputStyle           *persona.PresetV2
	observeCalls            int
	registryAutomationCalls int

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

type setupConfigAwareSkillInstallerStub struct {
	*setupAgentStub
	installSkillsWithConfigCalls    int
	installSkillsWithConfigCfg      *config.AppConfig
	installSkillsWithConfigSelected []string
	installSkillsWithConfigErr      error
}

func (a *setupConfigAwareSkillInstallerStub) InstallSkillsWithConfig(_ fs.FS, selected []string, cfg *config.AppConfig) error {
	a.installSkillsWithConfigCalls++
	a.installSkillsWithConfigCfg = cfg
	a.installSkillsWithConfigSelected = append([]string(nil), selected...)
	return a.installSkillsWithConfigErr
}

func (a *setupAgentStub) MergeConfig(entry agent.MCPEntry) error {
	a.mergeCalls++
	if a.mergeErrAt > 0 && a.mergeCalls == a.mergeErrAt {
		return errors.New("merge failed: " + entry.Name)
	}
	return nil
}

func (a *setupAgentStub) WriteInstructions(_ string, layer2 string, _ []config.SkillInfo) error {
	a.layer2 = layer2
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

func (a *setupAgentStub) InstallRegistryAutomation(fs.FS) error {
	a.registryAutomationCalls++
	return a.registryAutomationErr
}

func (a *setupAgentStub) WriteOutputStyle(*persona.Preset) error {
	return a.outputStyleErr
}

func (a *setupAgentStub) WriteOutputStyleV2(preset *persona.PresetV2) error {
	a.v2OutputStyle = preset
	return a.outputStyleErr
}

// setupStatuslineAgentStub wraps setupAgentStub and implements statuslineInstaller
// so that the InstallStatusline error-propagation path can be exercised.
type setupStatuslineAgentStub struct {
	*setupAgentStub
	installStatuslineErr error
}

func (a *setupStatuslineAgentStub) InstallStatusline(_ fs.FS, _ func() bool) error {
	return a.installStatuslineErr
}

// setupAgentInstallerStub wraps setupAgentStub and also implements agent.AgentInstaller
// so that the InstallAgents wiring path can be exercised.
type setupAgentInstallerStub struct {
	*setupAgentStub
	installAgentsCalls int
	installAgentsFS    fs.FS
	installAgentsErr   error
}

func (a *setupAgentInstallerStub) InstallAgents(agentsSubFS fs.FS) error {
	a.installAgentsCalls++
	a.installAgentsFS = agentsSubFS
	return a.installAgentsErr
}

// setupClaudeHookStub wraps setupAgentStub and implements agent.CompactHookInstaller
// and agent.SubagentStopHookInstaller so that the Claude-only hook wiring path can be tested.
type setupClaudeHookStub struct {
	*setupAgentStub
	installCompactHookCalls      int
	installCompactHookErr        error
	installSubagentStopHookCalls int
	installSubagentStopHookErr   error
}

func (a *setupClaudeHookStub) InstallCompactHook() error {
	a.installCompactHookCalls++
	return a.installCompactHookErr
}

func (a *setupClaudeHookStub) InstallSubagentStopHook() error {
	a.installSubagentStopHookCalls++
	return a.installSubagentStopHookErr
}

// TestConfigureWizardAgent_InstallsAgents asserts that when the agent implements
// AgentInstaller, InstallAgents is called during configureWizardAgent, and that
// when the agent does NOT implement AgentInstaller, no error occurs.
func TestConfigureWizardAgent_InstallsAgents(t *testing.T) {
	t.Run("calls InstallAgents when agent implements AgentInstaller", func(t *testing.T) {
		stub := &setupAgentInstallerStub{
			setupAgentStub: &setupAgentStub{name: "claude"},
		}
		agentsSubFS := fstest.MapFS{"review-risk.md": {Data: []byte("# review-risk")}}

		_, err := configureWizardAgent(stub, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, agentsSubFS, func() bool { return true })
		if err != nil {
			t.Fatalf("configureWizardAgent returned error: %v", err)
		}
		if stub.installAgentsCalls != 1 {
			t.Fatalf("InstallAgents calls = %d, want 1", stub.installAgentsCalls)
		}
		if stub.installAgentsFS == nil {
			t.Fatal("InstallAgents was not called with the provided agentsSubFS (got nil)")
		}
	})

	t.Run("does not call InstallAgents when agent does not implement AgentInstaller", func(t *testing.T) {
		stub := &setupAgentStub{name: "opencode"}

		_, err := configureWizardAgent(stub, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, nil, func() bool { return true })
		if err != nil {
			t.Fatalf("configureWizardAgent returned error for non-AgentInstaller agent: %v", err)
		}
	})

	t.Run("propagates InstallAgents error", func(t *testing.T) {
		stub := &setupAgentInstallerStub{
			setupAgentStub:   &setupAgentStub{name: "claude"},
			installAgentsErr: errors.New("agents dir write failed"),
		}
		agentsSubFS := fstest.MapFS{"review-risk.md": {Data: []byte("# review-risk")}}

		_, err := configureWizardAgent(stub, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, agentsSubFS, func() bool { return true })
		if err == nil {
			t.Fatal("expected error when InstallAgents fails")
		}
		if !strings.Contains(err.Error(), "install agents") {
			t.Fatalf("error = %q, want contains 'install agents'", err.Error())
		}
	})

}

// TestConfigureWizardAgent_InstallsClaudeOnlyHooks asserts that when the agent
// implements CompactHookInstaller and SubagentStopHookInstaller (Claude-only
// capabilities), configureWizardAgent calls both install methods, and that
// non-Claude agents without those interfaces are unaffected.
func TestConfigureWizardAgent_InstallsClaudeOnlyHooks(t *testing.T) {
	t.Run("calls InstallCompactHook and InstallSubagentStopHook when agent implements them", func(t *testing.T) {
		stub := &setupClaudeHookStub{
			setupAgentStub: &setupAgentStub{name: "claude"},
		}

		_, err := configureWizardAgent(stub, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, nil, func() bool { return true })
		if err != nil {
			t.Fatalf("configureWizardAgent returned error: %v", err)
		}
		if stub.installCompactHookCalls != 1 {
			t.Fatalf("InstallCompactHook calls = %d, want 1", stub.installCompactHookCalls)
		}
		if stub.installSubagentStopHookCalls != 1 {
			t.Fatalf("InstallSubagentStopHook calls = %d, want 1", stub.installSubagentStopHookCalls)
		}
	})

	t.Run("does not fail when agent does not implement Claude-only hook interfaces", func(t *testing.T) {
		stub := &setupAgentStub{name: "opencode"}

		_, err := configureWizardAgent(stub, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, nil, func() bool { return true })
		if err != nil {
			t.Fatalf("configureWizardAgent returned error for agent without Claude-only hooks: %v", err)
		}
	})
}

func TestConfigureWizardAgent_ErrorPropagation(t *testing.T) {
	tests := []struct {
		name    string
		agent   agent.Agent
		wantErr string
	}{
		{name: "hive merge fails", agent: &setupAgentStub{name: "claude", mergeErrAt: 1}, wantErr: "hive MCP config"},
		{name: "context7 merge fails", agent: &setupAgentStub{name: "claude", mergeErrAt: 2}, wantErr: "context7 MCP config"},
		{name: "install skills fails", agent: &setupAgentStub{name: "claude", installSkillsErr: errors.New("skills fail")}, wantErr: "install skills"},
		{name: "install orchestrator fails", agent: &setupAgentStub{name: "claude", installOrchErr: errors.New("orchestrator fail")}, wantErr: "install orchestrator"},
		{
			name: "install statusline fails",
			agent: &setupStatuslineAgentStub{
				setupAgentStub:       &setupAgentStub{name: "claude"},
				installStatuslineErr: errors.New("statusline fail"),
			},
			wantErr: "install statusline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := configureWizardAgent(tt.agent, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, nil, func() bool { return true })
			if err == nil {
				t.Fatalf("configureWizardAgent expected error containing %q", tt.wantErr)
			}
			if got := err.Error(); got == "" || !strings.Contains(got, tt.wantErr) {
				t.Fatalf("error = %q, want contains %q", got, tt.wantErr)
			}
		})
	}
}

func TestConfigureWizardAgent_InstallsSupportedRegistryAutomation(t *testing.T) {
	a := &setupAgentStub{name: "claude"}

	warnings, err := configureWizardAgent(a, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, nil, func() bool { return true })
	if err != nil {
		t.Fatalf("configureWizardAgent returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected registry automation warnings: %v", warnings)
	}
	if a.registryAutomationCalls != 1 {
		t.Fatalf("registry automation calls = %d, want 1", a.registryAutomationCalls)
	}
}

func TestConfigureWizardAgent_RegistryAutomationFailureIsWarningOnly(t *testing.T) {
	a := &setupAgentStub{name: "claude", registryAutomationErr: errors.New("disk full")}

	warnings, err := configureWizardAgent(a, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil, nil, func() bool { return true })
	if err != nil {
		t.Fatalf("configureWizardAgent returned error for optional registry automation failure: %v", err)
	}
	if a.registryAutomationCalls != 1 {
		t.Fatalf("registry automation calls = %d, want 1", a.registryAutomationCalls)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Project skill registry warning: automation not installed for claude") || !strings.Contains(warnings[0], "disk full") {
		t.Fatalf("expected visible registry automation warning, got %v", warnings)
	}
}

func TestConfigureWizardAgents_SurfacesRegistryAutomationWarningsWithoutFailing(t *testing.T) {
	assignments, err := sddruntime.DefaultAssignmentsForPlatform(sddruntime.PlatformClaude)
	if err != nil {
		t.Fatalf("resolve default assignments: %v", err)
	}
	a := &setupAgentStub{
		name:                  "claude",
		registryAutomationErr: errors.New("readonly config"),
		observeRuntime:        passingRuntimeObservation(t, "claude", assignments, nil),
	}

	results := configureWizardAgents([]agent.Agent{a}, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil, nil, func() bool { return true })
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Err != nil || !results[0].State.Configured {
		t.Fatalf("registry automation failure should not fail agent setup, got %+v", results[0])
	}
	if len(results[0].Warnings) != 1 || !strings.Contains(results[0].Warnings[0], "readonly config") {
		t.Fatalf("expected surfaced registry automation warning, got %+v", results[0])
	}
}

func TestConfigureWizardAgent_PrefersConfigAwareSkillInstallation(t *testing.T) {
	tests := []struct {
		name                       string
		installSkillsWithConfigErr error
		wantErr                    string
	}{
		{name: "config-aware installer succeeds"},
		{name: "config-aware installer error is returned", installSkillsWithConfigErr: errors.New("config-aware skills fail"), wantErr: "config-aware skills fail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.AppConfig{}
			selectedIDs := []string{"sdd-apply", "sdd-verify"}
			a := &setupConfigAwareSkillInstallerStub{
				setupAgentStub: &setupAgentStub{
					name:             "opencode",
					installSkillsErr: errors.New("legacy InstallSkills should not be called"),
				},
				installSkillsWithConfigErr: tt.installSkillsWithConfigErr,
			}

			warnings, err := configureWizardAgent(a, cfg, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, selectedIDs, nil, func() bool { return true })
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("configureWizardAgent expected error containing %q", tt.wantErr)
				}
				if got := err.Error(); !strings.Contains(got, tt.wantErr) {
					t.Fatalf("error = %q, want contains %q", got, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("configureWizardAgent returned error: %v", err)
			}
			if tt.wantErr == "" && len(warnings) != 0 {
				t.Fatalf("unexpected registry automation warnings: %v", warnings)
			}

			if a.installSkillsWithConfigCalls != 1 {
				t.Fatalf("InstallSkillsWithConfig calls = %d, want 1", a.installSkillsWithConfigCalls)
			}
			if a.installSkillsWithConfigCfg != cfg {
				t.Fatalf("InstallSkillsWithConfig cfg = %p, want %p", a.installSkillsWithConfigCfg, cfg)
			}
			if got := strings.Join(a.installSkillsWithConfigSelected, ","); got != strings.Join(selectedIDs, ",") {
				t.Fatalf("InstallSkillsWithConfig selected = %q, want %q", got, strings.Join(selectedIDs, ","))
			}
		})
	}
}

func TestRunAgentConfigSequence_RefreshesProjectRegistryAfterSuccessfulApplyAndReportsWarnings(t *testing.T) {
	tmpHome := isolateTestHome(t)
	projectRoot := t.TempDir()

	called := false
	originalRefresh := refreshProjectSkillRegistry
	refreshProjectSkillRegistry = func(ctx context.Context, opts projectregistry.RefreshOptions) (projectregistry.Result, error) {
		called = true
		if opts.CWD != projectRoot {
			t.Fatalf("refresh cwd = %q, want %q", opts.CWD, projectRoot)
		}
		if _, err := os.Stat(filepath.Join(tmpHome, ".jarvis", "config.yaml")); err != nil {
			t.Fatalf("project registry refresh should run after config save, got stat err=%v", err)
		}
		return projectregistry.Result{Warnings: []projectregistry.Warning{{Message: "legacy registry imported"}}}, nil
	}
	t.Cleanup(func() { refreshProjectSkillRegistry = originalRefresh })

	m := Model{
		Step:       StepAgentConfig,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalOnly},
		ProjectCWD: projectRoot,
	}

	msg := runAgentConfigSequence(m)()
	progress, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}
	if !progress.done || progress.failed {
		t.Fatalf("expected successful completion despite registry warning, got %+v", progress)
	}
	if !called {
		t.Fatal("expected project registry refresh to run after TUI apply")
	}
	if !strings.Contains(progress.line, "Project skill registry warning: legacy registry imported") {
		t.Fatalf("expected registry warning in progress line, got %q", progress.line)
	}
}

func TestRunAgentConfigSequence_ProjectRegistryNonProjectFailureIsWarningOnly(t *testing.T) {
	isolateTestHome(t)
	projectRoot := t.TempDir()

	originalRefresh := refreshProjectSkillRegistry
	refreshProjectSkillRegistry = func(context.Context, projectregistry.RefreshOptions) (projectregistry.Result, error) {
		return projectregistry.Result{}, projectregistry.ErrNotGitWorktree
	}
	t.Cleanup(func() { refreshProjectSkillRegistry = originalRefresh })

	m := Model{
		Step:       StepAgentConfig,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalOnly},
		ProjectCWD: projectRoot,
	}

	msg := runAgentConfigSequence(m)()
	progress, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}
	if !progress.done || progress.failed {
		t.Fatalf("expected successful completion despite registry refresh failure, got %+v", progress)
	}
	if !strings.Contains(progress.line, "Project skill registry warning: not a git worktree") {
		t.Fatalf("expected refresh failure warning in progress line, got %q", progress.line)
	}
}

func TestRunAgentConfigSequence_ProjectRegistryWriteFailureIsBlocking(t *testing.T) {
	isolateTestHome(t)
	projectRoot := t.TempDir()

	originalRefresh := refreshProjectSkillRegistry
	refreshProjectSkillRegistry = func(context.Context, projectregistry.RefreshOptions) (projectregistry.Result, error) {
		return projectregistry.Result{}, errors.New("write skill registry: finalize registry: permission denied")
	}
	t.Cleanup(func() { refreshProjectSkillRegistry = originalRefresh })

	m := Model{
		Step:       StepAgentConfig,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalOnly},
		ProjectCWD: projectRoot,
	}

	msg := runAgentConfigSequence(m)()
	progress, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}
	if !progress.done || !progress.failed {
		t.Fatalf("expected registry write failure to block successful completion, got %+v", progress)
	}
	if !strings.Contains(progress.line, "Project skill registry refresh failed") || !strings.Contains(progress.line, "permission denied") {
		t.Fatalf("expected blocking registry failure in progress line, got %q", progress.line)
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
			results := configureWizardAgents(tt.agents, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, tt.resolved, wizardPresetApplyContext{}, testSkillsFS, nil, nil, func() bool { return true })
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

func TestApplyWizardPresetSelectionAcceptsDormantV2(t *testing.T) {
	stub := &setupAgentStub{name: "claude"}
	selection := persona.PresetSelection{V2: &persona.ResolvedPresetV2{
		Slug: "custom-mentor",
		Preset: &persona.PresetV2{
			Name: "custom-mentor",
			Presentation: persona.PresentationV2{
				Language: "en-us", Register: "friendly-professional", Vocabulary: "plain-technical", Cadence: "measured",
				Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured",
				TeachingMetaphors: "construction", Examples: "practical", AddressPack: "peer", PhrasePack: "plain", AntiCaricature: "grounded",
			},
		},
	}}

	if err := applyWizardPresetSelection([]agent.Agent{stub}, selection, wizardPresetApplyContext{Layer1: "layer1"}); err != nil {
		t.Fatalf("applyWizardPresetSelection() error = %v", err)
	}
	if !strings.Contains(stub.layer2, "### Presentation") || strings.Contains(stub.layer2, "Behavioral Rules") {
		t.Fatalf("V2 wizard layer2 = %q, want presentation-only content", stub.layer2)
	}
	if stub.v2OutputStyle == nil || stub.v2OutputStyle.Name != "custom-mentor" {
		t.Fatalf("V2 output style preset = %+v, want custom-mentor", stub.v2OutputStyle)
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
			results := configureWizardAgents([]agent.Agent{tt.agent}, &config.AppConfig{}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil, nil, func() bool { return true })
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

	results := configureWizardAgents([]agent.Agent{a}, pendingCfg, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil, nil, func() bool { return true })
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
		"review-risk", "review-readability", "review-reliability", "review-resilience",
	}
	return sddruntime.ObservedOpenCodeConfig{
		ParseSucceeded:               true,
		ShareMode:                    "disabled",
		DefaultAgent:                 "sdd-orchestrator",
		OrchestratorMode:             "primary",
		OrchestratorModel:            "legacy=opus",
		OrchestratorPrompt:           "{file:./sdd-orchestrator.md}",
		AgentNames:                   append([]string{"sdd-orchestrator"}, subagents...),
		HiddenSubagents:              subagents,
		TaskAllows:                   subagents,
		TaskWildcardDeny:             true,
		BashWildcardAllow:            true,
		ReadSecretDenies:             true,
		MCPHivePresent:               true,
		MCPContext7Present:           true,
		PluginHiveExists:             true,
		SDDSubagentHiveGrantEvidence: compliantOpenCodeHiveGrantEvidenceForTUI(),
	}
}

func compliantOpenCodeHiveGrantEvidenceForTUI() map[string][]sddruntime.OpenCodePermissionEvidence {
	evidence := make(map[string][]sddruntime.OpenCodePermissionEvidence)
	for _, definition := range agent.SDDPhaseAgentDefinitions() {
		evidence[definition.Name] = compliantOpenCodeHiveToolEvidenceForTUI()
	}
	return evidence
}

func compliantOpenCodeHiveToolEvidenceForTUI() []sddruntime.OpenCodePermissionEvidence {
	tools := agent.RequiredOpenCodeHiveMCPTools()
	evidence := make([]sddruntime.OpenCodePermissionEvidence, 0, len(tools))
	for _, tool := range tools {
		evidence = append(evidence, sddruntime.OpenCodePermissionEvidence{Key: tool, Action: "allow"})
	}
	return evidence
}
