package tui

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/projectregistry"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// AgentApplyResult captures per-agent setup outcome before final config commit.
type AgentApplyResult struct {
	AgentName string
	State     config.AgentState
	Warnings  []string
	Err       error
}

type wizardPresetApplyContext struct {
	Layer1               string
	Skills               []config.SkillInfo
	PreviousPresetSlug   string
	PreviousPresetSource persona.PresetSource
}

const claudeRestartGuidance = agentapply.ClaudeRestartGuidance

const mcpReplacementAcknowledgement = "I ACKNOWLEDGE"

const mcpReplacementWarning = "WARNING: Manually configured MCPs with a Jarvis-managed name at the user level will be replaced. Prior same-name configuration cannot be guaranteed restored. A failure may leave that MCP absent or partial. The operation stops; fix the cause and rerun. Do not edit the managed user-level MCP configuration while this operation runs."

var refreshProjectSkillRegistry = projectregistry.Refresh

// wizardMCPExecutor is the production boundary for the wizard's managed-MCP
// handoff. Production uses the concrete executor; tests can drive the same
// TUI and no-TUI routes without a real home or native CLI.
type wizardMCPExecutor = agentapply.MCPExecutor

var newWizardMCPExecutor = func() wizardMCPExecutor { return agent.NewProductionExecutor() }

var wizardHiveDaemonPath = agent.HiveDaemonBinaryPath

// configureWizardAgent applies the same MCP + instruction + skills setup flow
// for both TUI and no-TUI wizards. The wizard always attempts the statusline
// install and answers an existing script with its own interactive prompt.
// agentsSubFS is the sub-FS rooted at embed/agents/<platform> for file-based
// agent install (ClaudeAgent). Pass nil for platforms that use the JSON config
// builder path instead (OpenCodeAgent).
func configureWizardAgent(
	a agent.Agent,
	cfg *config.AppConfig,
	hiveEntry agent.MCPEntry,
	context7Entry agent.MCPEntry,
	skillsSubFS fs.FS,
	selectedIDs []string,
	agentsSubFS fs.FS,
	statuslineConfirm func() bool,
) ([]string, error) {
	return agentapply.ConfigureAgent(a, cfg, hiveEntry, context7Entry, skillsSubFS, selectedIDs, agentsSubFS, statuslineConfirm)
}

func requiresMCPReplacementAcknowledgement(agents []agent.Agent) bool {
	for _, configured := range agents {
		name := strings.ToLower(strings.TrimSpace(configured.Name()))
		if name == "claude" || name == "opencode" {
			return true
		}
	}
	return false
}

func mcpReplacementAcknowledged(input string) bool {
	return strings.TrimSpace(input) == mcpReplacementAcknowledgement
}

// reconcileWizardMCPs is the sole setup handoff for managed MCPs. The wizard
// supplies its own executor and daemon-path seams so tests can drive both the
// TUI and no-TUI routes without a real home or native CLI.
func reconcileWizardMCPs(agents []agent.Agent, home string) error {
	return agentapply.ReconcileMCPs(agents, home, agentapply.MCPDeps{
		NewExecutor:    newWizardMCPExecutor,
		HiveDaemonPath: wizardHiveDaemonPath,
	})
}

