package tui

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/projectregistry"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

// embeddedTestPersonaFS and testSkillsFS embed the minimal fixture files used
// exclusively by tests in this package.
//
//go:embed embed/personas embed/personas-v2
var embeddedTestPersonaFS embed.FS

var testPersonaFS fs.FS = v2CatalogTestFS{embeddedTestPersonaFS}

type v2CatalogTestFS struct {
	fs.FS
}

func (fsys v2CatalogTestFS) Open(name string) (fs.File, error) {
	if name == "embed/personas" || strings.HasPrefix(name, "embed/personas/") {
		name = strings.Replace(name, "embed/personas", "embed/personas-v2", 1)
	}
	return fsys.FS.Open(name)
}

//go:embed embed/skills
var testSkillsFS embed.FS

// testWizardConfig returns a WizardConfig backed by the test fixture FSes.
func testWizardConfig() WizardConfig {
	return WizardConfig{
		PersonaFS: testPersonaFS,
		SkillsFS:  testSkillsFS,
	}
}

func remainingPhasePromptNewlines() string {
	return strings.Repeat("\n", 3*(len(sddruntime.DefaultContract().Phases)-1))
}

func phaseEditorPromptNewlinesBefore(target string) string {
	for i, phase := range sddruntime.DefaultContract().Phases {
		if phase == target {
			return strings.Repeat("\n", 3*i)
		}
	}
	return ""
}

func phaseEditorPromptNewlinesAfter(target string) string {
	phases := sddruntime.DefaultContract().Phases
	for i, phase := range phases {
		if phase == target {
			return strings.Repeat("\n", 3*(len(phases)-i-1))
		}
	}
	return ""
}

func TestRunNoTUI_RefreshesProjectRegistryAfterSuccessfulApplyAndPrintsWarnings(t *testing.T) {
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")
	projectRoot := t.TempDir()

	called := false
	originalRefresh := refreshProjectSkillRegistry
	refreshProjectSkillRegistry = func(ctx context.Context, opts projectregistry.RefreshOptions) (projectregistry.Result, error) {
		called = true
		if opts.CWD != projectRoot {
			t.Fatalf("refresh cwd = %q, want %q", opts.CWD, projectRoot)
		}
		if _, err := os.Stat(filepath.Join(tmpHome, ".jarvis", "config.yaml")); err != nil {
			t.Fatalf("project registry refresh should run after config save, got stat err=%v", err)
		}
		return projectregistry.Result{Warnings: []projectregistry.Warning{{Message: "legacy registry imported", Path: filepath.Join(projectRoot, ".atl", "skill-registry.md")}}}, nil
	}
	t.Cleanup(func() { refreshProjectSkillRegistry = originalRefresh })

	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	wcfg := testWizardConfig()
	wcfg.ProjectCWD = projectRoot
	if err := runNoTUI(wcfg, strings.NewReader("\n\nyes\n")); err != nil {
		t.Fatalf("runNoTUI: %v", err)
	}

	if !called {
		t.Fatal("expected project registry refresh to run after no-TUI apply")
	}
	if !strings.Contains(output.String(), "Project skill registry warning: legacy registry imported") {
		t.Fatalf("expected non-blocking project registry warning in output, got:\n%s", output.String())
	}
}

func TestRunNoTUI_ProjectRegistryNonProjectFailureIsWarningOnly(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")
	projectRoot := t.TempDir()

	originalRefresh := refreshProjectSkillRegistry
	refreshProjectSkillRegistry = func(context.Context, projectregistry.RefreshOptions) (projectregistry.Result, error) {
		return projectregistry.Result{}, projectregistry.ErrNotGitWorktree
	}
	t.Cleanup(func() { refreshProjectSkillRegistry = originalRefresh })

	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	wcfg := testWizardConfig()
	wcfg.ProjectCWD = projectRoot
	err := runNoTUI(wcfg, strings.NewReader("\n\nyes\n"))

	if err != nil {
		t.Fatalf("runNoTUI returned error for non-project refresh failure: %v", err)
	}
	if !strings.Contains(output.String(), "Project skill registry warning: not a git worktree") {
		t.Fatalf("expected non-project registry warning in output, got:\n%s", output.String())
	}
}

