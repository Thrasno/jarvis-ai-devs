package tui

import (
	"fmt"
	"strings"
)

func viewCockpit(m Model) string {
	if m.cockpitMode != cockpitModeMenu {
		return viewCockpitPanel(m)
	}

	var sb strings.Builder

	sb.WriteString(strings.TrimRight(CockpitLogo(), "\n"))
	sb.WriteString("\n\n")
	sb.WriteString("Jarvis Cockpit\n")
	sb.WriteString("Choose what you want to do.\n\n")

	actions := CockpitActions()
	for i, action := range actions {
		cursor := " "
		if i == m.cockpitCursor {
			cursor = ">"
		}

		status := cockpitActionStatus(action)
		sb.WriteString(fmt.Sprintf("%s %s — %s%s\n", cursor, action.Label, action.Description, status))
	}

	if m.cockpitMessage != "" {
		sb.WriteString("\n")
		sb.WriteString(m.cockpitMessage)
		sb.WriteString("\n")
	}

	sb.WriteString("\n↑/↓ or j/k: navigate  Enter: select  q: exit")
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
		sb.WriteString(fmt.Sprintf("%s Password: %s\n", passwordCursor, strings.Repeat("*", len(m.Password))))
		if m.cockpitMessage != "" && m.cockpitMessage != "Hive Cloud Login" {
			sb.WriteString("\n")
			sb.WriteString(m.cockpitMessage)
		}
		sb.WriteString("\n\nEnter: continue/login")
	}
	return sb.String()
}
