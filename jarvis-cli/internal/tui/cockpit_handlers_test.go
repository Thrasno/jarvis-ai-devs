package tui

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

func TestCockpitHandlers_ProviderSelectorExecutesReadOnlyActionAndReturnsToMenu(t *testing.T) {
	runner := &fakeCockpitRunner{
		doctorPlan: lifecycle.DoctorPlan{Provider: "opencode", Status: sddruntime.StatusPass, ReadOnly: true},
	}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionDoctor)

	assertViewContains(t, m.View(), "Provider", "all", "claude", "opencode")
	m = sendCockpitKey(m, tea.KeyDown)
	m = sendCockpitKey(m, tea.KeyDown)
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"doctor:opencode"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runner calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Doctor result", "provider=opencode", "status=PASS")

	m = sendCockpitKey(m, tea.KeyEnter)
	if m.cockpitMode != cockpitModeMenu {
		t.Fatalf("expected result enter to return to menu, got mode %v", m.cockpitMode)
	}
	assertViewContains(t, m.View(), "Jarvis Cockpit")
}

func TestCockpitHandlers_BackupShowsSnapshotResult(t *testing.T) {
	runner := &fakeCockpitRunner{backupSnapshotID: "backup-123"}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionBackup)
	m = sendCockpitKey(m, tea.KeyEnter) // provider=all
	m = sendCockpitKey(m, tea.KeyEnter) // confirmation prompt

	if got, want := runner.calls, []string{"backup:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runner calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Backup result", "snapshot=backup-123")
}

func TestCockpitHandlers_ErrorSurfacingKeepsUserInResultPanel(t *testing.T) {
	runner := &fakeCockpitRunner{verifyErr: errors.New("verify failed")}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionVerify)
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"verify:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runner calls: got %v want %v", got, want)
	}
	if m.cockpitMode != cockpitModeResult {
		t.Fatalf("expected error to surface as result panel, got mode %v", m.cockpitMode)
	}
	assertViewContains(t, m.View(), "Verify error", "verify failed", "Enter: return")
}

func TestCockpitHandlers_PersonaUsesCockpitNativePresetFlowAndReturnsToMenu(t *testing.T) {
	runner := &fakeCockpitRunner{personaSummary: "persona argentino applied to 1 agent"}
	m := newCockpitHandlerTestModel(runner)
	m.Presets = []persona.ProfileOption{
		{Name: "argentino", DisplayName: "Argentino", Description: "Rioplatense Spanish"},
		{Name: "neutra", DisplayName: "Neutra", Description: "Neutral Spanish"},
		{Name: "custom", DisplayName: "Custom (crear nuevo)", Description: "Use installer custom validation path"},
	}
	m.presetCur = 0
	m.personaSelectionErr = nil
	m = selectCockpitAction(t, m, CockpitActionPersona)

	if m.Screen == ScreenWizard || m.Step == StepSkills {
		t.Fatalf("persona action must stay cockpit-native, got screen=%v step=%v", m.Screen, m.Step)
	}
	assertViewContains(t, m.View(), "Persona", "Argentino", "Neutra", "Custom", "Enter: apply")
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"persona:argentino"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected persona calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Persona result", "persona argentino applied", "Enter: return")
	m = sendCockpitKey(m, tea.KeyEnter)
	if m.cockpitMode != cockpitModeMenu || m.Screen != ScreenCockpit {
		t.Fatalf("expected persona result to return to cockpit menu, got mode=%v screen=%v", m.cockpitMode, m.Screen)
	}
}

func TestCockpitHandlers_PersonaBlocksMissingConfiguredPresetBeforeDefaultAcceptance(t *testing.T) {
	runner := &fakeCockpitRunner{}
	m := newCockpitHandlerTestModel(runner)
	m.Presets = []persona.ProfileOption{{Name: "argentino", DisplayName: "Argentino"}}
	m.presetCur = -1
	m.personaSelectionErr = errors.New("configured persona preset \"deleted-custom\" is stale or deleted; Recovery: explicitly select an available schema v2 preset")
	m = selectCockpitAction(t, m, CockpitActionPersona)
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("missing configured preset must not apply the default option, got calls %v", runner.calls)
	}
	assertViewContains(t, m.View(), "Persona error", "deleted-custom", "stale or deleted", "Recovery")
}

