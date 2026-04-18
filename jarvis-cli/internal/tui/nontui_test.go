package tui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
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

func TestRunNoTUI_RerunKeepsExistingSelectionsOnBlankInput(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	seed := &config.AppConfig{
		SchemaVersion:    2,
		APIURL:           config.DefaultAPIURL,
		PersonaPreset:    "fixture",
		SelectedSkills:   []string{"fixture-skill"},
		ConfiguredAgents: []string{},
		Install: config.InstallState{
			Mode:      "reconfigure",
			Completed: true,
			Agents:    map[string]config.AgentState{},
		},
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	// email keep blank, persona keep default, extra skills keep defaults.
	input := strings.NewReader("\n\n\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI rerun: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config after rerun: %v", err)
	}
	if loaded.PersonaPreset != "fixture" {
		t.Fatalf("expected persona preset to remain fixture, got %q", loaded.PersonaPreset)
	}
	if len(loaded.SelectedSkills) != 1 || loaded.SelectedSkills[0] != "fixture-skill" {
		t.Fatalf("expected existing selected skills preserved, got %v", loaded.SelectedSkills)
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

// ──────────────────────────────────────────────────────────────────────────────
// Mock Agent for testing Context7 configuration
// ──────────────────────────────────────────────────────────────────────────────

// mockAgent is a test double that tracks MergeConfig calls.
type mockAgent struct {
	name          string
	configDir     string
	mergedEntries []agent.MCPEntry
}

func (m *mockAgent) Name() string      { return m.name }
func (m *mockAgent) IsInstalled() bool { return true }
func (m *mockAgent) ConfigDir() string { return m.configDir }

func (m *mockAgent) MergeConfig(entry agent.MCPEntry) error {
	m.mergedEntries = append(m.mergedEntries, entry)
	// Write to a test file to verify the config was written
	settingsPath := filepath.Join(m.configDir, "settings.json")
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return err
	}

	// Read existing or create new
	var settings map[string]any
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		_ = json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	// Add the entry
	mcpServers, ok := settings["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
		settings["mcpServers"] = mcpServers
	}
	mcpServers[entry.Name] = map[string]any{"configured": true}

	// Write back
	out, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(settingsPath, out, 0644)
}

func (m *mockAgent) WriteInstructions(layer1, layer2 string, skills []config.SkillInfo) error {
	return nil
}

func (m *mockAgent) InstallSkills(skillsFS fs.FS, selected []string) error {
	return nil
}

func (m *mockAgent) InstallOrchestrator(orchestratorFS fs.FS) error {
	return nil
}

func (m *mockAgent) SupportsOutputStyles() bool {
	return false
}

func (m *mockAgent) WriteOutputStyle(preset *persona.Preset) error {
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// TestRunAgentConfigSequence_Context7AfterHive
// ──────────────────────────────────────────────────────────────────────────────

// TestRunAgentConfigSequence_Context7AfterHive verifies Context7 is configured
// AFTER Hive in the wizard sequence (Spec R1).
func TestRunAgentConfigSequence_Context7AfterHive(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	mockConfigDir := filepath.Join(tmpHome, ".mock-agent")
	mock := &mockAgent{
		name:      "mock",
		configDir: mockConfigDir,
	}

	m := Model{
		Step:      StepAgentConfig,
		Selected:  make(map[string]bool),
		SkillList: []skills.Skill{},
		cfg: &config.AppConfig{
			APIURL: "https://hivemem.dev",
			Email:  "test@example.com",
		},
		Agents:    []agent.Agent{mock},
		PersonaFS: testPersonaFS,
	}

	cmd := runAgentConfigSequence(m)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd from runAgentConfigSequence")
	}

	// Execute the command synchronously
	msg := cmd()
	pr, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}

	if !pr.done {
		t.Errorf("expected done=true, got done=%v line=%q", pr.done, pr.line)
	}

	// Verify BOTH hive and context7 were configured
	if len(mock.mergedEntries) != 2 {
		t.Fatalf("expected 2 MergeConfig calls (hive + context7), got %d", len(mock.mergedEntries))
	}

	// Verify ORDER: hive first, context7 second
	if mock.mergedEntries[0].Name != "hive" {
		t.Errorf("expected first MergeConfig call to be 'hive', got %q", mock.mergedEntries[0].Name)
	}

	if mock.mergedEntries[1].Name != "context7" {
		t.Errorf("expected second MergeConfig call to be 'context7', got %q", mock.mergedEntries[1].Name)
	}

	// Verify settings.json was written with both entries
	settingsPath := filepath.Join(mockConfigDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	mcpServers := settings["mcpServers"].(map[string]any)
	if _, ok := mcpServers["hive"]; !ok {
		t.Error("hive entry missing from settings.json")
	}
	if _, ok := mcpServers["context7"]; !ok {
		t.Error("context7 entry missing from settings.json")
	}
}
