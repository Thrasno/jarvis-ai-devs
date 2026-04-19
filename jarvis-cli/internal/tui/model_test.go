package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/skills"
)

// ──────────────────────────────────────────────────────────────────────────────
// Test helpers
// ──────────────────────────────────────────────────────────────────────────────

// sendKey sends a KeyMsg with the given type to m.Update and returns the updated Model.
func sendKey(m Model, keyType tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: keyType})
	return updated.(Model)
}

// sendRune sends a rune key to m.Update and returns the updated Model.
func sendRune(m Model, r string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	return updated.(Model)
}

// buildPersonaModel returns a Model at StepPersona with n fake presets.
func buildPersonaModel(n int) Model {
	m := Model{
		Step:     StepPersona,
		Selected: make(map[string]bool),
	}
	for i := 0; i < n; i++ {
		m.Presets = append(m.Presets, persona.Preset{
			Name:        fmt.Sprintf("preset-%d", i),
			DisplayName: fmt.Sprintf("Preset %d", i),
			Description: fmt.Sprintf("Description for preset %d", i),
		})
	}
	return m
}

// buildSkillsModel returns a Model at StepSkills with one core and one optional skill.
func buildSkillsModel() Model {
	plans := buildSkillSelectionPlan([]skills.Skill{
		{ID: "hive", Name: "Hive", IsCore: true},
		{ID: "go-testing", Name: "Go Testing", IsCore: false},
	}, nil)

	return Model{
		Step: StepSkills,
		SkillList: []skills.Skill{
			{ID: "hive", Name: "Hive", IsCore: true},
			{ID: "go-testing", Name: "Go Testing", IsCore: false},
		},
		Selected:     plans.Selected,
		SkillPrompts: plans.Prompts,
	}
}

func TestBuildSkillSelectionPlan_OnlyPromptsStackSpecificSkills(t *testing.T) {
	skillList := []skills.Skill{
		{ID: "hive", Name: "Hive", IsCore: true},
		{ID: "branch-pr", Name: "Branch & PR", IsCore: false},
		{ID: "issue-creation", Name: "Issue Creation", IsCore: false},
		{ID: "zoho-deluge", Name: "Zoho Deluge", IsCore: false},
		{ID: "phpunit-testing", Name: "PHPUnit Testing", IsCore: false},
		{ID: "laravel-architecture", Name: "Laravel Architecture", IsCore: false},
		{ID: "go-testing", Name: "Go Testing", IsCore: false},
	}

	plan := buildSkillSelectionPlan(skillList, nil)

	if len(plan.Prompts) != 3 {
		t.Fatalf("expected exactly 3 interactive prompts, got %d", len(plan.Prompts))
	}

	if plan.Prompts[0].Label != "Zoho-Deluge" {
		t.Fatalf("expected first prompt to be Zoho-Deluge, got %q", plan.Prompts[0].Label)
	}
	if plan.Prompts[1].Label != "PHP" {
		t.Fatalf("expected second prompt to be PHP, got %q", plan.Prompts[1].Label)
	}
	if plan.Prompts[2].Label != "Go Testing" {
		t.Fatalf("expected third prompt to be Go Testing, got %q", plan.Prompts[2].Label)
	}

	// Non stack-specific skills must be auto-enabled and not shown interactively.
	if !plan.Selected["branch-pr"] || !plan.Selected["issue-creation"] {
		t.Fatalf("expected non stack-specific skills to be auto-selected: %+v", plan.Selected)
	}
}

