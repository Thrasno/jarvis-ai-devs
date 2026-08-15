package sync

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// capturingConfigure stands in for agentapply.ConfigureAgent so a test can see
// exactly what the runner hands the installer without installing anything.
type capturingConfigure struct {
	agents      []string
	phaseModels []state.PhaseModels
	selected    [][]string
	statusline  []agentapply.StatuslineDecision
	err         map[string]error
}

type outputStyleReplayAgent struct {
	agent.Agent
	supports          bool
	instructionWrites int
	outputWrites      int
	outputErr         error
}

func (a *outputStyleReplayAgent) WriteInstructions(string, string, []config.SkillInfo) error {
	a.instructionWrites++
	return nil
}

func (a *outputStyleReplayAgent) SupportsOutputStyles() bool { return a.supports }

func (a *outputStyleReplayAgent) WriteOutputStyle(*persona.Profile) error {
	a.outputWrites++
	return a.outputErr
}

func TestRunnerApplyPersonaInstructionsOutputStyleWiring(t *testing.T) {
	boom := errors.New("write output style")
	tests := []struct {
		name       string
		supports   bool
		profile    *persona.Profile
		outputErr  error
		wantWrites int
		wantErr    error
	}{
		{name: "supported agent writes output style", supports: true, profile: &persona.Profile{Name: "neutra"}, wantWrites: 1},
		{name: "unsupported agent is excluded", profile: &persona.Profile{Name: "neutra"}},
		{name: "nil profile skips output style", supports: true},
		{name: "write error propagates", supports: true, profile: &persona.Profile{Name: "neutra"}, outputErr: boom, wantWrites: 1, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			instructions := filepath.Join(root, ".agent", "AGENTS.md")
			fake := &outputStyleReplayAgent{supports: tt.supports, outputErr: tt.outputErr}
			runner := NewRunner(ReplayInput{
				Root:    root,
				State:   &state.State{InstalledAgents: []state.Agent{{ID: "test", InstructionsPath: instructions}}},
				Profile: tt.profile,
				Resolve: func(string) (agent.Agent, bool) { return fake, true },
			})
			err := runner.ApplyPersonaInstructions(AgentTarget{ID: "test", Root: root, InstructionsPath: instructions})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if fake.instructionWrites != 1 {
				t.Fatalf("instruction writes = %d, want 1", fake.instructionWrites)
			}
			if fake.outputWrites != tt.wantWrites {
				t.Fatalf("output-style writes = %d, want %d", fake.outputWrites, tt.wantWrites)
			}
		})
	}
}

func (c *capturingConfigure) configure(
	a agent.Agent,
	phaseModels state.PhaseModels,
	_ agent.MCPEntry,
	_ agent.MCPEntry,
	_ fs.FS,
	selectedIDs []string,
	_ fs.FS,
	statusline agentapply.StatuslineDecision,
) ([]string, error) {
	c.agents = append(c.agents, a.Name())
	c.phaseModels = append(c.phaseModels, phaseModels)
	c.selected = append(c.selected, selectedIDs)
	c.statusline = append(c.statusline, statusline)
	return nil, c.err[a.Name()]
}

func newReplayFixture(t *testing.T) (ReplayInput, *capturingConfigure) {
	t.Helper()
	home, agents, daemon := mcpReplayFixture(t)
	skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("sub skills FS: %v", err)
	}
	configure := &capturingConfigure{err: map[string]error{}}
	in := ReplayInput{
		Root: home,
		State: &state.State{
			InstalledAgents: []state.Agent{
				{ID: "claude", InstructionsPath: filepath.Join(home, ".claude", "CLAUDE.md")},
				{ID: "opencode", InstructionsPath: filepath.Join(home, ".config", "opencode", "AGENTS.md")},
			},
			Skills: []string{"go-testing"},
			// Populated so the hazard-1 assertion below compares a real value
			// rather than passing on two empty structs.
			PhaseModels: state.PhaseModels{
				Aliases: map[string]state.PhaseModelSelection{"default": {Claude: "opus", OpenCode: "gpt-5"}},
			},
		},
		Templates: jarvis.TemplatesFS,
		SkillsFS:  skillsSubFS,
		HooksFS:   jarvis.HooksFS,
		Skills:    []config.SkillInfo{{Name: "go-testing"}},
		Layer1:    "layer one",
		Layer2:    "layer two",
		Resolve:   func(id string) (agent.Agent, bool) { a, ok := agents[id]; return a, ok },
		MCPDeps: agentapply.MCPDeps{
			NewExecutor:    func() agentapply.MCPExecutor { return &capturingExecutor{} },
			HiveDaemonPath: func(string) string { return daemon },
		},
		Configure: configure.configure,
	}
	return in, configure
}

// Hazard 1, the model-assignment identity trap. If the planner digests skill
// files rendered from one set of per-phase assignments while the installer
// writes files rendered from another, every run reports drift forever. One
// value, the manifest's, reaches both ends.
func TestReplayInput_HandsTheSamePointerToThePlannerAndTheSkillInstall(t *testing.T) {
	in, configure := newReplayFixture(t)

	planned := PlanInputFor(in)
	if err := NewRunner(in).ApplyModels(TargetsFor(in)[0]); err != nil {
		t.Fatalf("ApplyModels: %v", err)
	}

	if len(configure.phaseModels) != 1 {
		t.Fatalf("configure calls = %d, want 1", len(configure.phaseModels))
	}
	if !reflect.DeepEqual(configure.phaseModels[0], planned.State.PhaseModels) {
		t.Fatalf("installer phase models %+v are not the planner's %+v", configure.phaseModels[0], planned.State.PhaseModels)
	}
	if !reflect.DeepEqual(configure.selected[0], in.State.Skills) {
		t.Fatalf("installed skill IDs = %v, want the manifest's %v", configure.selected[0], in.State.Skills)
	}
	if &planned.Skills[0] != &in.Skills[0] {
		t.Fatal("the planner must render instructions from the caller's own skill list, not a copy")
	}
}

