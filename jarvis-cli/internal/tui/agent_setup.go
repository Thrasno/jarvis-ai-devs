package tui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// AgentApplyResult captures per-agent setup outcome before final config commit.
type AgentApplyResult struct {
	AgentName string
	State     state.AgentRecord
	Warnings  []string
	Err       error

	// ResetApplied and ResetSnapshotID report the consented configuration
	// reset (issue #767 T7) that ran for this agent before its normal
	// install/merge steps, if the wizard's reset step was accepted.
	ResetApplied    bool
	ResetSnapshotID string

	// ResetNotInDurableSnapshot mirrors agent.ResetResult.NotInDurableSnapshot
	// for the reset this agent ran: paths recoverable only through
	// resetRollback below while this wizard run is still active (or its own
	// mid-apply rollback while ApplyReset itself was running), never from
	// the durable snapshot named by ResetSnapshotID afterward (issue #767
	// hardening R1-001/R4-002/R3-001/R2-004).
	ResetNotInDurableSnapshot []string

	// ResetRolledBack and ResetRollbackErr report what happened to an already
	// -applied reset when a later stage of this same agent's setup failed:
	// either this agent's own subsequent install (issue #767 hardening
	// R4-002), or the shared persona profile apply step that runs only after
	// every agent's install already succeeded. ResetRolledBack is true only
	// when the reset's rollback fully restored the pre-reset configuration.
	// ResetRollbackErr carries the rollback failure otherwise (which may be a
	// *agent.ResetRestoreError naming unrecovered paths); it is folded into
	// Err's text too, so every surface that already reports Err shows it.
	//
	// A runtime-verification failure after a complete reinstall (the loop
	// below, after every install and the profile apply already succeeded)
	// deliberately does NOT roll back: the reinstall is complete, so
	// ResetRolledBack stays false and ResetRollbackErr stays nil for that
	// case, and configResetSummaryLines still reports the reset ran.
	ResetRolledBack  bool
	ResetRollbackErr error

	// resetRollback undoes this exact agent's already-applied reset. It
	// stays set after a successful install so a LATER shared-step failure
	// (persona profile apply) can still roll this agent's reset back; it is
	// cleared to nil once invoked.
	resetRollback func() error
}

// managedDirectoryRollbackWarning names the directories a's reset may have
// emptied and its subsequent install may have then written fresh content
// into, for a rollback report to warn that rolling back the reset's own
// journaled mutations does not remove files the install wrote afterward
// (issue #767 hardening R3-003). It returns "" when the platform cannot be
// resolved or the platform's reset owns no directories.
func managedDirectoryRollbackWarning(a agent.Agent) string {
	platform, err := agent.PlatformForAgentName(a.Name())
	if err != nil {
		return ""
	}
	dirs := agent.ResetManagedDirectories(platform)
	if len(dirs) == 0 {
		return ""
	}
	full := make([]string, 0, len(dirs))
	for _, d := range dirs {
		full = append(full, filepath.Join(a.ConfigDir(), d))
	}
	return "files written by the install attempt may remain under: " + strings.Join(full, ", ")
}

