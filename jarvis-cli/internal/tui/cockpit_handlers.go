package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

type cockpitMode int

const (
	cockpitModeMenu cockpitMode = iota
	cockpitModeProvider
	cockpitModeInput
	cockpitModeConfirm
	cockpitModeResult
	cockpitModePersona
	cockpitModeLogin
)

var cockpitProviders = []string{"all", "claude", "opencode"}

func startCockpitHandler(m Model, action CockpitAction) Model {
	m.cockpitAction = action.ID
	m.cockpitMessage = ""
	m.cockpitInput = ""
	m.cockpitSnapshot = ""
	m.cockpitPlan = ""
	m.cockpitProviderCursor = 0
	m.cockpitProvider = cockpitProviders[0]

	switch action.ID {
	case CockpitActionPersona:
		m.cockpitMode = cockpitModePersona
		if len(m.Presets) == 0 {
			m.cockpitMessage = "No persona presets available."
		}
	case CockpitActionHiveCloudLogin:
		m.cockpitMode = cockpitModeLogin
		m.activeField = 0
		m.Password = ""
		m.cockpitMessage = "Hive Cloud Login"
	case CockpitActionConfig:
		return runConfigCockpitAction(m)
	case CockpitActionDoctor, CockpitActionVerify, CockpitActionBackup, CockpitActionRestore, CockpitActionReconcile, CockpitActionUninstall:
		m.cockpitMode = cockpitModeProvider
	default:
		m.cockpitMessage = action.Label + " is not wired yet."
	}
	return m
}

func updateCockpitHandler(m Model, key keyInput) Model {
	switch m.cockpitMode {
	case cockpitModeProvider:
		return updateCockpitProvider(m, key)
	case cockpitModeInput:
		return updateCockpitInput(m, key)
	case cockpitModeConfirm:
		return updateCockpitConfirm(m, key)
	case cockpitModeResult:
		if key.enter {
			return resetCockpitToMenu(m)
		}
	case cockpitModePersona:
		return updateCockpitPersona(m, key)
	case cockpitModeLogin:
		return updateCockpitLogin(m, key)
	}
	return m
}

type keyInput struct {
	up, down, enter, backspace bool
	runes                      []rune
}

func updateCockpitProvider(m Model, key keyInput) Model {
	if key.up {
		m.cockpitProviderCursor = previousCockpitIndex(m.cockpitProviderCursor, len(cockpitProviders))
	}
	if key.down {
		m.cockpitProviderCursor = nextCockpitIndex(m.cockpitProviderCursor, len(cockpitProviders))
	}
	m.cockpitProvider = cockpitProviders[m.cockpitProviderCursor]
	if !key.enter {
		return m
	}

	switch m.cockpitAction {
	case CockpitActionDoctor:
		plan, err := m.runner().Doctor(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Doctor", err)
		}
		return cockpitResult(m, "Doctor result", formatDoctorPlan(plan))
	case CockpitActionVerify:
		result, err := m.runner().Verify(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Verify", err)
		}
		return cockpitResult(m, "Verify result", fmt.Sprintf("provider=%s status=%s", m.cockpitProvider, strings.ToUpper(string(result.Status))))
	case CockpitActionBackup:
		m.cockpitMode = cockpitModeConfirm
		m.cockpitMessage = "Confirm backup for provider " + m.cockpitProvider + " with Enter."
		return m
	case CockpitActionRestore:
		m.cockpitMode = cockpitModeInput
		m.cockpitMessage = "Snapshot id"
		return m
	case CockpitActionReconcile:
		plan, err := m.runner().ReconcileDryRun(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Reconcile dry-run", err)
		}
		m.cockpitPlan = formatDoctorPlan(plan)
		m.cockpitMode = cockpitModeConfirm
		m.cockpitMessage = "Reconcile dry-run\n" + m.cockpitPlan + "\nType RECONCILE to mutate."
		return m
	case CockpitActionUninstall:
		plan, err := m.runner().UninstallDryRun(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Uninstall safety diagnosis", err)
		}
		m.cockpitPlan = formatDoctorPlan(plan)
		m.cockpitMode = cockpitModeConfirm
		m.cockpitMessage = "Uninstall safety diagnosis\n" + m.cockpitPlan + "\nThis is a read-only diagnosis, not a full uninstall plan.\nType UNINSTALL to mutate."
		return m
	}
	return m
}

