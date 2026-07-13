package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
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

type generatedConfigAgent interface {
	MergeGeneratedConfig(*config.AppConfig) error
}

type configAwareSkillInstaller interface {
	InstallSkillsWithConfig(fs.FS, []string, *config.AppConfig) error
}

type sddPhaseAgentInstaller interface {
	InstallSDDPhaseAgents(*config.AppConfig) error
}

const claudeRestartGuidance = "Restart Claude Code to discover refreshed Jarvis-managed SDD agents."

const mcpReplacementAcknowledgement = "I ACKNOWLEDGE"

const mcpReplacementWarning = "WARNING: Manually configured MCPs with a Jarvis-managed name at the user level will be replaced. Prior same-name configuration cannot be guaranteed restored. A failure may leave that MCP absent or partial. The operation stops; fix the cause and rerun. Do not edit the managed user-level MCP configuration while this operation runs."

var refreshProjectSkillRegistry = projectregistry.Refresh

// wizardMCPExecutor is the production boundary for the wizard's managed-MCP
// handoff. Production uses the concrete executor; tests can drive the same
// TUI and no-TUI routes without a real home or native CLI.
type wizardMCPExecutor interface {
	ExecuteWizard(agent.WizardReconcileInput) (agent.ReconcileInstallResult, error)
}

var newWizardMCPExecutor = func() wizardMCPExecutor { return agent.NewProductionExecutor() }

var wizardHiveDaemonPath = agent.HiveDaemonBinaryPath

// statuslineInstaller is implemented by agents that support the Jarvis-managed
// Claude Code statusline. The confirm callback decides overwrite vs. skip when
// the script already exists; it is never called on a fresh install.
type statuslineInstaller interface {
	InstallStatusline(hooksFS fs.FS, confirm func() bool) error
}

// configureWizardAgent applies the same MCP + instruction + skills setup flow
// for both TUI and no-TUI wizards.
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
	// MCP reconciliation is intentionally performed once by ExecuteWizard before
	// this per-agent artifact pipeline. Keep these legacy parameters while callers
	// converge so no agent can directly merge a managed MCP configuration here.
	_ = hiveEntry
	_ = context7Entry
	if generatedAgent, ok := a.(generatedConfigAgent); ok {
		if err := generatedAgent.MergeGeneratedConfig(cfg); err != nil {
			return nil, fmt.Errorf("generated config guardrails: %w", err)
		}
	}
	if skillInstaller, ok := a.(configAwareSkillInstaller); ok {
		if err := skillInstaller.InstallSkillsWithConfig(skillsSubFS, selectedIDs, cfg); err != nil {
			return nil, fmt.Errorf("install skills: %w", err)
		}
	} else if err := a.InstallSkills(skillsSubFS, selectedIDs); err != nil {
		return nil, fmt.Errorf("install skills: %w", err)
	}
	orchestratorTemplate, err := fs.ReadFile(jarvis.OrchestratorFS, "embed/orchestrator/sdd-orchestrator.md")
	if err != nil {
		return nil, fmt.Errorf("read orchestrator template: %w", err)
	}
	renderedOrchestrator, err := sddruntime.RenderOrchestrator(a.Name(), cfg, string(orchestratorTemplate))
	if err != nil {
		return nil, fmt.Errorf("render orchestrator: %w", err)
	}
	if err := a.InstallOrchestrator([]byte(renderedOrchestrator)); err != nil {
		return nil, fmt.Errorf("install orchestrator: %w", err)
	}
	if ai, ok := a.(agent.AgentInstaller); ok {
		if err := ai.InstallAgents(agentsSubFS); err != nil {
			return nil, fmt.Errorf("install agents: %w", err)
		}
	}
	if sddInstaller, ok := a.(sddPhaseAgentInstaller); ok {
		if err := sddInstaller.InstallSDDPhaseAgents(cfg); err != nil {
			return nil, fmt.Errorf("install Claude SDD agents: %w", err)
		}
	}
	if err := a.InstallPromptHook(jarvis.HooksFS); err != nil {
		return nil, fmt.Errorf("install prompt hook: %w", err)
	}
	if err := a.InstallSessionHooks(jarvis.HooksFS); err != nil {
		return nil, fmt.Errorf("install session hooks: %w", err)
	}
	if err := agent.InstallCompactHookIfSupported(a); err != nil {
		return nil, fmt.Errorf("install compact hook: %w", err)
	}
	if err := agent.InstallSubagentStopHookIfSupported(a); err != nil {
		return nil, fmt.Errorf("install subagent stop hook: %w", err)
	}
	warnings := []string(nil)
	if _, ok := a.(sddPhaseAgentInstaller); ok && strings.EqualFold(strings.TrimSpace(a.Name()), "claude") {
		warnings = append(warnings, claudeRestartGuidance)
	}
	if _, err := agent.InstallRegistryAutomationIfSupported(a, jarvis.HooksFS); err != nil {
		warnings = append(warnings, fmt.Sprintf("Project skill registry warning: automation not installed for %s: %v", a.Name(), err))
	}
	if slAgent, ok := a.(statuslineInstaller); ok {
		if err := slAgent.InstallStatusline(jarvis.HooksFS, statuslineConfirm); err != nil {
			return warnings, fmt.Errorf("install statusline: %w", err)
		}
	}
	return warnings, nil
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

// reconcileWizardMCPs is the sole setup handoff for managed MCPs. It renders
// OpenCode's fixed user-global JSON target and supplies canonical Claude
// definitions to the agent-layer executor; it never calls Agent.MergeConfig.
func reconcileWizardMCPs(agents []agent.Agent, home string) error {
	selected := make([]string, 0, 2)
	for _, configured := range agents {
		name := strings.ToLower(strings.TrimSpace(configured.Name()))
		if name == "claude" || name == "opencode" {
			selected = append(selected, name)
		}
	}
	if len(selected) == 0 {
		return nil
	}

	input := agent.WizardReconcileInput{
		SelectedAgents: selected,
		Root:           home,
		EvidencePath:   filepath.Join(home, ".jarvis", "metadata", "reconcile", "recovery.json"),
	}
	if hasSelectedAgent(selected, "claude") {
		hive, context7, err := agent.ClaudeUserMCPDefinitions(wizardHiveDaemonPath(home))
		if err != nil {
			return err
		}
		input.ClaudeHive, input.ClaudeContext7 = hive, context7
	}
	if hasSelectedAgent(selected, "opencode") {
		managed, err := renderWizardOpenCodeMCPs(home)
		if err != nil {
			return err
		}
		input.OpenCodeMCPs = managed
	}
	_, err := newWizardMCPExecutor().ExecuteWizard(input)
	return err
}

func hasSelectedAgent(selected []string, wanted string) bool {
	for _, name := range selected {
		if name == wanted {
			return true
		}
	}
	return false
}

func renderWizardOpenCodeMCPs(home string) (agent.OpenCodeManagedMCPs, error) {
	hive, err := json.Marshal(map[string]any{
		"type":    "local",
		"command": []string{wizardHiveDaemonPath(home)},
	})
	if err != nil {
		return nil, fmt.Errorf("render OpenCode Hive MCP desired state: %w", err)
	}
	return agent.OpenCodeManagedMCPs{
		"hive":     string(hive),
		"context7": `{"type":"remote","url":"https://mcp.context7.com/mcp","enabled":true}`,
	}, nil
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
