package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/apiclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

const localOnlyReviewWarning = "Se ha seleccionado modo local, se borrará cualquier credencial almacenada sobre hive-api"

// ──────────────────────────────────────────────────────────────────────────────
// Style helpers — Catppuccin Mocha semantic styles derived from terminalui
// ──────────────────────────────────────────────────────────────────────────────

var (
	// errorStyle renders error messages in Catppuccin Red.
	errorStyle = lipgloss.NewStyle().Foreground(terminalui.ColorRed).Bold(true)
	// warningStyle renders warning messages in Catppuccin Yellow.
	warningStyle = lipgloss.NewStyle().Foreground(terminalui.ColorYellow)
)

// ──────────────────────────────────────────────────────────────────────────────
// Step 1: Scope
// ──────────────────────────────────────────────────────────────────────────────

func updateScope(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.Scope = config.ScopeLocalOnly
		m.cfg.Scope = m.Scope
	case tea.KeyDown:
		m.Scope = config.ScopeLocalCloud
		m.cfg.Scope = m.Scope
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			m.Scope = config.ScopeLocalOnly
			m.cfg.Scope = m.Scope
		case "j":
			m.Scope = config.ScopeLocalCloud
			m.cfg.Scope = m.Scope
		}
	case tea.KeyEnter:
		m.Err = nil
		if m.Scope == config.ScopeLocalCloud {
			m.Step = StepHiveCloud
		} else {
			m.Step = StepPersona
		}
	}
	return m, nil
}