func TestCockpitHandlers_PersonaCustomOptionUsesExtensionSeamWithoutClaimingApply(t *testing.T) {
	runner := &fakeCockpitRunner{}
	m := newCockpitHandlerTestModel(runner)
	m.Presets = []persona.ProfileOption{
		{Name: "argentino", DisplayName: "Argentino", Description: "Rioplatense Spanish"},
		{Name: "custom", DisplayName: "Custom (crear nuevo)", Description: "Use installer custom validation path"},
	}
	m = selectCockpitAction(t, m, CockpitActionPersona)
	m = sendCockpitKey(m, tea.KeyDown)
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("custom seam must not apply an unsupported custom preset: %v", runner.calls)
	}
	assertViewContains(t, m.View(), "Custom persona", "Install/Reconfigure", "installer-equivalent validation", "Enter: return")
}

func TestCockpitHandlers_PersonaEmptyAndRunnerErrorSurfaceAsResults(t *testing.T) {
	runner := &fakeCockpitRunner{personaErr: errors.New("preset apply failed")}
	m := newCockpitHandlerTestModel(runner)
	m.Presets = nil
	m = selectCockpitAction(t, m, CockpitActionPersona)
	m = sendCockpitKey(m, tea.KeyEnter)

	assertViewContains(t, m.View(), "Persona error", "no persona presets available", "Enter: return")

	m = newCockpitHandlerTestModel(runner)
	m.Presets = []persona.ProfileOption{{Name: "neutra", DisplayName: "Neutra"}}
	m = selectCockpitAction(t, m, CockpitActionPersona)
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"persona:neutra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected persona calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Persona error", "preset apply failed", "Enter: return")
}

func TestCockpitHandlers_HiveCloudLoginPromptsCredentialsAndReturnsToMenu(t *testing.T) {
	runner := &fakeCockpitRunner{loginEmail: "resolved@example.com"}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
	m.Email = ""
	m.Password = ""

	if m.Screen == ScreenWizard || m.Step == StepPersona {
		t.Fatalf("login action must stay cockpit-native, got screen=%v step=%v", m.Screen, m.Step)
	}
	assertViewContains(t, m.View(), "Hive Cloud Login", "Email", "Password")
	m = typeCockpitText(m, "input@example.com")
	m = sendCockpitKey(m, tea.KeyEnter)
	m = typeCockpitText(m, "secret")
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"login:input@example.com:secret"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected login calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Hive Cloud Login result", "resolved@example.com", "Enter: return")
	m = sendCockpitKey(m, tea.KeyEnter)
	if m.cockpitMode != cockpitModeMenu || m.Screen != ScreenCockpit {
		t.Fatalf("expected login result to return to cockpit menu, got mode=%v screen=%v", m.cockpitMode, m.Screen)
	}
}

func TestCockpitHandlers_HiveCloudLoginMasksPasswordWithBullets(t *testing.T) {
	m := newCockpitHandlerTestModel(&fakeCockpitRunner{})
	m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
	m.Email = "input@example.com"
	m.Password = "secret"
	m.activeField = 1

	view := m.View()
	if strings.Contains(view, "secret") {
		t.Fatalf("login view must not render raw password:\n%s", view)
	}
	if !strings.Contains(view, "••••••") {
		t.Fatalf("login view must render bullet password mask:\n%s", view)
	}
	if strings.Contains(view, "*") {
		t.Fatalf("login view must not use star password mask:\n%s", view)
	}
}

func TestCockpitHandlers_HiveCloudLoginRequiresCredentialsAllowsBackspaceAndSurfacesErrors(t *testing.T) {
	runner := &fakeCockpitRunner{loginErr: errors.New("cloud unavailable")}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
	m.Email = ""
	m.Password = ""
	m = sendCockpitKey(m, tea.KeyEnter)
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("login must not call runner with blank credentials: %v", runner.calls)
	}
	assertViewContains(t, m.View(), "Email and password are required")

	m = newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
	m.Email = ""
	m.Password = ""
	m = typeCockpitText(m, "user@example.comm")
	m = sendCockpitKey(m, tea.KeyBackspace)
	m = sendCockpitKey(m, tea.KeyEnter)
	m = typeCockpitText(m, "wrong")
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"login:user@example.com:wrong"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected login calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Hive Cloud Login error", "cloud unavailable", "Enter: return")
}

func TestCockpitHandlers_HiveCloudLoginRejectsInvalidEmail(t *testing.T) {
	for _, tt := range []struct {
		name  string
		email string
	}{
		{name: "missing at", email: "invalid-email"},
		{name: "missing domain dot", email: "user@example"},
		{name: "embedded whitespace", email: "user @example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeCockpitRunner{}
			m := newCockpitHandlerTestModel(runner)
			m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
			m.Email = ""
			m.Password = ""
			m = typeCockpitText(m, tt.email)
			m = sendCockpitKey(m, tea.KeyEnter)
			m = typeCockpitText(m, "secret")
			m = sendCockpitKey(m, tea.KeyEnter)

			if len(runner.calls) != 0 {
				t.Fatalf("invalid email must not call runner: %v", runner.calls)
			}
			view := m.View()
			assertViewContains(t, view, "Email inválido", "usuario@dominio.com")
			if strings.Contains(view, "jarvis --no-tui") {
				t.Fatalf("invalid email error must not recommend no-TUI fallback:\n%s", view)
			}
		})
	}
}

