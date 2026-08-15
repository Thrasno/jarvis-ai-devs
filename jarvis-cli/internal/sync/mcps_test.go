package sync

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
)

// capturingExecutor records the desired state handed to the managed-MCP
// executor, so a test can inspect what replay asked to be replaced without a
// native Claude CLI on the machine.
type capturingExecutor struct{ inputs []agent.WizardReconcileInput }

func (e *capturingExecutor) ExecuteWizard(in agent.WizardReconcileInput) (agent.ReconcileInstallResult, error) {
	e.inputs = append(e.inputs, in)
	return agent.ReconcileInstallResult{}, nil
}

// mcpReplayFixture points HOME at a temporary directory, makes both agents
// detectable there, and returns that home, the detected agents by ID, and an
// executable stand-in for the Hive daemon.
func mcpReplayFixture(t *testing.T) (string, map[string]agent.Agent, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads USERPROFILE on Windows, so HOME alone leaves the
	// real home in play and neither agent is detected under the fixture.
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, dir := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".config", "opencode")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	// validHiveDaemonExecutable checks the executable bit on Unix and the .exe
	// extension on Windows, so the stand-in must be valid under both rules.
	daemonName := "hive-daemon"
	if runtime.GOOS == "windows" {
		daemonName += ".exe"
	}
	daemon := filepath.Join(home, ".jarvis", "bin", daemonName)
	writeFile(t, daemon, "#!/bin/sh\n")
	if err := os.Chmod(daemon, 0o755); err != nil {
		t.Fatalf("chmod %s: %v", daemon, err)
	}

	byID := map[string]agent.Agent{}
	for _, detected := range agent.Detect(jarvis.TemplatesFS) {
		byID[detected.Name()] = detected
	}
	return home, byID, daemon
}

func newMCPComponent(agents map[string]agent.Agent, daemon string, exec *capturingExecutor) MCPComponent {
	return MCPComponent{
		Resolve: func(id string) (agent.Agent, bool) { a, ok := agents[id]; return a, ok },
		Deps: agentapply.MCPDeps{
			NewExecutor:    func() agentapply.MCPExecutor { return exec },
			HiveDaemonPath: func(string) string { return daemon },
		},
	}
}

// Jarvis-managed MCPs are replaced on every run. They are derived from embedded
// code (agent/native_mcp_recovery.go:44-72) and are never a persisted user
// choice: the wizard receives MCP entries and discards them, and both call sites
// pass an empty agent.MCPEntry. There is nothing for replay to consult, so the
// second run of an already-correct machine replaces exactly as the first did.
func TestMCPComponent_ReplacesManagedMCPsUnconditionally(t *testing.T) {
	home, agents, daemon := mcpReplayFixture(t)
	exec := &capturingExecutor{}
	component := newMCPComponent(agents, daemon, exec)

	for run := 1; run <= 2; run++ {
		if err := component.Apply(AgentTarget{ID: "claude", Root: home}); err != nil {
			t.Fatalf("Apply() run %d error = %v", run, err)
		}
	}

	if len(exec.inputs) != 2 {
		t.Fatalf("executor invoked %d times, want 2: replacement must not be skipped once the MCPs are already in place", len(exec.inputs))
	}
	if !reflect.DeepEqual(exec.inputs[0], exec.inputs[1]) {
		t.Fatalf("the second run asked for a different desired state:\n first: %+v\nsecond: %+v", exec.inputs[0], exec.inputs[1])
	}

	// The component has no seam through which a stored decision could reach it.
	// Adding one must fail this test rather than become a quiet skip condition.
	wantFields := []string{"Resolve", "Reconcile", "Deps"}
	componentType := reflect.TypeOf(MCPComponent{})
	gotFields := make([]string, 0, componentType.NumField())
	for i := 0; i < componentType.NumField(); i++ {
		gotFields = append(gotFields, componentType.Field(i).Name)
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("MCPComponent fields = %v, want %v", gotFields, wantFields)
	}
}

