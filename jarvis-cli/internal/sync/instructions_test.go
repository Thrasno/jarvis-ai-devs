package sync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// replaySkills is the installed skill set every instruction test renders with,
// so the Skills section has something identifiable to assert on.
var replaySkills = []config.SkillInfo{
	{Name: "sdd-apply", Description: "implements tasks", Trigger: "apply"},
	{Name: "sdd-verify", Description: "verifies tasks", Trigger: "verify"},
}

// instructionsRunner performs the real instruction write for the
// persona-instructions component and no-ops every other one, so a test can run
// the whole locked order and still assert on a real file.
type instructionsRunner struct {
	own    InstructionOwnership
	writer InstructionsWriter
}

func (r *instructionsRunner) ApplyModels(AgentTarget) error        { return nil }
func (r *instructionsRunner) ApplySkills(AgentTarget) error        { return nil }
func (r *instructionsRunner) ApplyRuntimeAssets(AgentTarget) error { return nil }
func (r *instructionsRunner) ApplyMCPs(AgentTarget) error          { return nil }
func (r *instructionsRunner) ApplyStatusline(AgentTarget) error    { return nil }
func (r *instructionsRunner) ApplyPersonaInstructions(t AgentTarget) error {
	return ApplyInstructions(r.own, t, r.writer, "layer one", "layer two", replaySkills)
}

// claudeReplayFixture points HOME at a temporary directory, creates ~/.claude so
// the real ClaudeAgent is detected there, and returns that agent plus the path
// of the instruction file it owns.
func claudeReplayFixture(t *testing.T) (agent.Agent, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("create claude config dir: %v", err)
	}
	for _, detected := range agent.Detect(jarvis.TemplatesFS) {
		if detected.Name() == "claude" {
			return detected, filepath.Join(home, ".claude", "CLAUDE.md")
		}
	}
	t.Fatal("claude agent was not detected under the temporary home")
	return nil, ""
}

// runReplay walks the locked component order for one agent with the real
// instruction writer wired into the persona-instructions component.
func runReplay(t *testing.T, own InstructionOwnership, writer InstructionsWriter, target AgentTarget) Report {
	t.Helper()
	report := Apply(ApplyInput{
		Runner:  &instructionsRunner{own: own, writer: writer},
		Targets: []AgentTarget{target},
	})
	return report
}

// D6: a managed instruction file that lost its Jarvis sentinels is rendered
// fresh and its previous content is discarded, exactly as the installer does
// (agent/claude.go:350-356). Replay deliberately matches installer behaviour
// rather than inventing a second ownership rule for the same path; the
// pre-apply backup snapshot is the recovery path for the discarded content.
func TestApplyInstructions_RendersFreshWhenTheManagedFileLostItsSentinels(t *testing.T) {
	claude, path := claudeReplayFixture(t)
	writeFile(t, path, "# Hand-written notes\n\nNo Jarvis sentinels anywhere in this file.\n")

	own := NewInstructionOwnership([]state.Agent{{ID: "claude", InstructionsPath: path}})
	report := runReplay(t, own, claude, AgentTarget{ID: "claude", InstructionsPath: path})

	if !report.Converged() {
		t.Fatalf("replay must converge, got %+v", report.Agents)
	}
	content := readFileString(t, path)
	if err := agent.ValidateSentinels(content); err != nil {
		t.Fatalf("rendered file must carry valid sentinels: %v", err)
	}
	for _, skill := range replaySkills {
		if !strings.Contains(content, skill.Name) {
			t.Errorf("Skills section is missing installed skill %q", skill.Name)
		}
	}
	if !strings.Contains(content, agent.HiveProtocolStart) || !strings.Contains(content, agent.HiveProtocolEnd) {
		t.Error("rendered file must carry the Hive protocol block")
	}
	if !strings.Contains(content, agent.OrchestratorImportStart) {
		t.Error("rendered file must carry the orchestrator import block")
	}
	if strings.Contains(content, "Hand-written notes") {
		t.Error("the no-sentinel branch discards the previous content by design; it must not be merged back")
	}
}

