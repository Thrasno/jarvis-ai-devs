package tui

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

// TestViewScope_HasHeaderAndHelpBar verifies that viewScope renders a breadcrumb
// and a help hint key in the output.
func TestViewScope_HasHeaderAndHelpBar(t *testing.T) {
	m := Model{
		Step:     StepScope,
		Scope:    config.ScopeLocalOnly,
		width:    100,
		cfg:      &config.AppConfig{},
		Selected: make(map[string]bool),
	}
	result := viewScope(m)
	if result == "" {
		t.Fatal("viewScope returned empty string")
	}
	if !strings.Contains(result, "Scope") {
		t.Errorf("expected breadcrumb containing 'Scope' in viewScope output, got:\n%s", result)
	}
	// Help bar must contain at least one key hint indicator.
	if !strings.Contains(result, "↑") && !strings.Contains(result, "Enter") && !strings.Contains(result, "Ctrl") {
		t.Errorf("expected viewScope to contain help hint keys, got:\n%s", result)
	}
}

// TestViewScope_WidthFloor80 verifies that passing a model with width=0 does not
// panic and returns a non-empty string (PanelWidth floors to 80).
func TestViewScope_WidthFloor80(t *testing.T) {
	m := Model{
		Step:     StepScope,
		Scope:    config.ScopeLocalOnly,
		width:    0,
		cfg:      &config.AppConfig{},
		Selected: make(map[string]bool),
	}
	result := viewScope(m)
	if result == "" {
		t.Fatal("viewScope with width=0 returned empty string (expected floor to 80)")
	}
}

// TestViewReview_HasBorderedPanel verifies that viewReview wraps the summary in a
// bordered panel (contains a rounded border character).
func TestViewReview_HasBorderedPanel(t *testing.T) {
	m := Model{
		Step:     StepReview,
		width:    100,
		cfg:      &config.AppConfig{},
		Selected: make(map[string]bool),
	}
	m = initializePhaseModelEditor(m)
	result := viewReview(m)
	if result == "" {
		t.Fatal("viewReview returned empty string")
	}
	// Rounded border uses │ or ╭ characters.
	if !strings.ContainsAny(result, "│╭╰╯╮") {
		t.Errorf("expected viewReview to contain a bordered panel (│ or ╭), got:\n%s", result)
	}
}

// TestViewSkills_HasSelectedRow verifies that viewSkills does not crash when a
// skill is selected and returns a non-empty output.
func TestViewSkills_HasSelectedRow(t *testing.T) {
	plans := buildSkillSelectionPlan(testStyleSkillList(), nil)
	m := Model{
		Step:         StepSkills,
		width:        100,
		SkillList:    testStyleSkillList(),
		Selected:     plans.Selected,
		SkillPrompts: plans.Prompts,
		cfg:          &config.AppConfig{},
	}
	// Select the first prompt item to trigger SelectedRow rendering.
	m.presetCur = 0

	result := viewSkills(m)
	if result == "" {
		t.Fatal("viewSkills returned empty string")
	}
}

// TestViewAuth_TokenNotInClear verifies that a non-empty token/password field is
// never rendered as plain text in viewHiveCloud output.
func TestViewAuth_TokenNotInClear(t *testing.T) {
	const secret = "s3cr3t-password"
	m := Model{
		Step:        StepHiveCloud,
		width:       100,
		Password:    secret,
		Email:       "user@example.com",
		activeField: 1,
		cfg:         &config.AppConfig{},
		Selected:    make(map[string]bool),
	}
	result := viewHiveCloud(m)
	if strings.Contains(result, secret) {
		t.Errorf("viewHiveCloud must not render the raw password in clear text; got:\n%s", result)
	}
}

