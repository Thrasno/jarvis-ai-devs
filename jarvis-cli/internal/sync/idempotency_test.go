package sync

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

type plannedWrite struct {
	path string
	body string
	mode fs.FileMode
}

// desiredStateRunner mimics the real components where it matters: it removes and
// rewrites every path it owns on every single run, exactly the way
// InstallStatusline does (agent/claude.go:882-885). A run measured by
// modification time would therefore report a change forever, and a count taken
// from the plan would report one too.
type desiredStateRunner struct {
	*recordingRunner
	writes map[string][]plannedWrite
}

func (r *desiredStateRunner) ApplyPersonaInstructions(target AgentTarget) error {
	if err := r.recordingRunner.ApplyPersonaInstructions(target); err != nil {
		return err
	}
	for _, write := range r.writes[target.ID] {
		if err := os.MkdirAll(filepath.Dir(write.path), 0o755); err != nil {
			return err
		}
		if err := os.Remove(write.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(write.path, []byte(write.body), write.mode); err != nil {
			return err
		}
	}
	return nil
}

func measuredRun(t *testing.T, home string, plan Plan, runner ComponentRunner, agents ...string) RunResult {
	t.Helper()
	targets := make([]AgentTarget, 0, len(agents))
	for _, id := range agents {
		targets = append(targets, AgentTarget{ID: id})
	}
	result, err := Run(RunInput{
		Plan:   plan,
		Apply:  ApplyInput{Runner: runner, Targets: targets},
		Backup: lifecycle.NewBackupStore(home).CreateSnapshotOfTargets,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	return result
}

// Measured idempotency. The second run rewrites every tracked path with the
// bytes that are already there, so the honest answer is zero changed files. The
// count comes from the content+mode diff and from nowhere else: the plan
// describes desired state rather than drift, so any number derived from
// Plan.Artifacts would claim a change on every run.
func TestRun_ASecondRunOverUnchangedDesiredStateReportsNoChangedFile(t *testing.T) {
	home := t.TempDir()
	instructions := filepath.Join(home, ".claude", "CLAUDE.md")
	script := filepath.Join(home, ".claude", statuslineScriptName)
	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: instructions, Mode: ManagedFileMode},
		{Agent: "claude", Path: script, Mode: ManagedExecutableMode},
	}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {
			{path: instructions, body: "<!-- jarvis -->\n", mode: 0o644},
			{path: script, body: "#!/bin/sh\necho jarvis\n", mode: 0o755},
		},
	}}

	first := measuredRun(t, home, plan, runner, "claude")
	if want := []string{instructions, script}; !reflect.DeepEqual(first.Report.Changed, want) {
		t.Fatalf("first run Changed = %v, want %v", first.Report.Changed, want)
	}

	second := measuredRun(t, home, plan, runner, "claude")
	if len(second.Report.Changed) != 0 {
		t.Fatalf("second run over unchanged desired state reported %v", second.Report.Changed)
	}
	for _, result := range second.Report.Agents {
		if len(result.Changed) != 0 {
			t.Fatalf("agent %s reported %v on an unchanged second run", result.Agent, result.Changed)
		}
	}
}

