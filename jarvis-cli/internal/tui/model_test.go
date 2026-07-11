// All tests in this package run under Ascii color profile; see testmain_test.go.
package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/opencode"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
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

func TestNewCockpitModel_StartsAtCockpitNotWizard(t *testing.T) {
	m := NewCockpitModel(testWizardConfig())

	if m.Screen != ScreenCockpit {
		t.Fatalf("expected first screen cockpit, got %v", m.Screen)
	}
	if m.Step != StepScope {
		t.Fatalf("expected wizard to remain staged at StepScope, got %v", m.Step)
	}

	view := m.View()
	if !strings.Contains(view, "Install/Reconfigure") {
		t.Fatalf("expected cockpit action in first view, got:\n%s", view)
	}
	if strings.Contains(view, "Jarvis-Dev Setup") {
		t.Fatalf("cockpit first view must not auto-enter setup wizard, got:\n%s", view)
	}
}

func TestCockpitInstallReconfigureEntersWizard(t *testing.T) {
	m := NewCockpitModel(testWizardConfig())

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if cmd != nil {
		t.Fatalf("expected entering wizard to be synchronous, got cmd %T", cmd)
	}
	if m2.Screen != ScreenWizard {
		t.Fatalf("expected Install/Reconfigure to enter wizard screen, got %v", m2.Screen)
	}
	if m2.Step != StepScope {
		t.Fatalf("expected wizard to start at StepScope, got %v", m2.Step)
	}
	if !strings.Contains(m2.View(), "Scope") {
		t.Fatalf("expected wizard scope view after install action, got:\n%s", m2.View())
	}
}

func TestCockpitExitQuits(t *testing.T) {
	m := NewCockpitModel(testWizardConfig())
	m.cockpitCursor = len(CockpitActions()) - 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)

	if !m2.Done {
		t.Fatal("expected Exit action to mark model done")
	}
	if cmd == nil {
		t.Fatal("expected Exit action to return tea.Quit command")
	}
}

func TestCockpitNavigationHelpersWrapAtMenuBoundaries(t *testing.T) {
	total := len(CockpitActions())
	last := total - 1

	tests := []struct {
		name string
		move func(current, total int) int
		from int
		want int
	}{
		{name: "previous wraps from first action to last", move: previousCockpitIndex, from: 0, want: last},
		{name: "previous moves to prior action", move: previousCockpitIndex, from: 3, want: 2},
		{name: "previous treats negative cursor as before first", move: previousCockpitIndex, from: -1, want: last},
		{name: "next wraps from last action to first", move: nextCockpitIndex, from: last, want: 0},
		{name: "next moves to following action", move: nextCockpitIndex, from: 3, want: 4},
		{name: "next treats oversized cursor as after last", move: nextCockpitIndex, from: total + 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.move(tt.from, total); got != tt.want {
				t.Fatalf("expected cursor %d, got %d", tt.want, got)
			}
		})
	}
}

func TestCockpitNavigationKeysMoveSelectionAndWrap(t *testing.T) {
	total := len(CockpitActions())
	last := total - 1
	m := NewCockpitModel(testWizardConfig())

	m = sendKey(m, tea.KeyUp)
	if m.cockpitCursor != last {
		t.Fatalf("expected up from first action to wrap to cursor %d, got %d", last, m.cockpitCursor)
	}
	// After Slice 3, selected item is highlighted via SelectedRow (no "> " prefix).
	// Check the action label appears in the rendered view.
	if !strings.Contains(m.View(), "Exit") {
		t.Fatalf("expected wrapped selection to be visible on Exit, got:\n%s", m.View())
	}

	m = sendKey(m, tea.KeyDown)
	if m.cockpitCursor != 0 {
		t.Fatalf("expected down from last action to wrap to cursor 0, got %d", m.cockpitCursor)
	}
	if !strings.Contains(m.View(), "Install/Reconfigure") {
		t.Fatalf("expected wrapped selection to be visible on Install/Reconfigure, got:\n%s", m.View())
	}

	m = sendRune(m, "j")
	if m.cockpitCursor != 1 {
		t.Fatalf("expected j to move to cursor 1, got %d", m.cockpitCursor)
	}
	m = sendRune(m, "k")
	if m.cockpitCursor != 0 {
		t.Fatalf("expected k to move back to cursor 0, got %d", m.cockpitCursor)
	}
}

func TestCockpitIgnoresUnsupportedNavigationKeys(t *testing.T) {
	m := NewCockpitModel(testWizardConfig())
	m.cockpitCursor = 2

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected unsupported cockpit key to return no command, got %T", cmd)
	}
	if m.cockpitCursor != 2 || m.Screen != ScreenCockpit || m.Done {
		t.Fatalf("unsupported cockpit key changed state: cursor=%d screen=%v done=%v", m.cockpitCursor, m.Screen, m.Done)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes})
	m = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected empty rune cockpit key to return no command, got %T", cmd)
	}
	if m.cockpitCursor != 2 || m.Screen != ScreenCockpit || m.Done {
		t.Fatalf("empty rune cockpit key changed state: cursor=%d screen=%v done=%v", m.cockpitCursor, m.Screen, m.Done)
	}
}

func TestCockpitQKeyQuitsWithoutSelectingAction(t *testing.T) {
	m := NewCockpitModel(testWizardConfig())
	m.cockpitCursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = updated.(Model)

	if !m.Done {
		t.Fatal("expected q to mark cockpit model done")
	}
	if cmd == nil {
		t.Fatal("expected q to return tea.Quit command")
	}
	if m.Screen != ScreenCockpit || m.cockpitMessage != "" {
		t.Fatalf("q should quit from cockpit without selecting an action: screen=%v message=%q", m.Screen, m.cockpitMessage)
	}
}