func updateCockpitInput(m Model, key keyInput) Model {
	m = editCockpitInput(m, key)
	if key.enter {
		m.cockpitSnapshot = strings.TrimSpace(m.cockpitInput)
		if m.cockpitSnapshot == "" {
			m.cockpitMessage = "Snapshot id is required"
			return m
		}
		m.cockpitInput = ""
		m.cockpitMode = cockpitModeConfirm
		m.cockpitMessage = "Type RESTORE to mutate."
	}
	return m
}

func updateCockpitPersona(m Model, key keyInput) Model {
	if len(m.Presets) == 0 {
		if key.enter {
			return cockpitError(m, "Persona", fmt.Errorf("no persona presets available"))
		}
		return m
	}
	if key.up {
		m.presetCur = previousCockpitIndex(m.presetCur, len(m.Presets))
	}
	if key.down {
		m.presetCur = nextCockpitIndex(m.presetCur, len(m.Presets))
	}
	if !key.enter {
		return m
	}
	if m.presetCur < 0 || m.presetCur >= len(m.Presets) {
		if m.personaSelectionErr != nil {
			return cockpitError(m, "Persona", m.personaSelectionErr)
		}
		m.presetCur = 0
	}

	selected := m.Presets[m.presetCur]
	if persona.NormalizeSlug(selected.Name) == "custom" {
		return cockpitResult(m, "Custom persona", "Custom persona editing keeps the installer-equivalent validation path in Install/Reconfigure. Cockpit custom editing is an extension seam and is not applied here.")
	}

	// ApplyPersonaPreset rewrites every agent's instructions and output styles
	// before its own state.Update refuses, so the refusal has to happen here,
	// before the first file is written.
	if err := m.manifestWriteGuard(); err != nil {
		return cockpitError(m, "Persona", err)
	}

	summary, err := m.runner().ApplyPersonaPreset(context.Background(), personaApplyRequest{
		PresetName:           selected.Name,
		PersonaFS:            m.PersonaFS,
		Agents:               m.Agents,
		Skills:               buildSkillInfoList(m),
		PreviousPresetSlug:   m.previousPresetSlug,
		PreviousPresetSource: m.previousPresetSource,
	})
	if err != nil {
		return cockpitError(m, "Persona", err)
	}
	if summary == "" {
		summary = "preset=" + persona.NormalizeSlug(selected.Name)
	}
	// ApplyPersonaPreset already recorded the persona in ~/.jarvis/state.yaml.
	// This keeps the in-memory manifest the cockpit reads from in step with it.
	if m.manifest != nil {
		m.manifest.Persona = persona.NormalizeSlug(selected.Name)
		m.manifest.PersonaSource = state.PersonaSource(persona.PresetSourceBuiltin)
	}
	return cockpitResult(m, "Persona result", summary)
}

func updateCockpitLogin(m Model, key keyInput) Model {
	if key.backspace {
		if m.activeField == 0 && m.Email != "" {
			m.Email = m.Email[:len(m.Email)-1]
		} else if m.activeField == 1 && m.Password != "" {
			m.Password = m.Password[:len(m.Password)-1]
		}
	}
	if len(key.runes) > 0 {
		if m.activeField == 0 {
			m.Email += string(key.runes)
		} else {
			m.Password += string(key.runes)
		}
	}
	if !key.enter {
		return m
	}
	if m.activeField == 0 {
		m.activeField = 1
		return m
	}
	email, err := normalizeHiveCloudEmail(m.Email)
	if err != nil {
		m.cockpitMessage = err.Error()
		return m
	}
	password := m.Password
	if email == "" || password == "" {
		m.cockpitMessage = "Email and password are required."
		return m
	}
	// LoginHiveCloud writes the sync credentials before its own state.Update
	// refuses, so the refusal has to happen here, before that write.
	if err := m.manifestWriteGuard(); err != nil {
		return cockpitError(m, "Hive Cloud Login", err)
	}
	resolvedEmail, err := m.runner().LoginHiveCloud(context.Background(), email, password)
	if err != nil {
		return cockpitError(m, "Hive Cloud Login", err)
	}
	if resolvedEmail == "" {
		resolvedEmail = email
	}
	if m.cfg != nil {
		m.cfg.Email = resolvedEmail
		if m.cfg.Cloud == nil {
			m.cfg.Cloud = &config.CloudConfig{}
		}
		m.cfg.Cloud.Email = resolvedEmail
		m.cfg.Cloud.SyncConfigured = true
		m.Scope = state.ScopeLocalCloud
	}
	// LoginHiveCloud already recorded the scope in ~/.jarvis/state.yaml, which
	// owns it. This keeps the in-memory manifest in step with it.
	if m.manifest != nil {
		m.manifest.Scope = state.ScopeLocalCloud
	}
	m.Email = resolvedEmail
	m.Password = ""
	return cockpitResult(m, "Hive Cloud Login result", "email="+resolvedEmail)
}