// The runner satisfies the applier's contract, and the three IDs that share one
// ConfigureAgent pass run that pass exactly once.
func TestRunner_RunsTheInstallerPassOnceAcrossTheFirstThreeComponents(t *testing.T) {
	var _ ComponentRunner = (*Runner)(nil)
	in, configure := newReplayFixture(t)
	runner := NewRunner(in)
	target := TargetsFor(in)[0]

	for _, apply := range []func(AgentTarget) error{
		runner.ApplyModels, runner.ApplySkills, runner.ApplyRuntimeAssets,
	} {
		if err := apply(target); err != nil {
			t.Fatalf("component: %v", err)
		}
	}

	if !reflect.DeepEqual(configure.agents, []string{"claude"}) {
		t.Fatalf("configure ran for %v, want exactly one claude pass", configure.agents)
	}
	// The statusline is the last component and runs after the instruction write,
	// so the installer pass must not touch it.
	if configure.statusline[0].Install {
		t.Fatal("ConfigureAgent must be told not to install the statusline")
	}
}

// D1 with the production runner. Attribution across the first three IDs is
// coarse, but the failing agent is still named and its sibling still converges.
func TestRunner_NamesTheFailingAgentAndLeavesItsSiblingConverged(t *testing.T) {
	boom := errors.New("generated config guardrails")
	in, configure := newReplayFixture(t)
	configure.err["claude"] = boom

	report := Apply(ApplyInput{Runner: NewRunner(in), Targets: TargetsFor(in)})

	failed := report.Agents[0]
	if failed.Agent != "claude" || failed.Converged || !errors.Is(failed.Err, boom) {
		t.Fatalf("claude must be named as the failing agent, got %+v", failed)
	}
	if failed.FailedAt != ComponentModels {
		t.Fatalf("FailedAt = %q, want %q", failed.FailedAt, ComponentModels)
	}
	if healthy := report.Agents[1]; healthy.Agent != "opencode" || !healthy.Converged {
		t.Fatalf("opencode must converge independently, got %+v", healthy)
	}
}

// The instruction write stays behind the manifest ownership guard: a path the
// manifest does not record for this agent is refused before the writer opens it.
func TestRunner_RefusesAnInstructionWriteOutsideTheManifestOwnedPath(t *testing.T) {
	in, _ := newReplayFixture(t)
	runner := NewRunner(in)

	stolen := AgentTarget{ID: "claude", Root: in.Root, InstructionsPath: filepath.Join(in.Root, "elsewhere", "CLAUDE.md")}
	if err := runner.ApplyPersonaInstructions(stolen); !errors.Is(err, ErrUnownedInstructionsPath) {
		t.Fatalf("error = %v, want it to wrap ErrUnownedInstructionsPath", err)
	}
}

// A manifest agent this machine does not have is refused, never substituted.
func TestRunner_RefusesAManifestAgentThisMachineDoesNotHave(t *testing.T) {
	in, _ := newReplayFixture(t)
	runner := NewRunner(in)

	err := runner.ApplyModels(AgentTarget{ID: "ghost", Root: in.Root})
	if !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("error = %v, want it to wrap ErrUnknownAgent", err)
	}
}

// The statusline component's resolver and hooks FS do not come from the
// manifest, so a nil State must not leave them unwired: only Consent is
// State-derived, and its zero value already means "never asked".
func TestNewRunner_WiresStatuslineResolverWhenStateIsNil(t *testing.T) {
	fake := &outputStyleReplayAgent{}
	runner := NewRunner(ReplayInput{
		HooksFS: jarvis.HooksFS,
		Resolve: func(string) (agent.Agent, bool) { return fake, true },
	})

	if runner.statusline.Resolve == nil {
		t.Fatal("statusline resolver is unwired, so replay would refuse every agent it can resolve")
	}
	if runner.statusline.HooksFS == nil {
		t.Fatal("statusline hooks FS is unwired, so the embedded script is unreachable")
	}
	if runner.statusline.Consent != (state.StatuslineState{}) {
		t.Fatalf("statusline consent = %+v, want the zero \"never asked\" value without a manifest", runner.statusline.Consent)
	}
}

// Targets come from the manifest in order, with its recorded instruction path.
func TestTargetsFor_ProjectsTheManifestOntoPerAgentTargets(t *testing.T) {
	in, _ := newReplayFixture(t)

	want := []AgentTarget{
		{ID: "claude", Root: in.Root, InstructionsPath: filepath.Join(in.Root, ".claude", "CLAUDE.md")},
		{ID: "opencode", Root: in.Root, InstructionsPath: filepath.Join(in.Root, ".config", "opencode", "AGENTS.md")},
	}
	if got := TargetsFor(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetsFor = %+v, want %+v", got, want)
	}
}
