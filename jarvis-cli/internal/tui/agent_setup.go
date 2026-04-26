package tui

import (
	"fmt"
	"io/fs"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// AgentApplyResult captures per-agent setup outcome before final config commit.
type AgentApplyResult struct {
	AgentName string
	State     config.AgentState
	Err       error
}

type wizardPresetApplyContext struct {
	Layer1               string
	Skills               []config.SkillInfo
	PreviousPresetSlug   string
	PreviousPresetSource persona.PresetSource
}

// configureWizardAgent applies the same MCP + instruction + skills setup flow
// for both TUI and no-TUI wizards.
func configureWizardAgent(
	a agent.Agent,
	hiveEntry agent.MCPEntry,
	context7Entry agent.MCPEntry,
	skillsSubFS fs.FS,
	selectedIDs []string,
) error {
	if err := a.MergeConfig(hiveEntry); err != nil {
		return fmt.Errorf("hive MCP config: %w", err)
	}
	if err := a.MergeConfig(context7Entry); err != nil {
		return fmt.Errorf("context7 MCP config: %w", err)
	}
	if err := a.InstallSkills(skillsSubFS, selectedIDs); err != nil {
		return fmt.Errorf("install skills: %w", err)
	}
	if err := a.InstallOrchestrator(jarvis.OrchestratorFS); err != nil {
		return fmt.Errorf("install orchestrator: %w", err)
	}
	return nil
}

// configureWizardAgents applies setup to all detected agents and returns
// per-agent structured outcomes. If one agent fails, callers can abort before
// committing canonical config and still report the failing agent explicitly.
func configureWizardAgents(
	agents []agent.Agent,
	hiveEntry agent.MCPEntry,
	context7Entry agent.MCPEntry,
	resolvedPreset *persona.ResolvedPreset,
	presetCtx wizardPresetApplyContext,
	skillsSubFS fs.FS,
	selectedIDs []string,
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
		if err := configureWizardAgent(a, hiveEntry, context7Entry, skillsSubFS, selectedIDs); err != nil {
			res.Err = err
			results = append(results, res)
			return results
		}
		res.State.Configured = true
		results = append(results, res)
	}

	pipelineAgents := make([]persona.PresetAgent, 0, len(agents))
	for _, a := range agents {
		pipelineAgents = append(pipelineAgents, a)
	}
	if resolvedPreset == nil {
		return results
	}

	if err := persona.ApplyPresetPipeline(pipelineAgents, resolvedPreset, persona.ApplyOptions{
		Layer1:               presetCtx.Layer1,
		Skills:               presetCtx.Skills,
		PreviousPresetSlug:   presetCtx.PreviousPresetSlug,
		PreviousPresetSource: presetCtx.PreviousPresetSource,
		PersistConfig:        false,
	}); err != nil {
		if len(results) == 0 {
			return []AgentApplyResult{{AgentName: "persona-apply", Err: fmt.Errorf("apply preset pipeline: %w", err)}}
		}
		results[len(results)-1].Err = fmt.Errorf("apply preset pipeline: %w", err)
		return results
	}

	return results
}