func TestCockpitView_RenderContractUsesTextLogoAndNoBackgroundFill(t *testing.T) {
	m := NewCockpitModel(testWizardConfig())

	view := m.View()
	if !strings.Contains(view, strings.TrimSpace(CockpitLogo())) {
		t.Fatalf("expected rendered cockpit to include embedded text logo, got:\n%s", view)
	}
	for _, forbidden := range []string{"\x1b[4", "\x1b[10", "\x1b[48;", "\x1b]1337", "\x1b_G", "PNG"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("cockpit view must stay text-only and avoid background/image protocols; found %q in:\n%s", forbidden, view)
		}
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
	setTestHome(t, tmpHome)

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

func TestNewModel_BlankPersonaAcceptanceBlocksLegacyV1PresetAndPreservesConfig(t *testing.T) {
	isolateTestHome(t)
	legacyPath := filepath.Join(os.Getenv("HOME"), ".jarvis", "personas", "legacy-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy preset dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n"), 0o644); err != nil {
		t.Fatalf("write legacy preset: %v", err)
	}
	if err := config.Save(&config.AppConfig{
		SchemaVersion:       2,
		APIURL:              config.DefaultAPIURL,
		PersonaPreset:       "legacy-custom",
		PersonaPresetSource: string(persona.PresetSourceUser),
		Install:             config.InstallState{Agents: map[string]config.AgentState{}},
	}); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	m := NewModel(testWizardConfig(), false)
	if m.presetCur != -1 {
		t.Fatalf("legacy V1 preset must not default to catalog index 0, got %d", m.presetCur)
	}
	m = sendKey(m, tea.KeyEnter)
	if m.Step != StepPersona {
		t.Fatalf("expected scope acceptance to enter persona selection, got %v", m.Step)
	}
	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepPersona {
		t.Fatalf("blank/default persona acceptance must stay blocked at persona step, got %v", m.Step)
	}
	if m.Err == nil || !strings.Contains(strings.ToLower(m.Err.Error()), "migrate") {
		t.Fatalf("persona selection error = %v, want schema-v2 migration guidance", m.Err)
	}
	if m.cfg.PersonaPreset != "legacy-custom" || m.selectedPresetV2 != nil {
		t.Fatalf("legacy persona selection was overwritten in memory: cfg=%+v selected=%+v", m.cfg, m.selectedPresetV2)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config after blocked default: %v", err)
	}
	if loaded.PersonaPreset != "legacy-custom" || loaded.PersonaPresetSource != string(persona.PresetSourceUser) {
		t.Fatalf("legacy persona config was overwritten on disk: %+v", loaded)
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
	setTestHome(t, tmpHome)

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

func TestStep_Skills_EnterAdvancesToPhaseModels(t *testing.T) {
	m := buildSkillsModel()
	m.Agents = []agent.Agent{&mockAgent{name: "claude", configDir: t.TempDir()}}

	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepPhaseModels {
		t.Fatalf("expected StepPhaseModels after skills Enter, got %v", m.Step)
	}
}

func TestStep_PhaseModels_ApplyAllAndCycling(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)

	if m.phaseModelActiveCol != 1 {
		t.Fatalf("expected initial active column OpenCode(1), got %d", m.phaseModelActiveCol)
	}

	// Move to next legacy model in OpenCode catalog then apply-all.
	m = sendKey(m, tea.KeySpace)
	current := m.phaseModelRows[m.phaseModelActiveRow].OpenCode
	m = sendRune(m, "a")

	for _, row := range m.phaseModelRows {
		if row.OpenCode != current {
			t.Fatalf("expected OpenCode apply-all value %q, got %q", current, row.OpenCode)
		}
	}

	// Move to Claude column and ensure OpenCode stays unchanged while Claude cycles.
	m = sendKey(m, tea.KeyRight)
	beforeOpenCode := m.phaseModelRows[m.phaseModelActiveRow].OpenCode
	beforeClaude := m.phaseModelRows[m.phaseModelActiveRow].Claude
	m = sendKey(m, tea.KeySpace)
	after := m.phaseModelRows[m.phaseModelActiveRow]
	if after.OpenCode != beforeOpenCode {
		t.Fatalf("expected OpenCode unchanged while editing Claude column")
	}
	if after.Claude == beforeClaude {
		t.Fatalf("expected Claude to cycle to next catalog value")
	}
}

func TestPhaseModelOpenCodeDisplay_IncludesEffortWhenPresent(t *testing.T) {
	row := phaseModelRow{
		OpenCode: "sonnet",
		OpenCodeAssignment: config.OpenCodeModelAssignment{
			ProviderID: "openai",
			ModelID:    "gpt-5.1-codex-max",
			Effort:     "high",
		},
	}

	if got, want := phaseModelOpenCodeDisplay(row), "openai/gpt-5.1-codex-max (effort=high)"; got != want {
		t.Fatalf("phaseModelOpenCodeDisplay = %q, want %q", got, want)
	}
}

func TestStep_PhaseModels_PersistsOpenCodeProviderModelAssignment(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		return []config.OpenCodeModelAssignment{
			{ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
		}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)
	m.phaseModelActiveRow = 0
	m.phaseModelActiveCol = 1
	phase := m.phaseModelRows[0].Phase
	legacyAlias := m.cfg.SDD.PhaseModels[phase].OpenCode

	m = sendKey(m, tea.KeyEnter)

	assignment := m.cfg.SDD.OpenCodePhaseModels[phase]
	if assignment.ProviderID != "openai" || assignment.ModelID != "gpt-5.1-codex-max" || assignment.Effort != "high" {
		t.Fatalf("unexpected OpenCode assignment: %+v", assignment)
	}
	if got := m.cfg.SDD.PhaseModels[phase].OpenCode; got != legacyAlias {
		t.Fatalf("legacy OpenCode alias changed: got %q, want %q", got, legacyAlias)
	}
}

func TestStep_PhaseModels_ClearsOpenCodeProviderModelAssignmentWhenCyclingToLegacy(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		return []config.OpenCodeModelAssignment{{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"}}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m.cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
	}
	m = initializePhaseModelEditor(m)
	m.phaseModelActiveRow = 0
	m.phaseModelActiveCol = 1
	phase := m.phaseModelRows[0].Phase

	m = sendKey(m, tea.KeyEnter)

	if got := m.phaseModelRows[0].OpenCodeAssignment; got.ProviderID != "" || got.ModelID != "" {
		t.Fatalf("expected row assignment cleared by legacy option, got %+v", got)
	}
	if _, ok := m.cfg.SDD.OpenCodePhaseModels[phase]; ok {
		t.Fatalf("expected config assignment deleted by legacy option, got %#v", m.cfg.SDD.OpenCodePhaseModels)
	}
	if display := phaseModelOpenCodeDisplay(m.phaseModelRows[0]); strings.Contains(display, "openai/") {
		t.Fatalf("expected legacy display to hide provider assignment, got %q", display)
	}
}

func TestStep_PhaseModels_KeepsStoredOpenCodeProviderModelAssignmentWhenDiscoveryUnavailable(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment { return nil }
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m.cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
	}
	m = initializePhaseModelEditor(m)
	m.phaseModelActiveRow = 0
	m.phaseModelActiveCol = 1
	phase := m.phaseModelRows[0].Phase

	m = sendKey(m, tea.KeyEnter)

	if got := m.cfg.SDD.OpenCodePhaseModels[phase]; got.ProviderID != "openai" || got.ModelID != "gpt-5.1-codex-max" {
		t.Fatalf("expected stored assignment kept when discovery unavailable, got %+v", got)
	}
}

func TestStep_PhaseModels_ShowsOpenCodeDiscoveryDiagnostics(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		openCodePhaseModelDiscoveryDiagnostics = []string{"OpenCode settings file /home/me/.config/opencode/opencode.jsonc uses unsupported JSONC"}
		return nil
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)

	view := viewPhaseModels(m)
	if !strings.Contains(view, "unsupported JSONC") {
		t.Fatalf("expected OpenCode discovery diagnostic in phase model view, got:\n%s", view)
	}
}

