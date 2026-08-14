// Package agentapply holds the agent artifact pipeline shared by the
// installation wizard and desired-state replay. Both callers run the same
// pipeline; they differ only in how the statusline decision is made.
package agentapply

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// ClaudeRestartGuidance is the warning emitted after Claude SDD phase agents
// are refreshed.
const ClaudeRestartGuidance = "Restart Claude Code to discover refreshed Jarvis-managed SDD agents."

type generatedConfigAgent interface {
	MergeGeneratedConfig(state.PhaseModels) error
}

type configAwareSkillInstaller interface {
	InstallSkillsWithConfig(fs.FS, []string, state.PhaseModels) error
}

type sddPhaseAgentInstaller interface {
	InstallSDDPhaseAgents(state.PhaseModels) error
}

// StatuslineInstaller is implemented by agents that support the Jarvis-managed
// Claude Code statusline. The confirm callback decides overwrite vs. skip when
// the script already exists; it is never called on a fresh install.
type StatuslineInstaller interface {
	InstallStatusline(hooksFS fs.FS, confirm func() bool) error
}

// StatuslineDecision tells the pipeline whether to touch the statusline and,
// when it does, how to answer an already-present script. Confirm is never nil:
// InstallStatusline invokes it unguarded whenever the script exists, which is
// every upgrade.
type StatuslineDecision struct {
	Install bool
	Confirm func() bool
}

// StatuslineDecisionFromState derives the decision from the persisted
// tri-state. "Never asked" and "decided against" both mean do not touch, so the
// install is skipped entirely rather than answered with a false confirm. Once
// the decision is recorded and enabled, the manifest is the authority: a script
// missing from disk is drift and is reinstalled, not a revoked consent.
func StatuslineDecisionFromState(st state.StatuslineState) StatuslineDecision {
	if !st.ShouldManage() {
		return StatuslineDecision{Install: false, Confirm: func() bool { return false }}
	}
	return StatuslineDecision{Install: true, Confirm: func() bool { return true }}
}

// ApplyStatusline runs the decision against one agent. Agents without
// statusline support and unauthorized decisions are both no-ops.
func ApplyStatusline(a any, hooksFS fs.FS, decision StatuslineDecision) error {
	if !decision.Install {
		return nil
	}
	slAgent, ok := a.(StatuslineInstaller)
	if !ok {
		return nil
	}
	if err := slAgent.InstallStatusline(hooksFS, decision.Confirm); err != nil {
		return fmt.Errorf("install statusline: %w", err)
	}
	return nil
}

// MCPExecutor is the production boundary for the managed-MCP handoff.
// Production uses the concrete executor; tests can drive the same routes
// without a real home or native CLI.
type MCPExecutor interface {
	ExecuteWizard(agent.WizardReconcileInput) (agent.ReconcileInstallResult, error)
}

// MCPDeps carries the caller-owned seams ReconcileMCPs needs. Callers own them
// so the pipeline stays free of package-level state.
type MCPDeps struct {
	NewExecutor    func() MCPExecutor
	HiveDaemonPath func(home string) string
}

// ConfigureAgent applies the MCP + instruction + skills setup flow for one
// agent.
// agentsSubFS is the sub-FS rooted at embed/agents/<platform> for file-based
// agent install (ClaudeAgent). Pass nil for platforms that use the JSON config
// builder path instead (OpenCodeAgent).
func ConfigureAgent(
	a agent.Agent,
	phaseModels state.PhaseModels,
	hiveEntry agent.MCPEntry,
	context7Entry agent.MCPEntry,
	skillsSubFS fs.FS,
	selectedIDs []string,
	agentsSubFS fs.FS,
	statusline StatuslineDecision,
) ([]string, error) {
	// MCP reconciliation is intentionally performed once by ExecuteWizard before
	// this per-agent artifact pipeline. Keep these legacy parameters while callers
	// converge so no agent can directly merge a managed MCP configuration here.
	_ = hiveEntry
	_ = context7Entry
	if generatedAgent, ok := a.(generatedConfigAgent); ok {
		if err := generatedAgent.MergeGeneratedConfig(phaseModels); err != nil {
			return nil, fmt.Errorf("generated config guardrails: %w", err)
		}
	}
	if skillInstaller, ok := a.(configAwareSkillInstaller); ok {
		if err := skillInstaller.InstallSkillsWithConfig(skillsSubFS, selectedIDs, phaseModels); err != nil {
			return nil, fmt.Errorf("install skills: %w", err)
		}
	} else if err := a.InstallSkills(skillsSubFS, selectedIDs); err != nil {
		return nil, fmt.Errorf("install skills: %w", err)
	}
	orchestratorTemplate, err := fs.ReadFile(jarvis.OrchestratorFS, "embed/orchestrator/sdd-orchestrator.md")
	if err != nil {
		return nil, fmt.Errorf("read orchestrator template: %w", err)
	}
	renderedOrchestrator, err := sddruntime.RenderOrchestrator(a.Name(), phaseModels, string(orchestratorTemplate))
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
		if err := sddInstaller.InstallSDDPhaseAgents(phaseModels); err != nil {
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
		warnings = append(warnings, ClaudeRestartGuidance)
	}
	if _, err := agent.InstallRegistryAutomationIfSupported(a, jarvis.HooksFS); err != nil {
		warnings = append(warnings, fmt.Sprintf("Project skill registry warning: automation not installed for %s: %v", a.Name(), err))
	}
	if err := ApplyStatusline(a, jarvis.HooksFS, statusline); err != nil {
		return warnings, err
	}
	return warnings, nil
}

// ReconcileMCPs is the sole setup handoff for managed MCPs. It renders
// OpenCode's fixed user-global JSON target and supplies canonical Claude
// definitions to the agent-layer executor; it never calls Agent.MergeConfig.
func ReconcileMCPs(agents []agent.Agent, home string, deps MCPDeps) error {
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
		hive, context7, err := agent.ClaudeUserMCPDefinitions(deps.HiveDaemonPath(home))
		if err != nil {
			return err
		}
		input.ClaudeHive, input.ClaudeContext7 = hive, context7
	}
	if hasSelectedAgent(selected, "opencode") {
		managed, err := renderOpenCodeMCPs(deps.HiveDaemonPath(home))
		if err != nil {
			return err
		}
		input.OpenCodeMCPs = managed
	}
	_, err := deps.NewExecutor().ExecuteWizard(input)
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

func renderOpenCodeMCPs(daemonPath string) (agent.OpenCodeManagedMCPs, error) {
	hive, err := json.Marshal(map[string]any{
		"type":    "local",
		"command": []string{daemonPath},
	})
	if err != nil {
		return nil, fmt.Errorf("render OpenCode Hive MCP desired state: %w", err)
	}
	return agent.OpenCodeManagedMCPs{
		"hive":     string(hive),
		"context7": `{"type":"remote","url":"https://mcp.context7.com/mcp","enabled":true}`,
	}, nil
}
