package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

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
