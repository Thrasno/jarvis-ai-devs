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

// ErrDependencyUnwired reports a replay component constructed without one of the
// nilable dependencies it needs. The wrapping message names which one.
//
// It is deliberately separate from ErrUnknownAgent. Both refuse the same work,
// but they describe different worlds: ErrUnknownAgent is a fact about the user's
// machine, while this is a fact about how the component was built, and the agent
// it names may be installed perfectly well. Reporting one as the other would
// send whoever reads the failure to inspect the wrong thing.
//
// One sentinel covers the class rather than one per field: the distinction worth
// making in an error value is construction defect versus installation fact, and
// the message carries the rest.
var ErrDependencyUnwired = errors.New("replay component was built without a required dependency")

// ErrHiveDaemonUnavailable reports that the Hive daemon binary this machine
// records is missing, is not a regular file, or cannot be executed, so the
// managed-MCP desired state cannot be rendered at all. An interrupted install,
// a half-applied upgrade or an antivirus quarantine all land here.
//
// It is the only failure class the handoff below actually separates. Everything
// under agentapply.ReconcileMCPs -- the daemon check, the recovery-evidence
// directory, the native reconciliation itself -- returns bare errors.New values
// with no sentinel, no type and no wrapping, so a caller has nothing to key on
// and a taxonomy invented here would be a claim the code cannot back. This one
// class is different because its condition is decidable independently of the
// error value, by asking the same authority the reconciler consults.
var ErrHiveDaemonUnavailable = errors.New("the recorded Hive daemon binary is missing or not executable")

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
		return fmt.Errorf("replay managed MCPs for %q: agent resolver: %w", target.ID, ErrDependencyUnwired)
	}
	resolved, ok := c.Resolve(target.ID)
	if !ok {
		return fmt.Errorf("replay managed MCPs for %q: %w", target.ID, ErrUnknownAgent)
	}
	reconcile := c.Reconcile
	if reconcile == nil {
		// Deps belongs to the default reconciler, not to the component: an
		// injected one is free to need nothing from it. Guard it only where it is
		// actually about to be dereferenced, or the override seam breaks.
		if c.Deps.HiveDaemonPath == nil || c.Deps.NewExecutor == nil {
			return fmt.Errorf("replay managed MCPs for %q: managed-MCP dependencies: %w", target.ID, ErrDependencyUnwired)
		}
		reconcile = agentapply.ReconcileMCPs
	}
	if err := reconcile([]agent.Agent{resolved}, target.Root, c.Deps); err != nil {
		if path, unusable := c.unusableHiveDaemon(target.Root); unusable {
			// Both are wrapped: the sentinel makes the class checkable, and the
			// reconciler's own error keeps whatever specifics it carried, because a
			// classification that swallowed them would trade one blind spot for
			// another.
			return fmt.Errorf("replay managed MCPs for %q: %w (%s); run `jarvis` to reinstall it: %w", target.ID, ErrHiveDaemonUnavailable, path, err)
		}
		return fmt.Errorf("replay managed MCPs for %q: %w", target.ID, err)
	}
	return nil
}

// unusableHiveDaemon reports whether this machine's recorded Hive daemon binary
// is one the managed-MCP handoff can use, and where it looked.
//
// It asks agent.ClaudeUserMCPDefinitions rather than stat-ing the path itself,
// because that function *is* the rule: a daemon path is the only thing it
// rejects, and what counts as executable differs by platform (the permission
// bits on Unix, the .exe extension on Windows). Re-deriving those rules here
// would let the diagnostic drift away from the check it claims to describe.
//
// It runs only after a failure, so it never makes replay stricter than the
// reconciler -- notably the OpenCode path renders its desired state without
// consulting the daemon at all. What it asserts when it fires is a fact about
// this machine that is true regardless of which condition the reconciler
// tripped on, and it is the first thing worth repairing either way.
func (c MCPComponent) unusableHiveDaemon(root string) (string, bool) {
	if c.Deps.HiveDaemonPath == nil {
		return "", false
	}
	path := c.Deps.HiveDaemonPath(root)
	if _, _, err := agent.ClaudeUserMCPDefinitions(path); err != nil {
		return path, true
	}
	return "", false
}
