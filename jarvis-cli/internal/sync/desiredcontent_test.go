package sync

import (
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// skillsSourceFS stands in for the embedded skill tree: a selected skill with
// two files, the always-installed _shared directory, and a skill the manifest
// does not list. One manifest entry is therefore several installed files, which
// is exactly what a derived one-SKILL.md-per-skill path list got wrong.
func skillsSourceFS() fstest.MapFS {
	return fstest.MapFS{
		"sdd-apply/SKILL.md":            &fstest.MapFile{Data: []byte("# sdd-apply\n")},
		"sdd-apply/references/notes.md": &fstest.MapFile{Data: []byte("notes\n")},
		"_shared/sdd-phase-common.md":   &fstest.MapFile{Data: []byte("shared\n")},
		"sdd-verify/SKILL.md":           &fstest.MapFile{Data: []byte("# sdd-verify\n")},
	}
}

func hooksSourceFS(script string) fstest.MapFS {
	return fstest.MapFS{agent.StatuslineScriptSource: &fstest.MapFile{Data: []byte(script)}}
}

// Every tracked path must carry the content it will be compared against. A path
// whose desired state is unknown cannot answer "is there anything to do here?",
// so skills and the statusline are rendered rather than derived.
func TestBuildPlan_TracksDesiredContentForEveryPathIncludingSkillsAndStatusline(t *testing.T) {
	root := t.TempDir()
	st := replayableState(state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md", ConfigPath: ".claude/settings.json"})
	st.Skills = []string{"sdd-apply"}
	st.Statusline = state.StatuslineState{Decided: true, Enabled: true}
	const scriptBody = "#!/bin/sh\necho jarvis\n"

	plan, err := BuildPlan(PlanInput{
		Root: root, State: st, Templates: jarvis.TemplatesFS,
		SkillsFS: skillsSourceFS(), HooksFS: hooksSourceFS(scriptBody),
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	claudeDir := filepath.Join(root, ".claude")
	want := map[string]string{
		filepath.Join(claudeDir, "CLAUDE.md"):                                     digestOf(plan.Artifacts[0].Bytes),
		filepath.Join(claudeDir, "skills", "sdd-apply", "SKILL.md"):               digestOf([]byte("# sdd-apply\n")),
		filepath.Join(claudeDir, "skills", "sdd-apply", "references", "notes.md"): digestOf([]byte("notes\n")),
		filepath.Join(claudeDir, "skills", "_shared", "sdd-phase-common.md"):      digestOf([]byte("shared\n")),
		filepath.Join(claudeDir, statuslineScriptName):                            digestOf([]byte(scriptBody)),
		filepath.Join(claudeDir, "settings.json"):                                 "",
	}
	got := map[string]string{}
	for _, tracked := range plan.Tracked {
		if _, duplicate := got[tracked.Path]; duplicate {
			t.Fatalf("tracked path %s is listed twice", tracked.Path)
		}
		got[tracked.Path] = tracked.Desired
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tracked desired digests = %v, want %v", got, want)
	}
}

// A manifest recording artifacts this binary cannot render must fail closed.
// Planning a path whose desired content is unknowable would hand the
// short-circuit a question it cannot answer, which is the partial picture that
// made a pre-apply comparison unsafe in the first place.
func TestBuildPlan_FailsClosedWhenDesiredContentCannotBeRendered(t *testing.T) {
	tests := map[string]func(*state.State, *PlanInput){
		"skills are recorded but the binary embeds no skills source": func(st *state.State, _ *PlanInput) {
			st.Skills = []string{"sdd-apply"}
		},
		"the statusline is managed but the binary embeds no hooks source": func(st *state.State, in *PlanInput) {
			st.Statusline = state.StatuslineState{Decided: true, Enabled: true}
			in.HooksFS = fstest.MapFS{}
		},
	}
	for name, setUp := range tests {
		t.Run(name, func(t *testing.T) {
			st := replayableState(state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md", ConfigPath: ".claude/settings.json"})
			in := PlanInput{Root: t.TempDir(), State: st, Templates: jarvis.TemplatesFS}
			setUp(st, &in)

			plan, err := BuildPlan(in)
			if err == nil {
				t.Fatal("BuildPlan succeeded, want a fail-closed error")
			}
			if len(plan.Tracked) != 0 {
				t.Fatalf("failed plan carries %d tracked paths, want 0", len(plan.Tracked))
			}
		})
	}
}
