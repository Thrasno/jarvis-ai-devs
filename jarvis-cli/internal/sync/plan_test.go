package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// replayableState is a manifest that passes validation, so every planner test
// exercises the planner's own rules rather than state validation.
func replayableState(agents ...state.Agent) *state.State {
	st := state.New()
	st.Persona = "gentleman"
	st.ManagedAssetDigest = "sha256:embedded-assets"
	st.InstalledAgents = agents
	return st
}

// snapshotTree records every file under root with its permission bits, so a
// test can prove the read-only planner wrote nothing.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		tree[filepath.ToSlash(rel)] = fmt.Sprintf("%04o:%s", info.Mode().Perm(), data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return tree
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// An agent-less manifest must block. The planner must never recover by looking
// for agent files on disk, so both installed agents are present on the
// filesystem and neither may be planned.
func TestBuildPlan_AgentlessManifestBlocksAndNeverRedetects(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "installed claude instructions")
	writeFile(t, filepath.Join(root, ".config", "opencode", "AGENTS.md"), "installed opencode instructions")
	before := snapshotTree(t, root)

	tests := []struct {
		name  string
		state *state.State
	}{
		{name: "manifest records no agents", state: replayableState()},
		{name: "manifest is absent", state: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := BuildPlan(PlanInput{Root: root, State: tt.state, Templates: jarvis.TemplatesFS})

			if !errors.Is(err, ErrNoConfiguredAgents) {
				t.Fatalf("BuildPlan error = %v, want ErrNoConfiguredAgents", err)
			}
			if !strings.Contains(err.Error(), "jarvis") {
				t.Fatalf("block message %q does not name the recovery command %q", err, "jarvis")
			}
			if len(plan.Artifacts) != 0 {
				t.Fatalf("blocked plan carries %d artifacts, want 0", len(plan.Artifacts))
			}
			if after := snapshotTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatalf("planner mutated the filesystem:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// A planned instruction target must equal the bytes the writer actually
// produces for it. That is the contract, and it is the whole point of planning:
// the plan's digests are what a run measures the machine against, so a plan
// describing a file no writer produces makes every replayed machine fail its own
// verification and report drift forever.
//
// This assertion used to compare the plan against config.RenderCLAUDEMd and
// config.RenderAGENTSMd -- the raw template renderers -- and it was wrong. The
// writers render the template and then inject the Hive protocol block, and
// Claude additionally the orchestrator @import, so the planned bytes were a
// strict prefix of the written ones. Comparing against the renderer instead of
// against the writer is precisely what let that gap look correct: the test
// passed, the digests never matched on a real machine, and `jarvis sync` exited
// non-zero on every run telling the user to run `jarvis sync` to repair.
//
// So the comparison is against the real writers, driven over a real home. A
// planner-side rewrite that stops agreeing with the writer fails here, and so
// does a writer that grows an assembly step the planner does not know about --
// which no comparison against a renderer, and no test in this package driving a
// fake ComponentRunner, is able to catch.
//
// The read-only claim rides along: the targets still come from the assets
// embedded in the running binary, whatever a previous version left on disk, and
// planning writes nothing.
func TestBuildPlan_PlansExactlyWhatTheWriterProduces(t *testing.T) {
	root, agents, _ := mcpReplayFixture(t)
	claudePath := filepath.Join(root, ".claude", "CLAUDE.md")
	agentsPath := filepath.Join(root, ".config", "opencode", "AGENTS.md")
	// Stale content from an older version, so the plan is proven to come from the
	// embedded assets rather than from what is already there.
	writeFile(t, claudePath, "stale instructions rendered by an older version")
	writeFile(t, agentsPath, "stale instructions rendered by an older version")
	before := snapshotTree(t, root)

	in := PlanInput{
		Root: root,
		State: replayableState(
			state.Agent{ID: "claude", InstructionsPath: claudePath, ConfigPath: "settings.json"},
			state.Agent{ID: "opencode", InstructionsPath: agentsPath, ConfigPath: "opencode.json"},
		),
		Templates: jarvis.TemplatesFS,
		Layer1:    "layer one",
		Layer2:    "layer two",
		Skills:    []config.SkillInfo{{Name: "sdd-apply", Description: "implements tasks", Trigger: "apply"}},
	}

	plan, err := BuildPlan(in)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("planner mutated the filesystem; planning is read-only")
	}

	planned := map[string]string{}
	for _, artifact := range plan.Artifacts {
		planned[artifact.Location] = string(artifact.Bytes)
	}
	// Without this the loop below would pass on an empty plan, asserting nothing.
	if len(planned) != 2 {
		t.Fatalf("planned %d instruction targets, want 2: %v", len(planned), planned)
	}

	for _, tc := range []struct{ id, location, path string }{
		{"claude", ".claude/CLAUDE.md", claudePath},
		{"opencode", ".config/opencode/AGENTS.md", agentsPath},
	} {
		t.Run(tc.id, func(t *testing.T) {
			installed, detected := agents[tc.id]
			if !detected {
				t.Fatalf("%s is not detected under the fixture home, so this proves nothing", tc.id)
			}
			if err := installed.WriteInstructions(in.Layer1, in.Layer2, in.Skills); err != nil {
				t.Fatalf("WriteInstructions: %v", err)
			}
			written, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read the written instruction file: %v", err)
			}
			if planned[tc.location] != string(written) {
				t.Fatalf("the plan for %s does not describe what the writer produced\nplanned %d bytes, written %d bytes",
					tc.location, len(planned[tc.location]), len(written))
			}
		})
	}
}

// Managed instruction files carry no provenance marker on disk, so no marker was
// ever observed and none may be claimed. Their ownership proof is manifest
// membership, the same rule ApplyInstructions enforces before it writes.
func TestBuildPlan_InstructionTargetsAreProvenByManifestNotByMarker(t *testing.T) {
	root := t.TempDir()
	in := PlanInput{
		Root:      root,
		State:     replayableState(state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md", ConfigPath: "settings.json"}),
		Templates: jarvis.TemplatesFS,
	}

	plan, err := BuildPlan(in)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Artifacts) != 1 {
		t.Fatalf("planned %d artifacts, want 1", len(plan.Artifacts))
	}

	artifact := plan.Artifacts[0]
	proof, isIdentity := artifact.Proof.(IdentityProof)
	if !isIdentity {
		t.Fatalf("proof is %T, want IdentityProof", artifact.Proof)
	}
	if proof.Source != IdentitySourceManifest {
		t.Fatalf("proof source is %q, want %q", proof.Source, IdentitySourceManifest)
	}
}

// A marker proof asserts that reconcile compared an observed on-disk marker
// against the manifest. That comparison never runs for instruction targets,
// because nothing populates the inventory reconcile classifies against. Planning
// must therefore never hand back a MarkerProof for one, whatever is on disk.
func TestBuildPlan_NeverClaimsAnUnobservedMarkerProof(t *testing.T) {
	root := t.TempDir()
	claudePath := filepath.Join(root, ".claude", "CLAUDE.md")
	// Content owned by someone else: no Jarvis marker, no Jarvis bytes.
	writeFile(t, claudePath, "# my own notes\nhand-written by the user, never by Jarvis\n")

	plan, err := BuildPlan(PlanInput{
		Root:      root,
		State:     replayableState(state.Agent{ID: "claude", InstructionsPath: claudePath, ConfigPath: "settings.json"}),
		Templates: jarvis.TemplatesFS,
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	// Without this the loop below would pass on an empty plan, asserting nothing.
	if len(plan.Artifacts) != 1 {
		t.Fatalf("planned %d artifacts, want 1", len(plan.Artifacts))
	}
	for _, artifact := range plan.Artifacts {
		if _, isMarker := artifact.Proof.(MarkerProof); isMarker {
			t.Fatalf("artifact %q at %q claims MarkerProof, but no marker was ever read from disk", artifact.Identity, artifact.Location)
		}
	}
}

func TestBuildPlan_FailsClosedOnUnrenderableAgents(t *testing.T) {
	root := t.TempDir()
	outsideRoot := filepath.Join(t.TempDir(), "CLAUDE.md")

	tests := []struct {
		name  string
		agent state.Agent
	}{
		{
			name:  "the installed binary embeds no instruction template for the agent",
			agent: state.Agent{ID: "cursor", InstructionsPath: ".cursor/rules.md", ConfigPath: "cursor.json"},
		},
		{
			name:  "the recorded instructions path escapes the managed root",
			agent: state.Agent{ID: "claude", InstructionsPath: outsideRoot, ConfigPath: "settings.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := BuildPlan(PlanInput{Root: root, State: replayableState(tt.agent), Templates: jarvis.TemplatesFS})
			if err == nil {
				t.Fatal("BuildPlan succeeded, want a fail-closed error")
			}
			if len(plan.Artifacts) != 0 {
				t.Fatalf("failed plan carries %d artifacts, want 0", len(plan.Artifacts))
			}
		})
	}
}

// Failing closed is the right call when the installed binary embeds no source
// for something state.yaml records, but an abort that only names the problem
// leaves the operator with nowhere to go. The way in is a downgrade: a binary
// whose embedded assets no longer cover a statusline or the skills the previous
// installation recorded, which reinstalling from the running binary resolves.
//
// Every other fail-closed error on this path already ends by naming that
// command, so this is about parity as much as actionability: an operator who
// meets one of these two must not be the only one left guessing.
func TestBuildPlan_NamesTheRecoveryActionWhenTheBinaryEmbedsNoSourceForRecordedState(t *testing.T) {
	tests := map[string]func(*state.State, *PlanInput){
		"skills are recorded but the binary embeds no skills source": func(st *state.State, in *PlanInput) {
			st.Skills = []string{"sdd-apply"}
			in.SkillsFS = nil
		},
		"the statusline is managed but the binary embeds no hooks source": func(st *state.State, in *PlanInput) {
			st.Statusline = state.StatuslineState{Decided: true, Enabled: true}
			in.HooksFS = nil
		},
	}

	for name, setUp := range tests {
		t.Run(name, func(t *testing.T) {
			st := replayableState(state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md", ConfigPath: ".claude/settings.json"})
			in := PlanInput{Root: t.TempDir(), State: st, Templates: jarvis.TemplatesFS}
			setUp(st, &in)

			_, err := BuildPlan(in)
			if err == nil {
				t.Fatal("BuildPlan succeeded, want a fail-closed error")
			}
			if !strings.Contains(err.Error(), "run `jarvis` to reinstall") {
				t.Fatalf("error = %q, want it to name the recovery action every sibling fail-closed error names", err)
			}
		})
	}
}

// A relative recorded path can leave the managed root exactly as an absolute one
// can, and the doc comment promises both are refused rather than clamped. What
// managedLocation returns is joined back onto the root at every call site, so an
// escaped location does not stay a string: it becomes a tracked path, a snapshot
// entry, a diff subject and a backup target.
func TestManagedLocation_RefusesARelativePathThatLeavesTheManagedRoot(t *testing.T) {
	root := t.TempDir()

	for _, escaping := range []string{
		"..",
		filepath.Join("..", "outside", "CLAUDE.md"),
		filepath.Join("..", "..", "etc", "CLAUDE.md"),
		filepath.Join(".", "..", "outside", "CLAUDE.md"),
		filepath.Join(".claude", "..", "..", "outside", "CLAUDE.md"),
	} {
		if location, err := managedLocation(root, escaping); err == nil {
			t.Fatalf("managedLocation(%q) = %q, want a refusal: the location leaves the managed root", escaping, location)
		}
	}

	// A relative path that stays inside is still normalized, not refused. The
	// leading "..name" is a directory whose name starts with dots, not a climb.
	for path, want := range map[string]string{
		filepath.Join(".claude", "CLAUDE.md"):              ".claude/CLAUDE.md",
		filepath.Join(".claude", "sub", "..", "CLAUDE.md"): ".claude/CLAUDE.md",
		filepath.Join("..config", "AGENTS.md"):             "..config/AGENTS.md",
	} {
		got, err := managedLocation(root, path)
		if err != nil {
			t.Fatalf("managedLocation(%q) refused a location inside the root: %v", path, err)
		}
		if got != want {
			t.Fatalf("managedLocation(%q) = %q, want %q", path, got, want)
		}
	}
}

// Dropping an agent whose recorded path cannot be projected would remove it from
// Plan.Tracked, and Tracked is what the pre-apply backup captures and what the
// diff measures. A silently shortened list is the one outcome the single-list
// invariant exists to prevent, so the derivation fails closed like every other
// step in this function.
func TestTrackedPaths_FailsClosedOnAnAgentPathItCannotProject(t *testing.T) {
	in := PlanInput{
		Root:  t.TempDir(),
		State: replayableState(state.Agent{ID: "claude", InstructionsPath: filepath.Join("..", "outside", "CLAUDE.md"), ConfigPath: "settings.json"}),
	}

	tracked, err := trackedPaths(in, nil, nil)
	if err == nil {
		t.Fatalf("trackedPaths succeeded with %d paths, want a refusal: the agent's recorded path leaves the managed root", len(tracked))
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Fatalf("error %q does not name the agent it refused", err)
	}
}