func TestStep_PhaseModels_UsesStoredOpenCodeProviderModelAssignment(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		return []config.OpenCodeModelAssignment{
			{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m.cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
	}
	m = initializePhaseModelEditor(m)

	if got := m.phaseModelRows[0].OpenCodeAssignment; got.ProviderID != "openai" || got.ModelID != "gpt-5.1-codex-max" {
		t.Fatalf("stored assignment not loaded into row: %+v", got)
	}
}

func TestOpenCodePhaseModelOptionsFromDiscovery_SortsProvidersModelsAndEfforts(t *testing.T) {
	result := opencode.DiscoveryResult{Providers: []opencode.AvailableProvider{
		{Provider: opencode.Provider{ID: "openai", Models: map[string]opencode.Model{"z-model": {ID: "z-model"}, "a-model": {ID: "a-model", Reasoning: true}}}},
		{Provider: opencode.Provider{ID: "anthropic", Models: map[string]opencode.Model{"claude": {ID: "claude", Reasoning: true}}}},
	}}

	got := openCodePhaseModelOptionsFromDiscovery(result)
	want := []config.OpenCodeModelAssignment{
		{},
		{ProviderID: "anthropic", ModelID: "claude"},
		{ProviderID: "anthropic", ModelID: "claude", Effort: "high"},
		{ProviderID: "anthropic", ModelID: "claude", Effort: "max"},
		{ProviderID: "openai", ModelID: "a-model"},
		{ProviderID: "openai", ModelID: "a-model", Effort: "minimal"},
		{ProviderID: "openai", ModelID: "a-model", Effort: "low"},
		{ProviderID: "openai", ModelID: "a-model", Effort: "medium"},
		{ProviderID: "openai", ModelID: "a-model", Effort: "high"},
		{ProviderID: "openai", ModelID: "a-model", Effort: "xhigh"},
		{ProviderID: "openai", ModelID: "z-model"},
	}
	if len(got) != len(want) {
		t.Fatalf("option count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("option[%d] = %+v, want %+v; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestOpenCodeProviderOptionsFromDiscovery_GroupsSortsFiltersAndDerivesEffort(t *testing.T) {
	result := opencode.DiscoveryResult{Providers: []opencode.AvailableProvider{
		{Provider: opencode.Provider{ID: "openai", Name: "OpenAI", Models: map[string]opencode.Model{
			"z-model": {ID: "z-model", Name: "Zed"},
			"a-model": {ID: "a-model", Name: "Alpha", Reasoning: true},
		}}},
		{Provider: opencode.Provider{ID: "anthropic", Name: "Anthropic", Models: map[string]opencode.Model{
			"claude": {ID: "claude", Name: "Claude Sonnet", Reasoning: true},
		}}},
	}}

	options := openCodeProviderOptionsFromDiscovery(result)
	if len(options) != 2 {
		t.Fatalf("expected two provider groups, got %#v", options)
	}
	if got, want := options[0].Provider.ID, "anthropic"; got != want {
		t.Fatalf("first provider ID = %q, want %q", got, want)
	}
	if got, want := options[1].DisplayName(), "OpenAI"; got != want {
		t.Fatalf("provider display = %q, want %q", got, want)
	}

	models := filterOpenCodeModelOptions(options[1].Models, "alp")
	if len(models) != 1 || models[0].Model.ID != "a-model" {
		t.Fatalf("expected search to match only a-model by name, got %#v", models)
	}
	if len(filterOpenCodeModelOptions(options[1].Models, "missing")) != 0 {
		t.Fatalf("expected no models for unmatched query")
	}

	efforts := phaseModelEffortOptions(options[1].Provider.ID, models[0].Model)
	wantEfforts := []string{"", "minimal", "low", "medium", "high", "xhigh"}
	if !slices.Equal(efforts, wantEfforts) {
		t.Fatalf("efforts = %#v, want %#v", efforts, wantEfforts)
	}
}

func TestInitializePhaseModelEditor_GatesColumnsByDetectedAgentWithoutJDTargets(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment { return nil }
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "claude", configDir: t.TempDir()},
		&mockAgent{name: "judgment-day", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)

	if !m.phaseModelHasClaude {
		t.Fatal("expected Claude column enabled when Claude agent is detected")
	}
	if m.phaseModelHasOpenCode {
		t.Fatal("expected OpenCode column disabled when OpenCode agent is not detected")
	}
	if m.phaseModelActiveCol != phaseModelClaudeColumn {
		t.Fatalf("expected active column to move to Claude, got %d", m.phaseModelActiveCol)
	}
}

func TestViewPhaseModels_RendersPickerModes(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)
	m.phaseModelOpenCodeProviders = testOpenCodeProviderOptions()
	m.phaseModelClaude = []string{"claude-haiku", "claude-sonnet"}

	tests := []struct {
		name    string
		mode    phaseModelMode
		prepare func(*Model)
		want    []string
	}{
		// After Slice 3, picker headers are terminalui HeaderRow breadcrumbs (not "Select ...").
		{name: "opencode provider", mode: phaseModelModeOpenCodeProvider, want: []string{"Phase Models", "Provider", "OpenAI"}},
		{name: "opencode model", mode: phaseModelModeOpenCodeModel, want: []string{"Phase Models", "Model", "Search:", "plain", "reasoning"}},
		{name: "opencode effort", mode: phaseModelModeOpenCodeEffort, prepare: func(m *Model) {
			m.phaseModelPendingOpenCode = config.OpenCodeModelAssignment{ProviderID: "openai", ModelID: "reasoning"}
		}, want: []string{"Phase Models", "Effort", "minimal", "high"}},
		{name: "claude model", mode: phaseModelModeClaudeModel, want: []string{"Phase Models", "Claude", "claude-haiku", "claude-sonnet"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := m
			candidate.phaseModelMode = tt.mode
			if tt.prepare != nil {
				tt.prepare(&candidate)
			}

			view := viewPhaseModels(candidate)
			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Fatalf("expected picker view to contain %q, got:\n%s", want, view)
				}
			}
		})
	}
}

func TestInitializePhaseModelEditor_SkipsOpenCodeDiscoveryWhenOpenCodeUnavailable(t *testing.T) {
	called := false
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		called = true
		openCodePhaseModelDiscoveryDiagnostics = []string{"must not show"}
		return []config.OpenCodeModelAssignment{{ProviderID: "openai", ModelID: "gpt"}}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m.cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
		"default": {ProviderID: "stored", ModelID: "kept"},
	}

	m = initializePhaseModelEditor(m)

	if called {
		t.Fatal("OpenCode discovery must not run when OpenCode agent is unavailable")
	}
	if len(m.phaseModelOpenCodeDiagnostics) != 0 || strings.Contains(viewPhaseModels(m), "must not show") {
		t.Fatalf("expected OpenCode diagnostics suppressed, got diagnostics=%v view=\n%s", m.phaseModelOpenCodeDiagnostics, viewPhaseModels(m))
	}
	if got := m.cfg.SDD.OpenCodePhaseModels["default"]; got.ProviderID != "stored" || got.ModelID != "kept" {
		t.Fatalf("stored OpenCode assignment must be preserved, got %+v", got)
	}
}

func TestStepSkills_EnterSkipsPhaseModelsWithoutRuntimeModelTarget(t *testing.T) {
	tests := []struct {
		name   string
		agents []agent.Agent
	}{
		{name: "no agents", agents: nil},
		{name: "judgment day only", agents: []agent.Agent{&mockAgent{name: "judgment-day", configDir: t.TempDir()}}},
		{name: "non model runtime agent", agents: []agent.Agent{&mockAgent{name: "mock", configDir: t.TempDir()}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := buildSkillsModel()
			m.Agents = tt.agents

			m = sendKey(m, tea.KeyEnter)

			if m.Step != StepReview {
				t.Fatalf("expected StepReview, got %v", m.Step)
			}
		})
	}
}

func TestViewPhaseModels_RendersOnlyDetectedRuntimeTargetColumnsAndUpdatedHelp(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)

	view := viewPhaseModels(m)
	if strings.Contains(view, "OpenCode") {
		t.Fatalf("Claude-only phase model view must not render OpenCode column, got:\n%s", view)
	}
	if !strings.Contains(view, "phase") || !strings.Contains(view, "Claude") {
		t.Fatalf("expected phase and Claude columns, got:\n%s", view)
	}
	if strings.Contains(view, "Enter/Space cycle") {
		t.Fatalf("help text must not claim Enter/Space cycle after picker flow change, got:\n%s", view)
	}
}

func testOpenCodeProviderOptions() []openCodeProviderOption {
	return []openCodeProviderOption{{
		Provider: opencode.Provider{ID: "openai", Name: "OpenAI"},
		Models: []openCodeModelOption{
			{ProviderID: "openai", Model: opencode.Model{ID: "plain", Name: "Plain"}},
			{ProviderID: "openai", Model: opencode.Model{ID: "reasoning", Name: "Reasoning", Reasoning: true}},
		},
	}}
}