// Required changed-path output: the report names the paths, not a count. A run
// that changed three files says which three, and a tracked path that was
// rewritten with identical bytes is not one of them.
func TestRun_NamesTheExactPathsItChangedAndAttributesThemToTheirAgent(t *testing.T) {
	home := t.TempDir()
	claudeMd := filepath.Join(home, ".claude", "CLAUDE.md")
	claudeSkill := filepath.Join(home, ".claude", "skills", "sdd-apply", "SKILL.md")
	agentsMd := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	untouched := filepath.Join(home, ".claude", statuslineScriptName)
	writeFile(t, untouched, "#!/bin/sh\n")
	if err := os.Chmod(untouched, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	plan := Plan{Tracked: []TrackedPath{
		{Agent: "claude", Path: claudeMd, Mode: ManagedFileMode},
		{Agent: "claude", Path: claudeSkill, Mode: ManagedFileMode},
		{Agent: "claude", Path: untouched, Mode: ManagedExecutableMode},
		{Agent: "opencode", Path: agentsMd, Mode: ManagedFileMode},
	}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {
			{path: claudeMd, body: "claude instructions\n", mode: 0o644},
			{path: claudeSkill, body: "# sdd-apply\n", mode: 0o644},
			{path: untouched, body: "#!/bin/sh\n", mode: 0o755},
		},
		"opencode": {{path: agentsMd, body: "opencode instructions\n", mode: 0o644}},
	}}

	result := measuredRun(t, home, plan, runner, "claude", "opencode")

	want := []string{claudeMd, claudeSkill, agentsMd}
	if !reflect.DeepEqual(result.Report.Changed, want) {
		t.Fatalf("Report.Changed = %v, want %v", result.Report.Changed, want)
	}
	perAgent := map[string][]string{}
	for _, agent := range result.Report.Agents {
		perAgent[agent.Agent] = agent.Changed
	}
	wantPerAgent := map[string][]string{
		"claude":   {claudeMd, claudeSkill},
		"opencode": {agentsMd},
	}
	if !reflect.DeepEqual(perAgent, wantPerAgent) {
		t.Fatalf("per-agent changed paths = %v, want %v", perAgent, wantPerAgent)
	}
}

// EnforceModes belongs to the mutation pass, not to the measurement: a writer
// that recreated a deleted file under the process umask left it 0600, and the
// asserted 0755 must be back before the after-snapshot is taken. Restoring it
// makes the run genuinely idempotent instead of merely reporting the drift.
func TestRun_AssertsManagedModesBeforeItMeasuresTheDiff(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(home, ".claude", statuslineScriptName)
	writeFile(t, script, "#!/bin/sh\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	plan := Plan{Tracked: []TrackedPath{{Agent: "claude", Path: script, Mode: ManagedExecutableMode}}}
	runner := &desiredStateRunner{recordingRunner: &recordingRunner{}, writes: map[string][]plannedWrite{
		"claude": {{path: script, body: "#!/bin/sh\n", mode: 0o600}},
	}}

	result := measuredRun(t, home, plan, runner, "claude")

	info, err := os.Lstat(script)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode().Perm() != ManagedExecutableMode {
		t.Fatalf("mode = %04o, want %04o", info.Mode().Perm(), ManagedExecutableMode)
	}
	if len(result.Report.Changed) != 0 {
		t.Fatalf("restored mode and identical bytes are not a change, got %v", result.Report.Changed)
	}
}

// Attribution is recorded where the owner is known, not derived later from a
// path prefix or an identity string, so the per-agent report cannot drift from
// the manifest.
func TestBuildPlan_TrackedPathsRecordTheAgentThatOwnsThem(t *testing.T) {
	root := t.TempDir()
	st := replayableState(
		state.Agent{ID: "claude", InstructionsPath: ".claude/CLAUDE.md", ConfigPath: "settings.json"},
		state.Agent{ID: "opencode", InstructionsPath: ".config/opencode/AGENTS.md", ConfigPath: "opencode.json"},
	)
	st.Skills = []string{"sdd-apply"}
	st.Statusline = state.StatuslineState{Decided: true, Enabled: true}

	plan, err := BuildPlan(PlanInput{Root: root, State: st, Templates: jarvis.TemplatesFS})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	got := map[string]string{}
	for _, tracked := range plan.Tracked {
		got[tracked.Path] = tracked.Agent
	}
	want := map[string]string{
		filepath.Join(root, ".claude", "CLAUDE.md"):                                   "claude",
		filepath.Join(root, ".claude", "skills", "sdd-apply", "SKILL.md"):             "claude",
		filepath.Join(root, ".claude", statuslineScriptName):                          "claude",
		filepath.Join(root, ".config", "opencode", "AGENTS.md"):                       "opencode",
		filepath.Join(root, ".config", "opencode", "skills", "sdd-apply", "SKILL.md"): "opencode",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tracked owners = %v, want %v", got, want)
	}
}