func TestCockpitHandlers_HiveCloudLoginTrimsEmailBeforeLogin(t *testing.T) {
	runner := &fakeCockpitRunner{}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
	m.Email = "  input@example.com  "
	m.activeField = 1
	m.Password = " secret "
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"login:input@example.com: secret "}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected login calls: got %v want %v", got, want)
	}
}

func TestNormalizeHiveCloudEmail_SharedSemantics(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "trims valid email", input: "  usuario@dominio.com\t", want: "usuario@dominio.com"},
		{name: "empty stays empty", input: "  ", want: ""},
		{name: "missing at", input: "usuario", wantErr: true},
		{name: "missing domain dot", input: "usuario@dominio", wantErr: true},
		{name: "embedded whitespace", input: "usuario @dominio.com", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeHiveCloudEmail(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "usuario@dominio.com") || strings.Contains(err.Error(), "jarvis --no-tui") {
					t.Fatalf("unexpected error text: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCockpitHandlers_ConfigShowsReadOnlyStructuredResultAndReturnsToMenu(t *testing.T) {
	runner := &fakeCockpitRunner{configSummary: "preset=argentino\napi_url=https://hivemem.dev\nemail=dev@example.com"}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionConfig)

	if got, want := runner.calls, []string{"config"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected config calls: got %v want %v", got, want)
	}
	view := m.View()
	assertViewContains(t, view, "Config result", "preset=argentino", "api_url=https://hivemem.dev", "email=dev@example.com", "Enter: return")
	if strings.Contains(strings.ToLower(view), "edit") {
		t.Fatalf("config cockpit view must be read-only; got edit affordance in:\n%s", view)
	}
	m = sendCockpitKey(m, tea.KeyEnter)
	if m.cockpitMode != cockpitModeMenu {
		t.Fatalf("expected config result enter to return to menu, got mode %v", m.cockpitMode)
	}
}

func TestCockpitHandlers_ConfigErrorSurfacesInResultPanel(t *testing.T) {
	runner := &fakeCockpitRunner{configErr: errors.New("config unreadable")}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionConfig)

	if got, want := runner.calls, []string{"config"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected config calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Config error", "config unreadable", "Enter: return")
}

func TestCockpitHandlers_RestoreRequiresSnapshotAndStrongConfirmation(t *testing.T) {
	runner := &fakeCockpitRunner{}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionRestore)
	m = sendCockpitKey(m, tea.KeyEnter) // provider=all
	assertViewContains(t, m.View(), "Snapshot id")
	m = typeCockpitText(m, "snap-001")
	m = sendCockpitKey(m, tea.KeyEnter)
	assertViewContains(t, m.View(), "Type RESTORE")
	m = typeCockpitText(m, "restore")
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("restore mutated before exact confirmation: %v", runner.calls)
	}
	assertViewContains(t, m.View(), "confirmation did not match")
	m = clearCockpitInput(m)
	m = typeCockpitText(m, "RESTORE")
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"restore:all:snap-001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected runner calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Restore result", "restored=3")
}

func TestCockpitHandlers_RestoreRequiresNonEmptySnapshotBeforeConfirmation(t *testing.T) {
	runner := &fakeCockpitRunner{}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionRestore)
	m = sendCockpitKey(m, tea.KeyEnter)
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("restore must not proceed without snapshot id: %v", runner.calls)
	}
	assertViewContains(t, m.View(), "Snapshot id is required")
}

func TestCockpitHandlers_ReconcileDryRunsBeforeConfirmedMutation(t *testing.T) {
	runner := &fakeCockpitRunner{
		reconcilePlan:   lifecycle.DoctorPlan{Provider: "claude", Status: sddruntime.StatusFail, ReadOnly: true, Steps: []lifecycle.DoctorStep{{CheckKey: "instructions", AssetID: "instructions", NextAction: "rewrite"}}},
		reconcileResult: lifecycle.ReconcileResult{Applied: 1, ManualRequired: 2, SkippedNonOwned: []string{"manual edit"}},
	}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionReconcile)
	m = sendCockpitKey(m, tea.KeyDown)  // claude
	m = sendCockpitKey(m, tea.KeyEnter) // dry-run first

	if got, want := runner.calls, []string{"reconcile-dry-run:claude"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dry-run calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Reconcile dry-run", "steps=1", "Type RECONCILE")
	m = typeCockpitText(m, "RECONCILE")
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"reconcile-dry-run:claude", "reconcile:claude"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected reconcile calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Reconcile result", "applied=1", "manual_required=2", "skipped=1")
}