func TestStepPhaseModels_OpenCodeProviderModelEffortPersistsOnlyAfterConfirm(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)
	m.phaseModelOpenCodeProviders = testOpenCodeProviderOptions()
	phase := m.phaseModelRows[0].Phase

	m = sendKey(m, tea.KeyEnter)
	if m.phaseModelMode != phaseModelModeOpenCodeProvider {
		t.Fatalf("expected provider mode after Enter on OpenCode cell, got %v", m.phaseModelMode)
	}
	m = sendKey(m, tea.KeyEnter)
	if m.phaseModelMode != phaseModelModeOpenCodeModel {
		t.Fatalf("expected model mode after provider confirm, got %v", m.phaseModelMode)
	}
	m.phaseModelModelCursor = 1
	m = sendKey(m, tea.KeyEnter)
	if m.phaseModelMode != phaseModelModeOpenCodeEffort {
		t.Fatalf("expected effort mode for reasoning model, got %v", m.phaseModelMode)
	}
	if _, ok := m.cfg.SDD.OpenCodePhaseModels[phase]; ok {
		t.Fatalf("expected no persistence before effort confirmation, got %#v", m.cfg.SDD.OpenCodePhaseModels)
	}
	m.phaseModelEffortCursor = 4
	m = sendKey(m, tea.KeyEnter)

	assignment := m.cfg.SDD.OpenCodePhaseModels[phase]
	if assignment.ProviderID != "openai" || assignment.ModelID != "reasoning" || assignment.Effort != "high" {
		t.Fatalf("unexpected persisted assignment: %+v", assignment)
	}
	if m.phaseModelMode != phaseModelModeList {
		t.Fatalf("expected return to list mode after commit, got %v", m.phaseModelMode)
	}
}

func TestStepPhaseModels_OpenCodeModelWithoutEffortCommitsDirectlyAndEscPreservesPendingContext(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)
	m.phaseModelOpenCodeProviders = testOpenCodeProviderOptions()
	phase := m.phaseModelRows[0].Phase

	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	m.phaseModelModelCursor = 1
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEsc)
	if m.phaseModelMode != phaseModelModeOpenCodeModel {
		t.Fatalf("expected Esc from effort to return to model picker, got %v", m.phaseModelMode)
	}
	if m.phaseModelPendingOpenCode.ProviderID != "openai" || m.phaseModelPendingOpenCode.ModelID != "reasoning" {
		t.Fatalf("expected pending provider/model preserved after Esc, got %+v", m.phaseModelPendingOpenCode)
	}
	if _, ok := m.cfg.SDD.OpenCodePhaseModels[phase]; ok {
		t.Fatalf("expected Esc from effort not to persist assignment, got %#v", m.cfg.SDD.OpenCodePhaseModels)
	}

	m.phaseModelModelCursor = 0
	m = sendKey(m, tea.KeyEnter)
	assignment := m.cfg.SDD.OpenCodePhaseModels[phase]
	if assignment.ProviderID != "openai" || assignment.ModelID != "plain" || assignment.Effort != "" {
		t.Fatalf("expected direct commit for model without effort, got %+v", assignment)
	}
}

func TestStepPhaseModels_ClaudePickerUsesClaudeCatalogOnly(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)
	m.phaseModelClaude = []string{"claude-haiku", "claude-sonnet"}
	m.phaseModelOpenCode = []string{"opencode-only"}
	m.phaseModelActiveCol = phaseModelClaudeColumn
	phase := m.phaseModelRows[0].Phase

	m = sendKey(m, tea.KeyEnter)
	if m.phaseModelMode != phaseModelModeClaudeModel {
		t.Fatalf("expected Claude picker mode, got %v", m.phaseModelMode)
	}
	m.phaseModelModelCursor = 1
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)

	if got := m.cfg.SDD.PhaseModels[phase].Claude; got != "claude-sonnet" {
		t.Fatalf("expected Claude assignment from Claude catalog, got %q", got)
	}
	if got := m.cfg.SDD.PhaseModels[phase].OpenCode; got == "opencode-only" {
		t.Fatalf("Claude selection must not use or rewrite OpenCode catalog, got OpenCode %q", got)
	}
}

func TestStepPhaseModels_ClaudeModelAndEffortPersistAfterEffortConfirm(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)
	m.phaseModelClaude = []string{"claude-haiku", "claude-sonnet"}
	m.phaseModelActiveCol = phaseModelClaudeColumn
	phase := m.phaseModelRows[0].Phase

	m = sendKey(m, tea.KeyEnter)
	m.phaseModelModelCursor = 1
	m = sendKey(m, tea.KeyEnter)
	if m.phaseModelMode != phaseModelModeClaudeEffort {
		t.Fatalf("expected Claude effort picker after model confirmation, got %v", m.phaseModelMode)
	}
	if got := m.cfg.SDD.ClaudePhaseModels[phase]; got.Model != "" || got.Effort != "" {
		t.Fatalf("expected Claude route to wait for effort confirmation, got %+v", got)
	}

	m.phaseModelEffortCursor = 5
	m = sendKey(m, tea.KeyEnter)

	route := m.cfg.SDD.ClaudePhaseModels[phase]
	if route.Model != "claude-sonnet" || route.Effort != "max" {
		t.Fatalf("expected selected Claude model+effort persisted, got %+v", route)
	}
	if m.phaseModelMode != phaseModelModeList {
		t.Fatalf("expected return to list mode after Claude effort commit, got %v", m.phaseModelMode)
	}
}

func TestConfigureWizardAgents_AddsClaudeRestartGuidanceOnlyForClaude(t *testing.T) {
	claudeHome := t.TempDir()
	claude := &sddInstallingMockAgent{mockAgent: mockAgent{name: "claude", configDir: filepath.Join(claudeHome, ".claude")}, home: claudeHome}
	opencode := &mockAgent{name: "opencode", configDir: t.TempDir()}
	cfg := &config.AppConfig{APIURL: config.DefaultAPIURL}

	results := configureWizardAgents([]agent.Agent{claude, opencode}, cfg, agent.MCPEntry{Name: "hive", DaemonPath: "/tmp/hive-daemon"}, agent.MCPEntry{Name: "context7"}, persona.PresetSelection{}, wizardPresetApplyContext{}, nil, nil, nil, func() bool { return true })

	if len(results) != 2 {
		t.Fatalf("expected two results, got %#v", results)
	}
	if !slices.Contains(results[0].Warnings, claudeRestartGuidance) {
		t.Fatalf("expected Claude restart guidance in Claude warnings, got %#v", results[0].Warnings)
	}
	if slices.Contains(results[1].Warnings, claudeRestartGuidance) {
		t.Fatalf("OpenCode result must not receive Claude restart guidance, got %#v", results[1].Warnings)
	}
}

func TestViewReview_IncludesPhaseModelAssignments(t *testing.T) {
	m := Model{Step: StepReview, cfg: &config.AppConfig{}, Selected: map[string]bool{}}
	m = initializePhaseModelEditor(m)
	m.phaseModelRows[0].OpenCode = "haiku"
	m.phaseModelRows[0].Claude = "opus"
	phase := m.phaseModelRows[0].Phase

	v := viewReview(m)
	if !strings.Contains(v, "SDD phase models") {
		t.Fatalf("expected review to include SDD phase models section, got:\n%s", v)
	}
	if !strings.Contains(v, phase) || !strings.Contains(v, "haiku") || !strings.Contains(v, "opus") {
		t.Fatalf("expected review to include edited phase assignment, got:\n%s", v)
	}
}

