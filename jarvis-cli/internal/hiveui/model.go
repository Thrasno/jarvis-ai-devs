package hiveui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

type DashboardState int

const (
	DashboardHealthy DashboardState = iota
	DashboardDegraded
	DashboardLocalOnly
	DashboardDaemonUnavailable
)

type Screen int

const ScreenDashboard Screen = 0

type Snapshot struct {
	DashboardState DashboardState
	DaemonURL      string
	Projects       []hiveclient.Project
	Health         []hiveclient.Health
	Warnings       []hiveclient.Warning
	LoadError      error
}

type Model struct {
	snapshot Snapshot
	cursor   int
	message  string
}

func NewModelWithSnapshot(snapshot Snapshot) Model {
	if snapshot.DaemonURL == "" {
		snapshot.DaemonURL = "http://127.0.0.1:7438"
	}
	return Model{snapshot: snapshot}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.message = ""
	switch {
	case key.Type == tea.KeyCtrlC || runeKey(key, 'q'):
		return m, tea.Quit
	case key.Type == tea.KeyDown || runeKey(key, 'j'):
		m.cursor = (m.cursor + 1) % len(dashboardActions())
	case key.Type == tea.KeyUp || runeKey(key, 'k'):
		m.cursor = (m.cursor - 1 + len(dashboardActions())) % len(dashboardActions())
	case key.Type == tea.KeyEnter:
		action := dashboardActions()[m.cursor]
		if action.disabled {
			m.message = action.label + " is disabled in this read-only TUI slice. No local Hive state was changed."
		} else {
			m.message = "Navigation is not available in this read-only TUI slice. No local Hive state was changed."
		}
	}
	return m, nil
}

func (m Model) Screen() Screen { return ScreenDashboard }

func (m Model) View() string {
	if m.snapshot.DashboardState == DashboardDaemonUnavailable || m.snapshot.LoadError != nil {
		return fmt.Sprintf("dashboard · offline\nCannot reach hive-daemon\nNo response from %s\nThe local Hive daemon is not running, so the TUI has nothing to read.\nprojects — memories — unsynced n/a warnings —\nq quit", m.snapshot.DaemonURL)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", dashboardTitle(m.snapshot.DashboardState))
	if notice := dashboardNotice(m.snapshot); notice != "" {
		sb.WriteString(notice + "\n")
	}
	fmt.Fprintf(&sb, "daemon running · %s · %s\n", apiStatus(m.snapshot), syncStatus(m.snapshot))
	fmt.Fprintf(&sb, "projects %d memories %s unsynced %s warnings %d\n", len(m.snapshot.Projects), comma(totalMemories(m.snapshot.Projects)), unsyncedText(m.snapshot), len(m.snapshot.Warnings))
	for i, action := range dashboardActions() {
		mark := "  "
		if i == m.cursor {
			mark = "▌ "
		}
		state := ""
		if action.disabled {
			state = " (disabled)"
		}
		fmt.Fprintf(&sb, "%s%s%s — %s\n", mark, action.label, state, action.description)
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("j/k move  enter read-only notice  q quit")
	return sb.String()
}

type dashboardAction struct {
	label       string
	description string
	disabled    bool
}

func dashboardActions() []dashboardAction {
	return []dashboardAction{
		{"Project viewer", "read-only placeholder; navigation deferred", false},
		{"Project timeline", "read-only placeholder; navigation deferred", false},
		{"Merge projects", "destructive operation deferred", true},
		{"Delete projects", "destructive operation deferred", true},
		{"Delete memories", "destructive operation deferred", true},
		{"Hive API config", "read-only placeholder; navigation deferred", false},
		{"Hive API health", "read-only placeholder; navigation deferred", false},
		{"Memory warnings", "read-only placeholder; navigation deferred", false},
		{"Backup / Restore", "list-only placeholder; restore execution deferred", true},
	}
}

func dashboardTitle(state DashboardState) string {
	switch state {
	case DashboardDegraded:
		return "dashboard · degraded"
	case DashboardLocalOnly:
		return "dashboard · local-only"
	default:
		return "dashboard"
	}
}

func dashboardNotice(snapshot Snapshot) string {
	switch snapshot.DashboardState {
	case DashboardDegraded:
		return "Cloud sync is paused — Hive API auth failed."
	case DashboardLocalOnly:
		return "Running local-only. Memory stays on this machine; nothing is sent anywhere."
	default:
		return ""
	}
}

func apiStatus(snapshot Snapshot) string {
	switch snapshot.DashboardState {
	case DashboardDegraded:
		return "api auth failed"
	case DashboardLocalOnly:
		return "api not configured"
	default:
		return "api healthy"
	}
}

func syncStatus(snapshot Snapshot) string {
	switch snapshot.DashboardState {
	case DashboardDegraded:
		return "sync status unknown"
	case DashboardLocalOnly:
		return "sync disabled"
	default:
		return "sync ok"
	}
}

func unsyncedText(snapshot Snapshot) string {
	return "n/a"
}

func totalMemories(projects []hiveclient.Project) int {
	total := 0
	for _, project := range projects {
		total += project.ActiveMemoryCount
	}
	return total
}

func comma(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func runeKey(msg tea.KeyMsg, r rune) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}