func TestRunNoTUI_ProjectRegistryWriteFailureIsBlocking(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")
	projectRoot := t.TempDir()

	originalRefresh := refreshProjectSkillRegistry
	refreshProjectSkillRegistry = func(context.Context, projectregistry.RefreshOptions) (projectregistry.Result, error) {
		return projectregistry.Result{}, errors.New("write skill registry: finalize registry: permission denied")
	}
	t.Cleanup(func() { refreshProjectSkillRegistry = originalRefresh })

	wcfg := testWizardConfig()
	wcfg.ProjectCWD = projectRoot
	err := runNoTUI(wcfg, strings.NewReader("\n\nyes\n"))

	if err == nil {
		t.Fatal("expected blocking registry write failure from runNoTUI")
	}
	if !strings.Contains(err.Error(), "project skill registry refresh failed") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected blocking registry failure error, got: %v", err)
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
	tmpHome := isolateTestHome(t)
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

func TestNewModel_NoTUIFallbackStartsWizardNotCockpit(t *testing.T) {
	m := NewModel(testWizardConfig(), true)

	if m.Screen != ScreenWizard {
		t.Fatalf("expected no-TUI fallback model to stay on wizard screen, got %v", m.Screen)
	}
	if m.Step != StepScope {
		t.Fatalf("expected no-TUI fallback to start at StepScope, got %v", m.Step)
	}
	if strings.Contains(m.View(), "Install/Reconfigure") {
		t.Fatalf("no-TUI fallback must not render cockpit menu, got:\n%s", m.View())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestRunNoTUI_SelectsSkill
// ──────────────────────────────────────────────────────────────────────────────

// TestRunNoTUI_SelectsSkill verifies that answering 'y' for the optional skill
// installs it (no crash, no error).
func TestRunNoTUI_SelectsSkill(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")

	// scope=default, persona=default, fixture-skill=yes, apply=yes
	input := strings.NewReader("\n\nyes\n")

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI with skill selected: %v", err)
	}
}

func TestRunNoTUI_RerunKeepsExistingSelectionsOnBlankInput(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")

	seed := &config.AppConfig{
		SchemaVersion:    2,
		APIURL:           config.DefaultAPIURL,
		PersonaPreset:    "second",
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
	if loaded.PersonaPreset != "second" {
		t.Fatalf("expected persona preset to remain second, got %q", loaded.PersonaPreset)
	}
	if len(loaded.SelectedSkills) != 1 || loaded.SelectedSkills[0] != "fixture-skill" {
		t.Fatalf("expected existing selected skills preserved, got %v", loaded.SelectedSkills)
	}
}

func TestRunNoTUI_BlankPersonaInputBlocksLegacyV1PresetAndPreservesConfig(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	legacyPath := filepath.Join(home, ".jarvis", "personas", "legacy-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy preset dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n"), 0o644); err != nil {
		t.Fatalf("write legacy preset: %v", err)
	}

	seed := &config.AppConfig{
		SchemaVersion:       2,
		APIURL:              config.DefaultAPIURL,
		PersonaPreset:       "legacy-custom",
		PersonaPresetSource: string(persona.PresetSourceUser),
		Install:             config.InstallState{Agents: map[string]config.AgentState{}},
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	err := runNoTUI(testWizardConfig(), strings.NewReader("\n\n"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "migrate") {
		t.Fatalf("runNoTUI() error = %v, want schema-v2 migration guidance", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config after blocked default: %v", err)
	}
	if loaded.PersonaPreset != "legacy-custom" || loaded.PersonaPresetSource != string(persona.PresetSourceUser) {
		t.Fatalf("legacy persona config was overwritten: %+v", loaded)
	}
}

func TestRunNoTUI_BlankPersonaInputBlocksMissingPresetAndPreservesConfig(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")
	seed := &config.AppConfig{
		SchemaVersion:       2,
		APIURL:              config.DefaultAPIURL,
		PersonaPreset:       "deleted-custom",
		PersonaPresetSource: string(persona.PresetSourceUser),
		Install:             config.InstallState{Agents: map[string]config.AgentState{}},
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	err := runNoTUI(testWizardConfig(), strings.NewReader("\n\n"))
	if err == nil {
		t.Fatal("expected stale/deleted preset recovery guidance")
	}
	for _, want := range []string{"deleted-custom", "stale", "deleted", "recovery"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Fatalf("runNoTUI() error = %q, want contains %q", err, want)
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "migrate") {
		t.Fatalf("missing preset error = %q, must not use V1 migration guidance", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config after blocked default: %v", err)
	}
	if loaded.PersonaPreset != "deleted-custom" || loaded.PersonaPresetSource != string(persona.PresetSourceUser) {
		t.Fatalf("missing persona config was overwritten: %+v", loaded)
	}
}

func TestRunNoTUI_BlankPersonaInputBlocksMalformedPresetAndPreservesConfig(t *testing.T) {
	home := isolateTestHome(t)
	t.Setenv("PATH", "")
	malformedPath := filepath.Join(home, ".jarvis", "personas", "broken-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(malformedPath), 0o755); err != nil {
		t.Fatalf("create malformed preset dir: %v", err)
	}
	if err := os.WriteFile(malformedPath, []byte("schema_version: 2\nname: [\n"), 0o644); err != nil {
		t.Fatalf("write malformed preset: %v", err)
	}

	seed := &config.AppConfig{
		SchemaVersion:       2,
		APIURL:              config.DefaultAPIURL,
		PersonaPreset:       "broken-custom",
		PersonaPresetSource: string(persona.PresetSourceUser),
		Install:             config.InstallState{Agents: map[string]config.AgentState{}},
	}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}

	err = runNoTUI(testWizardConfig(), strings.NewReader("\n\n"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "repair") {
		t.Fatalf("runNoTUI() error = %v, want malformed schema-v2 repair guidance", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after blocked default: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("malformed persona config was rewritten:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRunNoTUI_CustomPresetPersistsUserFileAndCanonicalIdentity(t *testing.T) {
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")

	// scope default, choose custom option, provide name/display, keep generated YAML,
	// default optional skills answer, apply=yes.
	input := strings.NewReader("\n3\nmi persona\nMi Persona Display\n\nyes\n")

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
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")

	// scope default, choose custom option, provide name/display, invalid YAML override.
	input := strings.NewReader("\n3\nbroken persona\nBroken Persona\nname: [\n")

	err := runNoTUI(testWizardConfig(), input)
	if err == nil {
		t.Fatal("expected error when legacy custom YAML is provided")
	}
	if !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("runNoTUI custom YAML error = %v, want actionable migration guidance", err)
	}

	customPath := filepath.Join(tmpHome, ".jarvis", "personas", "broken-persona.yaml")
	if _, statErr := os.Stat(customPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected invalid custom preset not to be persisted, got err=%v", statErr)
	}
}

func TestResolveNoTUIPresetUsesValidatedV2Route(t *testing.T) {
	resolved, err := resolveNoTUIPreset(testPersonaFS, "fixture", nil)
	if err != nil {
		t.Fatalf("resolveNoTUIPreset: %v", err)
	}
	if resolved == nil || resolved.Slug != "fixture" || resolved.Preset.SchemaVersion != 2 {
		t.Fatalf("resolved = %+v, want validated V2 fixture", resolved)
	}
}

func TestResolveNoTUIPresetRejectsLegacyCustomProfileWithMigrationGuidance(t *testing.T) {
	home := isolateTestHome(t)
	legacyPath := filepath.Join(home, ".jarvis", "personas", "legacy-custom.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy preset dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("name: legacy-custom\ndisplay_name: Legacy Custom\ntone: {}\n"), 0o644); err != nil {
		t.Fatalf("write legacy preset: %v", err)
	}

	_, err := resolveNoTUIPreset(testPersonaFS, "legacy custom", nil)
	if err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("resolveNoTUIPreset() error = %v, want actionable migration guidance", err)
	}
}

func TestPrintNoTUIPhaseModelReview_IncludesOpenCodeProviderModelAssignments(t *testing.T) {
	resolved := sddruntime.ResolvePhaseModels(&config.AppConfig{})
	assignments := map[string]config.OpenCodeModelAssignment{
		"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
	}
	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	printNoTUIPhaseModelReview(resolved, assignments, nil)

	if !strings.Contains(output.String(), "- default: opencode=openai/gpt-5.1-codex-max") {
		t.Fatalf("expected provider-qualified OpenCode assignment in no-TUI review, got:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "effort=high") {
		t.Fatalf("expected OpenCode effort in no-TUI review, got:\n%s", output.String())
	}
}

func TestPrintNoTUIPhaseModelReview_IncludesClaudeSpecificModelAndEffort(t *testing.T) {
	resolved := sddruntime.ResolvePhaseModels(&config.AppConfig{})
	claudeAssignments := map[string]config.ClaudeModelAssignment{
		"default": {Model: "haiku", Effort: "max"},
	}
	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	printNoTUIPhaseModelReview(resolved, nil, claudeAssignments)

	if !strings.Contains(output.String(), "- default: opencode=") || !strings.Contains(output.String(), "claude=haiku") {
		t.Fatalf("expected Claude-specific model in no-TUI review, got:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "claude=haiku, effort=max") {
		t.Fatalf("expected Claude effort in no-TUI review, got:\n%s", output.String())
	}
}

func TestRunNoTUI_PrintsOpenCodeProviderModelOptionsBeforeNumericSelection(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		return []config.OpenCodeModelAssignment{{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"}}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	isolateTestHome(t)
	t.Setenv("PATH", "")

	input := strings.NewReader("\n\n" + "edit\n" + "1\n\n\n" + remainingPhasePromptNewlines() + "yes\nyes\n")
	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI provider model options: %v", err)
	}

	if !strings.Contains(output.String(), "1) openai/gpt-5.1-codex-max") {
		t.Fatalf("expected numbered OpenCode provider/model option in output, got:\n%s", output.String())
	}
}

func TestRunNoTUI_PrintsOpenCodeDiscoveryDiagnostics(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")

	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		openCodePhaseModelDiscoveryDiagnostics = []string{"OpenCode settings file /home/me/.config/opencode/opencode.jsonc uses unsupported JSONC"}
		return nil
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	if err := runNoTUI(testWizardConfig(), strings.NewReader("\n\nno\n")); err != nil {
		t.Fatalf("runNoTUI: %v", err)
	}
	if !strings.Contains(output.String(), "unsupported JSONC") {
		t.Fatalf("expected OpenCode discovery diagnostic in no-TUI output, got:\n%s", output.String())
	}
}

func TestSelectOpenCodeAssignmentForPrompt_AcceptsLegacyClear(t *testing.T) {
	options := []config.OpenCodeModelAssignment{{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"}}

	for _, input := range []string{"0", "legacy"} {
		assignment, ok := selectOpenCodeAssignmentForPrompt(input, options)
		if !ok {
			t.Fatalf("expected %q to select legacy clear", input)
		}
		if assignment.ProviderID != "" || assignment.ModelID != "" {
			t.Fatalf("expected legacy clear assignment for %q, got %+v", input, assignment)
		}
	}
}

func TestOpenCodeAssignmentPromptValue_ShowsLegacyAliasWhenNoProviderAssignment(t *testing.T) {
	assignment := config.OpenCodeModelAssignment{}

	if got, want := openCodeAssignmentPromptValue(assignment, "sonnet"), "legacy=sonnet"; got != want {
		t.Fatalf("openCodeAssignmentPromptValue = %q, want %q", got, want)
	}
}

func TestPrintOpenCodeAssignmentOptions_FiltersCatalogLegacyOption(t *testing.T) {
	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	printOpenCodeAssignmentOptions([]config.OpenCodeModelAssignment{
		{},
		{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
	})

	text := output.String()
	if !strings.Contains(text, "0) legacy") || !strings.Contains(text, "1) openai/gpt-5.1-codex-max") {
		t.Fatalf("expected legacy zero plus first provider option, got:\n%s", text)
	}
	if strings.Contains(text, "1) none") || strings.Contains(text, "2) openai/gpt-5.1-codex-max") {
		t.Fatalf("expected catalog legacy option filtered from numbered provider list, got:\n%s", text)
	}
}

func TestPrintOpenCodeAssignmentOptions_IncludesEffortWhenPresent(t *testing.T) {
	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	printOpenCodeAssignmentOptions([]config.OpenCodeModelAssignment{
		{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		{ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"},
	})

	if !strings.Contains(output.String(), "0) legacy") {
		t.Fatalf("expected legacy option, got:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "1) openai/gpt-5.1-codex-max") {
		t.Fatalf("expected default effort option, got:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "2) openai/gpt-5.1-codex-max (effort=high)") {
		t.Fatalf("expected effort option, got:\n%s", output.String())
	}
}

func TestRunNoTUI_KeepsExistingOpenCodeProviderAssignmentWhenDiscoveryUnavailable(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment { return nil }
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	isolateTestHome(t)
	t.Setenv("PATH", "")

	seed := &config.AppConfig{
		SchemaVersion:  2,
		APIURL:         config.DefaultAPIURL,
		PersonaPreset:  "fixture",
		SelectedSkills: []string{},
	}
	seed.SDD.PhaseModels = map[string]config.PhaseModelSelection{"default": {OpenCode: "sonnet", Claude: "sonnet"}}
	seed.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"}}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	input := strings.NewReader("\n\n" + "edit\n" + "\n\n\n" + remainingPhasePromptNewlines() + "yes\nyes\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI keep provider assignment: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	assignment := loaded.SDD.OpenCodePhaseModels["default"]
	if assignment.ProviderID != "openai" || assignment.ModelID != "gpt-5.1-codex-max" || assignment.Effort != "high" {
		t.Fatalf("expected existing assignment kept, got %+v", assignment)
	}
}

func TestRunNoTUI_LegacySelectionDeletesExistingOpenCodeProviderAssignment(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		return []config.OpenCodeModelAssignment{{}, {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"}}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	isolateTestHome(t)
	t.Setenv("PATH", "")

	seed := &config.AppConfig{SchemaVersion: 2, APIURL: config.DefaultAPIURL, PersonaPreset: "fixture"}
	seed.SDD.PhaseModels = map[string]config.PhaseModelSelection{"default": {OpenCode: "sonnet", Claude: "sonnet"}}
	seed.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{"default": {ProviderID: "openai", ModelID: "gpt-5.1-codex-max"}}
	if err := config.Save(seed); err != nil {
		t.Fatalf("save seed config: %v", err)
	}

	input := strings.NewReader("\n\n" + "edit\n" + "0\n\n\n" + remainingPhasePromptNewlines() + "yes\nyes\n")
	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI legacy clear: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := loaded.SDD.OpenCodePhaseModels["default"]; ok {
		t.Fatalf("expected legacy selection to delete provider assignment, got %#v", loaded.SDD.OpenCodePhaseModels)
	}
}

func TestRunNoTUI_PersistsEditedOpenCodeProviderModelAssignment(t *testing.T) {
	previousDiscover := discoverOpenCodePhaseModelOptions
	discoverOpenCodePhaseModelOptions = func() []config.OpenCodeModelAssignment {
		return []config.OpenCodeModelAssignment{{ProviderID: "openai", ModelID: "gpt-5.1-codex-max", Effort: "high"}}
	}
	t.Cleanup(func() { discoverOpenCodePhaseModelOptions = previousDiscover })

	isolateTestHome(t)
	t.Setenv("PATH", "")

	// scope default, persona default, skills default,
	// request phase editor, select OpenCode provider/model option 1 for default, keep the rest, then apply yes.
	input := strings.NewReader("\n\n" + "edit\n" + "1\n\n\n" + remainingPhasePromptNewlines() + "yes\nyes\n")

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI provider model assignment: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	assignment := loaded.SDD.OpenCodePhaseModels["default"]
	if assignment.ProviderID != "openai" || assignment.ModelID != "gpt-5.1-codex-max" || assignment.Effort != "high" {
		t.Fatalf("unexpected OpenCode assignment: %+v", assignment)
	}
	if resolved := sddruntime.ResolvePhaseModels(loaded); resolved["default"].OpenCode == "" {
		t.Fatal("expected legacy OpenCode alias to remain populated")
	}
}

func TestRunNoTUI_PersistsEditedPhaseModels(t *testing.T) {
	isolateTestHome(t)
	t.Setenv("PATH", "")
	var output bytes.Buffer
	previousStdout := noTUIStdout
	noTUIStdout = &output
	t.Cleanup(func() { noTUIStdout = previousStdout })

	// scope default, persona default, skills default,
	// request phase editor from review using 'edit', set default.claude=haiku with effort=high, keep rest, then apply yes.
	input := strings.NewReader("\n\n" + "edit\n" + "\nhaiku\nhigh\n" + remainingPhasePromptNewlines() + "yes\nyes\n")

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI phase models: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	resolved := sddruntime.ResolvePhaseModels(loaded)
	if resolved["default"].Claude != "haiku" {
		t.Fatalf("expected default.claude=haiku after no-tui edit, got %q", resolved["default"].Claude)
	}
	if got := loaded.SDD.ClaudePhaseModels["default"].Effort; got != "high" {
		t.Fatalf("expected default Claude effort=high after no-tui edit, got %q", got)
	}
	if !strings.Contains(output.String(), "- default: opencode=") || !strings.Contains(output.String(), "claude=haiku, effort=high") {
		t.Fatalf("expected Review & Apply output to include edited Claude model and effort, got:\n%s", output.String())
	}
}

func TestRunNoTUI_InstallsClaudeSDDAgentsWithSelectedModelAndEffort(t *testing.T) {
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")
	executor := &recordingWizardMCPExecutor{}
	originalExecutor := newWizardMCPExecutor
	originalDaemonPath := wizardHiveDaemonPath
	newWizardMCPExecutor = func() wizardMCPExecutor { return executor }
	daemon := createPortableHiveDaemon(t, tmpHome)
	wizardHiveDaemonPath = func(string) string { return daemon }
	t.Cleanup(func() {
		newWizardMCPExecutor = originalExecutor
		wizardHiveDaemonPath = originalDaemonPath
	})

	originalDetect := detectInstalledAgents
	detectInstalledAgents = func(fsys fs.FS) []agent.Agent {
		return []agent.Agent{&sddInstallingMockAgent{mockAgent: mockAgent{name: "claude", configDir: filepath.Join(tmpHome, ".claude")}, home: tmpHome}}
	}
	t.Cleanup(func() { detectInstalledAgents = originalDetect })

	input := strings.NewReader("\n\n" + "edit\n" +
		phaseEditorPromptNewlinesBefore("sdd-design") +
		"\nhaiku\nmax\n" +
		phaseEditorPromptNewlinesAfter("sdd-design") +
		"yes\nI ACKNOWLEDGE\n")

	if err := runNoTUI(testWizardConfig(), input); err != nil {
		t.Fatalf("runNoTUI Claude SDD install: %v", err)
	}
	if len(executor.inputs) != 1 || strings.Join(executor.inputs[0].SelectedAgents, ",") != "claude" {
		t.Fatalf("managed MCP handoff = %+v, want one Claude production request", executor.inputs)
	}

	content, err := os.ReadFile(filepath.Join(tmpHome, ".claude", "agents", "sdd-design.md"))
	if err != nil {
		t.Fatalf("expected generated sdd-design.md to be installed: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "model: haiku") || !strings.Contains(text, "effort: max") {
		t.Fatalf("generated sdd-design.md did not use selected Claude route:\n%s", text)
	}
}

func TestRunNoTUIManagedMCPRoutesUseConcreteExecutorForInstallAndReconfigure(t *testing.T) {
	for _, tt := range []struct {
		name          string
		seedConfigure bool
	}{
		{name: "install"},
		{name: "reconfigure", seedConfigure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateTestHome(t)
			t.Setenv("PATH", "")
			if tt.seedConfigure {
				if err := config.Save(&config.AppConfig{APIURL: config.DefaultAPIURL, PersonaPreset: "fixture"}); err != nil {
					t.Fatalf("seed reconfigure config: %v", err)
				}
			}
			replacement := &nativeMCPReplacerStub{}
			daemon := useConcreteWizardExecutor(t, home, replacement)
			seedProvenancedOpenCodeConfig(t, home, daemon)

			originalDetect := detectInstalledAgents
			detectInstalledAgents = func(fs.FS) []agent.Agent {
				return []agent.Agent{
					&mockAgent{name: "claude", configDir: filepath.Join(home, ".claude")},
					&mockAgent{name: "opencode", configDir: filepath.Join(home, ".config", "opencode")},
				}
			}
			t.Cleanup(func() { detectInstalledAgents = originalDetect })

			err := runNoTUI(testWizardConfig(), strings.NewReader("\n\nyes\nI ACKNOWLEDGE\n"))
			if err == nil || strings.Contains(err.Error(), "reconcile managed MCPs") {
				t.Fatalf("no-TUI route did not pass concrete reconciliation: %v", err)
			}
			assertConcreteWizardMutation(t, home, replacement)
		})
	}
}

func TestRunNoTUIManagedMCPCancellationAndNoAgentRemainMutationFree(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		home := isolateTestHome(t)
		replacement := &nativeMCPReplacerStub{}
		useConcreteWizardExecutor(t, home, replacement)
		originalDetect := detectInstalledAgents
		detectInstalledAgents = func(fs.FS) []agent.Agent {
			return []agent.Agent{&mockAgent{name: "claude", configDir: filepath.Join(home, ".claude")}}
		}
		t.Cleanup(func() { detectInstalledAgents = originalDetect })

		if err := runNoTUI(testWizardConfig(), strings.NewReader("\n\nyes\nno\n")); err != nil {
			t.Fatalf("no-TUI cancellation: %v", err)
		}
		assertNoManagedMCPMutation(t, home, replacement)
	})

	t.Run("no agents", func(t *testing.T) {
		home := isolateTestHome(t)
		replacement := &nativeMCPReplacerStub{}
		useConcreteWizardExecutor(t, home, replacement)
		originalDetect := detectInstalledAgents
		detectInstalledAgents = func(fs.FS) []agent.Agent { return nil }
		t.Cleanup(func() { detectInstalledAgents = originalDetect })

		if err := runNoTUI(testWizardConfig(), strings.NewReader("\n\nyes\n")); err != nil {
			t.Fatalf("no-TUI no-agent continuation: %v", err)
		}
		assertNoManagedMCPMutation(t, home, replacement)
	})
}

func TestRunNoTUIManagedMCPFailureStopsAtConcreteExecutorBoundary(t *testing.T) {
	home := isolateTestHome(t)
	replacement := &nativeMCPReplacerStub{err: errors.New("native boundary unavailable")}
	daemon := useConcreteWizardExecutor(t, home, replacement)
	seedProvenancedOpenCodeConfig(t, home, daemon)
	originalDetect := detectInstalledAgents
	detectInstalledAgents = func(fs.FS) []agent.Agent {
		return []agent.Agent{
			&mockAgent{name: "claude", configDir: filepath.Join(home, ".claude")},
			&mockAgent{name: "opencode", configDir: filepath.Join(home, ".config", "opencode")},
		}
	}
	t.Cleanup(func() { detectInstalledAgents = originalDetect })

	err := runNoTUI(testWizardConfig(), strings.NewReader("\n\nyes\nI ACKNOWLEDGE\n"))
	if err == nil || !strings.Contains(err.Error(), "reconcile managed MCPs: reconciliation failed") {
		t.Fatalf("expected sanitized fail-stop evidence from no-TUI route, got %v", err)
	}
	if len(replacement.definitions) != 1 {
		t.Fatalf("native replacement calls = %d, want 1", len(replacement.definitions))
	}
	if _, statErr := os.Stat(filepath.Join(home, ".jarvis", "config.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("config persisted after managed MCP failure = %v, want absent", statErr)
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
	isolateTestHome(t)

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

func (m *mockAgent) InstallOrchestrator(orchestratorContent []byte) error {
	return nil
}

func (m *mockAgent) InstallPromptHook(hooksFS fs.FS) error {
	return nil
}

func (m *mockAgent) InstallSessionHooks(fs.FS) error { return nil }

func (m *mockAgent) SupportsOutputStyles() bool {
	return false
}

func (m *mockAgent) WriteOutputStyle(preset *persona.Profile) error {
	return nil
}

func (m *mockAgent) ClearOutputStyle(name string) error {
	return nil
}

func (m *mockAgent) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return sddruntime.Build("claude")
}

func (m *mockAgent) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            true,
			ContractVersion:    sddruntime.DefaultContractVersion,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath: sddruntime.DefaultRegistryPath,
		ModelAssignments: map[string]string{
			"orchestrator": "opus",
			"sdd-apply":    "sonnet",
			"default":      "sonnet",
		},
		Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}, nil
}

type failingMockAgent struct{ mockAgent }

func (m *failingMockAgent) MergeConfig(entry agent.MCPEntry) error {
	return errors.New("boom merge")
}

type sddInstallingMockAgent struct {
	mockAgent
	home string
}

func (m *sddInstallingMockAgent) InstallSDDPhaseAgents(cfg *config.AppConfig) error {
	files, err := agent.RenderClaudeSDDPhaseAgents(jarvis.TemplatesFS, cfg)
	if err != nil {
		return err
	}
	dir := filepath.Join(m.home, ".claude", "agents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "sdd-design.md"), files["sdd-design.md"], 0644)
}

func (m *sddInstallingMockAgent) ObserveRuntimeWithConfig(cfg *config.AppConfig) (sddruntime.ObservedRuntime, error) {
	plan, err := sddruntime.Build("claude")
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	assignments, err := sddruntime.ResolveAssignmentsForPlatform(sddruntime.PlatformClaude, cfg)
	if err != nil {
		return sddruntime.ObservedRuntime{}, err
	}
	return sddruntime.ObservedRuntime{
		Manifest: sddruntime.RuntimeManifestState{
			Present:            true,
			ContractVersion:    plan.Contract.Version,
			ManagedArtifactIDs: []string{"instructions", "orchestrator", "skills"},
		},
		RegistryPath:             plan.Contract.RegistryPath,
		PromptSourceIDs:          []string{"layer1.behavior", "layer2.persona", "skill.sdd-orchestrator", "registry.skill-index", "protocol.hive"},
		StoreMode:                "hybrid",
		StoreReadFrom:            []string{"hive", "openspec"},
		StoreWriteTo:             []string{"hive", "openspec"},
		ModelAssignments:         assignments,
		ResolvedModelAssignments: assignments,
		Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: true},
			"skills":       {Exists: true},
		},
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// TestRunAgentConfigSequence_Context7AfterHive
// ──────────────────────────────────────────────────────────────────────────────

// TestRunAgentConfigSequence_DoesNotDirectlyMergeManagedMCPs verifies the
// per-agent pipeline cannot bypass the ExecuteWizard reconciliation boundary.
func TestRunAgentConfigSequence_DoesNotDirectlyMergeManagedMCPs(t *testing.T) {
	tmpHome := isolateTestHome(t)

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

	if len(mock.mergedEntries) != 0 {
		t.Fatalf("direct managed MCP MergeConfig calls = %d, want 0", len(mock.mergedEntries))
	}
	if _, err := os.Stat(filepath.Join(mockConfigDir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy settings mutation = %v, want no direct managed MCP write", err)
	}
}

func TestRunNoTUI_LocalOnlyPurgesStoredCredentialsOnApply(t *testing.T) {
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")

	jarvisDir := filepath.Join(tmpHome, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jarvisDir, "sync.json"), []byte(`{"email":"old@example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}

	seed := &config.AppConfig{Scope: config.ScopeLocalCloud, Cloud: &config.CloudConfig{Email: "old@example.com", SyncConfigured: true}, Email: "old@example.com", APIURL: config.DefaultAPIURL, PersonaPreset: "fixture"}
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
	tmpHome := isolateTestHome(t)
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
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad creds"})
	}))
	defer server.Close()

	seed := &config.AppConfig{APIURL: server.URL, Scope: config.ScopeLocalOnly, PersonaPreset: "fixture"}
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
	isolateTestHome(t)
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
	tmpHome := isolateTestHome(t)
	t.Setenv("PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "jwt-token",
			"user":  map[string]string{"email": "resolved@example.com"},
		})
	}))
	defer server.Close()

	seed := &config.AppConfig{APIURL: server.URL, Scope: config.ScopeLocalOnly, PersonaPreset: "fixture"}
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
	tmpHome := isolateTestHome(t)
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
	isolateTestHome(t)
	t.Setenv("PATH", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "jwt-token",
			"user":  map[string]string{"email": ""},
		})
	}))
	defer server.Close()

	seed := &config.AppConfig{APIURL: server.URL, Scope: config.ScopeLocalOnly, PersonaPreset: "fixture"}
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

func TestBuildNoTUIStatuslineConfirm(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		input       string
		wantErr     bool
		wantConfirm bool
	}{
		{
			name: "file absent returns true without prompting",
			setup: func(t *testing.T, dir string) {
				// No .claude directory — script is absent.
			},
			input:       "y\n", // sentinel: scanner should NOT be consumed
			wantErr:     false,
			wantConfirm: true,
		},
		{
			name: "file present and answer y returns true",
			setup: func(t *testing.T, dir string) {
				claudeDir := filepath.Join(dir, ".claude")
				if err := os.MkdirAll(claudeDir, 0755); err != nil {
					t.Fatalf("mkdir .claude: %v", err)
				}
				if err := os.WriteFile(filepath.Join(claudeDir, "statusline-command.sh"), []byte("#!/bin/bash\n"), 0644); err != nil {
					t.Fatalf("write statusline script: %v", err)
				}
			},
			input:       "y\n",
			wantErr:     false,
			wantConfirm: true,
		},
		{
			name: "file present and answer yes returns true",
			setup: func(t *testing.T, dir string) {
				claudeDir := filepath.Join(dir, ".claude")
				if err := os.MkdirAll(claudeDir, 0755); err != nil {
					t.Fatalf("mkdir .claude: %v", err)
				}
				if err := os.WriteFile(filepath.Join(claudeDir, "statusline-command.sh"), []byte("#!/bin/bash\n"), 0644); err != nil {
					t.Fatalf("write statusline script: %v", err)
				}
			},
			input:       "yes\n",
			wantErr:     false,
			wantConfirm: true,
		},
		{
			name: "file present and answer n returns false",
			setup: func(t *testing.T, dir string) {
				claudeDir := filepath.Join(dir, ".claude")
				if err := os.MkdirAll(claudeDir, 0755); err != nil {
					t.Fatalf("mkdir .claude: %v", err)
				}
				if err := os.WriteFile(filepath.Join(claudeDir, "statusline-command.sh"), []byte("#!/bin/bash\n"), 0644); err != nil {
					t.Fatalf("write statusline script: %v", err)
				}
			},
			input:       "n\n",
			wantErr:     false,
			wantConfirm: false,
		},
		{
			name: "file present and empty input (Enter) returns false",
			setup: func(t *testing.T, dir string) {
				claudeDir := filepath.Join(dir, ".claude")
				if err := os.MkdirAll(claudeDir, 0755); err != nil {
					t.Fatalf("mkdir .claude: %v", err)
				}
				if err := os.WriteFile(filepath.Join(claudeDir, "statusline-command.sh"), []byte("#!/bin/bash\n"), 0644); err != nil {
					t.Fatalf("write statusline script: %v", err)
				}
			},
			input:       "\n",
			wantErr:     false,
			wantConfirm: false,
		},
		{
			name: "stat returns non-ENOENT error",
			setup: func(t *testing.T, dir string) {
				// Create .claude as a regular file so stat on .claude/statusline-command.sh
				// fails with a "not a directory" error (Linux/macOS only; skipped on Windows
				// because Windows returns ENOENT in this situation).
				if runtime.GOOS == "windows" {
					t.Skip("Windows returns ENOENT when a path component is a file; non-ENOENT stat test not applicable")
				}
				claudePath := filepath.Join(dir, ".claude")
				if err := os.WriteFile(claudePath, []byte("not a dir"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			input:       "",
			wantErr:     true,
			wantConfirm: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome := t.TempDir()
			tt.setup(t, tmpHome)

			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			confirm, err := buildNoTUIStatuslineConfirm(tmpHome, scanner)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if confirm != nil {
					t.Fatal("expected nil confirm on error, got non-nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if confirm == nil {
				t.Fatal("expected non-nil confirm closure")
			}
			if got := confirm(); got != tt.wantConfirm {
				t.Fatalf("confirm() = %v, want %v", got, tt.wantConfirm)
			}

			// For the "file absent" case: verify the scanner was NOT consumed
			// by buildNoTUIStatuslineConfirm. The input "y\n" should still be
			// readable, proving no I/O happened in the absent-file path.
			if tt.name == "file absent returns true without prompting" {
				if !scanner.Scan() {
					t.Fatal("expected scanner to still have 'y' token (scanner was incorrectly consumed by the absent-file path)")
				}
				if got := strings.TrimSpace(scanner.Text()); got != "y" {
					t.Fatalf("expected scanner to still hold 'y', got %q", got)
				}
			}
		})
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
	isolateTestHome(t)
	t.Setenv("PATH", "")

	originalList := listPersonaPresets
	listPersonaPresets = func(fsys fs.FS) ([]persona.Profile, error) {
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
	isolateTestHome(t)
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