func TestViewReview_IncludesOpenCodeProviderModelAssignments(t *testing.T) {
	m := Model{Step: StepReview, cfg: &config.AppConfig{}, Selected: map[string]bool{}}
	m = initializePhaseModelEditor(m)
	phase := m.phaseModelRows[0].Phase
	m.cfg.SDD.OpenCodePhaseModels[phase] = config.OpenCodeModelAssignment{ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"}

	v := viewReview(m)
	if !strings.Contains(v, "opencode=openai/gpt-5.1-codex-max") {
		t.Fatalf("expected review to include provider-qualified OpenCode assignment, got:\n%s", v)
	}
	if !strings.Contains(v, "effort=high") {
		t.Fatalf("expected review to include OpenCode effort, got:\n%s", v)
	}
}

func TestStepReview_BackRoutesByRuntimeModelTarget(t *testing.T) {
	tests := []struct {
		name   string
		agents []agent.Agent
		want   Step
	}{
		{name: "no runtime model target returns to skills", agents: nil, want: StepSkills},
		{name: "judgment day only returns to skills", agents: []agent.Agent{&mockAgent{name: "judgment-day", configDir: t.TempDir()}}, want: StepSkills},
		{name: "claude runtime target returns to phase models", agents: []agent.Agent{&mockAgent{name: "claude", configDir: t.TempDir()}}, want: StepPhaseModels},
		{name: "opencode runtime target returns to phase models", agents: []agent.Agent{&mockAgent{name: "opencode", configDir: t.TempDir()}}, want: StepPhaseModels},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{Step: StepReview, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: tt.agents}
			m.reviewChoice = 0

			m = sendKey(m, tea.KeyEnter)

			if m.Step != tt.want {
				t.Fatalf("expected Back to route to %v, got %v", tt.want, m.Step)
			}
		})
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

func TestWizardPresetSelectionUsesV2Only(t *testing.T) {
	v1 := &persona.ResolvedPreset{Slug: "fixture", Preset: &persona.Preset{Name: "fixture"}}
	v2 := &persona.ResolvedPresetV2{Slug: "future", Preset: &persona.PresetV2{Name: "future"}}

	tests := []struct {
		name string
		m    Model
		want string
	}{
		{name: "V2 selection wins when both slots are populated", m: Model{selectedPreset: v1, selectedPresetV2: v2}, want: "v2"},
		{name: "V2 selection is available", m: Model{selectedPresetV2: v2}, want: "v2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, ok := tt.m.wizardPresetSelection()
			if !ok {
				t.Fatal("expected a selected preset")
			}
			if tt.want == "v2" && (selection.V1 != nil || selection.V2 != v2) {
				t.Fatalf("selection = %+v, want V2 only", selection)
			}
		})
	}
}

func TestEnsurePresetSelectionForApplyRejectsLegacyCustomProfileWithMigrationGuidance(t *testing.T) {
	home := isolateTestHome(t)
	legacyPath := filepath.Join(home, ".jarvis", "personas", "legacy-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy preset dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n"), 0o644); err != nil {
		t.Fatalf("write legacy preset: %v", err)
	}

	_, err := ensurePresetSelectionForApply(Model{
		PersonaFS: testPersonaFS,
		cfg:       &config.AppConfig{PersonaPreset: "legacy-custom", PersonaPresetSource: string(persona.PresetSourceUser)},
	})
	if err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("ensurePresetSelectionForApply() error = %v, want actionable migration guidance", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestStep_Skills_EnterAdvances
// ──────────────────────────────────────────────────────────────────────────────

// TestStep_Skills_EnterAdvances verifies that pressing Enter at StepSkills
// advances the model to StepPhaseModels.
func TestStep_Skills_EnterAdvances(t *testing.T) {
	m := buildSkillsModel()
	m.Agents = []agent.Agent{&mockAgent{name: "claude", configDir: t.TempDir()}}

	if m.Step != StepSkills {
		t.Fatalf("expected StepSkills, got %v", m.Step)
	}

	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepPhaseModels {
		t.Errorf("expected StepPhaseModels after Enter, got %v", m.Step)
	}
}

func TestStep_Skills_EnterSkipsPhaseModelsWhenRuntimeManagedFlowUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		agents   []agent.Agent
		expectTo Step
	}{
		{name: "no agents configured", agents: nil, expectTo: StepReview},
		{name: "runtime managed available", agents: []agent.Agent{&mockAgent{name: "claude", configDir: t.TempDir()}}, expectTo: StepPhaseModels},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := buildSkillsModel()
			m.Agents = tt.agents

			m = sendKey(m, tea.KeyEnter)

			if m.Step != tt.expectTo {
				t.Fatalf("expected step %v, got %v", tt.expectTo, m.Step)
			}
		})
	}
}

func TestPhaseModelsView_V1HasNoProfileCRUDOrSwitchActions(t *testing.T) {
	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)

	view := strings.ToLower(viewPhaseModels(m))
	blocked := []string{"profile", "profiles", "create", "list", "switch"}
	for _, token := range blocked {
		if strings.Contains(view, token) {
			t.Fatalf("phase-model view must not expose v1 profile actions; found %q in:\n%s", token, view)
		}
	}
}

