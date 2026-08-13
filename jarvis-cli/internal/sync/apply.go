// This file holds the mutation half of replay: it walks the fixed component
// order once per configured agent and reports, per agent, whether that agent
// converged. It owns sequencing and reporting only; each component's behaviour
// lives behind ComponentRunner.
package sync

import "fmt"

// Component IDs are stable strings. They appear in reports and in the order
// contract test, so renaming one is a visible, deliberate change.
const (
	ComponentModels              = "models"
	ComponentSkills              = "skills"
	ComponentRuntimeAssets       = "orchestrator-agents-hooks"
	ComponentMCPs                = "mcps"
	ComponentPersonaInstructions = "persona-instructions"
	ComponentStatusline          = "statusline"
)

// AgentTarget is one agent's slice of a replay run. Every seam below takes a
// single target, which is what keeps a ReconcileInstallRequest scoped to one
// agent: there is no shape in this package through which a caller could build a
// cross-agent request, so reconcile's transactional compensation can never roll
// back a sibling agent's marker-backed artifacts.
type AgentTarget struct {
	// ID is the manifest agent ID, such as "claude" or "opencode".
	ID string
	// Root is the machine root the agent's artifacts live under.
	Root string
}

// ComponentRunner performs one component for one agent. The applier owns the
// order; the runner owns the work. Later slices fill in the production
// implementation without touching the sequencing contract.
type ComponentRunner interface {
	ApplyModels(AgentTarget) error
	ApplySkills(AgentTarget) error
	ApplyRuntimeAssets(AgentTarget) error
	ApplyMCPs(AgentTarget) error
	ApplyPersonaInstructions(AgentTarget) error
	ApplyStatusline(AgentTarget) error
}

type component struct {
	id    string
	apply func(ComponentRunner, AgentTarget) error
}

// components is the locked application order.
//
// Persona and the instruction write run LAST, after every component that
// injects content into shared files. This inverts the gentle-ai reference order
// on purpose; do not "fix" it back. The reasons are structural:
//
//   - WriteInstructions is the sole writer of CLAUDE.md / AGENTS.md
//     (agent/claude.go:327, agent/opencode.go:424); skills land in separate
//     files (agent/install.go:83), so nothing injects prose after persona.
//   - It rebuilds or patches the whole file and re-injects the Hive protocol and
//     the orchestrator import in the same pass (claude.go:365-372).
//   - Its no-sentinel branch REPLACES the file (claude.go:350-356), which makes
//     it a destructive last writer: anything writing to the instruction file
//     after it would be discarded on the next run.
//   - It renders the Skills section from the installed skill set, whose bodies
//     render from model assignments (claude.go:532, install.go:76-82). That data
//     dependency forces models -> skills -> instructions.
//   - Production already does this: the wizard applies agents and then the
//     persona profile (internal/tui/agent_setup.go:245-277).
var components = []component{
	{id: ComponentModels, apply: ComponentRunner.ApplyModels},
	{id: ComponentSkills, apply: ComponentRunner.ApplySkills},
	{id: ComponentRuntimeAssets, apply: ComponentRunner.ApplyRuntimeAssets},
	{id: ComponentMCPs, apply: ComponentRunner.ApplyMCPs},
	{id: ComponentPersonaInstructions, apply: ComponentRunner.ApplyPersonaInstructions},
	{id: ComponentStatusline, apply: ComponentRunner.ApplyStatusline},
}

// AgentResult is one agent's outcome.
type AgentResult struct {
	Agent     string
	Converged bool
	// FailedAt is the component ID that stopped this agent, empty on success.
	FailedAt string
	Err      error
	// Changed lists the paths this agent actually modified. It stays empty in
	// this slice: measuring real change needs the snapshot/diff pass, and the
	// plan describes desired state rather than drift, so counting planned
	// artifacts here would report a change on every run. PR 5 populates it.
	Changed []string
}

// ApplyInput is everything a replay pass needs.
type ApplyInput struct {
	Runner  ComponentRunner
	Targets []AgentTarget
}

// Report is the honest outcome of a replay pass.
type Report struct {
	Agents []AgentResult
}

// Converged reports global convergence, which requires at least one agent and
// every agent succeeding. A run with no targets never claims convergence.
func (r Report) Converged() bool {
	if len(r.Agents) == 0 {
		return false
	}
	for _, result := range r.Agents {
		if !result.Converged {
			return false
		}
	}
	return true
}

// ExitCode is non-zero unless every agent converged.
func (r Report) ExitCode() int {
	if r.Converged() {
		return 0
	}
	return 1
}

// Apply replays the component order for each configured agent.
//
// A failure stops that agent's remaining components, records the cause, and the
// loop continues with the next agent. There is no global rollback: the agents
// that already converged keep their changes, and the report says plainly which
// agent failed and why. That is the opposite of the wizard, which early-returns
// on the first failure (internal/tui/agent_setup.go:255-259) because a user is
// present to retry. Nothing here calls back into an agent already visited, so
// no failure can reach across into a sibling's artifacts.
func Apply(in ApplyInput) Report {
	report := Report{Agents: make([]AgentResult, 0, len(in.Targets))}
	for _, target := range in.Targets {
		report.Agents = append(report.Agents, applyAgent(in.Runner, target))
	}
	return report
}

// applyAgent walks the locked order for a single agent and stops that agent at
// its first failure.
func applyAgent(runner ComponentRunner, target AgentTarget) AgentResult {
	result := AgentResult{Agent: target.ID}
	for _, c := range components {
		if err := c.apply(runner, target); err != nil {
			result.FailedAt = c.id
			result.Err = fmt.Errorf("apply %s for agent %s: %w", c.id, target.ID, err)
			return result
		}
	}
	result.Converged = true
	return result
}