// Replacement is identity-scoped. The desired state names only the two
// identities Jarvis derives from embedded code, so a user-defined MCP is never
// an argument to the replacement and cannot be reached by it.
func TestMCPComponent_DesiredStateNamesOnlyJarvisManagedIdentities(t *testing.T) {
	home, agents, daemon := mcpReplayFixture(t)
	exec := &capturingExecutor{}
	component := newMCPComponent(agents, daemon, exec)

	for _, id := range []string{"claude", "opencode"} {
		if err := component.Apply(AgentTarget{ID: id, Root: home}); err != nil {
			t.Fatalf("Apply(%s) error = %v", id, err)
		}
	}

	claudeInput := exec.inputs[0]
	if !reflect.DeepEqual(claudeInput.SelectedAgents, []string{"claude"}) {
		t.Fatalf("SelectedAgents = %v, want exactly one agent per request", claudeInput.SelectedAgents)
	}
	if claudeInput.ClaudeHive.Identity != "hive" || claudeInput.ClaudeContext7.Identity != "context7" {
		t.Fatalf("Claude desired state = %q/%q, want the canonical hive/context7 identities",
			claudeInput.ClaudeHive.Identity, claudeInput.ClaudeContext7.Identity)
	}
	if len(claudeInput.OpenCodeMCPs) != 0 {
		t.Fatalf("a Claude-scoped request must carry no OpenCode desired state, got %v", claudeInput.OpenCodeMCPs)
	}

	openCodeInput := exec.inputs[1]
	if !reflect.DeepEqual(openCodeInput.SelectedAgents, []string{"opencode"}) {
		t.Fatalf("SelectedAgents = %v, want exactly one agent per request", openCodeInput.SelectedAgents)
	}
	managed := make([]string, 0, len(openCodeInput.OpenCodeMCPs))
	for name := range openCodeInput.OpenCodeMCPs {
		managed = append(managed, name)
	}
	sort.Strings(managed)
	if !reflect.DeepEqual(managed, []string{"context7", "hive"}) {
		t.Fatalf("OpenCode desired state names %v, want only the Jarvis-managed identities", managed)
	}
	if openCodeInput.ClaudeHive.Identity != "" || openCodeInput.ClaudeContext7.Identity != "" {
		t.Fatalf("an OpenCode-scoped request must carry no Claude desired state, got %+v", openCodeInput)
	}
}

// A manifest agent this machine does not have installed is refused by both
// components, not guessed at, and nothing is handed to the executor.
func TestReplayComponents_RefuseAnAgentThatIsNotInstalled(t *testing.T) {
	home, agents, daemon := mcpReplayFixture(t)
	exec := &capturingExecutor{}
	target := AgentTarget{ID: "cursor", Root: home}
	unresolvable := func(string) (agent.Agent, bool) { return nil, false }

	if err := newMCPComponent(agents, daemon, exec).Apply(target); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("MCP component error = %v, want it to wrap ErrUnknownAgent", err)
	}
	if err := (StatuslineComponent{Resolve: unresolvable}).Apply(target); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("statusline component error = %v, want it to wrap ErrUnknownAgent", err)
	}
	if len(exec.inputs) != 0 {
		t.Fatalf("executor was invoked %d times for an unresolvable agent", len(exec.inputs))
	}
}

// AgentResolver is a nilable func type, so a caller that never wired one must
// meet the same refusal every other component gives an unresolvable agent. A
// nil resolver is a missing dependency, not a reason to panic mid-replay.
func TestRunnerComponents_RejectNilResolverWithoutPanicking(t *testing.T) {
	target := AgentTarget{ID: "claude", Root: t.TempDir()}

	if err := (MCPComponent{}).Apply(target); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("MCP component error = %v, want it to wrap ErrUnknownAgent", err)
	}
	if err := (StatuslineComponent{}).Apply(target); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("statusline component error = %v, want it to wrap ErrUnknownAgent", err)
	}
}

// Reconcile is an exported override seam. A caller that supplies one must see it
// used: silently falling back to agentapply.ReconcileMCPs would ignore the
// injected handoff and reach the real machine instead.
func TestMCPComponent_UsesInjectedReconciler(t *testing.T) {
	home, agents, daemon := mcpReplayFixture(t)
	exec := &capturingExecutor{}
	component := newMCPComponent(agents, daemon, exec)

	var got []agent.Agent
	injected := errors.New("injected reconciler")
	component.Reconcile = func(reconciled []agent.Agent, home string, _ agentapply.MCPDeps) error {
		got = reconciled
		return injected
	}

	err := component.Apply(AgentTarget{ID: "claude", Root: home})
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want it to wrap the injected reconciler's error", err)
	}
	if len(got) != 1 || got[0].Name() != "claude" {
		t.Fatalf("injected reconciler received %v, want exactly the resolved claude agent", got)
	}
	if len(exec.inputs) != 0 {
		t.Fatalf("the default reconciler ran %d times despite an injected override", len(exec.inputs))
	}
}