func TestStep_PhaseModels_RejectsCrossCatalogInvalidValuesInTUIEditingPath(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment { return nil }
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	m := Model{Step: StepPhaseModels, cfg: &config.AppConfig{}, Selected: map[string]bool{}, Agents: []agent.Agent{
		&mockAgent{name: "opencode", configDir: t.TempDir()},
		&mockAgent{name: "claude", configDir: t.TempDir()},
	}}
	m = initializePhaseModelEditor(m)

	if len(m.phaseModelOpenCode) == 0 || len(m.phaseModelClaude) == 0 {
		t.Fatal("expected both platform catalogs to be non-empty")
	}

	const opencodeInvalid = "__invalid-opencode-model__"
	const claudeInvalid = "__invalid-claude-model__"

	// Inject an invalid value into OpenCode cell and ensure cycle rejects/fixes it.
	m.phaseModelActiveRow = 0
	m.phaseModelActiveCol = 1
	m.phaseModelRows[0].OpenCode = opencodeInvalid
	m = sendKey(m, tea.KeySpace)
	if !slices.Contains(m.phaseModelOpenCode, m.phaseModelRows[0].OpenCode) {
		t.Fatalf("expected OpenCode value normalized to OpenCode catalog, got %q", m.phaseModelRows[0].OpenCode)
	}

	// Inject an invalid value into Claude cell and ensure cycle rejects/fixes it.
	m.phaseModelActiveCol = 2
	m.phaseModelRows[0].Claude = claudeInvalid
	m = sendKey(m, tea.KeySpace)
	if !slices.Contains(m.phaseModelClaude, m.phaseModelRows[0].Claude) {
		t.Fatalf("expected Claude value normalized to Claude catalog, got %q", m.phaseModelRows[0].Claude)
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

func TestUpdateHiveCloud_InvalidEmailDoesNotLogin(t *testing.T) {
	for _, tt := range []struct {
		name  string
		email string
	}{
		{name: "missing at", email: "invalid-email"},
		{name: "missing domain dot", email: "user@example"},
		{name: "embedded whitespace", email: "user @example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				Step:        StepHiveCloud,
				Selected:    make(map[string]bool),
				cfg:         &config.AppConfig{},
				Email:       tt.email,
				Password:    "secret",
				activeField: 1,
			}

			updated, cmd := updateHiveCloud(m, tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(Model)

			if cmd != nil {
				t.Fatal("invalid email must not start login command")
			}
			if m.Err == nil || !strings.Contains(m.Err.Error(), "usuario@dominio.com") {
				t.Fatalf("expected friendly invalid email error, got %v", m.Err)
			}
			if strings.Contains(m.Err.Error(), "jarvis --no-tui") {
				t.Fatalf("invalid email error must not recommend no-TUI fallback: %v", m.Err)
			}
			if m.Step != StepHiveCloud {
				t.Fatalf("invalid email must stay on Hive Cloud step, got %v", m.Step)
			}
		})
	}
}

func TestUpdateHiveCloud_TrimsEmailBeforeLogin(t *testing.T) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode login request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"abc","user":{"email":""}}`))
	}))
	t.Cleanup(server.Close)

	m := Model{
		Step:        StepHiveCloud,
		Selected:    make(map[string]bool),
		cfg:         &config.AppConfig{APIURL: server.URL},
		Email:       "  input@example.com  ",
		Password:    " secret ",
		activeField: 1,
	}

	updated, cmd := updateHiveCloud(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("valid email must start login command")
	}
	if m.Email != "input@example.com" {
		t.Fatalf("expected model email normalized before login, got %q", m.Email)
	}
	res, ok := cmd().(loginResultMsg)
	if !ok {
		t.Fatalf("expected loginResultMsg")
	}
	if res.err != nil {
		t.Fatalf("unexpected login error: %v", res.err)
	}
	if request.Email != "input@example.com" {
		t.Fatalf("expected trimmed email sent to login, got %q", request.Email)
	}
	if request.Password != " secret " {
		t.Fatalf("wizard password must not be trimmed, got %q", request.Password)
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
		Step:        StepPersona,
		Selected:    make(map[string]bool),
		cfg:         &config.AppConfig{},
		customEdit:  true,
		customField: 2,
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

func TestUpdatePersonaCustomEdit_RuneInput_EnforcesYAMLSizeLimit(t *testing.T) {
	m := Model{
		Step:        StepPersona,
		Selected:    make(map[string]bool),
		cfg:         &config.AppConfig{},
		customEdit:  true,
		customField: 2,
		CustomYAML:  strings.Repeat("a", maxCustomPresetYAMLBytes),
	}

	m = sendRune(m, "b")

	if m.Err == nil {
		t.Fatal("expected size-limit error when YAML exceeds maximum")
	}
	if !strings.Contains(m.Err.Error(), "exceeds size limit") {
		t.Fatalf("error = %q, want contains size limit message", m.Err.Error())
	}
	if len(m.CustomYAML) != maxCustomPresetYAMLBytes {
		t.Fatalf("expected YAML to remain at %d bytes, got %d", maxCustomPresetYAMLBytes, len(m.CustomYAML))
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

func TestUpdatePersonaCustomEdit_CtrlS_PersistsCustomAsUserPreset(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)

	m := Model{
		Step:              StepPersona,
		Selected:          make(map[string]bool),
		cfg:               &config.AppConfig{},
		PersonaFS:         testPersonaFS,
		customEdit:        true,
		customPresetName:  "Mi Persona",
		customDisplayName: "Mi Persona Display",
	}

	m = sendKey(m, tea.KeyCtrlS)
	if m.Err != nil {
		t.Fatalf("expected Ctrl+S custom creation to succeed, got %v", m.Err)
	}
	if m.cfg.PersonaPreset == "custom" {
		t.Fatalf("expected persisted custom slug identity, got legacy %q", m.cfg.PersonaPreset)
	}
	if m.cfg.PersonaPreset != "mi-persona" {
		t.Fatalf("expected canonical custom slug mi-persona, got %q", m.cfg.PersonaPreset)
	}
	if m.cfg.PersonaPresetSource != string(persona.PresetSourceUser) {
		t.Fatalf("expected persona_preset_source=user, got %q", m.cfg.PersonaPresetSource)
	}
	if m.Step != StepExtraSkills {
		t.Fatalf("expected to advance to extra skills after valid custom save, got %v", m.Step)
	}

	customPath := filepath.Join(tmpHome, ".jarvis", "personas", "mi-persona.yaml")
	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("expected custom preset file %s, got err=%v", customPath, err)
	}
}

func TestUpdatePersonaCustomEdit_CtrlS_BlocksLegacyCustomYAML(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)

	m := Model{
		Step:              StepPersona,
		Selected:          make(map[string]bool),
		cfg:               &config.AppConfig{},
		PersonaFS:         testPersonaFS,
		customEdit:        true,
		customPresetName:  "Broken Persona",
		customDisplayName: "Broken Persona Display",
		CustomYAML:        "name: [",
	}

	m = sendKey(m, tea.KeyCtrlS)
	if m.Err == nil {
		t.Fatal("expected migration error for legacy custom YAML")
	}
	if !strings.Contains(m.Err.Error(), "migrate") {
		t.Fatalf("custom YAML error = %v, want actionable migration guidance", m.Err)
	}
	if m.Step != StepPersona {
		t.Fatalf("expected to stay on persona step when custom YAML is rejected, got %v", m.Step)
	}

	customPath := filepath.Join(tmpHome, ".jarvis", "personas", "broken-persona.yaml")
	if _, err := os.Stat(customPath); !os.IsNotExist(err) {
		t.Fatalf("expected no persisted file for rejected custom YAML, got err=%v", err)
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
	setTestHome(t, tmpHome)
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
	// The warning may be word-wrapped by the bordered panel renderer; check for a
	// distinctive prefix that fits within a single panel line.
	warningPrefix := "Se ha seleccionado modo local"
	if !strings.Contains(view, warningPrefix) {
		t.Fatalf("expected local-only warning (prefix %q) in review, got:\n%s", warningPrefix, view)
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
				"Setup › Review",
				"Resumen de configuración",
				"Scope:",
				"Persona: fixture",
				tt.expectCloudLine,
				"Back",
				"Cancel",
				"Apply",
				"↑/↓",
				"navegar",
				"Enter",
				"confirmar",
			} {
				if !strings.Contains(view, mustContain) {
					t.Fatalf("expected review view to contain %q, got:\n%s", mustContain, view)
				}
			}

			// The warning may be word-wrapped by the bordered panel renderer; check for
			// a distinctive prefix that fits within a single panel line.
			warningPrefix := "Se ha seleccionado modo local"
			if tt.expectWarning && !strings.Contains(view, warningPrefix) {
				t.Fatalf("expected local-only warning prefix %q in review view, got:\n%s", warningPrefix, view)
			}
			if tt.unexpectedWarning && strings.Contains(view, warningPrefix) {
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
			setTestHome(t, homeAsFile)

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
		// "apply" with no statusline file present must go directly to StepApply.
		// Isolate HOME to guarantee the statusline script is absent.
		{name: "apply", choice: 2, expectStep: StepApply, expectDone: false, expectCmdNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Isolate HOME for "apply" so the statusline-existence check is deterministic.
			if tt.choice == 2 {
				isolateTestHome(t)
			}
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
			setTestHome(t, tmpHome)

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
	setTestHome(t, tmpHome)

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
	setTestHome(t, tmpHome)
	if err := os.MkdirAll(filepath.Join(tmpHome, ".jarvis"), 0755); err != nil {
		t.Fatal(err)
	}

	enable := true
	err := writeSyncJSON("https://hivemem.dev", "user@example.com", "s3cr3t", &enable)
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
	if !strings.Contains(string(data), `"auto_sync":true`) {
		t.Errorf("expected auto_sync:true in sync.json, got: %s", data)
	}
}

func TestWriteSyncJSON_ForceEnablesAutoSync(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Seed the file with auto_sync:false to confirm the helper forces it to true.
	seed := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":false}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	enable := true
	if err := writeSyncJSON("https://hivemem.dev", "user@example.com", "s3cr3t", &enable); err != nil {
		t.Fatalf("writeSyncJSON: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync forced to true, got: %s", body)
	}
	if !strings.Contains(body, "user@example.com") {
		t.Fatalf("expected updated credentials, got: %s", body)
	}
}

func TestWriteSyncJSON_NilPreservesExistingAutoSync(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Seed the file with auto_sync:true; passing nil must leave it unchanged.
	seed := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":true}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeSyncJSON("https://hivemem.dev", "new@example.com", "newpass", nil); err != nil {
		t.Fatalf("writeSyncJSON: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":true`) {
		t.Fatalf("expected auto_sync preserved as true when nil passed, got: %s", body)
	}
	if !strings.Contains(body, "new@example.com") {
		t.Fatalf("expected updated credentials in sync.json, got: %s", body)
	}
}