// testStyleSkillList returns a minimal skill list for style tests.
func testStyleSkillList() []skills.Skill {
	return []skills.Skill{
		{ID: "hive", Name: "Hive", IsCore: true},
		{ID: "go-testing", Name: "Go Testing", IsCore: false},
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Slice 3: complex screens + cockpit
// ──────────────────────────────────────────────────────────────────────────────

// TestViewPersonaList_HasSelectedRow verifies that when at least one persona
// preset is present, viewPersona renders the terminalui HeaderRow breadcrumb
// and a HelpBar footer (Slice 3 migration markers).
func TestViewPersonaList_HasSelectedRow(t *testing.T) {
	m := Model{
		Step:     StepPersona,
		width:    100,
		cfg:      &config.AppConfig{},
		Selected: make(map[string]bool),
		Presets: []persona.ProfileOption{
			{Name: "architect", DisplayName: "Architect", Description: "Architecture persona"},
			{Name: "junior", DisplayName: "Junior Dev", Description: "Junior persona"},
		},
		presetCur: 0,
	}
	result := viewPersona(m)
	if result == "" {
		t.Fatal("viewPersona returned empty string")
	}
	// After Slice 3, viewPersona must render a HeaderRow (contains "Persona" breadcrumb)
	// and a HelpBar with key hints.
	if !strings.Contains(result, "Persona") {
		t.Errorf("expected viewPersona to contain 'Persona' in HeaderRow breadcrumb, got:\n%s", result)
	}
	if !strings.Contains(result, "↑") && !strings.Contains(result, "Enter") {
		t.Errorf("expected viewPersona to contain HelpBar key hints (↑ or Enter), got:\n%s", result)
	}
	// Must also contain a border character from BorderedPanel.
	if !strings.ContainsAny(result, "│╭╰╯╮") {
		t.Errorf("expected viewPersona (persona list) to contain a border char from BorderedPanel, got:\n%s", result)
	}
}

// TestViewPersonaList_EmptyState verifies that viewPersona renders a BorderedPanel
// empty-state message when no presets are loaded (Slice 3: BorderedPanel).
func TestViewPersonaList_EmptyState(t *testing.T) {
	m := Model{
		Step:     StepPersona,
		width:    100,
		cfg:      &config.AppConfig{},
		Selected: make(map[string]bool),
		Presets:  []persona.ProfileOption{},
	}
	result := viewPersona(m)
	if result == "" {
		t.Fatal("viewPersona (empty presets) returned empty string")
	}
	// After Slice 3, empty state is rendered as BorderedPanel (contains border char).
	if !strings.ContainsAny(result, "│╭╰╯╮") {
		t.Errorf("expected viewPersona (empty) to contain BorderedPanel border char, got:\n%s", result)
	}
}

// TestViewPersonaEdit_HelpBarHints verifies that the custom-edit mode of
// viewPersona renders a HeaderRow breadcrumb and a HelpBar footer (Slice 3).
func TestViewPersonaEdit_HelpBarHints(t *testing.T) {
	m := Model{
		Step:        StepPersona,
		width:       100,
		cfg:         &config.AppConfig{},
		Selected:    make(map[string]bool),
		customEdit:  true,
		customField: 0,
	}
	result := viewPersona(m)
	if result == "" {
		t.Fatal("viewPersona (custom edit) returned empty string")
	}
	// After Slice 3, custom-edit mode renders HeaderRow with "Edit" breadcrumb
	// and HelpBar at the bottom.
	if !strings.Contains(result, "Edit") {
		t.Errorf("expected viewPersona (custom edit) to contain 'Edit' in HeaderRow, got:\n%s", result)
	}
	if !strings.Contains(result, "Tab") && !strings.Contains(result, "Ctrl") && !strings.Contains(result, "Esc") {
		t.Errorf("expected viewPersona (custom edit) to contain HelpBar hints (Tab/Ctrl/Esc), got:\n%s", result)
	}
}

// TestViewPhaseModels_HasSelectedRow verifies that viewPhaseModels renders
// the terminalui HeaderRow (contains "Phase Models" or breadcrumb) and a
// HelpBar footer after Slice 3 migration.
func TestViewPhaseModels_HasSelectedRow(t *testing.T) {
	m := Model{
		Step:     StepPhaseModels,
		width:    100,
		cfg:      &config.AppConfig{},
		Selected: make(map[string]bool),
		phaseModelRows: []phaseModelRow{
			{Phase: "sdd-apply", OpenCode: "gpt-4o", Claude: "sonnet"},
			{Phase: "sdd-verify", OpenCode: "gpt-4o", Claude: "sonnet"},
		},
		phaseModelMode:        phaseModelModeList,
		phaseModelActiveRow:   0,
		phaseModelActiveCol:   phaseModelOpenCodeColumn,
		phaseModelHasOpenCode: true,
		phaseModelHasClaude:   true,
	}
	result := viewPhaseModels(m)
	if result == "" {
		t.Fatal("viewPhaseModels returned empty string")
	}
	// After Slice 3, HeaderRow must contain breadcrumb "Phase Models".
	if !strings.Contains(result, "Phase Models") {
		t.Errorf("expected viewPhaseModels to contain 'Phase Models' in HeaderRow, got:\n%s", result)
	}
	// HelpBar must contain at least one key hint.
	if !strings.Contains(result, "↑") && !strings.Contains(result, "Tab") && !strings.Contains(result, "Enter") {
		t.Errorf("expected viewPhaseModels to contain HelpBar hints, got:\n%s", result)
	}
	// Selected row must use border char (BorderedPanel or SelectedRow).
	if !strings.ContainsAny(result, "│╭╰╯╮") {
		t.Errorf("expected viewPhaseModels to contain a border char from BorderedPanel, got:\n%s", result)
	}
}

// TestViewCockpit_LogoAtWide verifies that at width >= 120, viewCockpit renders
// the word "Jarvis" (the logo is present).
func TestViewCockpit_LogoAtWide(t *testing.T) {
	m := Model{
		Screen:      ScreenCockpit,
		width:       120,
		cfg:         &config.AppConfig{},
		Selected:    make(map[string]bool),
		cockpitMode: cockpitModeMenu,
	}
	result := viewCockpit(m)
	if result == "" {
		t.Fatal("viewCockpit (wide) returned empty string")
	}
	if !strings.Contains(result, "Jarvis") {
		t.Errorf("expected viewCockpit (wide) to contain 'Jarvis' (logo), got:\n%s", result)
	}
}

// TestViewCockpit_LogoAtNarrow verifies that at width < 80, viewCockpit is
// non-empty and does not panic.
func TestViewCockpit_LogoAtNarrow(t *testing.T) {
	m := Model{
		Screen:      ScreenCockpit,
		width:       60,
		cfg:         &config.AppConfig{},
		Selected:    make(map[string]bool),
		cockpitMode: cockpitModeMenu,
	}
	result := viewCockpit(m)
	if result == "" {
		t.Fatal("viewCockpit (narrow) returned empty string")
	}
}

// TestViewCockpit_HasHelpBar verifies that viewCockpit menu renders a terminalui
// HelpBar with key hints (Slice 3: uses terminalui.HelpBar instead of raw string).
func TestViewCockpit_HasHelpBar(t *testing.T) {
	m := Model{
		Screen:      ScreenCockpit,
		width:       100,
		cfg:         &config.AppConfig{},
		Selected:    make(map[string]bool),
		cockpitMode: cockpitModeMenu,
	}
	result := viewCockpit(m)
	if result == "" {
		t.Fatal("viewCockpit (help bar) returned empty string")
	}
	// After Slice 3, HelpBar renders with structured key hints.
	// The HelpBar renders key + desc format, so at least "Enter" and "q" should be present.
	if !strings.Contains(result, "Enter") {
		t.Errorf("expected viewCockpit HelpBar to contain 'Enter' key hint, got:\n%s", result)
	}
	if !strings.Contains(result, "q") {
		t.Errorf("expected viewCockpit HelpBar to contain 'q' key hint, got:\n%s", result)
	}
}

// TestViewCockpit_MenuSelectedRow verifies that when the cockpit menu cursor is
// on an item, viewCockpit uses terminalui.SelectedRow (Slice 3 migration).
// The selected item must render with a rounded border char (│╭╰╯╮) from SelectedRow's
// background fill or the surrounding BorderedPanel.
func TestViewCockpit_MenuSelectedRow(t *testing.T) {
	m := Model{
		Screen:        ScreenCockpit,
		width:         100,
		cfg:           &config.AppConfig{},
		Selected:      make(map[string]bool),
		cockpitMode:   cockpitModeMenu,
		cockpitCursor: 0,
	}
	result := viewCockpit(m)
	if result == "" {
		t.Fatal("viewCockpit (menu selected row) returned empty string")
	}
	// After Slice 3, the menu list is wrapped in a BorderedPanel so it contains border chars.
	if !strings.ContainsAny(result, "│╭╰╯╮") {
		t.Errorf("expected viewCockpit (menu) to contain border char from BorderedPanel/SelectedRow, got:\n%s", result)
	}
}