func TestCockpitHandlers_UninstallRequiresDryRunAndStrongConfirmation(t *testing.T) {
	runner := &fakeCockpitRunner{
		uninstallPlan:   lifecycle.DoctorPlan{Provider: "all", Status: sddruntime.StatusFail, ReadOnly: true, Steps: []lifecycle.DoctorStep{{CheckKey: "remove", AssetID: "instructions"}}},
		uninstallResult: lifecycle.UninstallResult{Applied: 3, VerifyStatus: sddruntime.StatusPass, LedgerRemoved: true},
	}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionUninstall)
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"uninstall-dry-run:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected uninstall dry-run calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Uninstall safety diagnosis", "read-only diagnosis, not a full uninstall plan", "Type UNINSTALL")
	m = typeCockpitText(m, "nope")
	m = sendCockpitKey(m, tea.KeyEnter)
	if got, want := runner.calls, []string{"uninstall-dry-run:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("uninstall mutated before exact confirmation: got %v want %v", got, want)
	}
	m = clearCockpitInput(m)
	m = typeCockpitText(m, "UNINSTALL")
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"uninstall-dry-run:all", "uninstall:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected uninstall calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Uninstall result", "applied=3", "ledger_removed=true")
}

func TestCockpitHandlers_BackupErrorAndConfirmTypingFailuresStaySafe(t *testing.T) {
	runner := &fakeCockpitRunner{backupErr: errors.New("backup failed")}
	m := newCockpitHandlerTestModel(runner)
	m = selectCockpitAction(t, m, CockpitActionBackup)
	m = sendCockpitKey(m, tea.KeyEnter)
	m = sendCockpitKey(m, tea.KeyEnter)

	if got, want := runner.calls, []string{"backup:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected backup calls: got %v want %v", got, want)
	}
	assertViewContains(t, m.View(), "Backup error", "backup failed", "Enter: return")

	m = newCockpitHandlerTestModel(&fakeCockpitRunner{})
	m = selectCockpitAction(t, m, CockpitActionReconcile)
	m = sendCockpitKey(m, tea.KeyEnter)
	m = typeCockpitText(m, "RECONCIL")
	m = sendCockpitKey(m, tea.KeyBackspace)
	m = sendCockpitKey(m, tea.KeyEnter)
	assertViewContains(t, m.View(), "confirmation did not match", "Type RECONCILE")
}

func TestDefaultCockpitRunner_ConfigSummaryUsesStructuredReadOnlyFields(t *testing.T) {
	setTestHome(t, t.TempDir())
	cfg := &config.AppConfig{
		SchemaVersion: 3,
		APIURL:        "https://hivemem.dev",
		Email:         "dev@example.com",
		Version:       "1.0.0",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config fixture: %v", err)
	}
	// ~/.jarvis/state.yaml owns the persona and the configured agents the
	// summary reports.
	manifest := state.New()
	manifest.Persona = "argentino"
	manifest.InstalledAgents = []state.Agent{
		{ID: "claude", InstructionsPath: "/i/claude", ConfigPath: "/c/claude"},
		{ID: "opencode", InstructionsPath: "/i/opencode", ConfigPath: "/c/opencode"},
	}
	if err := state.Save(manifest); err != nil {
		t.Fatalf("save manifest fixture: %v", err)
	}

	summary, err := (defaultCockpitRunner{}).ConfigSummary(context.Background())
	if err != nil {
		t.Fatalf("ConfigSummary returned error: %v", err)
	}
	assertViewContains(t, summary, "preset=argentino", "api_url=https://hivemem.dev", "email=dev@example.com", "configured_agents=claude, opencode", "version=1.0.0")
}

func TestDefaultCockpitRunner_AllProviderFansOutAndMergesPlans(t *testing.T) {
	if got, want := cockpitProviderTargets("all"), []string{"claude", "opencode"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected all provider targets: got %v want %v", got, want)
	}
	if got, want := cockpitProviderTargets("claude"), []string{"claude"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected concrete provider targets: got %v want %v", got, want)
	}

	merged := mergeCockpitDoctorPlans("all", []lifecycle.DoctorPlan{
		{Provider: "claude", Status: sddruntime.StatusPass, ReadOnly: true, Steps: []lifecycle.DoctorStep{{CheckKey: "claude"}}},
		{Provider: "opencode", Status: sddruntime.StatusFail, ReadOnly: true, Steps: []lifecycle.DoctorStep{{CheckKey: "opencode"}}},
	})
	if merged.Provider != "all" || merged.Status != sddruntime.StatusFail || !merged.ReadOnly || len(merged.Steps) != 2 {
		t.Fatalf("unexpected merged plan: %+v", merged)
	}
}

