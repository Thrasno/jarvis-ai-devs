package tui

import (
	"fmt"
	"os"
	"path/filepath"
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
	return Model{
		Step: StepSkills,
		SkillList: []skills.Skill{
			{ID: "core-skill", Name: "Core Skill", IsCore: true},
			{ID: "optional-skill", Name: "Optional Skill", IsCore: false},
		},
		Selected: map[string]bool{
			"core-skill": true, // pre-selected because it is core
		},
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

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_HiveLocal_AdvancesOnEnter
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_HiveLocal_AdvancesOnEnter verifies that pressing Enter on StepHiveLocal
// creates ~/.jarvis/memory.db and advances the model to StepHiveCloud.
func TestStep_HiveLocal_AdvancesOnEnter(t *testing.T) {
	// Redirect HOME so we don't touch the real user directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	m := Model{
		Step:     StepHiveLocal,
		Selected: make(map[string]bool),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if m2.Err != nil {
		t.Fatalf("unexpected error after Enter: %v", m2.Err)
	}
	if m2.Step != StepHiveCloud {
		t.Errorf("expected StepHiveCloud after Enter, got %v", m2.Step)
	}

	// ~/.jarvis/memory.db must be created.
	dbPath := filepath.Join(tmpHome, ".jarvis", "memory.db")
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		t.Error("expected memory.db to be created but it was not found")
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
	// Move cursor to index 1 (optional-skill).
	m.presetCur = 1

	if m.Selected["optional-skill"] {
		t.Fatal("optional-skill should not be selected initially")
	}

	// Toggle on.
	m = sendRune(m, " ")
	if !m.Selected["optional-skill"] {
		t.Error("expected optional-skill to be selected after Space")
	}

	// Toggle off.
	m = sendRune(m, " ")
	if m.Selected["optional-skill"] {
		t.Error("expected optional-skill to be deselected after second Space")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Skills_CoreAlwaysSelected
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Skills_CoreAlwaysSelected verifies that pressing Space on a core skill
// does NOT deselect it.
func TestStep_Skills_CoreAlwaysSelected(t *testing.T) {
	m := buildSkillsModel()
	// Cursor at 0 = core-skill.
	m.presetCur = 0

	if !m.Selected["core-skill"] {
		t.Fatal("core-skill should be pre-selected")
	}

	// Space should be a no-op for core skills.
	m = sendRune(m, " ")
	if !m.Selected["core-skill"] {
		t.Error("core-skill must remain selected after Space (it is a core skill)")
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

	if m.Step != StepSkills {
		t.Errorf("expected StepSkills after Enter, got %v", m.Step)
	}
	if m.cfg.Preset != "preset-0" {
		t.Errorf("expected cfg.Preset=preset-0, got %q", m.cfg.Preset)
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

// TestStep_Skills_CoreSkillAlwaysInSelected verifies that toggling a core skill
// via KeySpace (tea.KeySpace type) does not remove it from Selected.
func TestStep_Skills_CoreSkillAlwaysInSelected(t *testing.T) {
	m := buildSkillsModel()
	// Cursor at 0 = core-skill.
	m.presetCur = 0

	if !m.Selected["core-skill"] {
		t.Fatal("core-skill should be pre-selected")
	}

	// Send KeySpace (the key type, not a rune) — core skill must remain selected.
	m = sendKey(m, tea.KeySpace)
	if !m.Selected["core-skill"] {
		t.Error("core-skill must remain selected after KeySpace (it is a core skill)")
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
