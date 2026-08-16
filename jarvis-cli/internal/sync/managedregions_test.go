package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// planForInstructions plans a machine whose only managed output is one agent's
// instruction file: no skills, no statusline, no persona, so the tracked list is
// exactly the path under test.
func planForInstructions(t *testing.T, root, agentID, location string) Plan {
	t.Helper()
	plan, err := BuildPlan(PlanInput{
		Root:      root,
		State:     replayableState(state.Agent{ID: agentID, InstructionsPath: location}),
		Templates: jarvis.TemplatesFS,
		Layer1:    "layer one",
		Layer2:    "layer two",
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Artifacts) != 1 || len(plan.Tracked) != 1 {
		t.Fatalf("plan covers %d artifacts and %d tracked paths, want exactly one of each", len(plan.Artifacts), len(plan.Tracked))
	}
	return plan
}

// The defect this fixes. A user's own prose in their CLAUDE.md or AGENTS.md
// lives outside the Jarvis markers and the writer preserves it, so a whole-file
// comparison measured a file Jarvis had just written correctly as invalid --
// forever, because repairing it left the prose exactly where it was.
func TestSnapshot_InstructionFileIsMeasuredOnItsManagedRegionsOnly(t *testing.T) {
	for _, tt := range []struct {
		agentID  string
		location string
	}{
		{agentID: "claude", location: ".claude/CLAUDE.md"},
		{agentID: "opencode", location: ".config/opencode/AGENTS.md"},
	} {
		t.Run(tt.agentID, func(t *testing.T) {
			root := t.TempDir()
			plan := planForInstructions(t, root, tt.agentID, tt.location)
			path := plan.Tracked[0].Path
			const prose = "\n## My own section\n\nNotes Jarvis does not own.\n"
			writeFile(t, path, string(plan.Artifacts[0].Bytes)+prose)

			after := snapshotOrFail(t, plan.Tracked)

			if !after.Matches(plan.Tracked) {
				t.Error("a file carrying the managed regions plus the user's own prose must measure as converged")
			}
			if err := verifyApplied(after, plan.Tracked, []AgentTarget{{ID: tt.agentID}}); err != nil {
				t.Errorf("verification failed on a correctly written file: %v", err)
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read %s: %v", path, readErr)
			}
			if !strings.Contains(string(body), "Notes Jarvis does not own.") {
				t.Error("the fixture lost the user's prose, so it proves nothing")
			}
		})
	}
}

// The property the fix must not trade away: narrowing the comparison to the
// managed regions must not make Jarvis blind to its own sections being edited.
func TestSnapshot_InstructionFileStillDetectsAnEditInsideTheManagedRegions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		corrupt func(string) string
	}{
		{name: "layer edited", corrupt: func(content string) string {
			return strings.Replace(content, "layer two", "layer two, edited by hand", 1)
		}},
		{name: "protocol block emptied", corrupt: func(content string) string {
			start := strings.Index(content, agent.HiveProtocolStart)
			end := strings.Index(content, agent.HiveProtocolEnd)
			return content[:start+len(agent.HiveProtocolStart)] + "\n" + content[end:]
		}},
		{name: "file truncated", corrupt: func(content string) string {
			return content[:strings.Index(content, agent.HiveProtocolStart)]
		}},
		{name: "file emptied", corrupt: func(string) string { return "" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			plan := planForInstructions(t, root, "opencode", ".config/opencode/AGENTS.md")
			writeFile(t, plan.Tracked[0].Path, tt.corrupt(string(plan.Artifacts[0].Bytes)))

			after := snapshotOrFail(t, plan.Tracked)

			if after.Matches(plan.Tracked) {
				t.Error("a tampered managed region must not measure as converged")
			}
			if err := verifyApplied(after, plan.Tracked, []AgentTarget{{ID: "opencode"}}); err == nil {
				t.Error("verification must fail on a tampered managed region")
			}
		})
	}
}

// A file that was never written is still missing, whatever the comparison is
// narrowed to.
func TestSnapshot_InstructionFileAbsenceIsStillDetected(t *testing.T) {
	root := t.TempDir()
	plan := planForInstructions(t, root, "claude", ".claude/CLAUDE.md")

	after := snapshotOrFail(t, plan.Tracked)

	if after.Matches(plan.Tracked) {
		t.Error("an absent instruction file must never measure as converged")
	}
	if err := verifyApplied(after, plan.Tracked, []AgentTarget{{ID: "claude"}}); err == nil {
		t.Error("verification must fail when the instruction file was never written")
	}
}

// Skills and the statusline are files Jarvis owns whole. Their digests stay
// whole-file, so appending anything to one is drift rather than a user's own
// content.
func TestSnapshot_FilesJarvisOwnsWholeAreStillComparedWhole(t *testing.T) {
	root := t.TempDir()
	st := replayableState(state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md"})
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

	for _, whole := range []struct {
		path    string
		content string
	}{
		{path: filepath.Join(root, ".claude", statuslineScriptName), content: scriptBody},
		{path: filepath.Join(root, ".claude", "skills", "sdd-apply", "SKILL.md"), content: "# sdd-apply\n"},
	} {
		t.Run(filepath.Base(whole.path), func(t *testing.T) {
			var tracked []TrackedPath
			for _, candidate := range plan.Tracked {
				if candidate.Path == whole.path {
					tracked = []TrackedPath{candidate}
				}
			}
			if tracked == nil {
				t.Fatalf("the plan does not track %s", whole.path)
			}
			writeFile(t, whole.path, whole.content+"# appended by the user\n")
			if err := os.Chmod(whole.path, tracked[0].Mode); err != nil {
				t.Fatalf("chmod %s: %v", whole.path, err)
			}

			if snapshotOrFail(t, tracked).Matches(tracked) {
				t.Error("a file Jarvis owns whole must be compared whole, so appended content is drift")
			}
		})
	}
}

// The managed asset digest still moves when the installed version's assets move.
// Narrowing the instruction comparison must not make a plan indistinguishable
// from a plan for different content.
func TestManagedAssetDigest_StillChangesWhenTheInstructionAssetsChange(t *testing.T) {
	root := t.TempDir()
	planWith := func(layer2 string) Plan {
		t.Helper()
		plan, err := BuildPlan(PlanInput{
			Root:      root,
			State:     replayableState(state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md"}),
			Templates: jarvis.TemplatesFS,
			Layer1:    "layer one",
			Layer2:    layer2,
		})
		if err != nil {
			t.Fatalf("BuildPlan: %v", err)
		}
		return plan
	}

	if planWith("layer two").Tracked[0].Desired == planWith("a different layer two").Tracked[0].Desired {
		t.Error("changing managed instruction content must change the tracked digest")
	}
	if ManagedAssetDigest(planWith("layer two")) == ManagedAssetDigest(planWith("a different layer two")) {
		t.Error("changing managed instruction content must change the managed asset digest")
	}
}
