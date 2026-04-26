package tui

import (
	"bufio"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
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

	// scope, persona choice, optional skill prompt, explicit apply confirmation.
	input := strings.NewReader("\n\nyes\n")

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

	// scope=default, persona=default, fixture-skill=yes, apply=yes
	input := strings.NewReader("\n\nyes\n")

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

	// scope keep default, persona keep default, extra skills keep defaults, apply=yes.
	input := strings.NewReader("\n\nyes\n")
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

func TestRunNoTUI_CustomPresetPersistsUserFileAndCanonicalIdentity(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	// scope default, choose custom option, provide name/display, keep generated YAML,
	// default optional skills answer, apply=yes.
	input := strings.NewReader("\n2\nmi persona\nMi Persona Display\n\nyes\n")

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI custom preset: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.PersonaPreset != "mi-persona" {
		t.Fatalf("expected canonical custom slug mi-persona, got %q", loaded.PersonaPreset)
	}
	if loaded.PersonaPresetSource != string(persona.PresetSourceUser) {
		t.Fatalf("expected persona_preset_source=user, got %q", loaded.PersonaPresetSource)
	}

	customPath := filepath.Join(tmpHome, ".jarvis", "personas", "mi-persona.yaml")
	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("expected custom preset file %s, got err=%v", customPath, err)
	}
}

func TestRunNoTUI_CustomPresetInvalidYAMLBlocksContinuation(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	// scope default, choose custom option, provide name/display, invalid YAML override.
	input := strings.NewReader("\n2\nbroken persona\nBroken Persona\nname: [\n")

	err := runNoTUI(testWizardConfig(), input)
	if err == nil {
		t.Fatal("expected error when custom YAML is invalid")
	}

	customPath := filepath.Join(tmpHome, ".jarvis", "personas", "broken-persona.yaml")
	if _, statErr := os.Stat(customPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected invalid custom preset not to be persisted, got err=%v", statErr)
	}
}

