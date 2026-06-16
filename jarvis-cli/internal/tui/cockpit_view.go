package tui

import (
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

func viewCockpit(m Model) string {
	if m.cockpitMode != cockpitModeMenu {
		return viewCockpitPanel(m)
	}

	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	// Gate full logo render on width >= 80 (or 0 = not yet measured → show full).
	// Below 80, render a compact one-line title via TitleStyle instead.
	if m.width == 0 || m.width >= 80 {
		sb.WriteString(strings.TrimRight(CockpitLogo(), "\n"))
		sb.WriteString("\n\n")
		sb.WriteString(terminalui.TitleStyle.Render("Jarvis Cockpit"))
	} else {
		sb.WriteString(terminalui.TitleStyle.Render("Jarvis Cockpit"))
	}
	sb.WriteString("\n")
	sb.WriteString(terminalui.DimTextStyle.Render("Choose what you want to do."))
	sb.WriteString("\n\n")

	actions := CockpitActions()
	var listSB strings.Builder
	for i, action := range actions {
		status := cockpitActionStatus(action)
		line := fmt.Sprintf("%s — %s%s", action.Label, action.Description, status)
		if i == m.cockpitCursor {
			listSB.WriteString(terminalui.SelectedRow(line, w) + "\n")
		} else {
			listSB.WriteString("  " + line + "\n")
		}
	}
	sb.WriteString(terminalui.BorderedPanel(listSB.String(), w) + "\n")

	if m.cockpitMessage != "" {
		sb.WriteString("\n")
		sb.WriteString(m.cockpitMessage)
		sb.WriteString("\n")
	}

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "navigate"},
		{Key: "Enter", Desc: "select"},
		{Key: "q", Desc: "exit"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}

func cockpitActionStatus(action CockpitAction) string {
	return ""
}

func viewCockpitPanel(m Model) string {
	var sb strings.Builder
	sb.WriteString("Jarvis Cockpit\n\n")
	switch m.cockpitMode {
	case cockpitModeProvider:
		sb.WriteString("Provider\n")
		for i, provider := range cockpitProviders {
			cursor := " "
			if i == m.cockpitProviderCursor {
				cursor = ">"
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", cursor, provider))
		}
		sb.WriteString("\n↑/↓ or j/k: select provider  Enter: continue")
	case cockpitModeInput:
		sb.WriteString(m.cockpitMessage)
		sb.WriteString("\n")
		sb.WriteString(m.cockpitInput)
		sb.WriteString("\n\nEnter: continue")
	case cockpitModeConfirm:
		sb.WriteString(m.cockpitMessage)
		if m.cockpitAction != CockpitActionBackup {
			sb.WriteString("\n")
			sb.WriteString(m.cockpitInput)
		}
		sb.WriteString("\n\nEnter: confirm")
	case cockpitModeResult:
		sb.WriteString(m.cockpitMessage)
		sb.WriteString("\n\nEnter: return")
	case cockpitModePersona:
		sb.WriteString("Persona\n")
		if m.cockpitMessage != "" {
			sb.WriteString(m.cockpitMessage)
			sb.WriteString("\n")
		}
		for i, preset := range m.Presets {
			cursor := " "
			if i == m.presetCur {
				cursor = ">"
			}
			display := preset.DisplayName
			if display == "" {
				display = preset.Name
			}
			sb.WriteString(fmt.Sprintf("%s %s — %s\n", cursor, display, preset.Description))
		}
		sb.WriteString("\n↑/↓ or j/k: choose persona  Enter: apply")
	case cockpitModeLogin:
		sb.WriteString("Hive Cloud Login\n")
		emailCursor := " "
		passwordCursor := " "
		if m.activeField == 0 {
			emailCursor = ">"
		} else {
			passwordCursor = ">"
		}
		sb.WriteString(fmt.Sprintf("%s Email: %s\n", emailCursor, m.Email))
		sb.WriteString(fmt.Sprintf("%s Password: %s\n", passwordCursor, strings.Repeat("•", len(m.Password))))
		if m.cockpitMessage != "" && m.cockpitMessage != "Hive Cloud Login" {
			sb.WriteString("\n")
			sb.WriteString(m.cockpitMessage)
		}
		sb.WriteString("\n\nEnter: continue/login")
	}
	return sb.String()
}
