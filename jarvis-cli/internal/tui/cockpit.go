package tui

import tea "github.com/charmbracelet/bubbletea"

func updateCockpit(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := cockpitKeyInput(msg)
	if m.cockpitMode != cockpitModeMenu {
		return updateCockpitHandler(m, key), nil
	}

	actions := CockpitActions()
	if len(actions) == 0 {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		m.cockpitCursor = previousCockpitIndex(m.cockpitCursor, len(actions))
	case tea.KeyDown:
		m.cockpitCursor = nextCockpitIndex(m.cockpitCursor, len(actions))
	case tea.KeyEnter:
		return runCockpitAction(m, actions[m.cockpitCursor])
	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			return m, nil
		}
		switch msg.Runes[0] {
		case 'k':
			m.cockpitCursor = previousCockpitIndex(m.cockpitCursor, len(actions))
		case 'j':
			m.cockpitCursor = nextCockpitIndex(m.cockpitCursor, len(actions))
		case 'q':
			m.Done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func runCockpitAction(m Model, action CockpitAction) (tea.Model, tea.Cmd) {
	switch action.ID {
	case CockpitActionInstall:
		m.Screen = ScreenWizard
		m.Step = StepScope
		m.cockpitMessage = ""
		return m, nil
	case CockpitActionExit:
		m.Done = true
		return m, tea.Quit
	default:
		m = startCockpitHandler(m, action)
		return m, nil
	}
}

func cockpitKeyInput(msg tea.KeyMsg) keyInput {
	input := keyInput{}
	switch msg.Type {
	case tea.KeyUp:
		input.up = true
	case tea.KeyDown:
		input.down = true
	case tea.KeyEnter:
		input.enter = true
	case tea.KeyBackspace, tea.KeyCtrlH:
		input.backspace = true
	case tea.KeyRunes:
		input.runes = msg.Runes
		if len(msg.Runes) > 0 {
			switch msg.Runes[0] {
			case 'k':
				input.up = true
			case 'j':
				input.down = true
			}
		}
	}
	return input
}

func previousCockpitIndex(current, total int) int {
	if current <= 0 {
		return total - 1
	}
	return current - 1
}

func nextCockpitIndex(current, total int) int {
	if current >= total-1 {
		return 0
	}
	return current + 1
}