func updateCockpitConfirm(m Model, key keyInput) Model {
	if m.cockpitAction == CockpitActionBackup && key.enter {
		snapshot, err := m.runner().Backup(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Backup", err)
		}
		return cockpitResult(m, "Backup result", "snapshot="+snapshot)
	}
	m = editCockpitInput(m, key)
	if !key.enter {
		return m
	}

	confirm := strings.TrimSpace(m.cockpitInput)
	switch m.cockpitAction {
	case CockpitActionRestore:
		if confirm != "RESTORE" {
			m.cockpitMessage = "confirmation did not match; Type RESTORE to mutate."
			m.cockpitInput = ""
			return m
		}
		result, err := m.runner().Restore(context.Background(), m.cockpitProvider, m.cockpitSnapshot)
		if err != nil {
			return cockpitError(m, "Restore", err)
		}
		return cockpitResult(m, "Restore result", fmt.Sprintf("restored=%d", result.Restored))
	case CockpitActionReconcile:
		if confirm != "RECONCILE" {
			m.cockpitMessage = "confirmation did not match; Type RECONCILE to mutate."
			m.cockpitInput = ""
			return m
		}
		result, err := m.runner().Reconcile(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Reconcile", err)
		}
		return cockpitResult(m, "Reconcile result", fmt.Sprintf("applied=%d manual_required=%d skipped=%d", result.Applied, result.ManualRequired, len(result.SkippedNonOwned)))
	case CockpitActionUninstall:
		if confirm != "UNINSTALL" {
			m.cockpitMessage = "confirmation did not match; Type UNINSTALL to mutate."
			m.cockpitInput = ""
			return m
		}
		result, err := m.runner().Uninstall(context.Background(), m.cockpitProvider)
		if err != nil {
			return cockpitError(m, "Uninstall", err)
		}
		return cockpitResult(m, "Uninstall result", fmt.Sprintf("applied=%d verify_status=%s ledger_removed=%t", result.Applied, result.VerifyStatus, result.LedgerRemoved))
	}
	return m
}

func editCockpitInput(m Model, key keyInput) Model {
	if key.backspace && m.cockpitInput != "" {
		m.cockpitInput = m.cockpitInput[:len(m.cockpitInput)-1]
	}
	if len(key.runes) > 0 {
		m.cockpitInput += string(key.runes)
	}
	return m
}

func runConfigCockpitAction(m Model) Model {
	summary, err := m.runner().ConfigSummary(context.Background())
	if err != nil {
		return cockpitError(m, "Config", err)
	}
	return cockpitResult(m, "Config result", summary)
}

func cockpitResult(m Model, title, body string) Model {
	m.cockpitMode = cockpitModeResult
	m.cockpitMessage = title + "\n" + body
	m.cockpitInput = ""
	return m
}

func cockpitError(m Model, title string, err error) Model {
	return cockpitResult(m, title+" error", err.Error())
}

func resetCockpitToMenu(m Model) Model {
	m.cockpitMode = cockpitModeMenu
	m.cockpitAction = ""
	m.cockpitMessage = ""
	m.cockpitInput = ""
	m.cockpitSnapshot = ""
	m.cockpitPlan = ""
	return m
}

func (m Model) runner() cockpitRunner {
	if m.cockpitRunner == nil {
		return defaultCockpitRunner{}
	}
	return m.cockpitRunner
}

func formatDoctorPlan(plan lifecycle.DoctorPlan) string {
	return fmt.Sprintf("provider=%s status=%s read_only=%t steps=%d", plan.Provider, strings.ToUpper(string(plan.Status)), plan.ReadOnly, len(plan.Steps))
}
