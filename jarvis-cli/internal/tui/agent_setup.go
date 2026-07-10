package tui

import (
	"context"
	"fmt"
	"io/fs"
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

var refreshProjectSkillRegistry = projectregistry.Refresh

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
	if err := a.MergeConfig(hiveEntry); err != nil {
		return nil, fmt.Errorf("hive MCP config: %w", err)
	}
	if err := a.MergeConfig(context7Entry); err != nil {
		return nil, fmt.Errorf("context7 MCP config: %w", err)
	}
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
	selection persona.PresetSelection,
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

	if selection.V1 != nil || selection.V2 != nil {
		if err := applyWizardPresetSelection(agents, selection, wizardPresetApplyContext{
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

// applyWizardPresetSelection is the TUI adapter seam for an already resolved
// persona version. The normal wizard passes V1 explicitly; V2 remains dormant
// until a later activation path opts in.
func applyWizardPresetSelection(agents []agent.Agent, selection persona.PresetSelection, presetCtx wizardPresetApplyContext) error {
	pipelineAgents := make([]persona.PresetAgent, 0, len(agents))
	for _, a := range agents {
		pipelineAgents = append(pipelineAgents, a)
	}

	return persona.ApplyPresetSelectionPipeline(pipelineAgents, selection, persona.ApplyOptions{
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
