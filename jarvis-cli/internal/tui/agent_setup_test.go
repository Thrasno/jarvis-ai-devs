package tui

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

type setupAgentStub struct {
	name                 string
	mergeErrAt           int
	installSkillsErr     error
	installOrchErr       error
	writeInstructionsErr error
	outputStyleErr       error
	observeRuntime       sddruntime.ObservedRuntime
	observeRuntimeErr    error
	runtimePlan          sddruntime.RuntimePlan
	runtimePlanErr       error
	observeCalls         int

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

func (a *setupAgentStub) InstallOrchestrator(fs.FS) error {
	return a.installOrchErr
}

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
			err := configureWizardAgent(tt.agent, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, testSkillsFS, nil)
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
				&setupAgentStub{name: "a", mergeErrAt: 1},
				&setupAgentStub{name: "b"},
			},
			resolved:       nil,
			wantLen:        1,
			wantConfigured: false,
			wantErrSubstr:  "hive MCP config",
		},
		{
			name: "returns configured results when no preset to apply",
			agents: []agent.Agent{
				&setupAgentStub{name: "a"},
				&setupAgentStub{name: "b"},
			},
			resolved:       nil,
			wantLen:        2,
			wantConfigured: true,
		},
		{
			name: "pipeline error is attached to last result",
			agents: []agent.Agent{
				&setupAgentStub{name: "a"},
				&setupAgentStub{name: "b", writeInstructionsErr: errors.New("instruction fail")},
			},
			resolved:       &persona.ResolvedPreset{Slug: "neutra", Source: persona.PresetSourceBuiltin, Preset: &persona.Preset{Name: "neutra", DisplayName: "Neutra", Description: "x", Tone: persona.Tone{Formality: "neutral", Directness: "direct", Humor: "none", Language: "en-us"}, CommunicationStyle: persona.CommunicationStyle{Verbosity: "concise"}, CharacteristicPhrases: persona.CharacteristicPhrases{Greetings: []string{"Hi"}, Confirmations: []string{"OK"}}}},
			wantLen:        2,
			wantConfigured: true,
			wantErrSubstr:  "apply preset pipeline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := configureWizardAgents(tt.agents, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, tt.resolved, wizardPresetApplyContext{}, testSkillsFS, nil)
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
	passObserved := sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            true,
			ContractVersion:    sddruntime.DefaultContractVersion,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath: sddruntime.DefaultRegistryPath,
		ModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
			"default":      "sonnet",
		},
		Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}

	tests := []struct {
		name          string
		agent         *setupAgentStub
		wantConfigured bool
		wantErrSubstr string
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
				name:          "opencode",
				observeRuntime: passObserved,
			},
			wantConfigured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := configureWizardAgents([]agent.Agent{tt.agent}, agent.MCPEntry{Name: "hive"}, agent.MCPEntry{Name: "context7"}, nil, wizardPresetApplyContext{}, testSkillsFS, nil)
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