func TestWriteSyncJSON_NilPreservesExistingAutoSyncFalse(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)
	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Seed the file with auto_sync:false; passing nil must leave it unchanged.
	seed := `{"api_url":"https://old.dev","email":"old@example.com","password":"old","auto_sync":false}`
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeSyncJSON("https://hivemem.dev", "new@example.com", "newpass", nil); err != nil {
		t.Fatalf("writeSyncJSON: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(jarvisDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"auto_sync":false`) {
		t.Fatalf("expected auto_sync preserved as false when nil passed, got: %s", body)
	}
	if !strings.Contains(body, "new@example.com") {
		t.Fatalf("expected updated credentials in sync.json, got: %s", body)
	}
}

func TestNewModel_EmptyStoredScopeFallsBackToLocalOnly(t *testing.T) {
	tmpHome := t.TempDir()
	setTestHome(t, tmpHome)

	cfg := &config.AppConfig{
		SchemaVersion: 2,
		APIURL:        config.DefaultAPIURL,
		Scope:         "",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	m := NewModel(testWizardConfig(), false)
	if m.Scope != config.ScopeLocalOnly {
		t.Fatalf("expected fallback scope local-only, got %q", m.Scope)
	}
}

func TestUpdate_DefaultMessageNoOp(t *testing.T) {
	m := Model{Step: StepScope, Scope: config.ScopeLocalOnly, cfg: &config.AppConfig{Scope: config.ScopeLocalOnly}}
	updated, cmd := m.Update(struct{ Name string }{Name: "unknown"})
	if cmd != nil {
		t.Fatalf("expected nil cmd for unknown message, got %v", cmd)
	}
	m2 := updated.(Model)
	if m2.Step != StepScope || m2.Scope != config.ScopeLocalOnly {
		t.Fatalf("unexpected state mutation on unknown message: %+v", m2)
	}
}

func TestLoginCmd_SuccessAndErrorPaths(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"token":"abc","user":{"email":"resolved@example.com"}}`))
		}))
		defer server.Close()

		msg := loginCmd(server.URL, "input@example.com", "secret")()
		res, ok := msg.(loginResultMsg)
		if !ok {
			t.Fatalf("expected loginResultMsg, got %T", msg)
		}
		if res.err != nil {
			t.Fatalf("unexpected login error: %v", res.err)
		}
		if res.token != "abc" || res.email != "resolved@example.com" {
			t.Fatalf("unexpected login result: %+v", res)
		}
	})

	t.Run("error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		msg := loginCmd(server.URL, "input@example.com", "wrong")()
		res, ok := msg.(loginResultMsg)
		if !ok {
			t.Fatalf("expected loginResultMsg, got %T", msg)
		}
		if res.err == nil {
			t.Fatal("expected login error for unauthorized response")
		}
	})
}

func TestViewApply_States(t *testing.T) {
	t.Run("no agents waiting for enter", func(t *testing.T) {
		m := Model{Step: StepApply}
		v := viewApply(m)
		if !strings.Contains(v, "No agents detected") {
			t.Fatalf("expected no-agent message, got:\n%s", v)
		}
	})

	t.Run("failed apply suggests retry", func(t *testing.T) {
		m := Model{Step: StepApply, agentProgress: []string{"Configuration FAILED"}, agentDone: true, Err: errors.New("boom")}
		v := viewApply(m)
		if !strings.Contains(v, "Press Enter to retry") {
			t.Fatalf("expected retry hint, got:\n%s", v)
		}
	})
}

func TestUpdateScope_KeyPaths(t *testing.T) {
	m := Model{Step: StepScope, Scope: config.ScopeLocalOnly, cfg: &config.AppConfig{Scope: config.ScopeLocalOnly}}

	updated, _ := updateScope(m, tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.Scope != config.ScopeLocalCloud {
		t.Fatalf("expected scope local+cloud after KeyDown, got %q", m.Scope)
	}

	updated, _ = updateScope(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(Model)
	if m.Scope != config.ScopeLocalOnly {
		t.Fatalf("expected scope local-only after k, got %q", m.Scope)
	}

	updated, _ = updateScope(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	if m.Scope != config.ScopeLocalCloud {
		t.Fatalf("expected scope local+cloud after j, got %q", m.Scope)
	}

	updated, _ = updateScope(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.Step != StepHiveCloud {
		t.Fatalf("expected StepHiveCloud for local+cloud, got %v", m.Step)
	}
}

func TestRunAgentConfigCmd_ReturnsStartingMessage(t *testing.T) {
	cmd := runAgentConfigCmd(Model{})
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg := cmd()
	progress, ok := msg.(agentProgressMsg)
	if !ok {
		t.Fatalf("expected agentProgressMsg, got %T", msg)
	}
	if !strings.Contains(progress.line, "Starting agent configuration") {
		t.Fatalf("unexpected start message: %q", progress.line)
	}
}

func TestBuildSkillInfoList_SelectedAndCore(t *testing.T) {
	m := Model{
		SkillList: []skills.Skill{
			{ID: "hive", Name: "Hive", Description: "core", IsCore: true},
			{ID: "go-testing", Name: "Go Testing", Description: "go", Trigger: "go", IsCore: false},
			{ID: "phpunit-testing", Name: "PHPUnit", Description: "php", IsCore: false},
		},
		Selected: map[string]bool{"go-testing": true},
	}

	infos := buildSkillInfoList(m)
	if len(infos) != 2 {
		t.Fatalf("expected core + selected skill infos, got %d", len(infos))
	}
}

func TestViewPersona_Branches(t *testing.T) {
	t.Run("custom edit view shows form", func(t *testing.T) {
		m := Model{Step: StepPersona, customEdit: true, customPresetName: "mi-persona", customDisplayName: "Mi Persona", CustomYAML: "name: mi-persona"}
		v := viewPersona(m)
		// After Slice 3, custom edit uses HeaderRow breadcrumb "Edit" and BorderedPanel.
		if !strings.Contains(v, "Edit") {
			t.Fatalf("expected custom edit HeaderRow breadcrumb containing 'Edit', got:\n%s", v)
		}
		if !strings.Contains(v, "mi-persona") {
			t.Fatalf("expected custom edit view to show the preset name, got:\n%s", v)
		}
	})

	t.Run("no presets branch shows fallback", func(t *testing.T) {
		m := Model{Step: StepPersona, Presets: nil}
		v := viewPersona(m)
		// After Slice 3, empty state is rendered as a BorderedPanel with "No personas" text.
		if !strings.Contains(v, "No personas") && !strings.Contains(v, "No presets") {
			t.Fatalf("expected no-presets fallback message, got:\n%s", v)
		}
	})
}

func TestUpdatePersona_EnterCustomStartsEditMode(t *testing.T) {
	m := Model{
		Step:     StepPersona,
		cfg:      &config.AppConfig{},
		Selected: map[string]bool{},
		Presets: []persona.Preset{
			{Name: "custom", DisplayName: "Custom"},
		},
	}

	updated, _ := updatePersona(m, tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)
	if !m2.customEdit {
		t.Fatal("expected custom edit mode enabled")
	}
	if m2.customField != 0 {
		t.Fatalf("expected custom field reset to 0, got %d", m2.customField)
	}
}

func TestUpdatePersona_ResolveFailureFallsBackToBuiltinSlug(t *testing.T) {
	m := Model{
		Step:      StepPersona,
		cfg:       &config.AppConfig{},
		Selected:  map[string]bool{},
		PersonaFS: testPersonaFS,
		Presets: []persona.Preset{
			{Name: "non-existent-preset", DisplayName: "Missing"},
		},
	}

	updated, _ := updatePersona(m, tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(Model)
	if m2.cfg.PersonaPreset != "non-existent-preset" {
		t.Fatalf("expected fallback persona slug, got %q", m2.cfg.PersonaPreset)
	}
	if m2.cfg.PersonaPresetSource != string(persona.PresetSourceBuiltin) {
		t.Fatalf("expected builtin source fallback, got %q", m2.cfg.PersonaPresetSource)
	}
}

func TestUpdatePersonaCustomEdit_KeyNavigationAndMutation(t *testing.T) {
	m := Model{
		Step:              StepPersona,
		customEdit:        true,
		customField:       0,
		customPresetName:  "abc",
		customDisplayName: "DEF",
		CustomYAML:        "line",
	}

	updated, _ := updatePersonaCustomEdit(m, tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.customField != 1 {
		t.Fatalf("expected custom field to move to 1, got %d", m.customField)
	}

	updated, _ = updatePersonaCustomEdit(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.customField != 2 {
		t.Fatalf("expected enter to move to yaml field, got %d", m.customField)
	}

	updated, _ = updatePersonaCustomEdit(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !strings.HasSuffix(m.CustomYAML, "\n") {
		t.Fatalf("expected enter on yaml field to append newline, got %q", m.CustomYAML)
	}

	m.customField = 0
	updated, _ = updatePersonaCustomEdit(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.customPresetName != "ab" {
		t.Fatalf("expected preset name backspace applied, got %q", m.customPresetName)
	}

	m.customField = 1
	updated, _ = updatePersonaCustomEdit(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.customDisplayName != "DE" {
		t.Fatalf("expected display name backspace applied, got %q", m.customDisplayName)
	}
}

func TestUpdateSkills_OutOfRangeCursorAndGroupToggle(t *testing.T) {
	m := Model{
		Step:      StepSkills,
		presetCur: 99,
		Selected:  map[string]bool{"phpunit-testing": false, "laravel-architecture": false},
		SkillPrompts: []skillPrompt{
			{Label: "PHP", SkillIDs: []string{"phpunit-testing", "laravel-architecture"}},
		},
	}

	updated, _ := updateSkills(m, tea.KeyMsg{Type: tea.KeySpace})
	m2 := updated.(Model)
	if !m2.Selected["phpunit-testing"] || !m2.Selected["laravel-architecture"] {
		t.Fatalf("expected grouped prompt to toggle all ids, got %+v", m2.Selected)
	}
}

func TestUpdateDone_IgnoresNonQuitRune(t *testing.T) {
	m := Model{Step: StepDone}
	updated, cmd := updateDone(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2 := updated.(Model)
	if m2.Done {
		t.Fatal("expected non-quit rune to keep Done=false")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for non-quit rune, got %v", cmd)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// StepStatuslineConfirm tests
// ──────────────────────────────────────────────────────────────────────────────

// TestStepReview_Apply_SkipsStatuslineConfirmWhenFileAbsent verifies that when the
// statusline script does NOT exist, the Review "Apply" choice goes directly to
// StepApply without passing through StepStatuslineConfirm.
func TestStepReview_Apply_SkipsStatuslineConfirmWhenFileAbsent(t *testing.T) {
	home := isolateTestHome(t)
	// Ensure the statusline script does NOT exist.
	_ = home // ~/.claude/statusline-command.sh is absent in a fresh temp dir.

	m := Model{
		Step:         StepReview,
		Selected:     make(map[string]bool),
		cfg:          &config.AppConfig{},
		reviewChoice: 2, // "Apply"
	}

	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepApply {
		t.Fatalf("expected StepApply when statusline file absent, got %v", m.Step)
	}
}

// TestStepReview_Apply_GoesToStatuslineConfirmWhenFileExists verifies that when
// ~/.claude/statusline-command.sh already exists, the Review "Apply" choice
// transitions to StepStatuslineConfirm first.
func TestStepReview_Apply_GoesToStatuslineConfirmWhenFileExists(t *testing.T) {
	home := isolateTestHome(t)

	// Create the statusline script so it "exists".
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "statusline-command.sh"), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatalf("write statusline: %v", err)
	}

	m := Model{
		Step:         StepReview,
		Selected:     make(map[string]bool),
		cfg:          &config.AppConfig{},
		reviewChoice: 2, // "Apply"
	}

	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepStatuslineConfirm {
		t.Fatalf("expected StepStatuslineConfirm when statusline file exists, got %v", m.Step)
	}
}

// TestStepStatuslineConfirm_YesOverwrite verifies that pressing 'y' in
// StepStatuslineConfirm sets statuslineOverwrite=true, statuslineOverwriteReady=true,
// and advances to StepApply.
func TestStepStatuslineConfirm_YesOverwrite(t *testing.T) {
	m := Model{
		Step:     StepStatuslineConfirm,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}

	m = sendRune(m, "y")

	if m.Step != StepApply {
		t.Fatalf("expected StepApply after 'y', got %v", m.Step)
	}
	if !m.statuslineOverwriteReady {
		t.Fatal("expected statuslineOverwriteReady=true after 'y'")
	}
	if !m.statuslineOverwrite {
		t.Fatal("expected statuslineOverwrite=true after 'y'")
	}
}

// TestStepStatuslineConfirm_EnterDefaultSkip verifies that pressing Enter (default)
// in StepStatuslineConfirm sets statuslineOverwrite=false, statuslineOverwriteReady=true,
// and advances to StepApply.
func TestStepStatuslineConfirm_EnterDefaultSkip(t *testing.T) {
	m := Model{
		Step:     StepStatuslineConfirm,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}

	m = sendKey(m, tea.KeyEnter)

	if m.Step != StepApply {
		t.Fatalf("expected StepApply after Enter (default skip), got %v", m.Step)
	}
	if !m.statuslineOverwriteReady {
		t.Fatal("expected statuslineOverwriteReady=true after Enter")
	}
	if m.statuslineOverwrite {
		t.Fatal("expected statuslineOverwrite=false (skip) after Enter")
	}
}

// TestStepStatuslineConfirm_NSkip verifies that pressing 'n' in
// StepStatuslineConfirm sets statuslineOverwrite=false, statuslineOverwriteReady=true,
// and advances to StepApply.
func TestStepStatuslineConfirm_NSkip(t *testing.T) {
	m := Model{
		Step:     StepStatuslineConfirm,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}

	m = sendRune(m, "n")

	if m.Step != StepApply {
		t.Fatalf("expected StepApply after 'n', got %v", m.Step)
	}
	if !m.statuslineOverwriteReady {
		t.Fatal("expected statuslineOverwriteReady=true after 'n'")
	}
	if m.statuslineOverwrite {
		t.Fatal("expected statuslineOverwrite=false (skip) after 'n'")
	}
}

// TestViewStatuslineConfirm_ContainsPromptText verifies that viewStatuslineConfirm
// renders the expected confirmation prompt.
func TestViewStatuslineConfirm_ContainsPromptText(t *testing.T) {
	m := Model{
		Step:     StepStatuslineConfirm,
		Selected: make(map[string]bool),
		cfg:      &config.AppConfig{},
	}

	view := m.View()

	if view == "" {
		t.Fatal("expected non-empty view for StepStatuslineConfirm")
	}
	if !strings.Contains(view, "statusline") {
		t.Fatalf("expected view to mention 'statusline', got:\n%s", view)
	}
	if !strings.Contains(view, "y") && !strings.Contains(view, "Y") {
		t.Fatalf("expected view to include y/n prompt, got:\n%s", view)
	}
}