// D6, other half: once the file carries sentinels, only the managed sections
// change. Every byte the user wrote outside them survives in place.
func TestApplyInstructions_PreservesContentOutsideManagedSectionsByteForByte(t *testing.T) {
	claude, path := claudeReplayFixture(t)
	const (
		above   = "# My own notes\n\nKeep this exact text.\n\n"
		between = "\nA paragraph between the managed blocks.\n\n"
		below   = "\n## Trailing notes\n\nAlso exact.\n"
	)
	writeFile(t, path, above+
		agent.Layer1Start+"\nstale layer one\n"+agent.Layer1End+between+
		agent.Layer2Start+"\nstale layer two\n"+agent.Layer2End+below)

	own := NewInstructionOwnership([]state.Agent{{ID: "claude", InstructionsPath: path}})
	report := runReplay(t, own, claude, AgentTarget{ID: "claude", InstructionsPath: path})

	if !report.Converged() {
		t.Fatalf("replay must converge, got %+v", report.Agents)
	}
	// The exact bytes the patch must produce: the user's prose untouched in
	// place, with only the two managed blocks rewritten.
	want := above +
		agent.Layer1Start + "\nlayer one\n" + agent.Layer1End + between +
		agent.Layer2Start + "\nlayer two\n" + agent.Layer2End + below
	content := readFileString(t, path)
	if !strings.HasPrefix(content, want) {
		t.Fatalf("content outside the managed sections was not preserved byte-for-byte.\n got: %q\nwant prefix: %q", content, want)
	}
	// Whatever follows is appended managed content only, never user prose moved
	// or rewritten.
	appended := strings.TrimPrefix(content, want)
	if !strings.Contains(appended, agent.HiveProtocolStart) || !strings.Contains(appended, agent.OrchestratorImportStart) {
		t.Errorf("appended tail must hold the managed protocol and orchestrator blocks, got %q", appended)
	}
	if strings.Count(content, "Keep this exact text.") != 1 || strings.Count(content, "Also exact.") != 1 {
		t.Error("user prose must appear exactly once; it was duplicated or rewritten")
	}
}

// countingWriter stands in for the real agent so a refused target can be proven
// to reach no writer at all, rather than merely leaving the file unchanged.
type countingWriter struct{ calls int }

func (w *countingWriter) WriteInstructions(string, string, []config.SkillInfo) error {
	w.calls++
	return nil
}

// A path Jarvis does not own is not read, not modified and not replaced.
// "Does not own" is decided by the manifest alone: a path it never recorded, or
// a recorded path claimed on behalf of a different agent.
func TestApplyInstructions_NeverTouchesAPathJarvisDoesNotOwn(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the unreadable-file probe cannot prove anything while running as root")
	}
	home := t.TempDir()
	ownedPath := filepath.Join(home, ".claude", "CLAUDE.md")
	own := NewInstructionOwnership([]state.Agent{{ID: "claude", InstructionsPath: ownedPath}})

	tests := []struct {
		name   string
		target AgentTarget
	}{
		{
			name:   "path the manifest never recorded",
			target: AgentTarget{ID: "claude", InstructionsPath: filepath.Join(home, "notes", "CLAUDE.md")},
		},
		{
			name:   "owned path claimed on behalf of an agent the manifest does not list",
			target: AgentTarget{ID: "cursor", InstructionsPath: ownedPath},
		},
		{
			name:   "no recorded instruction path at all",
			target: AgentTarget{ID: "claude"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const userContent = "# Somebody else's file\n"
			path := tt.target.InstructionsPath
			if path == "" {
				path = filepath.Join(home, "unrecorded", "CLAUDE.md")
			}
			writeFile(t, path, userContent)
			// Mode 000 turns any read attempt into a permission error, so a
			// refusal that still names the ownership rule proves the guard
			// decided before the file was opened.
			if err := os.Chmod(path, 0o000); err != nil {
				t.Fatalf("chmod %s: %v", path, err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

			writer := &countingWriter{}
			err := ApplyInstructions(own, tt.target, writer, "layer one", "layer two", replaySkills)

			if !errors.Is(err, ErrUnownedInstructionsPath) {
				t.Fatalf("error = %v, want it to wrap ErrUnownedInstructionsPath", err)
			}
			if writer.calls != 0 {
				t.Fatalf("writer was invoked %d times for an unowned path; it must not be reached at all", writer.calls)
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatalf("restore mode on %s: %v", path, err)
			}
			if got := readFileString(t, path); got != userContent {
				t.Fatalf("unowned file content = %q, want it untouched (%q)", got, userContent)
			}
		})
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
