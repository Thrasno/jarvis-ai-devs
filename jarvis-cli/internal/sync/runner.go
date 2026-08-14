// This file holds the production ComponentRunner: the one implementation that
// drives real installer code behind the locked component order. Everything it
// needs arrives in a single ReplayInput, which is also what the planner reads,
// so the state a run measures and the content it writes are rendered from one
// value.
package sync

import (
	"fmt"
	"io/fs"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// AgentsSubFS resolves the embedded agent-definition sub-FS for one agent ID.
// A platform that has no such tree returns a nil FS and no error, which is what
// the JSON-config agents want.
type AgentsSubFS func(agentID string) (fs.FS, error)

// ConfigureAgentFunc is the artifact-pipeline seam, whose parameters are
// documented on the function that satisfies it, agentapply.ConfigureAgent.
// Keeping it a seam lets replay be driven without installing anything.
type ConfigureAgentFunc func(agent.Agent, state.PhaseModels, agent.MCPEntry, agent.MCPEntry, fs.FS, []string, fs.FS, agentapply.StatuslineDecision) ([]string, error)

// ReplayInput is the single source both halves of a replay pass read.
//
// It exists to close one hazard: the planner digests the skill files it renders
// from Config while the installer writes the files it renders from the config
// the runner hands it, so two different values would make the desired digest and
// the actual write disagree and every run report drift forever. One struct feeds
// PlanInputFor and NewRunner, so a caller has no shape here to supply two.
//
// Nothing on this path may call config.Save(): its bridge takes state.WithLock
// internally and that lock is fail-fast, so a nested acquisition deadlocks sync
// against itself. Persona therefore goes through ApplyInstructions.
type ReplayInput struct {
	Root      string
	State     *state.State
	Templates fs.FS
	// SkillsFS is the embedded skill tree rooted the way the installer expects
	// it, and HooksFS is the root FS carrying the embedded statusline script.
	SkillsFS fs.FS
	HooksFS  fs.FS
	AgentsFS AgentsSubFS
	Config   *config.AppConfig
	Skills   []config.SkillInfo
	Layer1   string
	Layer2   string
	Resolve  AgentResolver
	MCPDeps  agentapply.MCPDeps
	// Configure defaults to agentapply.ConfigureAgent.
	Configure ConfigureAgentFunc
}

// PlanInputFor projects the replay input onto the planner's input.
func PlanInputFor(in ReplayInput) PlanInput {
	return PlanInput{
		Root:      in.Root,
		State:     in.State,
		Templates: in.Templates,
		Layer1:    in.Layer1,
		Layer2:    in.Layer2,
		Skills:    in.Skills,
		SkillsFS:  in.SkillsFS,
		HooksFS:   in.HooksFS,
		Config:    in.Config,
	}
}

// TargetsFor projects the manifest onto one target per configured agent, in the
// recorded order. Nothing is discovered from disk: an agent the manifest does
// not record is not part of this run.
func TargetsFor(in ReplayInput) []AgentTarget {
	if in.State == nil {
		return nil
	}
	targets := make([]AgentTarget, 0, len(in.State.InstalledAgents))
	for _, configured := range in.State.InstalledAgents {
		targets = append(targets, AgentTarget{
			ID:               configured.ID,
			Root:             in.Root,
			InstructionsPath: configured.InstructionsPath,
		})
	}
	return targets
}

// Runner is the production ComponentRunner.
//
// One caveat is structural and is documented rather than worked around:
// agentapply.ConfigureAgent performs the generated-config merge, the skill
// install and the runtime-asset install as a single indivisible pass, in exactly
// the order this package locks. Splitting it would mean reimplementing the
// installer, so the whole pass runs under the first component ID and the next
// two are no-ops. Attribution across models, skills and
// orchestrator-agents-hooks is therefore coarse: a skill-install failure is
// reported at models. The agent is still named exactly, and the last three
// components keep their own precise attribution.
type Runner struct {
	resolve    AgentResolver
	configure  ConfigureAgentFunc
	config     *config.AppConfig
	skills     []config.SkillInfo
	skillIDs   []string
	skillsFS   fs.FS
	agentsFS   AgentsSubFS
	layer1     string
	layer2     string
	ownership  InstructionOwnership
	mcps       MCPComponent
	statusline StatuslineComponent
}

// NewRunner builds the production runner from the same input the planner reads.
func NewRunner(in ReplayInput) *Runner {
	configure := in.Configure
	if configure == nil {
		configure = agentapply.ConfigureAgent
	}
	runner := &Runner{
		resolve:   in.Resolve,
		configure: configure,
		config:    in.Config,
		skills:    in.Skills,
		skillsFS:  in.SkillsFS,
		agentsFS:  in.AgentsFS,
		layer1:    in.Layer1,
		layer2:    in.Layer2,
		mcps:      MCPComponent{Resolve: in.Resolve, Deps: in.MCPDeps},
	}
	if in.State != nil {
		runner.skillIDs = in.State.Skills
		runner.ownership = NewInstructionOwnership(in.State.InstalledAgents)
		runner.statusline = StatuslineComponent{Resolve: in.Resolve, HooksFS: in.HooksFS, Consent: in.State.Statusline}
	}
	return runner
}

// ApplyModels runs the whole indivisible installer pass. See the Runner comment
// for why three component IDs share it.
func (r *Runner) ApplyModels(target AgentTarget) error {
	resolved, err := r.agentFor(target, "replay generated config, skills and runtime assets")
	if err != nil {
		return err
	}
	agentsSubFS, err := r.agentsSubFS(target.ID)
	if err != nil {
		return err
	}
	// The statusline is the last component and runs after the instruction write,
	// so this pass is told explicitly not to touch it.
	if _, err := r.configure(
		resolved, r.config.PhaseModelsForState(), agent.MCPEntry{}, agent.MCPEntry{},
		r.skillsFS, r.skillIDs, agentsSubFS,
		agentapply.StatuslineDecision{Install: false, Confirm: func() bool { return false }},
	); err != nil {
		return fmt.Errorf("replay generated config, skills and runtime assets for %q: %w", target.ID, err)
	}
	return nil
}

// ApplySkills is a no-op: ApplyModels already installed the skills.
func (r *Runner) ApplySkills(AgentTarget) error { return nil }

// ApplyRuntimeAssets is a no-op: ApplyModels already installed the orchestrator,
// agent definitions and hooks.
func (r *Runner) ApplyRuntimeAssets(AgentTarget) error { return nil }

// ApplyMCPs replaces this agent's Jarvis-managed MCP definitions.
func (r *Runner) ApplyMCPs(target AgentTarget) error { return r.mcps.Apply(target) }

// ApplyPersonaInstructions writes this agent's managed instruction file, and
// only after the manifest ownership guard authorizes the path.
func (r *Runner) ApplyPersonaInstructions(target AgentTarget) error {
	resolved, err := r.agentFor(target, "replay instructions")
	if err != nil {
		return err
	}
	return ApplyInstructions(r.ownership, target, resolved, r.layer1, r.layer2, r.skills)
}

// ApplyStatusline converges this agent's statusline from the manifest's
// recorded consent.
func (r *Runner) ApplyStatusline(target AgentTarget) error { return r.statusline.Apply(target) }

func (r *Runner) agentFor(target AgentTarget, what string) (agent.Agent, error) {
	if r.resolve != nil {
		if resolved, ok := r.resolve(target.ID); ok {
			return resolved, nil
		}
	}
	return nil, fmt.Errorf("%s for %q: %w", what, target.ID, ErrUnknownAgent)
}

func (r *Runner) agentsSubFS(agentID string) (fs.FS, error) {
	if r.agentsFS == nil {
		return nil, nil
	}
	sub, err := r.agentsFS(agentID)
	if err != nil {
		return nil, fmt.Errorf("resolve embedded agent definitions for %q: %w", agentID, err)
	}
	return sub, nil
}