func TestDefaultCockpitRunner_UninstallAllUsesLifecycleAllModeForLedgerCleanup(t *testing.T) {
	runner := defaultCockpitRunner{}
	fakeEngine := &fakeCockpitLifecycleEngine{
		uninstallResults: map[string]lifecycle.UninstallResult{
			"all:all": {Applied: 5, VerifyStatus: sddruntime.StatusPass, LedgerRemoved: true},
		},
	}
	originalEngine := newCockpitLifecycleEngine
	newCockpitLifecycleEngine = func() cockpitLifecycleService { return fakeEngine }
	t.Cleanup(func() { newCockpitLifecycleEngine = originalEngine })

	result, err := runner.Uninstall(context.Background(), "all")
	if err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if got, want := fakeEngine.calls, []string{"uninstall:all:all"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all uninstall must be delegated as one lifecycle all-mode call so ledger cleanup is atomic: got %v want %v", got, want)
	}
	if result.Applied != 5 || result.VerifyStatus != sddruntime.StatusPass || !result.LedgerRemoved {
		t.Fatalf("unexpected merged uninstall result: %+v", result)
	}
}

func TestDefaultCockpitRunner_UninstallAllReportsLedgerTruthfully(t *testing.T) {
	fakeEngine := &fakeCockpitLifecycleEngine{
		uninstallResults: map[string]lifecycle.UninstallResult{
			"all:all": {Applied: 5, VerifyStatus: sddruntime.StatusPass, LedgerRemoved: false},
		},
	}
	originalEngine := newCockpitLifecycleEngine
	newCockpitLifecycleEngine = func() cockpitLifecycleService { return fakeEngine }
	t.Cleanup(func() { newCockpitLifecycleEngine = originalEngine })

	result, err := (defaultCockpitRunner{}).Uninstall(context.Background(), "all")
	if err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if result.LedgerRemoved {
		t.Fatalf("ledger_removed must come from lifecycle result, not provider fanout inference: %+v", result)
	}
}

func TestDefaultCockpitRunner_LifecycleEngineUsesHomeDirectoryForManagedState(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	setTestHome(t, home)
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWD) })

	deps := cockpitLifecycleEngineDeps(map[string]lifecycle.ProviderAdapter{})

	if deps.HomeDir != home {
		t.Fatalf("default cockpit lifecycle engine must use HOME for managed state: got %q want %q", deps.HomeDir, home)
	}
	if deps.HomeDir == work {
		t.Fatalf("default cockpit lifecycle engine must not resolve managed state relative to working directory %q", work)
	}
}

func TestDefaultCockpitRunner_LoginHiveCloudWritesConfigAndSyncCredentials(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/login" {
			t.Fatalf("unexpected login request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode login request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"tok","user":{"email":"resolved@example.com"}}`))
	}))
	t.Cleanup(server.Close)

	if err := config.Save(&config.AppConfig{SchemaVersion: 3, APIURL: server.URL}); err != nil {
		t.Fatalf("save config fixture: %v", err)
	}

	email, err := (defaultCockpitRunner{}).LoginHiveCloud(context.Background(), "input@example.com", "secret")
	if err != nil {
		t.Fatalf("LoginHiveCloud returned error: %v", err)
	}
	if email != "resolved@example.com" {
		t.Fatalf("unexpected resolved email: %q", email)
	}
	if request.Email != "input@example.com" || request.Password != "secret" {
		t.Fatalf("unexpected login request body: %+v", request)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if cfg.Email != "resolved@example.com" || cfg.Cloud == nil || !cfg.Cloud.SyncConfigured {
		t.Fatalf("login did not persist cloud config: %+v", cfg)
	}
	// ~/.jarvis/state.yaml owns the scope the login switches to.
	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Scope != state.ScopeLocalCloud {
		t.Fatalf("login did not record the scope in the manifest: %+v", manifest)
	}

	syncJSON, err := os.ReadFile(filepath.Join(home, ".jarvis", "sync.json"))
	if err != nil {
		t.Fatalf("read sync credentials: %v", err)
	}
	assertViewContains(t, string(syncJSON), server.URL, "resolved@example.com", "secret")
}

