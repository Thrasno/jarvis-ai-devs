package tui

import (
	"fmt"
	"io/fs"
	"strings"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
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
	if resolvedPreset != nil {
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
	}

	for i, a := range agents {
		if err := verifyConfiguredAgentRuntime(a); err != nil {
			results[i].State.Configured = false
			results[i].Err = err
			return results
		}
	}

	return results
}

func verifyConfiguredAgentRuntime(a agent.Agent) error {
	observed, err := a.ObserveRuntime()
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
