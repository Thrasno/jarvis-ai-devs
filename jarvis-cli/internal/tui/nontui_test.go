package tui

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

// testPersonaFS and testSkillsFS embed the minimal fixture files used exclusively
// by tests in this package. They mirror the path layout expected by persona.ListPresets
// and skills.ListSkills (embed/personas/*.yaml, embed/skills/*.md).
//
//go:embed embed/personas
var testPersonaFS embed.FS

//go:embed embed/skills
var testSkillsFS embed.FS

// testWizardConfig returns a WizardConfig backed by the test fixture FSes.
func testWizardConfig() WizardConfig {
	return WizardConfig{
		PersonaFS: testPersonaFS,
		SkillsFS:  testSkillsFS,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestRunNoTUI_SkipsAuthAndDefaultsPersona
// ──────────────────────────────────────────────────────────────────────────────

// TestRunNoTUI_SkipsAuthAndDefaultsPersona runs the full no-TUI wizard with:
//   - Empty email → skips cloud auth
//   - Empty persona choice → defaults to preset 0 (fixture)
//   - Empty skill answer → declines the optional fixture-skill
//
// This exercises RunNoTUI, runNoTUI, and readLine end-to-end.
func TestRunNoTUI_SkipsAuthAndDefaultsPersona(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "") // no agents detected

	// 3 readline calls: email, persona choice, fixture-skill (optional).
	input := strings.NewReader("\n\n\n")

	err := runNoTUI(testWizardConfig(), input)
	if err != nil {
		t.Fatalf("runNoTUI: %v", err)
	}

	// config.yaml should be created under HOME.
	cfgPath := filepath.Join(tmpHome, ".jarvis", "config.yaml")
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Error("expected config.yaml to be created after wizard:", statErr)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestRunNoTUI_SelectsSkill
// ──────────────────────────────────────────────────────────────────────────────

// TestRunNoTUI_SelectsSkill verifies that answering 'y' for the optional skill
// installs it (no crash, no error).
func TestRunNoTUI_SelectsSkill(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	// email=skip, persona=default, fixture-skill=yes
	input := strings.NewReader("\n\ny\n")

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI with skill selected: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestRunAgentConfigSequence_NoAgents
// ──────────────────────────────────────────────────────────────────────────────

// TestRunAgentConfigSequence_NoAgents invokes the async agent-config Cmd directly
// (without Bubbletea runtime) to verify it completes with done=true when there
// are no agents to configure.
func TestRunAgentConfigSequence_NoAgents(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	m := Model{
		Step:     StepAgentConfig,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{APIURL: "https://hivemem.dev"},
		Agents:   nil, // no agents
	}

	cmd := runAgentConfigSequence(m)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd from runAgentConfigSequence")
	}

	// Invoke the Cmd synchronously (Bubbletea Cmds are just func() tea.Msg).
	msg := cmd()
	pr, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}
	if !pr.done {
		t.Errorf("expected done=true with no agents, got done=%v line=%q", pr.done, pr.line)
	}
	if !strings.Contains(pr.line, "No agents detected") {
		t.Errorf("unexpected summary line: %q", pr.line)
	}
}