func TestDefaultCockpitRunner_LoginHiveCloudFallsBackToInputEmailWhenResponseEmailIsBlank(t *testing.T) {
	setTestHome(t, t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"tok","user":{"email":""}}`))
	}))
	t.Cleanup(server.Close)
	if err := config.Save(&config.AppConfig{SchemaVersion: 3, APIURL: server.URL}); err != nil {
		t.Fatalf("save config fixture: %v", err)
	}

	email, err := (defaultCockpitRunner{}).LoginHiveCloud(context.Background(), "input@example.com", "secret")
	if err != nil {
		t.Fatalf("LoginHiveCloud returned error: %v", err)
	}
	if email != "input@example.com" {
		t.Fatalf("expected input email fallback, got %q", email)
	}
}

func TestDefaultCockpitRunner_LoginHiveCloudSurfacesAuthErrorsWithoutPersistingCloudState(t *testing.T) {
	setTestHome(t, t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad credentials"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	if err := config.Save(&config.AppConfig{SchemaVersion: 3, APIURL: server.URL}); err != nil {
		t.Fatalf("save config fixture: %v", err)
	}
	seed := state.New()
	seed.Scope = state.ScopeLocalOnly
	if err := state.Save(seed); err != nil {
		t.Fatalf("save manifest fixture: %v", err)
	}

	_, err := (defaultCockpitRunner{}).LoginHiveCloud(context.Background(), "input@example.com", "bad")
	if err == nil || !strings.Contains(err.Error(), "invalid credentials") {
		t.Fatalf("expected invalid credentials error, got %v", err)
	}
	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if cfg.Cloud != nil {
		t.Fatalf("failed login must not persist cloud state: %+v", cfg)
	}
	manifest, loadErr := state.Load()
	if loadErr != nil {
		t.Fatalf("load manifest: %v", loadErr)
	}
	if manifest.Scope != state.ScopeLocalOnly {
		t.Fatalf("failed login must not switch the recorded scope: %+v", manifest)
	}
}

func TestDefaultCockpitRunner_ApplyPersonaPresetPersistsConfigAndWritesAgentArtifacts(t *testing.T) {
	setTestHome(t, t.TempDir())
	seed := state.New()
	seed.Persona = "old"
	seed.PersonaSource = state.PersonaSourceBuiltin
	if err := state.Save(seed); err != nil {
		t.Fatalf("save manifest fixture: %v", err)
	}
	previousResolver := resolveProfileForWizard
	resolveProfileForWizard = func(fs.FS, string) (*persona.ResolvedProfile, error) {
		return &persona.ResolvedProfile{
			Slug:   "neutra",
			Source: persona.PresetSourceBuiltin,
			Preset: &persona.Profile{SchemaVersion: 2, Name: "neutra", DisplayName: "Neutra", Presentation: persona.Presentation{Language: "en-us", Register: "friendly-professional", Vocabulary: "plain-technical", Cadence: "measured", Humor: "warm", EmotionalRange: "supportive", Verbosity: "balanced", Formatting: "structured", TeachingMetaphors: "construction", Examples: "practical", AddressPack: "peer", PhrasePack: "plain", AntiCaricature: "grounded"}},
		}, nil
	}
	t.Cleanup(func() { resolveProfileForWizard = previousResolver })
	fakeAgent := &fakePersonaAgent{name: "claude", supportsOutputStyles: true}

	summary, err := (defaultCockpitRunner{}).ApplyPersonaPreset(context.Background(), personaApplyRequest{
		PresetName:           "neutra",
		PersonaFS:            fstest.MapFS{},
		Agents:               []agent.Agent{fakeAgent},
		Skills:               []config.SkillInfo{{Name: "sdd-apply", Description: "SDD Apply"}},
		PreviousPresetSlug:   "old",
		PreviousPresetSource: persona.PresetSourceBuiltin,
	})
	if err != nil {
		t.Fatalf("ApplyPersonaPreset returned error: %v", err)
	}
	assertViewContains(t, summary, "preset=neutra", "source=builtin", "agents=1")
	if fakeAgent.instructionsWrites != 1 || fakeAgent.outputStyleWrites != 1 || fakeAgent.clearedStyle != "Old" {
		t.Fatalf("persona agent artifacts not updated: %+v", fakeAgent)
	}
	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load saved manifest: %v", err)
	}
	if manifest.Persona != "neutra" || manifest.PersonaSource != state.PersonaSourceBuiltin {
		t.Fatalf("persona not persisted in the manifest: %+v", manifest)
	}
}

func TestDefaultCockpitRunner_ApplyPersonaPresetSurfacesResolverErrors(t *testing.T) {
	previousResolver := resolveProfileForWizard
	resolveProfileForWizard = func(fs.FS, string) (*persona.ResolvedProfile, error) {
		return nil, errors.New("missing preset")
	}
	t.Cleanup(func() { resolveProfileForWizard = previousResolver })

	_, err := (defaultCockpitRunner{}).ApplyPersonaPreset(context.Background(), personaApplyRequest{PresetName: "missing", PersonaFS: fstest.MapFS{}})
	if err == nil || !strings.Contains(err.Error(), "resolve preset: missing preset") {
		t.Fatalf("expected resolver error, got %v", err)
	}
}

func TestDefaultCockpitRunner_LifecycleDefaultsReturnUnsupportedProviderWithoutMutation(t *testing.T) {
	setTestHome(t, t.TempDir())
	runner := defaultCockpitRunner{}
	if _, err := runner.Doctor(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider doctor error, got %v", err)
	}
	if _, err := runner.Verify(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider verify error, got %v", err)
	}
	if _, err := runner.Backup(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider backup error, got %v", err)
	}
	if _, err := runner.Restore(context.Background(), "missing", "snap"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider restore error, got %v", err)
	}
	if _, err := runner.ReconcileDryRun(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider reconcile dry-run error, got %v", err)
	}
	if _, err := runner.Reconcile(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider reconcile error, got %v", err)
	}
	if _, err := runner.UninstallDryRun(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider uninstall dry-run error, got %v", err)
	}
	if _, err := runner.Uninstall(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), `unsupported provider "missing"`) {
		t.Fatalf("expected unsupported provider uninstall error, got %v", err)
	}
}

func TestCockpitModelUsesDefaultRunnerWhenNoTestRunnerIsInjected(t *testing.T) {
	m := NewCockpitModel(WizardConfig{})
	if _, ok := m.runner().(defaultCockpitRunner); !ok {
		t.Fatalf("expected default cockpit runner when no runner is injected, got %T", m.runner())
	}
}

func newCockpitHandlerTestModel(runner cockpitRunner) Model {
	m := NewCockpitModel(WizardConfig{})
	m.cockpitRunner = runner
	return m
}

func selectCockpitAction(t *testing.T, m Model, id CockpitActionID) Model {
	t.Helper()
	actions := CockpitActions()
	for i, action := range actions {
		if action.ID == id {
			m.cockpitCursor = i
			return sendCockpitKey(m, tea.KeyEnter)
		}
	}
	t.Fatalf("action %s not found", id)
	return m
}

func sendCockpitKey(m Model, key tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	return updated.(Model)
}

func typeCockpitText(m Model, text string) Model {
	for _, r := range text {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func clearCockpitInput(m Model) Model {
	for m.cockpitInput != "" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = updated.(Model)
	}
	return m
}

func assertViewContains(t *testing.T, view string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(view, part) {
			t.Fatalf("expected view to contain %q\nview:\n%s", part, view)
		}
	}
}

type fakeCockpitRunner struct {
	calls            []string
	configSummary    string
	configErr        error
	personaSummary   string
	personaErr       error
	loginEmail       string
	loginErr         error
	doctorPlan       lifecycle.DoctorPlan
	verifyResult     lifecycle.VerifyResult
	backupSnapshotID string
	backupErr        error
	reconcilePlan    lifecycle.DoctorPlan
	reconcileResult  lifecycle.ReconcileResult
	uninstallPlan    lifecycle.DoctorPlan
	uninstallResult  lifecycle.UninstallResult
	verifyErr        error
}

type fakeCockpitLifecycleEngine struct {
	calls            []string
	uninstallResults map[string]lifecycle.UninstallResult
	uninstallErrors  map[string]error
}

func (f *fakeCockpitLifecycleEngine) Doctor(provider string) (lifecycle.DoctorPlan, error) {
	f.calls = append(f.calls, "doctor:"+provider)
	return lifecycle.DoctorPlan{Provider: provider, Status: sddruntime.StatusPass, ReadOnly: true}, nil
}
func (f *fakeCockpitLifecycleEngine) Verify(provider string) (lifecycle.VerifyResult, error) {
	f.calls = append(f.calls, "verify:"+provider)
	return lifecycle.VerifyResult{Status: sddruntime.StatusPass}, nil
}
func (f *fakeCockpitLifecycleEngine) Backup(provider, sourceOperation string) (string, error) {
	f.calls = append(f.calls, "backup:"+provider+":"+sourceOperation)
	return "snap", nil
}
func (f *fakeCockpitLifecycleEngine) Restore(provider, snapshotID string) (lifecycle.RestoreResult, error) {
	f.calls = append(f.calls, "restore:"+provider+":"+snapshotID)
	return lifecycle.RestoreResult{Restored: 1}, nil
}
func (f *fakeCockpitLifecycleEngine) ReconcileDryRun(provider string) (lifecycle.DoctorPlan, error) {
	f.calls = append(f.calls, "reconcile-dry-run:"+provider)
	return lifecycle.DoctorPlan{Provider: provider, Status: sddruntime.StatusPass, ReadOnly: true}, nil
}
func (f *fakeCockpitLifecycleEngine) Reconcile(provider string) (lifecycle.ReconcileResult, error) {
	f.calls = append(f.calls, "reconcile:"+provider)
	return lifecycle.ReconcileResult{Applied: 1}, nil
}
func (f *fakeCockpitLifecycleEngine) Uninstall(provider, mode string) (lifecycle.UninstallResult, error) {
	f.calls = append(f.calls, "uninstall:"+provider+":"+mode)
	if err := f.uninstallErrors[provider+":"+mode]; err != nil {
		return lifecycle.UninstallResult{}, err
	}
	return f.uninstallResults[provider+":"+mode], nil
}

func (f *fakeCockpitRunner) ConfigSummary(context.Context) (string, error) {
	f.calls = append(f.calls, "config")
	if f.configSummary == "" {
		f.configSummary = "config"
	}
	return f.configSummary, f.configErr
}
func (f *fakeCockpitRunner) ApplyPersonaPreset(_ context.Context, req personaApplyRequest) (string, error) {
	f.calls = append(f.calls, "persona:"+req.PresetName)
	return f.personaSummary, f.personaErr
}
func (f *fakeCockpitRunner) LoginHiveCloud(_ context.Context, email, password string) (string, error) {
	f.calls = append(f.calls, "login:"+email+":"+password)
	return f.loginEmail, f.loginErr
}
func (f *fakeCockpitRunner) Doctor(_ context.Context, provider string) (lifecycle.DoctorPlan, error) {
	f.calls = append(f.calls, "doctor:"+provider)
	return f.doctorPlan, nil
}
func (f *fakeCockpitRunner) Verify(_ context.Context, provider string) (lifecycle.VerifyResult, error) {
	f.calls = append(f.calls, "verify:"+provider)
	return f.verifyResult, f.verifyErr
}
func (f *fakeCockpitRunner) Backup(_ context.Context, provider string) (string, error) {
	f.calls = append(f.calls, "backup:"+provider)
	return f.backupSnapshotID, f.backupErr
}
func (f *fakeCockpitRunner) Restore(_ context.Context, provider, snapshotID string) (lifecycle.RestoreResult, error) {
	f.calls = append(f.calls, "restore:"+provider+":"+snapshotID)
	return lifecycle.RestoreResult{Restored: 3}, nil
}
func (f *fakeCockpitRunner) ReconcileDryRun(_ context.Context, provider string) (lifecycle.DoctorPlan, error) {
	f.calls = append(f.calls, "reconcile-dry-run:"+provider)
	return f.reconcilePlan, nil
}
func (f *fakeCockpitRunner) Reconcile(_ context.Context, provider string) (lifecycle.ReconcileResult, error) {
	f.calls = append(f.calls, "reconcile:"+provider)
	return f.reconcileResult, nil
}
func (f *fakeCockpitRunner) UninstallDryRun(_ context.Context, provider string) (lifecycle.DoctorPlan, error) {
	f.calls = append(f.calls, "uninstall-dry-run:"+provider)
	return f.uninstallPlan, nil
}
func (f *fakeCockpitRunner) Uninstall(_ context.Context, provider string) (lifecycle.UninstallResult, error) {
	f.calls = append(f.calls, "uninstall:"+provider)
	return f.uninstallResult, nil
}

type fakePersonaAgent struct {
	name                 string
	supportsOutputStyles bool
	instructionsWrites   int
	outputStyleWrites    int
	clearedStyle         string
}

func (f *fakePersonaAgent) Name() string                     { return f.name }
func (f *fakePersonaAgent) IsInstalled() bool                { return true }
func (f *fakePersonaAgent) ConfigDir() string                { return "" }
func (f *fakePersonaAgent) MergeConfig(agent.MCPEntry) error { return nil }
func (f *fakePersonaAgent) WriteInstructions(string, string, []config.SkillInfo) error {
	f.instructionsWrites++
	return nil
}
func (f *fakePersonaAgent) InstallSkills(fs.FS, []string) error { return nil }
func (f *fakePersonaAgent) InstallOrchestrator([]byte) error    { return nil }
func (f *fakePersonaAgent) SupportsOutputStyles() bool          { return f.supportsOutputStyles }
func (f *fakePersonaAgent) WriteOutputStyle(*persona.Profile) error {
	f.outputStyleWrites++
	return nil
}
func (f *fakePersonaAgent) ClearOutputStyle(name string) error {
	f.clearedStyle = name
	return nil
}
func (f *fakePersonaAgent) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return sddruntime.RuntimePlan{}, nil
}
func (f *fakePersonaAgent) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return sddruntime.ObservedRuntime{}, nil
}
func (f *fakePersonaAgent) InstallPromptHook(fs.FS) error   { return nil }
func (f *fakePersonaAgent) InstallSessionHooks(fs.FS) error { return nil }