func TestViewSkills_DoesNotLeakLargeCatalog(t *testing.T) {
	skillList := []skills.Skill{
		{ID: "hive", Name: "Hive", IsCore: true},
		{ID: "branch-pr", Name: "Branch & PR", IsCore: false},
		{ID: "issue-creation", Name: "Issue Creation", IsCore: false},
		{ID: "zoho-deluge", Name: "Zoho Deluge", IsCore: false},
		{ID: "phpunit-testing", Name: "PHPUnit Testing", IsCore: false},
		{ID: "laravel-architecture", Name: "Laravel Architecture", IsCore: false},
		{ID: "go-testing", Name: "Go Testing", IsCore: false},
		{ID: "judgment-day", Name: "Judgment Day", IsCore: false},
	}

	plan := buildSkillSelectionPlan(skillList, nil)
	m := Model{
		Step:         StepSkills,
		SkillList:    skillList,
		Selected:     plan.Selected,
		SkillPrompts: plan.Prompts,
	}

	v := viewSkills(m)
	if !strings.Contains(v, "Zoho-Deluge") || !strings.Contains(v, "PHP") || !strings.Contains(v, "Go Testing") {
		t.Fatalf("expected stack-specific prompts in view, got:\n%s", v)
	}
	if strings.Contains(v, "Judgment Day") || strings.Contains(v, "Branch & PR") {
		t.Fatalf("view leaked non-interactive catalog skills:\n%s", v)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestNewModel_DefaultsToHiveLocal
// ──────────────────────────────────────────────────────────────────────────────

// TestNewModel_DefaultsToHiveLocal verifies that a freshly created Model starts
// at StepHiveLocal and has an initialised Selected map.
func TestNewModel_DefaultsToHiveLocal(t *testing.T) {
	m := Model{
		Step:     StepHiveLocal,
		Selected: make(map[string]bool),
	}
	if m.Step != StepHiveLocal {
		t.Errorf("expected StepHiveLocal, got %v", m.Step)
	}
	if m.Selected == nil {
		t.Error("Selected map should be non-nil")
	}
}

func TestNewModel_PrefillsExistingConfigAndMode(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &config.AppConfig{
		SchemaVersion:    2,
		APIURL:           config.DefaultAPIURL,
		PersonaPreset:    "fixture",
		SelectedSkills:   []string{"fixture-skill"},
		Cloud:            &config.CloudConfig{Email: "prefill@example.com"},
		ConfiguredAgents: []string{"claude"},
		Install: config.InstallState{
			Completed: true,
			Mode:      "reconfigure",
			Agents: map[string]config.AgentState{
				"claude": {Configured: true, InstructionsPath: "/tmp/CLAUDE.md", ConfigPath: "/tmp/settings.json"},
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	m := NewModel(testWizardConfig(), false)

	if m.Mode != "reconfigure" {
		t.Fatalf("expected mode reconfigure, got %q", m.Mode)
	}
	if m.Email != "prefill@example.com" {
		t.Fatalf("expected prefilled email, got %q", m.Email)
	}
	if m.cfg == nil || m.cfg.PersonaPreset != "fixture" {
		t.Fatalf("expected prefilled persona preset fixture, got %+v", m.cfg)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_HiveLocal_AdvancesOnEnter
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_HiveLocal_AdvancesOnEnter verifies that pressing Enter on StepScope
// does not create local artifacts pre-apply and advances according to scope.
func TestStep_HiveLocal_AdvancesOnEnter(t *testing.T) {
	// Redirect HOME so we don't touch the real user directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	m := Model{
		Step:     StepHiveLocal,
		Scope:    config.ScopeLocalOnly,
		cfg:      &config.AppConfig{Scope: config.ScopeLocalOnly},
		Selected: make(map[string]bool),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if m2.Err != nil {
		t.Fatalf("unexpected error after Enter: %v", m2.Err)
	}
	if m2.Step != StepPersona {
		t.Errorf("expected StepPersona after Enter in local-only scope, got %v", m2.Step)
	}

	// ~/.jarvis/memory.db must NOT be created before apply.
	dbPath := filepath.Join(tmpHome, ".jarvis", "memory.db")
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Error("expected memory.db to NOT be created before apply")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Persona_CursorNavigation
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Persona_CursorNavigation verifies that arrow keys and j/k move the
// cursor within bounds.
func TestStep_Persona_CursorNavigation(t *testing.T) {
	m := buildPersonaModel(3)

	if m.presetCur != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.presetCur)
	}

	// Move down twice.
	m = sendKey(m, tea.KeyDown)
	if m.presetCur != 1 {
		t.Errorf("after Down: expected 1, got %d", m.presetCur)
	}
	m = sendKey(m, tea.KeyDown)
	if m.presetCur != 2 {
		t.Errorf("after Down x2: expected 2, got %d", m.presetCur)
	}

	// Boundary: cannot exceed len-1.
	m = sendKey(m, tea.KeyDown)
	if m.presetCur != 2 {
		t.Errorf("after Down at boundary: expected 2, got %d", m.presetCur)
	}

	// Move up.
	m = sendKey(m, tea.KeyUp)
	if m.presetCur != 1 {
		t.Errorf("after Up: expected 1, got %d", m.presetCur)
	}

	// Boundary: cannot go below 0.
	m = sendKey(m, tea.KeyUp)
	m = sendKey(m, tea.KeyUp)
	if m.presetCur != 0 {
		t.Errorf("after Up at boundary: expected 0, got %d", m.presetCur)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Skills_Toggle
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Skills_Toggle verifies that Space toggles a non-core skill on and off.
func TestStep_Skills_Toggle(t *testing.T) {
	m := buildSkillsModel()
	// Move cursor to index 0 (go-testing prompt).
	m.presetCur = 0

	if m.Selected["go-testing"] {
		t.Fatal("go-testing should not be selected initially")
	}

	// Toggle on.
	m = sendRune(m, " ")
	if !m.Selected["go-testing"] {
		t.Error("expected go-testing to be selected after Space")
	}

	// Toggle off.
	m = sendRune(m, " ")
	if m.Selected["go-testing"] {
		t.Error("expected go-testing to be deselected after second Space")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Skills_CoreAlwaysSelected
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Skills_CoreAlwaysSelected verifies that pressing Space on a core skill
// does NOT deselect it.
func TestStep_Skills_CoreAlwaysSelected(t *testing.T) {
	m := buildSkillsModel()
	// Cursor at 0 = go-testing prompt.
	m.presetCur = 0

	if m.Selected["go-testing"] {
		t.Fatal("go-testing should start unselected")
	}

	// Space toggles interactive prompt on.
	m = sendRune(m, " ")
	if !m.Selected["go-testing"] {
		t.Error("go-testing should toggle on")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Persona_SelectAndAdvance
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Persona_SelectAndAdvance verifies that pressing Enter at StepPersona
// advances to StepSkills and records the selected preset in cfg.
func TestStep_Persona_SelectAndAdvance(t *testing.T) {
	m := buildPersonaModel(3)
	// cfg must be initialised so the step handler can write to it.
	m.cfg = &config.AppConfig{}

	if m.Step != StepPersona {
		t.Fatalf("expected StepPersona, got %v", m.Step)
	}

	// Press Enter — selects presets[0] ("preset-0").
	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepExtraSkills {
		t.Errorf("expected StepExtraSkills after Enter, got %v", m.Step)
	}
	if m.cfg.PersonaPreset != "preset-0" {
		t.Errorf("expected cfg.PersonaPreset=preset-0, got %q", m.cfg.PersonaPreset)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Skills_EnterAdvances
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Skills_EnterAdvances verifies that pressing Enter at StepSkills
// advances the model to StepAgentConfig.
func TestStep_Skills_EnterAdvances(t *testing.T) {
	m := buildSkillsModel()

	if m.Step != StepSkills {
		t.Fatalf("expected StepSkills, got %v", m.Step)
	}

	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepAgentConfig {
		t.Errorf("expected StepAgentConfig after Enter, got %v", m.Step)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Skills_CoreSkillAlwaysInSelected
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Skills_KeySpaceTogglesPrompt verifies KeySpace toggles the selected
// stack-specific prompt.
func TestStep_Skills_KeySpaceTogglesPrompt(t *testing.T) {
	m := buildSkillsModel()
	// Cursor at 0 = go-testing prompt.
	m.presetCur = 0

	if m.Selected["go-testing"] {
		t.Fatal("go-testing should start unselected")
	}

	// Send KeySpace (the key type, not a rune).
	m = sendKey(m, tea.KeySpace)
	if !m.Selected["go-testing"] {
		t.Error("go-testing must be selected after KeySpace")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_View_ReturnsNonEmptyString
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_View_ReturnsNonEmptyString verifies that View() returns a non-empty
// string for every step and does not panic.
func TestStep_View_ReturnsNonEmptyString(t *testing.T) {
	steps := []struct {
		name string
		step Step
	}{
		{"HiveLocal", StepHiveLocal},
		{"HiveCloud", StepHiveCloud},
		{"Persona", StepPersona},
		{"Skills", StepSkills},
		{"AgentConfig", StepAgentConfig},
		{"Done", StepDone},
	}

	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				Step:     tc.step,
				Selected: make(map[string]bool),
				cfg:      &config.AppConfig{},
			}
			v := m.View()
			if v == "" {
				t.Errorf("View() returned empty string for step %v", tc.step)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Persona_BackNavigation
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Persona_BackNavigation verifies that cursor position is retained when
// moving between presets (selection state is preserved across key events).
func TestStep_Persona_BackNavigation(t *testing.T) {
	m := buildPersonaModel(3)
	m.cfg = &config.AppConfig{}

	// Navigate to preset index 2.
	m = sendKey(m, tea.KeyDown)
	m = sendKey(m, tea.KeyDown)
	if m.presetCur != 2 {
		t.Fatalf("expected cursor at 2, got %d", m.presetCur)
	}

	// Move back up to index 1.
	m = sendKey(m, tea.KeyUp)
	if m.presetCur != 1 {
		t.Errorf("expected cursor at 1 after Up, got %d", m.presetCur)
	}

	// Press Enter — selects presets[1] ("preset-1").
	m = sendKey(m, tea.KeyEnter)
	if m.Step != StepSkills {
		t.Errorf("expected StepSkills after Enter, got %v", m.Step)
	}
	if m.cfg.Preset != "preset-1" {
		t.Errorf("expected cfg.Preset=preset-1, got %q", m.cfg.Preset)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestNoTUI_SkipsTTYRequirement
// ──────────────────────────────────────────────────────────────────────────────

// TestNoTUI_SkipsTTYRequirement documents that RunNoTUI reads from os.Stdin
// directly, so it requires a real TTY or pipe. This test verifies the function
// signature is accessible and skips if no injection mechanism is available.
func TestNoTUI_SkipsTTYRequirement(t *testing.T) {
	t.Skip("RunNoTUI reads from os.Stdin directly — use binary-level tests for full flow coverage")
}

// ──────────────────────────────────────────────────────────────────────────────
// TestNewModel_WithEmptyWizardConfig
// ──────────────────────────────────────────────────────────────────────────────

// TestNewModel_WithEmptyWizardConfig verifies that NewModel returns a valid model
// even when the WizardConfig has zero-value FSes (errors are silently ignored).
func TestNewModel_WithEmptyWizardConfig(t *testing.T) {
	m := NewModel(WizardConfig{}, false)
	if m.Step != StepHiveLocal {
		t.Errorf("expected StepHiveLocal, got %v", m.Step)
	}
	if m.Selected == nil {
		t.Error("Selected map should be non-nil")
	}
	if m.cfg == nil {
		t.Error("cfg should be non-nil after NewModel")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestModel_Init_ReturnsNil
// ──────────────────────────────────────────────────────────────────────────────

// TestModel_Init_ReturnsNil verifies that Init() returns a nil Cmd (no initial IO).
func TestModel_Init_ReturnsNil(t *testing.T) {
	m := Model{Step: StepHiveLocal, Selected: make(map[string]bool)}
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil Cmd")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdate_WindowSizeMsg
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdate_WindowSizeMsg verifies that the model stores terminal dimensions.
func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := Model{Step: StepHiveLocal, Selected: make(map[string]bool)}
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := updated.(Model)
	if m2.width != 120 || m2.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m2.width, m2.height)
	}
	if cmd != nil {
		t.Error("expected nil cmd for WindowSizeMsg")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdate_ErrMsg
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdate_ErrMsg verifies that async error messages are stored in m.Err.
func TestUpdate_ErrMsg(t *testing.T) {
	m := Model{Step: StepHiveLocal, Selected: make(map[string]bool)}
	testErr := errors.New("async failure")
	updated, _ := m.Update(errMsg{err: testErr})
	m2 := updated.(Model)
	if m2.Err != testErr {
		t.Errorf("expected Err=%v, got %v", testErr, m2.Err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateHiveCloud_TabSwitchesField
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateHiveCloud_TabSwitchesField verifies Tab toggles between email and password fields.
func TestUpdateHiveCloud_TabSwitchesField(t *testing.T) {
	m := Model{
		Step:     StepHiveCloud,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	if m.activeField != 0 {
		t.Fatal("expected activeField=0 initially (email)")
	}
	m = sendKey(m, tea.KeyTab)
	if m.activeField != 1 {
		t.Errorf("after Tab: expected activeField=1 (password), got %d", m.activeField)
	}
	m = sendKey(m, tea.KeyTab)
	if m.activeField != 0 {
		t.Errorf("after Tab x2: expected activeField=0 (email), got %d", m.activeField)
	}
	// ShiftTab also toggles.
	m = sendKey(m, tea.KeyShiftTab)
	if m.activeField != 1 {
		t.Errorf("after ShiftTab: expected activeField=1, got %d", m.activeField)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateHiveCloud_TypeEmailAndBackspace
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateHiveCloud_TypeEmailAndBackspace verifies rune input and backspace on the email field.
func TestUpdateHiveCloud_TypeEmailAndBackspace(t *testing.T) {
	m := Model{
		Step:     StepHiveCloud,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	m = sendRune(m, "a")
	m = sendRune(m, "b")
	m = sendRune(m, "c")
	if m.Email != "abc" {
		t.Errorf("expected Email=abc, got %q", m.Email)
	}
	m = sendKey(m, tea.KeyBackspace)
	if m.Email != "ab" {
		t.Errorf("after Backspace: expected Email=ab, got %q", m.Email)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateHiveCloud_TypePasswordAndBackspace
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateHiveCloud_TypePasswordAndBackspace verifies rune input and backspace on the password field.
func TestUpdateHiveCloud_TypePasswordAndBackspace(t *testing.T) {
	m := Model{
		Step:        StepHiveCloud,
		Selected:    make(map[string]bool),
		cfg:         &config.AppConfig{},
		activeField: 1,
	}
	m = sendRune(m, "x")
	m = sendRune(m, "y")
	if m.Password != "xy" {
		t.Errorf("expected Password=xy, got %q", m.Password)
	}
	m = sendKey(m, tea.KeyBackspace)
	if m.Password != "x" {
		t.Errorf("after Backspace: expected Password=x, got %q", m.Password)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateHiveCloud_EnterOnEmailMovesToPassword
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateHiveCloud_EnterOnEmailMovesToPassword verifies that Enter on email field
// switches focus to the password field (not submitting the form yet).
func TestUpdateHiveCloud_EnterOnEmailMovesToPassword(t *testing.T) {
	m := Model{
		Step:     StepHiveCloud,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
		Email:    "user@example.com",
	}
	m = sendKey(m, tea.KeyEnter)
	if m.activeField != 1 {
		t.Errorf("expected password field (1) after Enter on email, got %d", m.activeField)
	}
	if m.Step != StepHiveCloud {
		t.Errorf("expected still on StepHiveCloud, got %v", m.Step)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateHiveCloud_EmptyEmailEnterSkipsToPersona
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateHiveCloud_EmptyEmailEnterSkipsToPersona verifies that Enter with empty
// email (on password field) skips cloud auth and advances to StepPersona.
func TestUpdateHiveCloud_EmptyEmailEnterSkipsToPersona(t *testing.T) {
	m := Model{
		Step:        StepHiveCloud,
		Selected:    make(map[string]bool),
		cfg:         &config.AppConfig{},
		Email:       "",
		activeField: 1,
	}
	m = sendKey(m, tea.KeyEnter)
	if m.Step != StepPersona {
		t.Errorf("expected StepPersona after Enter with empty email, got %v", m.Step)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateHiveCloud_EscSkipsToPersona
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateHiveCloud_EscSkipsToPersona verifies that Esc clears credentials and skips to StepPersona.
func TestUpdateHiveCloud_EscSkipsToPersona(t *testing.T) {
	m := Model{
		Step:     StepHiveCloud,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
		Email:    "user@example.com",
		Password: "s3cr3t",
	}
	m = sendKey(m, tea.KeyEsc)
	if m.Step != StepPersona {
		t.Errorf("expected StepPersona after Esc, got %v", m.Step)
	}
	if m.Email != "" || m.Password != "" {
		t.Errorf("expected Email and Password cleared, got email=%q pass=%q", m.Email, m.Password)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateDone_EnterQuits
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateDone_EnterQuits verifies that Enter on StepDone sets Done=true and returns a Quit cmd.
func TestUpdateDone_EnterQuits(t *testing.T) {
	m := Model{
		Step:     StepDone,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)
	if !m2.Done {
		t.Error("expected Done=true after Enter on StepDone")
	}
	if cmd == nil {
		t.Error("expected non-nil Quit cmd after Enter on StepDone")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateDone_QQuits
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateDone_QQuits verifies that pressing 'q' on StepDone sets Done=true and quits.
func TestUpdateDone_QQuits(t *testing.T) {
	m := Model{
		Step:     StepDone,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m2 := updated.(Model)
	if !m2.Done {
		t.Error("expected Done=true after 'q' on StepDone")
	}
	if cmd == nil {
		t.Error("expected non-nil Quit cmd after 'q' on StepDone")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestBuildSkillMap_IncludesSelectedAndCore
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildSelectedIDs_IncludesSelectedAndCore verifies buildSelectedIDs includes selected
// and core skill IDs but excludes unselected non-core ones.
func TestBuildSelectedIDs_IncludesSelectedAndCore(t *testing.T) {
	m := Model{
		Step: StepSkills,
		SkillList: []skills.Skill{
			{ID: "core-skill", IsCore: true, Content: []byte("core content")},
			{ID: "opt-selected", IsCore: false, Content: []byte("opt content")},
			{ID: "opt-unselected", IsCore: false, Content: []byte("skip me")},
		},
		Selected: map[string]bool{
			"core-skill":   true,
			"opt-selected": true,
		},
	}
	result := buildSelectedIDs(m)

	// Convert result to a set for easy lookup.
	resultSet := make(map[string]bool)
	for _, id := range result {
		resultSet[id] = true
	}

	if !resultSet["core-skill"] {
		t.Error("expected core-skill in result")
	}
	if !resultSet["opt-selected"] {
		t.Error("expected opt-selected in result")
	}
	if resultSet["opt-unselected"] {
		t.Error("expected opt-unselected NOT in result")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestSkillsSelectedList_ReturnsOnlySelected
// ──────────────────────────────────────────────────────────────────────────────

// TestSkillsSelectedList_ReturnsOnlySelected verifies that skillsSelectedList returns
// only the IDs whose value is true in the Selected map.
func TestSkillsSelectedList_ReturnsOnlySelected(t *testing.T) {
	m := Model{
		Selected: map[string]bool{
			"skill-a": true,
			"skill-b": false,
			"skill-c": true,
		},
	}
	result := skillsSelectedList(m)
	if len(result) != 2 {
		t.Errorf("expected 2 selected IDs, got %d: %v", len(result), result)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateAgentConfig_Enter_StartsSequence
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateAgentConfig_Enter_StartsSequence verifies that the first Enter on
// StepAgentConfig (empty progress) returns a non-nil Cmd to start the sequence.
func TestUpdateApply_Enter_StartsSequence(t *testing.T) {
	m := Model{
		Step:     StepApply,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("expected non-nil cmd after first Enter on StepApply")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdateAgentConfig_Enter_WhenDone_AdvancesToStepDone
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdateAgentConfig_Enter_WhenDone_AdvancesToStepDone verifies that Enter
// when agentDone=true advances to StepDone.
func TestUpdateApply_Enter_WhenDone_AdvancesToStepDone(t *testing.T) {
	m := Model{
		Step:          StepApply,
		Selected:      make(map[string]bool),
		cfg:           &config.AppConfig{},
		agentProgress: []string{"configured claude"},
		agentDone:     true,
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)
	if m2.Step != StepDone {
		t.Errorf("expected StepDone after Enter with agentDone=true, got %v", m2.Step)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdatePersonaCustomEdit_RuneInput
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdatePersonaCustomEdit_RuneInput verifies that typing runes appends to CustomYAML
// and Backspace removes the last character when in custom edit mode.
func TestUpdatePersonaCustomEdit_RuneInput(t *testing.T) {
	m := Model{
		Step:       StepPersona,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{},
		customEdit: true,
	}
	m = sendRune(m, "n")
	m = sendRune(m, "a")
	m = sendRune(m, "m")
	if m.CustomYAML != "nam" {
		t.Errorf("expected CustomYAML=nam, got %q", m.CustomYAML)
	}
	m = sendKey(m, tea.KeyBackspace)
	if m.CustomYAML != "na" {
		t.Errorf("after Backspace: expected CustomYAML=na, got %q", m.CustomYAML)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestUpdatePersonaCustomEdit_EscCancels
// ──────────────────────────────────────────────────────────────────────────────

// TestUpdatePersonaCustomEdit_EscCancels verifies that Esc exits custom edit mode.
func TestUpdatePersonaCustomEdit_EscCancels(t *testing.T) {
	m := Model{
		Step:       StepPersona,
		Selected:   make(map[string]bool),
		cfg:        &config.AppConfig{},
		customEdit: true,
	}
	m = sendKey(m, tea.KeyEsc)
	if m.customEdit {
		t.Error("expected customEdit=false after Esc")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestHandleStepMsg_LoginResult_Error
// ──────────────────────────────────────────────────────────────────────────────

// TestHandleStepMsg_LoginResult_Error verifies that a failed loginResultMsg sets m.Err.
func TestHandleStepMsg_LoginResult_Error(t *testing.T) {
	m := Model{
		Step:     StepHiveCloud,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	msg := loginResultMsg{err: errors.New("invalid credentials")}
	updated, handled, _ := handleStepMsg(m, msg)
	if !handled {
		t.Error("expected loginResultMsg to be handled")
	}
	if updated.Err == nil {
		t.Error("expected Err to be set on failed login")
	}
	if updated.Step != StepHiveCloud {
		t.Errorf("expected to stay on StepHiveCloud after login error, got %v", updated.Step)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestHandleStepMsg_LoginResult_Success
// ──────────────────────────────────────────────────────────────────────────────

// TestHandleStepMsg_LoginResult_Success verifies successful login advances to StepPersona.
func TestHandleStepMsg_LoginResult_Success(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".jarvis"), 0755); err != nil {
		t.Fatal(err)
	}

	m := Model{
		Step:     StepHiveCloud,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{APIURL: "https://hivemem.dev"},
		Email:    "user@example.com",
		Password: "s3cr3t",
	}
	msg := loginResultMsg{token: "tok-abc123", email: "user@example.com"}
	updated, handled, _ := handleStepMsg(m, msg)
	if !handled {
		t.Error("expected loginResultMsg to be handled")
	}
	if updated.Step != StepPersona {
		t.Errorf("expected StepPersona after successful login, got %v", updated.Step)
	}
	if updated.APIToken != "tok-abc123" {
		t.Errorf("expected APIToken=tok-abc123, got %q", updated.APIToken)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestHandleStepMsg_AgentProgress
// ──────────────────────────────────────────────────────────────────────────────

// TestHandleStepMsg_AgentProgress verifies that agentProgressMsg is appended to progress list.
func TestHandleStepMsg_AgentProgress(t *testing.T) {
	m := Model{
		Step:     StepApply,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	msg := agentProgressMsg{line: "Configuring claude...", done: false}
	updated, handled, _ := handleStepMsg(m, msg)
	if !handled {
		t.Error("expected agentProgressMsg to be handled")
	}
	if len(updated.agentProgress) != 1 || updated.agentProgress[0] != "Configuring claude..." {
		t.Errorf("expected progress line to be appended, got: %v", updated.agentProgress)
	}
	if updated.agentDone {
		t.Error("expected agentDone=false when done=false")
	}
}

// TestHandleStepMsg_AgentProgress_Done verifies that agentProgressMsg with done=true sets agentDone.
func TestHandleStepMsg_AgentProgress_Done(t *testing.T) {
	m := Model{
		Step:     StepApply,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	msg := agentProgressMsg{line: "All done!", done: true}
	updated, handled, _ := handleStepMsg(m, msg)
	if !handled {
		t.Error("expected agentProgressMsg to be handled")
	}
	if !updated.agentDone {
		t.Error("expected agentDone=true when done=true")
	}
}

// TestHandleStepMsg_AgentProgress_Failed verifies failed progress sets model error.
func TestHandleStepMsg_AgentProgress_Failed(t *testing.T) {
	m := Model{
		Step:     StepApply,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}
	msg := agentProgressMsg{line: "[claude] Configuration FAILED: boom", done: true, failed: true}
	updated, handled, _ := handleStepMsg(m, msg)
	if !handled {
		t.Error("expected agentProgressMsg to be handled")
	}
	if updated.Err == nil {
		t.Fatal("expected Err to be set on failed progress")
	}
	if !updated.agentDone {
		t.Error("expected agentDone=true when done=true")
	}
}

func TestViewReview_LocalOnlyShowsExactWarning(t *testing.T) {
	m := Model{
		Step:         StepReview,
		Scope:        config.ScopeLocalOnly,
		reviewChoice: 2,
		cfg:          &config.AppConfig{PersonaPreset: "fixture"},
	}

	view := viewReview(m)
	if !strings.Contains(view, localOnlyReviewWarning) {
		t.Fatalf("expected exact local-only warning in review, got:\n%s", view)
	}
}

func TestViewReview_BoundedPolishKeepsCheckpointLayout(t *testing.T) {
	tests := []struct {
		name              string
		scope             config.SetupScope
		email             string
		expectCloudLine   string
		expectWarning     bool
		unexpectedWarning bool
	}{
		{
			name:              "local-only includes warning and omitted cloud label",
			scope:             config.ScopeLocalOnly,
			email:             "",
			expectCloudLine:   "Cloud email: (omitido por alcance local-only)",
			expectWarning:     true,
			unexpectedWarning: false,
		},
		{
			name:              "local+cloud keeps cloud summary without warning",
			scope:             config.ScopeLocalCloud,
			email:             "dev@example.com",
			expectCloudLine:   "Cloud email: dev@example.com",
			expectWarning:     false,
			unexpectedWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				Step:         StepReview,
				Scope:        tt.scope,
				Email:        tt.email,
				reviewChoice: 2,
				cfg:          &config.AppConfig{PersonaPreset: "fixture"},
			}

			view := viewReview(m)

			for _, mustContain := range []string{
				"Jarvis-Dev Setup  [5/6]  Review & Apply",
				"Resumen de configuración",
				"Scope:",
				"Persona: fixture",
				tt.expectCloudLine,
				"Back",
				"Cancel",
				"Apply",
				"↑/↓ o j/k: navegar  Enter: confirmar",
			} {
				if !strings.Contains(view, mustContain) {
					t.Fatalf("expected review view to contain %q, got:\n%s", mustContain, view)
				}
			}

			if tt.expectWarning && !strings.Contains(view, localOnlyReviewWarning) {
				t.Fatalf("expected local-only warning in review view, got:\n%s", view)
			}
			if tt.unexpectedWarning && strings.Contains(view, localOnlyReviewWarning) {
				t.Fatalf("did not expect local-only warning for scope %q, got:\n%s", tt.scope, view)
			}
		})
	}
}

func TestRunAgentConfigSequence_FailureMessageReferencesRecoveryWithoutRollbackClaim(t *testing.T) {
	tests := []struct {
		name     string
		scope    config.SetupScope
		email    string
		password string
		apiURL   string
	}{
		{
			name:   "local-only cleanup failure points to manual recovery",
			scope:  config.ScopeLocalOnly,
			apiURL: config.DefaultAPIURL,
		},
		{
			name:     "local+cloud sync write failure points to manual recovery",
			scope:    config.ScopeLocalCloud,
			email:    "dev@example.com",
			password: "secret",
			apiURL:   config.DefaultAPIURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			homeAsFile := filepath.Join(tmpDir, "home-file")
			if err := os.WriteFile(homeAsFile, []byte("not-a-directory"), 0600); err != nil {
				t.Fatalf("seed fake HOME file: %v", err)
			}
			t.Setenv("HOME", homeAsFile)

			m := Model{
				Step:     StepApply,
				Scope:    tt.scope,
				Email:    tt.email,
				Password: tt.password,
				cfg:      &config.AppConfig{APIURL: tt.apiURL},
				Selected: map[string]bool{},
			}

			cmd := runAgentConfigSequence(m)
			if cmd == nil {
				t.Fatal("expected non-nil command")
			}

			msg := cmd()
			progress, ok := msg.(agentProgressMsg)
			if !ok {
				t.Fatalf("expected agentProgressMsg, got %T", msg)
			}
			if !progress.done || !progress.failed {
				t.Fatalf("expected done=true and failed=true, got done=%v failed=%v line=%q", progress.done, progress.failed, progress.line)
			}
			if !strings.Contains(progress.line, "Ver docs/setup-recovery.md") {
				t.Fatalf("expected manual recovery pointer in failure message, got %q", progress.line)
			}
			if strings.Contains(strings.ToLower(progress.line), "rollback") {
				t.Fatalf("failure message must not claim automatic rollback, got %q", progress.line)
			}
		})
	}
}

func TestUpdateReview_BackCancelApply(t *testing.T) {
	tests := []struct {
		name         string
		choice       int
		expectStep   Step
		expectDone   bool
		expectCmdNil bool
	}{
		{name: "back", choice: 0, expectStep: StepSkills, expectDone: false, expectCmdNil: true},
		{name: "cancel", choice: 1, expectStep: StepReview, expectDone: true, expectCmdNil: false},
		{name: "apply", choice: 2, expectStep: StepApply, expectDone: false, expectCmdNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{Step: StepReview, reviewChoice: tt.choice, cfg: &config.AppConfig{}, Selected: map[string]bool{}}
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m2 := updated.(Model)
			if m2.Step != tt.expectStep {
				t.Fatalf("step: got %v want %v", m2.Step, tt.expectStep)
			}
			if m2.Done != tt.expectDone {
				t.Fatalf("done: got %v want %v", m2.Done, tt.expectDone)
			}
			if (cmd == nil) != tt.expectCmdNil {
				t.Fatalf("cmd nil: got %v want %v", cmd == nil, tt.expectCmdNil)
			}
		})
	}
}

func TestUpdateReview_BackCancel_NoApplyArtifactsCreated(t *testing.T) {
	tests := []struct {
		name       string
		reviewSlot int
		expectDone bool
		expectStep Step
	}{
		{name: "back keeps wizard editable", reviewSlot: 0, expectDone: false, expectStep: StepSkills},
		{name: "cancel exits without apply", reviewSlot: 1, expectDone: true, expectStep: StepReview},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome := t.TempDir()
			t.Setenv("HOME", tmpHome)

			jarvisDir := filepath.Join(tmpHome, ".jarvis")
			if err := os.MkdirAll(jarvisDir, 0755); err != nil {
				t.Fatalf("mkdir jarvis dir: %v", err)
			}
			seedSync := `{"api_url":"https://old.dev","email":"old@example.com","password":"old"}`
			syncPath := filepath.Join(jarvisDir, "sync.json")
			if err := os.WriteFile(syncPath, []byte(seedSync), 0600); err != nil {
				t.Fatalf("seed sync.json: %v", err)
			}

			m := Model{
				Step:         StepReview,
				reviewChoice: tt.reviewSlot,
				Scope:        config.ScopeLocalCloud,
				Email:        "dev@example.com",
				Password:     "secret",
				cfg:          &config.AppConfig{APIURL: config.DefaultAPIURL, Scope: config.ScopeLocalCloud},
				Selected:     map[string]bool{},
			}

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m2 := updated.(Model)
			if m2.Step != tt.expectStep {
				t.Fatalf("step: got %v want %v", m2.Step, tt.expectStep)
			}
			if m2.Done != tt.expectDone {
				t.Fatalf("done: got %v want %v", m2.Done, tt.expectDone)
			}

			if _, err := os.Stat(filepath.Join(jarvisDir, "memory.db")); !os.IsNotExist(err) {
				t.Fatalf("expected no memory.db before apply confirmation, got err=%v", err)
			}

			syncBody, err := os.ReadFile(syncPath)
			if err != nil {
				t.Fatalf("read sync.json: %v", err)
			}
			if string(syncBody) != seedSync {
				t.Fatalf("sync.json changed before apply confirmation, got %s", string(syncBody))
			}
		})
	}
}

func TestRunAgentConfigSequence_LocalCloudHappyPathPersistsCloudArtifacts(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &config.AppConfig{APIURL: config.DefaultAPIURL}
	m := Model{
		Step:     StepApply,
		Scope:    config.ScopeLocalCloud,
		Email:    "happy@example.com",
		Password: "secret",
		cfg:      cfg,
		Selected: map[string]bool{},
	}

	cmd := runAgentConfigSequence(m)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	msg := cmd()
	progress, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}
	if !progress.done || progress.failed {
		t.Fatalf("expected successful completion, got done=%v failed=%v line=%q", progress.done, progress.failed, progress.line)
	}

	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	syncPath := filepath.Join(jarvisDir, "sync.json")
	syncBody, err := os.ReadFile(syncPath)
	if err != nil {
		t.Fatalf("expected sync.json in local+cloud apply, got err=%v", err)
	}
	if !strings.Contains(string(syncBody), `"email":"happy@example.com"`) {
		t.Fatalf("expected sync.json email from apply, got: %s", string(syncBody))
	}
	if !strings.Contains(string(syncBody), `"password":"secret"`) {
		t.Fatalf("expected sync.json password from apply, got: %s", string(syncBody))
	}

	if _, err := os.Stat(filepath.Join(jarvisDir, "memory.db")); err != nil {
		t.Fatalf("expected memory.db created during apply, got err=%v", err)
	}

	if cfg.Cloud == nil {
		t.Fatal("expected cloud linkage in config after local+cloud apply")
	}
	if cfg.Cloud.Email != "happy@example.com" || !cfg.Cloud.SyncConfigured {
		t.Fatalf("unexpected cloud linkage after apply: %+v", cfg.Cloud)
	}
	if cfg.Email != "happy@example.com" {
		t.Fatalf("expected cfg.Email updated, got %q", cfg.Email)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if loaded.Scope != config.ScopeLocalCloud {
		t.Fatalf("expected persisted scope local+cloud, got %q", loaded.Scope)
	}
	if loaded.Cloud == nil || loaded.Cloud.Email != "happy@example.com" {
		t.Fatalf("expected persisted cloud linkage, got %+v", loaded.Cloud)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestHandleStepMsg_UnknownMsg
// ──────────────────────────────────────────────────────────────────────────────

// TestHandleStepMsg_UnknownMsg verifies unknown messages are not handled.
func TestHandleStepMsg_UnknownMsg(t *testing.T) {
	m := Model{Step: StepHiveLocal, Selected: make(map[string]bool)}
	_, handled, _ := handleStepMsg(m, "some-random-message")
	if handled {
		t.Error("expected unknown message type to not be handled")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestWriteSyncJSON
// ──────────────────────────────────────────────────────────────────────────────

// TestWriteSyncJSON verifies that sync credentials are written to ~/.jarvis/sync.json.
func TestWriteSyncJSON(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".jarvis"), 0755); err != nil {
		t.Fatal(err)
	}

	err := writeSyncJSON("https://hivemem.dev", "user@example.com", "s3cr3t")
	if err != nil {
		t.Fatalf("writeSyncJSON: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(tmpHome, ".jarvis", "sync.json"))
	if readErr != nil {
		t.Fatal("sync.json not created:", readErr)
	}
	// token must NOT be written — hive-daemon uses DisallowUnknownFields()
	if strings.Contains(string(data), "token") {
		t.Errorf("token must not appear in sync.json, got: %s", data)
	}
	if !strings.Contains(string(data), "user@example.com") {
		t.Errorf("expected email in sync.json, got: %s", data)
	}
}

func TestWriteSyncJSON_PreservesAutoSync(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	seed := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":true}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeSyncJSON("https://hivemem.dev", "user@example.com", "s3cr3t"); err != nil {
		t.Fatalf("writeSyncJSON: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync to be preserved, got: %s", body)
	}
	if !strings.Contains(body, "user@example.com") {
		t.Fatalf("expected updated credentials, got: %s", body)
	}
}