// configureWizardAgents applies setup to all detected agents and returns
// per-agent structured outcomes. If one agent fails, callers can abort before
// committing canonical config and still report the failing agent explicitly.
// agentsSubFS is the sub-FS rooted at embed/agents/<platform> passed through to
// configureWizardAgent for file-based agent install (ClaudeAgent).
func configureWizardAgents(
	agents []agent.Agent,
	cfg *config.AppConfig,
	hiveEntry agent.MCPEntry,
	context7Entry agent.MCPEntry,
	resolved *persona.ResolvedProfile,
	presetCtx wizardPresetApplyContext,
	skillsSubFS fs.FS,
	selectedIDs []string,
	agentsSubFS fs.FS,
	statuslineConfirm func() bool,
) []AgentApplyResult {
	results := make([]AgentApplyResult, 0, len(agents))
	for _, a := range agents {
		res := AgentApplyResult{
			AgentName: a.Name(),
			State: config.AgentState{
				Configured: false,
				ConfigPath: a.ConfigDir(),
			},
		}
		warnings, err := configureWizardAgent(a, cfg, hiveEntry, context7Entry, skillsSubFS, selectedIDs, agentsSubFS, statuslineConfirm)
		res.Warnings = append(res.Warnings, warnings...)
		if err != nil {
			res.Err = err
			results = append(results, res)
			return results
		}
		res.State.Configured = true
		results = append(results, res)
	}

	if resolved != nil {
		if err := applyWizardProfile(agents, resolved, wizardPresetApplyContext{
			Layer1:               presetCtx.Layer1,
			Skills:               presetCtx.Skills,
			PreviousPresetSlug:   presetCtx.PreviousPresetSlug,
			PreviousPresetSource: presetCtx.PreviousPresetSource,
		}); err != nil {
			if len(results) == 0 {
				return []AgentApplyResult{{AgentName: "persona-apply", Err: fmt.Errorf("apply preset pipeline: %w", err)}}
			}
			results[len(results)-1].Err = fmt.Errorf("apply preset pipeline: %w", err)
			return results
		}
	}

	for i, a := range agents {
		if err := verifyConfiguredAgentRuntime(a, cfg); err != nil {
			results[i].State.Configured = false
			results[i].Err = err
			return results
		}
	}

	return results
}

// applyWizardProfile applies an already resolved schema-v2 profile through the
// canonical profile pipeline.
func applyWizardProfile(agents []agent.Agent, resolved *persona.ResolvedProfile, presetCtx wizardPresetApplyContext) error {
	pipelineAgents := make([]persona.ProfileAgent, 0, len(agents))
	for _, a := range agents {
		pipelineAgent, ok := persona.AdaptProfileAgent(a)
		if !ok {
			return fmt.Errorf("agent %q does not support schema v2 presentation profiles", a.Name())
		}
		pipelineAgents = append(pipelineAgents, pipelineAgent)
	}

	return persona.ApplyProfile(pipelineAgents, resolved, persona.ApplyOptions{
		Layer1:               presetCtx.Layer1,
		Skills:               presetCtx.Skills,
		PreviousPresetSlug:   presetCtx.PreviousPresetSlug,
		PreviousPresetSource: presetCtx.PreviousPresetSource,
		PersistConfig:        false,
	})
}

func verifyConfiguredAgentRuntime(a agent.Agent, cfg *config.AppConfig) error {
	observed, err := agent.ObserveRuntimeWithConfig(a, cfg)
	if err != nil {
		return fmt.Errorf("runtime verification observe failed: %w", err)
	}
	report := sddruntime.Verify(a.Name(), observed)
	if report.Status != sddruntime.StatusFail {
		return nil
	}

	failures := make([]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		if check.Status != sddruntime.StatusFail {
			continue
		}
		failures = append(failures, fmt.Sprintf("%s (%s)", check.Key, check.Message))
	}

	return fmt.Errorf("runtime verification failed [%s] contract=%s checks=%s", report.Agent, report.ContractVersion, strings.Join(failures, "; "))
}

func refreshProjectRegistryForApply(ctx context.Context, cwd string) ([]string, error) {
	if strings.TrimSpace(cwd) == "" {
		return nil, nil
	}
	result, err := refreshProjectSkillRegistry(ctx, projectregistry.RefreshOptions{CWD: cwd})
	if err != nil {
		if projectregistry.IsNonProjectError(err) {
			return []string{"Project skill registry warning: " + projectregistry.ErrNotGitWorktree.Error()}, nil
		}
		return nil, err
	}
	return projectRegistryWarningLines(result.Warnings), nil
}

func projectRegistryWarningLines(warnings []projectregistry.Warning) []string {
	return projectregistry.FormatWarningLines("Project skill registry warning: ", warnings)
}