func viewScope(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	sb.WriteString(terminalui.HeaderRow("Setup › Scope", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	sb.WriteString(terminalui.SectionHeader("SCOPE", w))

	var localLine string
	var cloudLine string
	if m.Scope == config.ScopeLocalOnly {
		localLine = terminalui.SelectedRow("local-only", w)
		cloudLine = terminalui.DimTextStyle.Render("  local+cloud")
	} else {
		localLine = terminalui.DimTextStyle.Render("  local-only")
		cloudLine = terminalui.SelectedRow("local+cloud", w)
	}
	content := localLine + "\n" + cloudLine + "\n\n" +
		terminalui.DimTextStyle.Render("local-only: setup local sin cloud. local+cloud: incluye auth/enlace cloud.")
	if m.Err != nil {
		content += "\n\n" + errorStyle.Render("Error: "+m.Err.Error())
	}
	sb.WriteString(terminalui.BorderedPanel(content, w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "cambiar"},
		{Key: "Enter", Desc: "continuar"},
		{Key: "Ctrl+C", Desc: "salir"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 2: HiveCloud
// ──────────────────────────────────────────────────────────────────────────────

func updateHiveCloud(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyShiftTab:
		// Toggle between email (0) and password (1) fields.
		if m.activeField == 0 {
			m.activeField = 1
		} else {
			m.activeField = 0
		}
	case tea.KeyEnter:
		if m.activeField == 0 {
			// Move focus to password field on Enter from email.
			m.activeField = 1
			return m, nil
		}
		// Enter on password field: attempt login.
		email, err := normalizeHiveCloudEmail(m.Email)
		if err != nil {
			m.Err = err
			return m, nil
		}
		if email == "" {
			// Skip cloud auth entirely.
			m.Step = StepPersona
			return m, nil
		}
		m.Email = email
		return m, loginCmd(m.cfg.APIURL, email, m.Password)
	case tea.KeyRunes:
		if m.activeField == 0 {
			m.Email += string(msg.Runes)
		} else {
			m.Password += string(msg.Runes)
		}
	case tea.KeyBackspace:
		if m.activeField == 0 && len(m.Email) > 0 {
			m.Email = m.Email[:len(m.Email)-1]
		} else if m.activeField == 1 && len(m.Password) > 0 {
			m.Password = m.Password[:len(m.Password)-1]
		}
	case tea.KeyEsc:
		// Skip cloud auth step.
		m.Email = ""
		m.Password = ""
		m.Step = StepPersona
	}
	return m, nil
}

// loginResultMsg is returned by the login async command.
type loginResultMsg struct {
	token string
	email string
	err   error
}

// loginCmd performs an async Hive Cloud login.
func loginCmd(apiURL, email, password string) tea.Cmd {
	return func() tea.Msg {
		c := apiclient.New(apiURL)
		resp, err := c.Login(email, password)
		if err != nil {
			return loginResultMsg{err: err}
		}
		resolvedEmail := strings.TrimSpace(resp.User.Email)
		if resolvedEmail == "" {
			resolvedEmail = email
		}
		return loginResultMsg{token: resp.Token, email: resolvedEmail}
	}
}

func (m Model) handleLoginResult(msg loginResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.Err = msg.err
		return m, nil
	}
	m.APIToken = msg.token
	if m.cfg.Cloud == nil {
		m.cfg.Cloud = &config.CloudConfig{}
	}
	m.cfg.Cloud.Email = msg.email
	m.cfg.Cloud.SyncConfigured = true
	m.Email = msg.email
	m.cfg.Email = msg.email
	m.Err = nil
	m.Step = StepPersona
	return m, nil
}

// writeSyncJSON writes ~/.jarvis/sync.json with cloud credentials.
// Only api_url, email, and password are stored — token is intentionally
// excluded because hive-daemon's syncFileConfig uses DisallowUnknownFields()
// and manages the token internally after login.
// autoSync follows the tri-state semantics of config.WriteSyncCredentials:
// nil preserves any existing value, &true forces enable, &false forces disable.
func writeSyncJSON(apiURL, email, password string, autoSync *bool) error {
	return config.WriteSyncCredentials(apiURL, email, password, autoSync)
}

// Override Update to also handle loginResultMsg (needs to be wired in root Update).
// We embed the handling here and call from model.go's Update.
func handleStepMsg(m Model, msg tea.Msg) (Model, bool, tea.Cmd) {
	if m.Step == StepHiveCloud {
		if lr, ok := msg.(loginResultMsg); ok {
			updated, cmd := m.handleLoginResult(lr)
			return updated.(Model), true, cmd
		}
	}
	if m.Step == StepApply {
		if pr, ok := msg.(agentProgressMsg); ok {
			m.agentProgress = append(m.agentProgress, pr.line)
			if pr.failed {
				m.Err = errors.New(pr.line)
			}
			if pr.done {
				m.agentDone = true
			}
			return m, true, nil
		}
	}
	return m, false, nil
}

func viewHiveCloud(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	breadcrumb := "Setup › Hive Cloud"
	if m.Mode == string(config.ConfigStatusReconfigure) {
		breadcrumb = "Setup › Hive Cloud (Reconfigure)"
	}
	sb.WriteString(terminalui.HeaderRow(breadcrumb, terminalui.ModeBadge("normal"), m.width) + "\n\n")

	// Email field
	var emailLine string
	if m.activeField == 0 {
		emailLine = terminalui.SelectedRow("Email:  "+m.Email, w)
	} else {
		emailLine = terminalui.DimTextStyle.Render("  Email:  ") + m.Email
	}

	// Password field — mask with bullet characters, never in clear text.
	masked := strings.Repeat("•", len(m.Password))
	var passLine string
	if m.activeField == 1 {
		passLine = terminalui.SelectedRow("Password: "+masked, w)
	} else {
		passLine = terminalui.DimTextStyle.Render("  Password:") + " " + masked
	}

	content := terminalui.DimTextStyle.Render("Connect to Hive Cloud for team memory sync (press Esc to skip).") + "\n\n" +
		emailLine + "\n" +
		passLine
	if m.Err != nil {
		content += "\n\n" + errorStyle.Render("Error: "+m.Err.Error())
	}
	sb.WriteString(terminalui.BorderedPanel(content, w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "Tab", Desc: "switch field"},
		{Key: "Enter", Desc: "next/login"},
		{Key: "Esc", Desc: "skip"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 3: Persona
// ──────────────────────────────────────────────────────────────────────────────

func updatePersona(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.customEdit {
		return updatePersonaCustomEdit(m, msg)
	}
	if m.personaSelectionErr != nil {
		if msg.Type == tea.KeyEnter {
			m.Err = m.personaSelectionErr
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.presetCur > 0 {
			m.presetCur--
		}
	case tea.KeyDown:
		if m.presetCur < len(m.Presets)-1 {
			m.presetCur++
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if m.presetCur > 0 {
				m.presetCur--
			}
		case "j":
			if m.presetCur < len(m.Presets)-1 {
				m.presetCur++
			}
		}
	case tea.KeyEnter:
		if len(m.Presets) == 0 {
			m.Step = StepExtraSkills
			return m, nil
		}
		selected := m.Presets[m.presetCur]
		if selected.Name == "custom" {
			// Enter custom creation mode.
			m.customEdit = true
			m.customField = 0
			m.Err = nil
			return m, nil
		}
		resolved, err := resolveWizardPresetSelection(m.PersonaFS, selected.Name, nil)
		if err == nil {
			m.selectedPreset = nil
			m.selectedPresetV2 = resolved
			m.cfg.PersonaPreset = resolved.Slug
			m.cfg.Preset = resolved.Slug
			m.cfg.PersonaPresetSource = string(resolved.Source)
		} else {
			m.selectedPreset = nil
			m.cfg.PersonaPreset = selected.Name
			m.cfg.Preset = selected.Name
			m.cfg.PersonaPresetSource = string(persona.PresetSourceBuiltin)
		}
		m.Err = nil
		m.Step = StepExtraSkills
	}
	return m, nil
}

func updatePersonaCustomEdit(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyShiftTab:
		m.customField = (m.customField + 1) % 3
	case tea.KeyCtrlS:
		resolved, err := resolveWizardPresetSelection(m.PersonaFS, "custom", &customPresetDraft{
			Name:        m.customPresetName,
			DisplayName: m.customDisplayName,
			YAML:        m.CustomYAML,
		})
		if err != nil {
			m.Err = err
			return m, nil
		}
		m.selectedPreset = nil
		m.selectedPresetV2 = resolved
		m.cfg.PersonaPreset = resolved.Slug
		m.cfg.Preset = resolved.Slug
		m.cfg.PersonaPresetSource = string(resolved.Source)
		m.customEdit = false
		m.Err = nil
		m.Step = StepExtraSkills
	case tea.KeyEsc:
		m.customEdit = false
		m.Err = nil
	case tea.KeyRunes:
		switch m.customField {
		case 0:
			m.customPresetName += string(msg.Runes)
		case 1:
			m.customDisplayName += string(msg.Runes)
		default:
			candidate := m.CustomYAML + string(msg.Runes)
			if err := validateCustomPresetYAMLSize(candidate); err != nil {
				m.Err = err
				return m, nil
			}
			m.CustomYAML = candidate
		}
	case tea.KeyBackspace:
		switch m.customField {
		case 0:
			if len(m.customPresetName) > 0 {
				m.customPresetName = m.customPresetName[:len(m.customPresetName)-1]
			}
		case 1:
			if len(m.customDisplayName) > 0 {
				m.customDisplayName = m.customDisplayName[:len(m.customDisplayName)-1]
			}
		default:
			if len(m.CustomYAML) > 0 {
				m.CustomYAML = m.CustomYAML[:len(m.CustomYAML)-1]
			}
		}
	case tea.KeyEnter:
		if m.customField < 2 {
			m.customField++
		} else {
			candidate := m.CustomYAML + "\n"
			if err := validateCustomPresetYAMLSize(candidate); err != nil {
				m.Err = err
				return m, nil
			}
			m.CustomYAML = candidate
		}
	}
	return m, nil
}

func viewPersona(m Model) string {
	w := terminalui.PanelWidth(m.width)

	if m.customEdit {
		var sb strings.Builder
		sb.WriteString(terminalui.HeaderRow("Setup › Persona › Edit", terminalui.ModeBadge("normal"), m.width) + "\n\n")

		if m.Err != nil {
			sb.WriteString(errorStyle.Render("Error: "+m.Err.Error()) + "\n\n")
		}

		nameLabel := "  Name (slug base): " + m.customPresetName
		displayLabel := "  Display name: " + m.customDisplayName
		yamlLabel := "  YAML draft:"
		switch m.customField {
		case 0:
			nameLabel = terminalui.SelectedRow("Name (slug base): "+m.customPresetName, w)
		case 1:
			displayLabel = terminalui.SelectedRow("Display name: "+m.customDisplayName, w)
		default:
			yamlLabel = terminalui.SelectedRow("YAML draft:", w)
		}

		var fieldsSB strings.Builder
		fieldsSB.WriteString(nameLabel + "\n")
		fieldsSB.WriteString(displayLabel + "\n\n")
		fieldsSB.WriteString(yamlLabel + "\n")
		fieldsSB.WriteString(m.CustomYAML + "_")
		sb.WriteString(terminalui.BorderedPanel(fieldsSB.String(), w) + "\n")

		hints := []terminalui.KeyHint{
			{Key: "Tab", Desc: "switch field"},
			{Key: "Enter", Desc: "next/newline"},
			{Key: "Ctrl+S", Desc: "save"},
			{Key: "Esc", Desc: "cancel"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
		return sb.String()
	}

	var sb strings.Builder
	sb.WriteString(terminalui.HeaderRow("Setup › Persona", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	if len(m.Presets) == 0 {
		sb.WriteString(terminalui.BorderedPanel("No personas configured. Press Enter to continue.", w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "Enter", Desc: "continue"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
		return sb.String()
	}

	sb.WriteString(terminalui.SectionHeader("PERSONAS", w))
	var listSB strings.Builder
	for i, p := range m.Presets {
		name := p.DisplayName
		if name == "" {
			name = p.Name
		}
		line := name
		if p.Description != "" {
			line += " — " + p.Description
		}
		if i == m.presetCur {
			listSB.WriteString(terminalui.SelectedRow(line, w) + "\n")
		} else {
			listSB.WriteString(terminalui.DimTextStyle.Render("  "+line) + "\n")
		}
	}
	if m.Err != nil {
		listSB.WriteString("\n" + errorStyle.Render("Error: "+m.Err.Error()))
	}
	sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "Enter", Desc: "select"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 4: Skills
// ──────────────────────────────────────────────────────────────────────────────

func updateSkills(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Find the index of the currently highlighted skill.
	// We track cursor in the same field reusing presetCur for simplicity.
	cur := m.presetCur
	if cur >= len(m.SkillPrompts) {
		cur = 0
	}

	switch msg.Type {
	case tea.KeyUp:
		if cur > 0 {
			m.presetCur = cur - 1
		}
	case tea.KeyDown:
		if cur < len(m.SkillPrompts)-1 {
			m.presetCur = cur + 1
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if cur > 0 {
				m.presetCur = cur - 1
			}
		case "j":
			if cur < len(m.SkillPrompts)-1 {
				m.presetCur = cur + 1
			}
		case " ":
			if cur < len(m.SkillPrompts) {
				prompt := m.SkillPrompts[cur]
				next := !m.Selected[prompt.SkillIDs[0]]
				for _, id := range prompt.SkillIDs {
					m.Selected[id] = next
				}
			}
		}
	case tea.KeySpace:
		if cur < len(m.SkillPrompts) {
			prompt := m.SkillPrompts[cur]
			next := !m.Selected[prompt.SkillIDs[0]]
			for _, id := range prompt.SkillIDs {
				m.Selected[id] = next
			}
		}
	case tea.KeyEnter:
		if hasPhaseModelRuntimeTarget(m.Agents) {
			m.Step = StepPhaseModels
		} else {
			m.Step = StepReview
		}
	}
	return m, nil
}

func viewSkills(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	sb.WriteString(terminalui.HeaderRow("Setup › Skills", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	var listSB strings.Builder
	listSB.WriteString(terminalui.SectionHeader("AVAILABLE", w))
	listSB.WriteString(terminalui.DimTextStyle.Render("Required/default skills are installed automatically. Select only stack-specific extras.") + "\n\n")

	cur := m.presetCur
	for i, prompt := range m.SkillPrompts {
		check := "[ ]"
		if len(prompt.SkillIDs) > 0 && m.Selected[prompt.SkillIDs[0]] {
			check = "[x]"
		}
		line := fmt.Sprintf("%s %s — %s", check, prompt.Label, prompt.Description)
		if i == cur {
			listSB.WriteString(terminalui.SelectedRow(line, w) + "\n")
		} else {
			listSB.WriteString("  " + line + "\n")
		}
	}
	if len(m.SkillPrompts) == 0 {
		listSB.WriteString(terminalui.DimTextStyle.Render("No stack-specific skill prompts available for this catalog.") + "\n")
	}
	sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "Space", Desc: "toggle"},
		{Key: "Enter", Desc: "confirm"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

type phaseModelRow struct {
	Phase              string
	OpenCode           string
	OpenCodeAssignment config.OpenCodeModelAssignment
	Claude             string
	ClaudeEffort       string
}

func updatePhaseModels(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.phaseModelRows) == 0 {
		m = initializePhaseModelEditor(m)
	}
	if m.phaseModelMode != phaseModelModeList {
		return updatePhaseModelPicker(m, msg), nil
	}
	changed := false
	switch msg.Type {
	case tea.KeyUp:
		if m.phaseModelActiveRow > 0 {
			m.phaseModelActiveRow--
		}
	case tea.KeyDown:
		if m.phaseModelActiveRow < len(m.phaseModelRows)-1 {
			m.phaseModelActiveRow++
		}
	case tea.KeyLeft:
		if m.phaseModelActiveCol == phaseModelClaudeColumn && m.phaseModelHasOpenCode {
			m.phaseModelActiveCol--
		}
	case tea.KeyRight:
		if m.phaseModelActiveCol == phaseModelOpenCodeColumn && m.phaseModelHasClaude {
			m.phaseModelActiveCol++
		}
	case tea.KeyEnter:
		m, changed = openOrCycleActivePhaseModel(m)
	case tea.KeySpace:
		m = cycleActivePhaseModel(m)
		changed = true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "a":
			m = applyAllForActiveColumn(m)
			changed = true
		case " ":
			m = cycleActivePhaseModel(m)
			changed = true
		}
	case tea.KeyEsc:
		m.Step = StepSkills
	case tea.KeyTab:
		m.Step = StepReview
	}
	if changed {
		persistPhaseModelRows(&m)
	}
	return m, nil
}

func updatePhaseModelPicker(m Model, msg tea.KeyMsg) Model {
	switch m.phaseModelMode {
	case phaseModelModeOpenCodeProvider:
		return updateOpenCodeProviderPicker(m, msg)
	case phaseModelModeOpenCodeModel:
		return updateOpenCodeModelPicker(m, msg)
	case phaseModelModeOpenCodeEffort:
		return updateOpenCodeEffortPicker(m, msg)
	case phaseModelModeClaudeModel:
		return updateClaudeModelPicker(m, msg)
	case phaseModelModeClaudeEffort:
		return updateClaudeEffortPicker(m, msg)
	default:
		m.phaseModelMode = phaseModelModeList
		return m
	}
}

func openOrCycleActivePhaseModel(m Model) (Model, bool) {
	if m.phaseModelColumnEnabled(m.phaseModelActiveCol) && m.phaseModelActiveCol == phaseModelOpenCodeColumn && len(m.phaseModelOpenCodeProviders) > 0 {
		m.phaseModelMode = phaseModelModeOpenCodeProvider
		m.phaseModelProviderCursor = clampIndex(m.phaseModelProviderCursor, len(m.phaseModelOpenCodeProviders))
		m.phaseModelModelCursor = 0
		m.phaseModelEffortCursor = 0
		m.phaseModelModelSearch = ""
		m.phaseModelPendingOpenCode = config.OpenCodeModelAssignment{}
		return m, false
	}
	if m.phaseModelColumnEnabled(m.phaseModelActiveCol) && m.phaseModelActiveCol == phaseModelClaudeColumn && len(m.phaseModelClaude) > 0 {
		m.phaseModelMode = phaseModelModeClaudeModel
		m.phaseModelModelCursor = catalogIndex(m.phaseModelRows[m.phaseModelActiveRow].Claude, m.phaseModelClaude)
		m.phaseModelEffortCursor = catalogIndex(m.phaseModelRows[m.phaseModelActiveRow].ClaudeEffort, claudeEffortOptions())
		m.phaseModelPendingClaude = config.ClaudeModelAssignment{}
		return m, false
	}
	m = cycleActivePhaseModel(m)
	return m, true
}

func updateOpenCodeProviderPicker(m Model, msg tea.KeyMsg) Model {
	switch msg.Type {
	case tea.KeyUp:
		if m.phaseModelProviderCursor > 0 {
			m.phaseModelProviderCursor--
		}
	case tea.KeyDown:
		if m.phaseModelProviderCursor < len(m.phaseModelOpenCodeProviders)-1 {
			m.phaseModelProviderCursor++
		}
	case tea.KeyEnter:
		m.phaseModelProviderCursor = clampIndex(m.phaseModelProviderCursor, len(m.phaseModelOpenCodeProviders))
		m.phaseModelModelCursor = 0
		m.phaseModelModelSearch = ""
		m.phaseModelMode = phaseModelModeOpenCodeModel
	case tea.KeyEsc:
		m.phaseModelMode = phaseModelModeList
	}
	return m
}

func updateOpenCodeModelPicker(m Model, msg tea.KeyMsg) Model {
	models := currentOpenCodeModelOptions(m)
	switch msg.Type {
	case tea.KeyUp:
		if m.phaseModelModelCursor > 0 {
			m.phaseModelModelCursor--
		}
	case tea.KeyDown:
		if m.phaseModelModelCursor < len(models)-1 {
			m.phaseModelModelCursor++
		}
	case tea.KeyBackspace:
		if len(m.phaseModelModelSearch) > 0 {
			m.phaseModelModelSearch = m.phaseModelModelSearch[:len(m.phaseModelModelSearch)-1]
			m.phaseModelModelCursor = 0
		}
	case tea.KeyCtrlU:
		m.phaseModelModelSearch = ""
		m.phaseModelModelCursor = 0
	case tea.KeyRunes:
		m.phaseModelModelSearch += string(msg.Runes)
		m.phaseModelModelCursor = 0
	case tea.KeyEnter:
		if len(models) == 0 {
			return m
		}
		selected := models[clampIndex(m.phaseModelModelCursor, len(models))]
		m.phaseModelPendingOpenCode = config.OpenCodeModelAssignment{ProviderID: selected.ProviderID, ModelID: selected.Model.ID}
		efforts := phaseModelEffortOptions(selected.ProviderID, selected.Model)
		if len(efforts) > 1 {
			m.phaseModelEffortCursor = 0
			m.phaseModelMode = phaseModelModeOpenCodeEffort
			return m
		}
		m = commitPendingOpenCodePhaseModel(m)
	case tea.KeyEsc:
		m.phaseModelMode = phaseModelModeOpenCodeProvider
	}
	return m
}

func updateOpenCodeEffortPicker(m Model, msg tea.KeyMsg) Model {
	efforts := currentOpenCodeEffortOptions(m)
	switch msg.Type {
	case tea.KeyUp:
		if m.phaseModelEffortCursor > 0 {
			m.phaseModelEffortCursor--
		}
	case tea.KeyDown:
		if m.phaseModelEffortCursor < len(efforts)-1 {
			m.phaseModelEffortCursor++
		}
	case tea.KeyEnter:
		if len(efforts) > 0 {
			m.phaseModelPendingOpenCode.Effort = efforts[clampIndex(m.phaseModelEffortCursor, len(efforts))]
		}
		m = commitPendingOpenCodePhaseModel(m)
	case tea.KeyEsc:
		m.phaseModelMode = phaseModelModeOpenCodeModel
	}
	return m
}

func updateClaudeModelPicker(m Model, msg tea.KeyMsg) Model {
	switch msg.Type {
	case tea.KeyUp:
		if m.phaseModelModelCursor > 0 {
			m.phaseModelModelCursor--
		}
	case tea.KeyDown:
		if m.phaseModelModelCursor < len(m.phaseModelClaude)-1 {
			m.phaseModelModelCursor++
		}
	case tea.KeyEnter:
		if len(m.phaseModelClaude) > 0 {
			m.phaseModelPendingClaude = config.ClaudeModelAssignment{
				Model:  m.phaseModelClaude[clampIndex(m.phaseModelModelCursor, len(m.phaseModelClaude))],
				Effort: m.phaseModelRows[m.phaseModelActiveRow].ClaudeEffort,
			}
			m.phaseModelEffortCursor = catalogIndex(m.phaseModelPendingClaude.Effort, claudeEffortOptions())
			m.phaseModelMode = phaseModelModeClaudeEffort
			return m
		}
		m.phaseModelMode = phaseModelModeList
	case tea.KeyEsc:
		m.phaseModelMode = phaseModelModeList
	}
	return m
}

func updateClaudeEffortPicker(m Model, msg tea.KeyMsg) Model {
	efforts := claudeEffortOptions()
	switch msg.Type {
	case tea.KeyUp:
		if m.phaseModelEffortCursor > 0 {
			m.phaseModelEffortCursor--
		}
	case tea.KeyDown:
		if m.phaseModelEffortCursor < len(efforts)-1 {
			m.phaseModelEffortCursor++
		}
	case tea.KeyEnter:
		if len(efforts) > 0 {
			m.phaseModelPendingClaude.Effort = efforts[clampIndex(m.phaseModelEffortCursor, len(efforts))]
		}
		m = commitPendingClaudePhaseModel(m)
	case tea.KeyEsc:
		m.phaseModelMode = phaseModelModeClaudeModel
	}
	return m
}

func commitPendingClaudePhaseModel(m Model) Model {
	if strings.TrimSpace(m.phaseModelPendingClaude.Model) != "" {
		row := &m.phaseModelRows[m.phaseModelActiveRow]
		row.Claude = strings.TrimSpace(m.phaseModelPendingClaude.Model)
		row.ClaudeEffort = strings.TrimSpace(m.phaseModelPendingClaude.Effort)
		persistPhaseModelRows(&m)
	}
	m.phaseModelPendingClaude = config.ClaudeModelAssignment{}
	m.phaseModelMode = phaseModelModeList
	return m
}

func claudeEffortOptions() []string {
	return []string{"", "low", "medium", "high", "xhigh", "max"}
}

func currentOpenCodeModelOptions(m Model) []openCodeModelOption {
	if len(m.phaseModelOpenCodeProviders) == 0 {
		return nil
	}
	provider := m.phaseModelOpenCodeProviders[clampIndex(m.phaseModelProviderCursor, len(m.phaseModelOpenCodeProviders))]
	return filterOpenCodeModelOptions(provider.Models, m.phaseModelModelSearch)
}

func currentOpenCodeEffortOptions(m Model) []string {
	if m.phaseModelPendingOpenCode.ProviderID == "" || m.phaseModelPendingOpenCode.ModelID == "" {
		return nil
	}
	models := currentOpenCodeModelOptions(m)
	for _, model := range models {
		if model.ProviderID == m.phaseModelPendingOpenCode.ProviderID && model.Model.ID == m.phaseModelPendingOpenCode.ModelID {
			return phaseModelEffortOptions(model.ProviderID, model.Model)
		}
	}
	return nil
}

func commitPendingOpenCodePhaseModel(m Model) Model {
	if m.phaseModelPendingOpenCode.ProviderID != "" && m.phaseModelPendingOpenCode.ModelID != "" {
		m.phaseModelRows[m.phaseModelActiveRow].OpenCodeAssignment = m.phaseModelPendingOpenCode
		persistPhaseModelRows(&m)
	}
	m.phaseModelPendingOpenCode = config.OpenCodeModelAssignment{}
	m.phaseModelMode = phaseModelModeList
	return m
}

func clampIndex(index, length int) int {
	if length <= 0 || index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func catalogIndex(current string, catalog []string) int {
	for i, item := range catalog {
		if item == current {
			return i
		}
	}
	return 0
}

func cycleActivePhaseModel(m Model) Model {
	if m.phaseModelActiveCol == 0 || len(m.phaseModelRows) == 0 || !m.phaseModelColumnEnabled(m.phaseModelActiveCol) {
		return m
	}
	row := &m.phaseModelRows[m.phaseModelActiveRow]
	if m.phaseModelActiveCol == 1 {
		if len(m.phaseModelOpenCodeAssignments) > 0 {
			row.OpenCodeAssignment = nextOpenCodeAssignment(row.OpenCodeAssignment, m.phaseModelOpenCodeAssignments)
		} else {
			row.OpenCode = nextCatalogValue(row.OpenCode, m.phaseModelOpenCode)
		}
	} else {
		row.Claude = nextCatalogValue(row.Claude, m.phaseModelClaude)
		row.ClaudeEffort = strings.TrimSpace(row.ClaudeEffort)
	}
	return m
}

func applyAllForActiveColumn(m Model) Model {
	if m.phaseModelActiveCol == 0 || len(m.phaseModelRows) == 0 || !m.phaseModelColumnEnabled(m.phaseModelActiveCol) {
		return m
	}
	active := m.phaseModelRows[m.phaseModelActiveRow]
	for i := range m.phaseModelRows {
		if m.phaseModelActiveCol == 1 {
			m.phaseModelRows[i].OpenCode = active.OpenCode
			m.phaseModelRows[i].OpenCodeAssignment = active.OpenCodeAssignment
		} else {
			m.phaseModelRows[i].Claude = active.Claude
			m.phaseModelRows[i].ClaudeEffort = active.ClaudeEffort
		}
	}
	return m
}

func nextCatalogValue(current string, catalog []string) string {
	if len(catalog) == 0 {
		return current
	}
	for i, item := range catalog {
		if item == current {
			return catalog[(i+1)%len(catalog)]
		}
	}
	return catalog[0]
}

func nextOpenCodeAssignment(current config.OpenCodeModelAssignment, catalog []config.OpenCodeModelAssignment) config.OpenCodeModelAssignment {
	if len(catalog) == 0 {
		return current
	}
	for i, item := range catalog {
		if item == current {
			return catalog[(i+1)%len(catalog)]
		}
	}
	return catalog[0]
}

func persistPhaseModelRows(m *Model) {
	if m == nil || m.cfg == nil {
		return
	}
	if m.cfg.SDD.PhaseModels == nil {
		m.cfg.SDD.PhaseModels = map[string]config.PhaseModelSelection{}
	}
	if m.cfg.SDD.OpenCodePhaseModels == nil {
		m.cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{}
	}
	if m.cfg.SDD.ClaudePhaseModels == nil {
		m.cfg.SDD.ClaudePhaseModels = map[string]config.ClaudeModelAssignment{}
	}
	for _, row := range m.phaseModelRows {
		m.cfg.SDD.PhaseModels[row.Phase] = config.PhaseModelSelection{OpenCode: row.OpenCode, Claude: row.Claude}
		m.cfg.SDD.ClaudePhaseModels[row.Phase] = config.ClaudeModelAssignment{Model: row.Claude, Effort: row.ClaudeEffort}
		if row.OpenCodeAssignment.ProviderID != "" && row.OpenCodeAssignment.ModelID != "" {
			m.cfg.SDD.OpenCodePhaseModels[row.Phase] = row.OpenCodeAssignment
		} else {
			delete(m.cfg.SDD.OpenCodePhaseModels, row.Phase)
		}
	}
}

func phaseModelOpenCodeDisplay(row phaseModelRow) string {
	if row.OpenCodeAssignment.ProviderID != "" && row.OpenCodeAssignment.ModelID != "" {
		display := row.OpenCodeAssignment.ProviderID + "/" + row.OpenCodeAssignment.ModelID
		if strings.TrimSpace(row.OpenCodeAssignment.Effort) != "" {
			display += " (effort=" + strings.TrimSpace(row.OpenCodeAssignment.Effort) + ")"
		}
		return display
	}
	return row.OpenCode
}

func phaseModelClaudeDisplay(row phaseModelRow) string {
	display := strings.TrimSpace(row.Claude)
	if strings.TrimSpace(row.ClaudeEffort) != "" {
		display += " (effort=" + strings.TrimSpace(row.ClaudeEffort) + ")"
	}
	return display
}

func viewPhaseModels(m Model) string {
	if m.phaseModelMode != phaseModelModeList {
		return viewPhaseModelPicker(m)
	}

	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder
	sb.WriteString(terminalui.HeaderRow("Setup › Phase Models", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	if !m.phaseModelHasOpenCode && !m.phaseModelHasClaude {
		sb.WriteString(terminalui.BorderedPanel("No Claude Code or OpenCode runtime target detected. Phase model editing is unavailable.", w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "Tab", Desc: "review"},
			{Key: "Esc", Desc: "back"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
		return sb.String()
	}

	var listSB strings.Builder
	listSB.WriteString(terminalui.DimTextStyle.Render("Edit models per phase for detected runtime targets.") + "\n\n")
	for _, diagnostic := range m.phaseModelOpenCodeDiagnostics {
		listSB.WriteString(warningStyle.Render(diagnostic) + "\n")
	}
	if len(m.phaseModelOpenCodeDiagnostics) > 0 {
		listSB.WriteString("\n")
	}

	header := "phase"
	if m.phaseModelHasOpenCode {
		header += "                 OpenCode"
	}
	if m.phaseModelHasClaude {
		header += "             Claude"
	}
	listSB.WriteString(terminalui.ColumnHeaderStyle.Render(header) + "\n")

	for i, row := range m.phaseModelRows {
		parts := []string{fmt.Sprintf("%-20s", row.Phase)}
		if m.phaseModelHasOpenCode {
			parts = append(parts, fmt.Sprintf("%-20s", phaseModelOpenCodeDisplay(row)))
		}
		if m.phaseModelHasClaude {
			parts = append(parts, fmt.Sprintf("%-10s", phaseModelClaudeDisplay(row)))
		}
		line := strings.Join(parts, " ")
		if i == m.phaseModelActiveRow {
			listSB.WriteString(terminalui.SelectedRow(line, w) + "\n")
		} else {
			listSB.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "row"},
		{Key: "←/→", Desc: "column"},
		{Key: "Enter", Desc: "picker"},
		{Key: "Space", Desc: "cycle"},
		{Key: "a", Desc: "apply-all"},
		{Key: "Tab", Desc: "review"},
		{Key: "Esc", Desc: "back"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

func viewPhaseModelPicker(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	switch m.phaseModelMode {
	case phaseModelModeOpenCodeProvider:
		sb.WriteString(terminalui.HeaderRow("Setup › Phase Models › Provider", terminalui.ModeBadge("normal"), m.width) + "\n\n")
		var listSB strings.Builder
		listSB.WriteString(terminalui.SectionHeader("PROVIDER", w))
		for i, provider := range m.phaseModelOpenCodeProviders {
			if i == m.phaseModelProviderCursor {
				listSB.WriteString(terminalui.SelectedRow(provider.DisplayName(), w) + "\n")
			} else {
				listSB.WriteString("  " + provider.DisplayName() + "\n")
			}
		}
		sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "select provider"},
			{Key: "Esc", Desc: "back"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))

	case phaseModelModeOpenCodeModel:
		sb.WriteString(terminalui.HeaderRow("Setup › Phase Models › Model", terminalui.ModeBadge("normal"), m.width) + "\n\n")
		var listSB strings.Builder
		listSB.WriteString(terminalui.SectionHeader("MODEL", w))
		listSB.WriteString("Search: " + m.phaseModelModelSearch + "\n\n")
		models := currentOpenCodeModelOptions(m)
		if len(models) == 0 {
			listSB.WriteString(terminalui.DimTextStyle.Render("No models match the current search.") + "\n")
		}
		for i, model := range models {
			label := model.Model.ID
			if strings.TrimSpace(model.Model.Name) != "" {
				label += " — " + strings.TrimSpace(model.Model.Name)
			}
			if i == m.phaseModelModelCursor {
				listSB.WriteString(terminalui.SelectedRow(label, w) + "\n")
			} else {
				listSB.WriteString("  " + label + "\n")
			}
		}
		sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "type", Desc: "search"},
			{Key: "Backspace", Desc: "delete"},
			{Key: "Ctrl+U", Desc: "clear"},
			{Key: "Enter", Desc: "select model"},
			{Key: "Esc", Desc: "providers"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))

	case phaseModelModeOpenCodeEffort:
		sb.WriteString(terminalui.HeaderRow("Setup › Phase Models › Effort", terminalui.ModeBadge("normal"), m.width) + "\n\n")
		var listSB strings.Builder
		listSB.WriteString(terminalui.SectionHeader("EFFORT", w))
		efforts := currentOpenCodeEffortOptions(m)
		for i, effort := range efforts {
			label := effort
			if label == "" {
				label = "default"
			}
			if i == m.phaseModelEffortCursor {
				listSB.WriteString(terminalui.SelectedRow(label, w) + "\n")
			} else {
				listSB.WriteString("  " + label + "\n")
			}
		}
		sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "confirm effort"},
			{Key: "Esc", Desc: "models"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))

	case phaseModelModeClaudeModel:
		sb.WriteString(terminalui.HeaderRow("Setup › Phase Models › Claude", terminalui.ModeBadge("normal"), m.width) + "\n\n")
		var listSB strings.Builder
		listSB.WriteString(terminalui.SectionHeader("CLAUDE MODEL", w))
		for i, model := range m.phaseModelClaude {
			if i == m.phaseModelModelCursor {
				listSB.WriteString(terminalui.SelectedRow(model, w) + "\n")
			} else {
				listSB.WriteString("  " + model + "\n")
			}
		}
		sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "select model"},
			{Key: "Esc", Desc: "back"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))

	case phaseModelModeClaudeEffort:
		sb.WriteString(terminalui.HeaderRow("Setup › Phase Models › Claude Effort", terminalui.ModeBadge("normal"), m.width) + "\n\n")
		var listSB strings.Builder
		listSB.WriteString(terminalui.SectionHeader("EFFORT", w))
		efforts := claudeEffortOptions()
		for i, effort := range efforts {
			label := effort
			if label == "" {
				label = "default"
			}
			if i == m.phaseModelEffortCursor {
				listSB.WriteString(terminalui.SelectedRow(label, w) + "\n")
			} else {
				listSB.WriteString("  " + label + "\n")
			}
		}
		sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")
		hints := []terminalui.KeyHint{
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "confirm effort"},
			{Key: "Esc", Desc: "models"},
		}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))

	default:
		return viewPhaseModels(Model{Step: m.Step, cfg: m.cfg, phaseModelRows: m.phaseModelRows, phaseModelHasOpenCode: m.phaseModelHasOpenCode, phaseModelHasClaude: m.phaseModelHasClaude})
	}
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 5: Review
// ──────────────────────────────────────────────────────────────────────────────

// agentProgressMsg reports a single status line from the agent config sequence.
type agentProgressMsg struct {
	line   string
	done   bool
	failed bool
}

func updateReview(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.reviewChoice > 0 {
			m.reviewChoice--
		}
	case tea.KeyDown:
		if m.reviewChoice < 2 {
			m.reviewChoice++
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if m.reviewChoice > 0 {
				m.reviewChoice--
			}
		case "j":
			if m.reviewChoice < 2 {
				m.reviewChoice++
			}
		}
	case tea.KeyEnter:
		switch m.reviewChoice {
		case 0: // Back
			if hasPhaseModelRuntimeTarget(m.Agents) {
				m.Step = StepPhaseModels
			} else {
				m.Step = StepSkills
			}
			return m, nil
		case 1: // Cancel
			m.Done = true
			return m, tea.Quit
		case 2: // Apply
			// Check if the statusline script already exists. If so, show the
			// overwrite/skip confirmation step before launching the apply pipeline.
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				// Cannot determine home dir; skip the pre-flight and proceed directly to apply.
				m.Step = StepApply
				m.agentProgress = nil
				m.agentDone = false
				m.Err = nil
				return m, runAgentConfigCmd(m)
			}
			scriptPath := filepath.Join(home, ".claude", "statusline-command.sh")
			if _, err := os.Stat(scriptPath); err == nil {
				// File exists: route through the pre-flight confirmation step.
				m.Step = StepStatuslineConfirm
				return m, nil
			} else if !os.IsNotExist(err) {
				m.Err = err
				return m, nil
			}
			// File absent (ENOENT): proceed directly to apply (fresh install, no prompt needed).
			m.Step = StepApply
			m.agentProgress = nil
			m.agentDone = false
			m.Err = nil
			return m, runAgentConfigCmd(m)
		}
	}
	return m, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Step 5b: StatuslineConfirm (pre-flight overwrite/skip prompt)
// ──────────────────────────────────────────────────────────────────────────────

// updateStatuslineConfirm handles the Y/N overwrite confirmation step for an
// existing ~/.claude/statusline-command.sh. 'y'/'Y' = overwrite; 'n'/'N' or
// Enter = skip (default). Any answer advances to StepApply.
func updateStatuslineConfirm(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		switch strings.ToLower(string(msg.Runes)) {
		case "y":
			m.statuslineOverwriteReady = true
			m.statuslineOverwrite = true
			m.Step = StepApply
			m.agentProgress = nil
			m.agentDone = false
			m.Err = nil
			return m, runAgentConfigCmd(m)
		case "n":
			m.statuslineOverwriteReady = true
			m.statuslineOverwrite = false
			m.Step = StepApply
			m.agentProgress = nil
			m.agentDone = false
			m.Err = nil
			return m, runAgentConfigCmd(m)
		}
	case tea.KeyEnter:
		// Default: skip (do not overwrite).
		m.statuslineOverwriteReady = true
		m.statuslineOverwrite = false
		m.Step = StepApply
		m.agentProgress = nil
		m.agentDone = false
		m.Err = nil
		return m, runAgentConfigCmd(m)
	}
	return m, nil
}

func viewStatuslineConfirm(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder
	sb.WriteString(terminalui.HeaderRow("Setup › Statusline Script", terminalui.ModeBadge("normal"), m.width) + "\n\n")
	content := warningStyle.Render("~/.claude/statusline-command.sh already exists.") + "\n\n" +
		"Overwrite with the Jarvis-managed statusline? [y/N]: "
	sb.WriteString(terminalui.BorderedPanel(content, w) + "\n")
	hints := []terminalui.KeyHint{
		{Key: "y", Desc: "overwrite"},
		{Key: "n/Enter", Desc: "keep existing"},
		{Key: "Ctrl+C", Desc: "exit"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

// Step 6: Apply
func updateApply(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if len(m.agentProgress) == 0 || (m.agentDone && m.Err != nil) {
			m.agentProgress = nil
			m.agentDone = false
			m.Err = nil
			return m, runAgentConfigCmd(m)
		}
		if m.agentDone && m.Err == nil {
			m.Step = StepDone
		}
	}
	return m, nil
}

// runAgentConfigCmd performs the full agent configuration sequence as a Cmd.
func runAgentConfigCmd(m Model) tea.Cmd {
	return func() tea.Msg {
		// We return a synthetic first progress message to start.
		return agentProgressMsg{line: "Starting agent configuration..."}
	}
}

// configureAgents performs the full agent setup and sends progress messages.
// This is called from the view/update flow after the first agentProgressMsg arrives.
func runAgentConfigSequence(m Model) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return agentProgressMsg{line: fmt.Sprintf("resolve home dir: %v", err), failed: true, done: true}
		}

		// Build the sub-FS rooted at embed/skills for InstallSkills.
		skillsSubFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
		if err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Skills FS error: %v", err), done: true, failed: true}
		}

		// Build the sub-FS rooted at embed/agents/claude for ClaudeAgent.InstallAgents.
		agentsSubFS, err := fs.Sub(jarvis.AgentsFS, "embed/agents/claude")
		if err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Agents FS error: %v", err), done: true, failed: true}
		}

		// Build the list of selected skill IDs.
		selectedIDs := buildSelectedIDs(m)

		// Build SkillInfo list from registry for template rendering.
		skillInfos := buildSkillInfoList(m)

		var resolvedPreset *persona.ResolvedPresetV2
		if len(m.Agents) > 0 {
			var err error
			resolvedPreset, err = ensurePresetV2ForApply(m)
			if err != nil {
				return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: resolve preset %q: %v", m.cfg.PersonaPreset, err), done: true, failed: true}
			}
		}

		previousSlug := m.previousPresetSlug
		if previousSlug == "" {
			if m.cfg != nil {
				previousSlug = m.cfg.PersonaPreset
			}
		}
		previousSource := m.previousPresetSource
		if previousSource == "" && m.cfg != nil && strings.TrimSpace(m.cfg.PersonaPresetSource) == string(persona.PresetSourceUser) {
			previousSource = persona.PresetSourceUser
		}

		// MCP entry for hive-daemon — point directly to the binary.
		// Credentials are read by hive-daemon from ~/.jarvis/sync.json (written above).
		entry := agent.MCPEntry{
			Name:       "hive",
			DaemonPath: agent.HiveDaemonBinaryPath(home),
		}

		// MCP entry for Context7 — auto-configured after Hive.
		context7Entry := agent.MCPEntry{Name: "context7"}

		// Determine statusline overwrite policy. The decision must be made here
		// (before the goroutine enters the pipeline) so the Bubbletea
		// single-thread contract is not violated. If the script is absent,
		// confirm is never called. If the pre-flight step captured an answer,
		// use it; otherwise default to skip (safe default without user consent).
		statuslineConfirm, statuslineErr := buildTUIStatuslineConfirm(home, m)
		if statuslineErr != nil {
			return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: check statusline script: %v. Ver docs/setup-recovery.md", statuslineErr), done: true, failed: true}
		}

		// Configure each detected agent and collect structured outcomes.
		results := configureWizardAgents(m.Agents, m.cfg, entry, context7Entry, resolvedPreset, wizardPresetApplyContext{
			Layer1:               config.Layer1Content(),
			Skills:               skillInfos,
			PreviousPresetSlug:   previousSlug,
			PreviousPresetSource: previousSource,
		}, skillsSubFS, selectedIDs, agentsSubFS, statuslineConfirm)
		var configuredAgents []string
		var automationWarnings []string
		for _, res := range results {
			if res.Err != nil {
				return agentProgressMsg{line: fmt.Sprintf("[%s] Configuration FAILED: %v", res.AgentName, res.Err), done: true, failed: true}
			}
			configuredAgents = append(configuredAgents, res.AgentName)
			automationWarnings = append(automationWarnings, res.Warnings...)
		}

		if m.Scope == config.ScopeLocalOnly {
			if err := config.DeleteSyncCredentials(); err != nil {
				return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: cleanup local credentials: %v. Ver docs/setup-recovery.md", err), done: true, failed: true}
			}
			m.cfg.Cloud = nil
			m.cfg.Email = ""
		} else if strings.TrimSpace(m.Email) != "" && strings.TrimSpace(m.Password) != "" {
			enable := true
			if err := writeSyncJSON(m.cfg.APIURL, m.Email, m.Password, &enable); err != nil {
				return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: write sync.json: %v. Ver docs/setup-recovery.md", err), done: true, failed: true}
			}
			if m.cfg.Cloud == nil {
				m.cfg.Cloud = &config.CloudConfig{}
			}
			m.cfg.Cloud.Email = strings.TrimSpace(m.Email)
			m.cfg.Cloud.SyncConfigured = true
			m.cfg.Email = m.cfg.Cloud.Email
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: home dir: %v. Ver docs/setup-recovery.md", err), done: true, failed: true}
		}
		jarvisDir := filepath.Join(homeDir, ".jarvis")
		if err := os.MkdirAll(jarvisDir, 0755); err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: create ~/.jarvis: %v. Ver docs/setup-recovery.md", err), done: true, failed: true}
		}
		dbPath := filepath.Join(jarvisDir, "memory.db")
		if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
			f, createErr := os.Create(dbPath)
			if createErr != nil {
				return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: create memory.db: %v. Ver docs/setup-recovery.md", createErr), done: true, failed: true}
			}
			_ = f.Close()
		}

		// Save canonical config as the final commit step.
		m.cfg.SchemaVersion = 2
		m.cfg.Scope = m.Scope
		m.cfg.ConfiguredAgents = configuredAgents
		m.cfg.SelectedSkills = selectedIDs
		m.cfg.Install.Mode = string(config.ConfigStatusReconfigure)
		m.cfg.Install.Completed = true
		if m.cfg.Install.Agents == nil {
			m.cfg.Install.Agents = map[string]config.AgentState{}
		}
		for _, res := range results {
			m.cfg.Install.Agents[res.AgentName] = res.State
		}
		m.cfg.Version = "1.0.0"
		if err := config.Save(m.cfg); err != nil {
			return agentProgressMsg{line: fmt.Sprintf("Configuration FAILED: save config: %v. Ver docs/setup-recovery.md", err), done: true, failed: true}
		}
		registryWarnings, registryErr := refreshProjectRegistryForApply(context.Background(), m.ProjectCWD)
		if registryErr != nil {
			return agentProgressMsg{line: fmt.Sprintf("Project skill registry refresh failed: %v", registryErr), done: true, failed: true}
		}

		summary := fmt.Sprintf("Configuration complete. Agents configured: %s", strings.Join(configuredAgents, ", "))
		if len(configuredAgents) == 0 {
			summary = "No agents detected. Install Claude Code or OpenCode and re-run jarvis."
		}
		allWarnings := append(automationWarnings, registryWarnings...)
		if len(allWarnings) > 0 {
			summary += "\n" + strings.Join(allWarnings, "\n")
		}
		return agentProgressMsg{line: summary, done: true}
	}
}

func ensurePresetV2ForApply(m Model) (*persona.ResolvedPresetV2, error) {
	if resolved, ok := m.wizardPresetV2(); ok {
		return resolved, nil
	}

	requested := ""
	if m.cfg != nil {
		requested = strings.TrimSpace(m.cfg.PersonaPreset)
	}
	if requested == "" {
		presets, err := persona.ListPresetsV2(m.PersonaFS)
		if err == nil && len(presets) > 0 {
			requested = presets[0].Name
		}
	}
	if requested == "" {
		return nil, fmt.Errorf("no preset selected")
	}

	resolved, err := resolveWizardPresetSelection(m.PersonaFS, requested, nil)
	if err != nil {
		return nil, err
	}

	return resolved, nil
}

// buildSelectedIDs returns a slice of skill IDs for all selected and core skills.
// Used to pass to InstallSkills(skillsFS, selected).
func buildSelectedIDs(m Model) []string {
	var ids []string
	for _, s := range m.SkillList {
		if m.Selected[s.ID] || s.IsCore {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// buildSkillInfoList returns a slice of SkillInfo structs for template rendering.
// Only includes selected and core skills from the SkillList.
func buildSkillInfoList(m Model) []config.SkillInfo {
	var infos []config.SkillInfo
	for _, s := range m.SkillList {
		if m.Selected[s.ID] || s.IsCore {
			infos = append(infos, config.SkillInfo{
				Name:        s.Name,
				Description: s.Description,
				Trigger:     s.Trigger,
			})
		}
	}
	return infos
}

func viewReview(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	sb.WriteString(terminalui.HeaderRow("Setup › Review", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	// Build summary content for the bordered panel.
	var summarySB strings.Builder
	summarySB.WriteString(terminalui.TitleStyle.Render("Resumen de configuración") + "\n\n")

	fmt.Fprintf(&summarySB, "Scope: %s", m.Scope)
	if m.Scope == config.ScopeLocalOnly {
		summarySB.WriteString("  " + errorStyle.Render(localOnlyReviewWarning))
	}
	summarySB.WriteString("\n")
	fmt.Fprintf(&summarySB, "Persona: %s\n", m.cfg.PersonaPreset)
	if m.Scope == config.ScopeLocalCloud {
		fmt.Fprintf(&summarySB, "Cloud email: %s\n", strings.TrimSpace(m.Email))
	} else {
		summarySB.WriteString("Cloud email: (omitido por alcance local-only)\n")
	}

	resolved := sddruntime.ResolvePhaseModels(m.cfg)
	summarySB.WriteString("SDD phase models:\n")
	for _, phase := range sddruntime.DefaultContract().Phases {
		sel := resolved[phase]
		opencodeDisplay := sel.OpenCode
		effortDisplay := ""
		if assignment := m.cfg.SDD.OpenCodePhaseModels[phase]; assignment.ProviderID != "" && assignment.ModelID != "" {
			opencodeDisplay = assignment.ProviderID + "/" + assignment.ModelID
			if strings.TrimSpace(assignment.Effort) != "" {
				effortDisplay = ", effort=" + strings.TrimSpace(assignment.Effort)
			}
		}
		claudeDisplay := sel.Claude
		claudeEffortDisplay := ""
		if assignment := m.cfg.SDD.ClaudePhaseModels[phase]; strings.TrimSpace(assignment.Model) != "" || strings.TrimSpace(assignment.Effort) != "" {
			if strings.TrimSpace(assignment.Model) != "" {
				claudeDisplay = strings.TrimSpace(assignment.Model)
			}
			if strings.TrimSpace(assignment.Effort) != "" {
				claudeEffortDisplay = ", effort=" + strings.TrimSpace(assignment.Effort)
			}
		}
		fmt.Fprintf(&summarySB, "- %s: opencode=%s%s, claude=%s%s\n", phase, opencodeDisplay, effortDisplay, claudeDisplay, claudeEffortDisplay)
	}
	sb.WriteString(terminalui.BorderedPanel(summarySB.String(), w) + "\n\n")

	choices := []string{"Back", "Cancel", "Apply"}
	for i, opt := range choices {
		line := "  " + opt
		if i == m.reviewChoice {
			line = terminalui.SelectedRow("> "+opt, w)
		}
		sb.WriteString(line + "\n")
	}

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "navegar"},
		{Key: "Enter", Desc: "confirmar"},
	}
	sb.WriteString("\n" + terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

func viewApply(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	sb.WriteString(terminalui.HeaderRow("Setup › Apply", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	var contentSB strings.Builder
	if len(m.agentProgress) == 0 {
		agentNames := make([]string, 0, len(m.Agents))
		for _, a := range m.Agents {
			agentNames = append(agentNames, a.Name())
		}
		if len(agentNames) == 0 {
			contentSB.WriteString("No agents detected on this system.\n")
			contentSB.WriteString(terminalui.DimTextStyle.Render("Install Claude Code or OpenCode, then re-run jarvis."))
		} else {
			contentSB.WriteString("Detected agents: " + strings.Join(agentNames, ", ") + "\n\n")
			contentSB.WriteString(terminalui.DimTextStyle.Render("Press Enter para ejecutar apply."))
		}
		sb.WriteString(terminalui.BorderedPanel(contentSB.String(), w) + "\n")
		hints := []terminalui.KeyHint{{Key: "Enter", Desc: "apply"}}
		sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
		return sb.String()
	}

	for _, line := range m.agentProgress {
		contentSB.WriteString(line + "\n")
	}
	if m.agentDone {
		if m.Err != nil {
			contentSB.WriteString("\n" + errorStyle.Render("Setup failed. Press Enter to retry."))
		} else {
			contentSB.WriteString("\n" + terminalui.TitleStyle.Render("All done!"))
			contentSB.WriteString("\n" + terminalui.DimTextStyle.Render("Press Enter to see the summary."))
		}
	}
	sb.WriteString(terminalui.BorderedPanel(contentSB.String(), w) + "\n")
	hints := []terminalui.KeyHint{{Key: "Enter", Desc: "continue"}}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

// ──────────────────────────────────────────────────────────────────────────────
// Step Done
// ──────────────────────────────────────────────────────────────────────────────

func updateDone(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyRunes:
		if string(msg.Runes) == "q" || msg.Type == tea.KeyEnter {
			m.Done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func viewDone(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	doneLabel := terminalui.StatusDot("healthy") + " Complete"
	sb.WriteString(terminalui.HeaderRow("Setup › Done", doneLabel, m.width) + "\n\n")

	var contentSB strings.Builder
	if m.Mode == string(config.ConfigStatusReconfigure) {
		contentSB.WriteString(terminalui.TitleStyle.Render("Jarvis-Dev Reconfiguration Complete!") + "\n\n")
	} else {
		contentSB.WriteString(terminalui.TitleStyle.Render("Jarvis-Dev Setup Complete!") + "\n\n")
	}
	contentSB.WriteString(terminalui.TitleStyle.Render("Your AI coding environment is configured.") + "\n\n")
	contentSB.WriteString(terminalui.ColumnHeaderStyle.Render("Next Steps:") + "\n")
	contentSB.WriteString("  1. Restart Claude Code or OpenCode to load the new MCP config.\n")
	contentSB.WriteString("  2. Use " + terminalui.HelpKeyStyle.Render("'jarvis persona set <preset>'") + " to change persona.\n")
	contentSB.WriteString("  3. Use mem_sync in your agent only when you want a manual cloud sync.")
	sb.WriteString(terminalui.BorderedPanel(contentSB.String(), w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "Enter", Desc: "exit"},
		{Key: "q", Desc: "quit"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

// skillsSelectedList returns IDs of non-core selected skills.
func skillsSelectedList(m Model) []string {
	var result []string
	for id, on := range m.Selected {
		if on {
			result = append(result, id)
		}
	}
	return result
}

// buildTUIStatuslineConfirm returns the statusline overwrite confirm closure for
// the TUI pipeline goroutine. If the script does not yet exist, confirm is never
// called (fresh install always proceeds). If the script exists, the pre-flight
// decision captured in Model.statuslineOverwrite is used; if no pre-flight step
// ran (statuslineOverwriteReady is false), the safe default is to skip.
// A non-ENOENT stat error (e.g. permission denied) is returned as an error so
// callers can handle it explicitly.
func buildTUIStatuslineConfirm(home string, m Model) (func() bool, error) {
	scriptPath := filepath.Join(home, ".claude", "statusline-command.sh")
	_, err := os.Stat(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return func() bool { return true }, nil
		}
		return nil, err
	}
	if m.statuslineOverwriteReady {
		overwrite := m.statuslineOverwrite
		return func() bool { return overwrite }, nil
	}
	// Default: skip (do not silently overwrite without explicit user consent).
	return func() bool { return false }, nil
}