func TestBuildSkillSelectionPlan_PHPPromptControlsPHPSkills(t *testing.T) {
	skillList := []skills.Skill{
		{ID: "phpunit-testing", Name: "PHPUnit Testing", IsCore: false},
		{ID: "laravel-architecture", Name: "Laravel Architecture", IsCore: false},
	}

	plan := buildSkillSelectionPlan(skillList, []string{"phpunit-testing", "laravel-architecture"})
	if len(plan.Prompts) != 1 {
		t.Fatalf("expected one PHP prompt, got %d", len(plan.Prompts))
	}
	if plan.Prompts[0].Label != "PHP" {
		t.Fatalf("expected PHP label, got %q", plan.Prompts[0].Label)
	}

	for _, id := range []string{"phpunit-testing", "laravel-architecture"} {
		if !plan.Selected[id] {
			t.Fatalf("expected %s selected from existing config", id)
		}
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

func (m *mockAgent) ClearOutputStyle(name string) error {
	return nil
}

type failingMockAgent struct{ mockAgent }

func (m *failingMockAgent) MergeConfig(entry agent.MCPEntry) error {
	return errors.New("boom merge")
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

func TestRunNoTUI_LocalOnlyPurgesStoredCredentialsOnApply(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(`{"email":"old@example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}

	seed := &config.AppConfig{Scope: config.ScopeLocalCloud, Cloud: &config.CloudConfig{Email: "old@example.com", SyncConfigured: true}, Email: "old@example.com", APIURL: config.DefaultAPIURL}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	// scope=local-only, persona default, skill default, apply=yes.
	input := strings.NewReader("local-only\n\nyes\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI local-only: %v", err)
	}

	if _, err := os.Stat(filepath.Join(jarvisDir, "sync.json")); !os.IsNotExist(err) {
		t.Fatalf("expected sync.json removed in local-only apply, got err=%v", err)
	}
}

func TestRunNoTUI_CancelBeforeApplyKeepsNoLocalArtifacts(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	// scope local-only, persona default, optional skill prompts default, apply=no.
	input := strings.NewReader("local-only\n\nno\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI cancel review: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpHome, ".jarvis", "memory.db")); !os.IsNotExist(err) {
		t.Fatalf("expected no memory.db when canceling before apply, got err=%v", err)
	}
}

func TestRunNoTUI_LocalCloudAuthFailureContinuesToApply(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad creds"})
	}))
	defer server.Close()

	seed := &config.AppConfig{APIURL: server.URL, Scope: config.ScopeLocalOnly}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	// scope local+cloud + credentials (auth fails), then persona default, skill default, apply yes.
	input := strings.NewReader("local+cloud\nuser@example.com\nwrong-password\n\nyes\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI should continue on auth failure: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpHome, ".jarvis", "config.yaml")); err != nil {
		t.Fatalf("expected config.yaml persisted even on auth failure: %v", err)
	}
}

func TestReadLine_ReturnsEmptyWhenScannerExhausted(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))
	if got := readLine(scanner); got != "" {
		t.Fatalf("expected empty string on exhausted scanner, got %q", got)
	}
}

func TestRunNoTUI_UsesStdinWrapper(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	if _, err := w.WriteString("\n\nyes\n"); err != nil {
		t.Fatalf("write stdin fixture: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close write pipe: %v", err)
	}

	if err := RunNoTUI(testWizardConfig()); err != nil {
		t.Fatalf("RunNoTUI wrapper: %v", err)
	}
}

func TestRunNoTUI_LocalCloudSuccessfulAuthWritesSyncJSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "jwt-token",
			"user":  map[string]string{"email": "resolved@example.com"},
		})
	}))
	defer server.Close()

	seed := &config.AppConfig{APIURL: server.URL, Scope: config.ScopeLocalOnly}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	input := strings.NewReader("local+cloud\nuser@example.com\nsecret\n\nyes\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI local+cloud success: %v", err)
	}

	syncPath := filepath.Join(tmpHome, ".jarvis", "sync.json")
	if _, err := os.Stat(syncPath); err != nil {
		t.Fatalf("expected sync.json written on local+cloud success, got %v", err)
	}
}

func TestRunNoTUI_AgentConfigurationFailureReturnsError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	originalDetect := detectInstalledAgents
	detectInstalledAgents = func(fsys fs.FS) []agent.Agent {
		return []agent.Agent{&failingMockAgent{mockAgent{name: "failing-agent", configDir: filepath.Join(tmpHome, ".mock")}}}
	}
	t.Cleanup(func() { detectInstalledAgents = originalDetect })

	err := runNoTUI(testWizardConfig(), strings.NewReader("\n\nyes\n"))
	if err == nil {
		t.Fatal("expected runNoTUI to return configuration error")
	}
	if !strings.Contains(err.Error(), "configure failing-agent") {
		t.Fatalf("expected wrapped configure error, got %v", err)
	}
}

func TestRunNoTUI_LocalCloudLoginWithoutResolvedEmailFallsBackToInput(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "jwt-token",
			"user":  map[string]string{"email": ""},
		})
	}))
	defer server.Close()

	seed := &config.AppConfig{APIURL: server.URL, Scope: config.ScopeLocalOnly}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	input := strings.NewReader("local+cloud\ninput@example.com\nsecret\n\nyes\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI local+cloud blank resolved email: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if loaded.Email != "input@example.com" {
		t.Fatalf("expected fallback to entered email, got %q", loaded.Email)
	}
}

func TestRunNoTUI_LoadConfigError(t *testing.T) {
	originalLoad := loadAppConfig
	loadAppConfig = func() (*config.AppConfig, error) {
		return nil, errors.New("boom load")
	}
	t.Cleanup(func() { loadAppConfig = originalLoad })

	err := runNoTUI(testWizardConfig(), strings.NewReader(""))
	if err == nil {
		t.Fatal("expected load config error")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("expected wrapped load config error, got %v", err)
	}
}

func TestRunNoTUI_ListPresetsError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	originalList := listPersonaPresets
	listPersonaPresets = func(fsys embed.FS) ([]persona.Preset, error) {
		return nil, errors.New("preset list failed")
	}
	t.Cleanup(func() { listPersonaPresets = originalList })

	err := runNoTUI(testWizardConfig(), strings.NewReader("\n"))
	if err == nil {
		t.Fatal("expected list presets error")
	}
	if !strings.Contains(err.Error(), "list presets") {
		t.Fatalf("expected wrapped list presets error, got %v", err)
	}
}

func TestRunNoTUI_ListSkillsError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("PATH", "")

	originalList := listAvailableSkills
	listAvailableSkills = func(fsys embed.FS) ([]skills.Skill, error) {
		return nil, errors.New("skills list failed")
	}
	t.Cleanup(func() { listAvailableSkills = originalList })

	err := runNoTUI(testWizardConfig(), strings.NewReader("\n\n"))
	if err == nil {
		t.Fatal("expected list skills error")
	}
	if !strings.Contains(err.Error(), "list skills") {
		t.Fatalf("expected wrapped list skills error, got %v", err)
	}
}
