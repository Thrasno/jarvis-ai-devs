// This file holds the managed-MCP component of replay.
//
// Jarvis-managed MCP definitions are not a persisted user choice, so replay has
// nothing to consult and replaces them unconditionally. The wizard receives
// hiveEntry and context7Entry and discards both (internal/tui/agent_setup.go),
// and both of its call sites pass an empty agent.MCPEntry; the canonical
// definitions are derived from embedded code -- the hive-daemon path and the
// Context7 URL -- under fixed identities
// (internal/agent/native_mcp_recovery.go:44-72). The wizard's "I ACKNOWLEDGE"
// gate is a destructive-action confirmation for an interactive operator, not a
// recorded decision, and sync is non-interactive. Replacement only ever reaches
// those two Jarvis identities, so no user-defined MCP is in its scope.
package sync

import (
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
)

// ErrUnknownAgent reports a manifest agent ID this machine does not have
// installed. Replay refuses rather than guessing at a substitute.
var ErrUnknownAgent = errors.New("agent is not installed on this machine")

// ErrResolverUnwired reports a component constructed without an AgentResolver.
//
// It is deliberately separate from ErrUnknownAgent. Both refuse the same work,
// but they describe different worlds: ErrUnknownAgent is a fact about the user's
// machine, while this is a fact about how the component was built, and the agent
// it names may be installed perfectly well. Reporting one as the other would
// send whoever reads the failure to inspect the wrong thing.
var ErrResolverUnwired = errors.New("replay component was built without an agent resolver")

// AgentResolver maps a manifest agent ID onto the installed agent it names.
type AgentResolver func(id string) (agent.Agent, bool)

// MCPReconciler is the managed-MCP handoff, satisfied by
// agentapply.ReconcileMCPs. Keeping it a seam lets replay be driven without a
// real home or a native Claude CLI.
type MCPReconciler func(agents []agent.Agent, home string, deps agentapply.MCPDeps) error

// MCPComponent replays the Jarvis-managed MCPs for one agent. It deliberately
// holds no manifest, config or consent field: there is no stored decision that
// could authorize skipping the replacement.
type MCPComponent struct {
	Resolve AgentResolver
	// Reconcile defaults to agentapply.ReconcileMCPs.
	Reconcile MCPReconciler
	Deps      agentapply.MCPDeps
}

// Apply replaces this agent's managed MCPs. The agent is handed over as a
// single-element slice on purpose: one ReconcileInstallRequest per agent is what
// keeps reconcile's transactional compensation from rolling back a sibling
// agent's marker-backed artifacts.
func (c MCPComponent) Apply(target AgentTarget) error {
	// AgentResolver is a nilable func type, so an unwired one is a missing
	// dependency rather than a resolution failure: replay must not panic mid-pass.
	if c.Resolve == nil {
		return fmt.Errorf("replay managed MCPs for %q: %w", target.ID, ErrResolverUnwired)
	}
	resolved, ok := c.Resolve(target.ID)
	if !ok {
		return fmt.Errorf("replay managed MCPs for %q: %w", target.ID, ErrUnknownAgent)
	}
	reconcile := c.Reconcile
	if reconcile == nil {
		reconcile = agentapply.ReconcileMCPs
	}
	if err := reconcile([]agent.Agent{resolved}, target.Root, c.Deps); err != nil {
		return fmt.Errorf("replay managed MCPs for %q: %w", target.ID, err)
	}
	return nil
}