// rollbackAgentReset invokes res.resetRollback for one agent whose reset was
// applied and not yet rolled back, recording the outcome (rolled back, or a
// rollback failure) on res and returning a description of what happened
// (including the managed-directory residual-file warning) for the caller to
// fold into whatever error message reports the triggering failure.
func rollbackAgentReset(a agent.Agent, res *AgentApplyResult) string {
	if res.resetRollback == nil {
		return ""
	}
	rollback := res.resetRollback
	res.resetRollback = nil
	if rbErr := rollback(); rbErr != nil {
		res.ResetRollbackErr = rbErr
		return fmt.Sprintf("configuration reset rollback also failed, snapshot %s: %v (%s)", res.ResetSnapshotID, rbErr, managedDirectoryRollbackWarning(a))
	}
	res.ResetRolledBack = true
	return fmt.Sprintf("configuration reset (snapshot %s) rolled back (%s)", res.ResetSnapshotID, managedDirectoryRollbackWarning(a))
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
	phaseModels state.PhaseModels,
	hiveEntry agent.MCPEntry,
	context7Entry agent.MCPEntry,
	skillsSubFS fs.FS,
	selectedIDs []string,
	agentsSubFS fs.FS,
	statuslineConfirm func() bool,
) ([]string, error) {
	return agentapply.ConfigureAgent(a, phaseModels, hiveEntry, context7Entry, skillsSubFS, selectedIDs, agentsSubFS, agentapply.StatuslineDecision{
		Install: true,
		Confirm: statuslineConfirm,
	})
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

// WizardAgentApplyOptions bundles the setup context configureWizardAgents
// applies uniformly to every detected agent. Grouping these fields (rather
// than passing them as positional parameters) keeps call sites readable as
// the reset step (issue #767 T7) and its hardening added more shared context.
type WizardAgentApplyOptions struct {
	PhaseModels   state.PhaseModels
	HiveEntry     agent.MCPEntry
	Context7Entry agent.MCPEntry
	Resolved      *persona.ResolvedProfile
	PresetCtx     wizardPresetApplyContext
	// SkillsSubFS is the sub-FS rooted at embed/skills for InstallSkills.
	SkillsSubFS fs.FS
	SelectedIDs []string
	// AgentsSubFS is the sub-FS rooted at embed/agents/<platform> for
	// file-based agent install (ClaudeAgent). nil for platforms that use the
	// JSON config builder path instead (OpenCodeAgent).
	AgentsSubFS       fs.FS
	StatuslineConfirm func() bool

	// ResetConsented is the human's explicit answer to the wizard's
	// configuration reset step (issue #767 T7): only when true does this run
	// agent.ApplyReset for each agent, BEFORE that agent's normal
	// install/merge steps, so the subsequent install regenerates managed
	// configuration from scratch exactly as a fresh install would.
	ResetConsented bool
	// Home is the home directory the reset's durable snapshot is stored
	// under.
	Home string
}

// configureWizardAgents applies setup to all detected agents and returns
// per-agent structured outcomes. If one agent fails, callers can abort before
// committing canonical config and still report the failing agent explicitly.
func configureWizardAgents(agents []agent.Agent, opts WizardAgentApplyOptions) []AgentApplyResult {
	results := make([]AgentApplyResult, 0, len(agents))
	for _, a := range agents {
		configPath, err := wizardAgentConfigPath(a)
		res := AgentApplyResult{
			AgentName: a.Name(),
			State: state.AgentRecord{
				Configured: false,
				// Recorded from the agent itself, because the manifest's
				// instructions_path is what replay projects onto the managed root
				// and what instruction ownership is keyed by. A machine that never
				// migrated has no other source for it.
				InstructionsPath: a.InstructionsPath(),
				ConfigPath:       configPath,
			},
		}
		if err != nil {
			res.Err = fmt.Errorf("resolve canonical config path: %w", err)
			results = append(results, res)
			return results
		}

		// res.resetRollback, when non-nil, undoes an already-applied reset for
		// THIS agent. It is invoked below immediately if this same agent's
		// subsequent install fails (issue #767 hardening R4-002), so a
		// consented reset never leaves an agent's configuration deleted
		// without either a working reinstall or a restored prior state. It
		// stays set on success, though, because a LATER shared failure
		// (persona profile apply, below) can still need to undo this
		// agent's reset even though this agent's own install succeeded; an
		// agent whose install failed is rolled back here and only here.
		if opts.ResetConsented {
			platform, platformErr := agent.PlatformForAgentName(a.Name())
			if platformErr != nil {
				res.Err = fmt.Errorf("resolve reset platform: %w", platformErr)
				results = append(results, res)
				return results
			}
			resetResult, resetErr := agent.ApplyReset(platform, a.ConfigDir(), opts.Home)
			if resetErr != nil {
				res.Err = fmt.Errorf("configuration reset: %w", resetErr)
				results = append(results, res)
				return results
			}
			res.ResetApplied = true
			res.ResetSnapshotID = resetResult.SnapshotID
			res.ResetNotInDurableSnapshot = resetResult.NotInDurableSnapshot
			res.resetRollback = resetResult.Rollback
		}

		warnings, err := configureWizardAgent(a, opts.PhaseModels, opts.HiveEntry, opts.Context7Entry, opts.SkillsSubFS, opts.SelectedIDs, opts.AgentsSubFS, opts.StatuslineConfirm)
		res.Warnings = append(res.Warnings, warnings...)
		if err != nil {
			if outcome := rollbackAgentReset(a, &res); outcome != "" {
				err = fmt.Errorf("%w (%s)", err, outcome)
			}
			res.Err = err
			results = append(results, res)
			return results
		}
		res.State.Configured = true
		results = append(results, res)
	}

	// A shared persona profile-apply failure runs only after every agent's
	// own install already succeeded, so any agent whose reset is still
	// pending rollback (res.resetRollback non-nil: this agent's own install
	// did not fail, or this is the first shared-step failure it hit) would
	// otherwise be left with its configuration reset and reinstalled but the
	// wizard reporting overall failure -- neither the pre-reset state nor a
	// complete, working setup. Roll every such agent's reset back here
	// (issue #767 hardening R4-001/R2-002/R2-003): a runtime-verification
	// failure below, by contrast, runs after this same profile apply already
	// succeeded, so the reinstall is genuinely complete and is deliberately
	// left in place instead.
	if opts.Resolved != nil {
		if err := applyWizardProfile(agents, opts.Resolved, wizardPresetApplyContext{
			Layer1:               opts.PresetCtx.Layer1,
			Skills:               opts.PresetCtx.Skills,
			PreviousPresetSlug:   opts.PresetCtx.PreviousPresetSlug,
			PreviousPresetSource: opts.PresetCtx.PreviousPresetSource,
		}); err != nil {
			profileErr := fmt.Errorf("apply preset pipeline: %w", err)
			var outcomes []string
			for i, a := range agents {
				if i >= len(results) {
					break
				}
				if outcome := rollbackAgentReset(a, &results[i]); outcome != "" {
					outcomes = append(outcomes, outcome)
				}
			}
			if len(outcomes) > 0 {
				profileErr = fmt.Errorf("%w (%s)", profileErr, strings.Join(outcomes, "; "))
			}
			if len(results) == 0 {
				return []AgentApplyResult{{AgentName: "persona-apply", Err: profileErr}}
			}
			results[len(results)-1].Err = profileErr
			return results
		}
	}

	for i, a := range agents {
		if err := verifyConfiguredAgentRuntime(a, &opts.PhaseModels); err != nil {
			// A runtime-verification failure after a complete reinstall does
			// NOT roll the reset back: the reinstall itself finished, so
			// undoing the reset here would leave a worse state (no
			// configuration at all) than the one that actually failed
			// (configuration installed, but not yet verified). Every
			// results[i].ResetApplied still reports its snapshot ID, and
			// configResetSummaryLines keeps the reset line for it.
			results[i].State.Configured = false
			results[i].Err = err
			return results
		}
	}

	return results
}

func wizardAgentConfigPath(a agent.Agent) (string, error) {
	plan, err := a.RuntimePlan()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(plan.Paths.Settings) == "" {
		return "", fmt.Errorf("agent %q runtime plan has no settings path", a.Name())
	}
	return filepath.Join(a.ConfigDir(), filepath.Base(filepath.FromSlash(plan.Paths.Settings))), nil
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

func verifyConfiguredAgentRuntime(a agent.Agent, models *state.PhaseModels) error {
	observed, err := agent.ObserveRuntimeWithConfig(a, models)
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
