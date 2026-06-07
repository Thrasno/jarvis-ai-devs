package hiveui

import (
	"fmt"
	"strings"
	"time"

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

const (
	ScreenDashboard Screen = iota
	ScreenProjects
	ScreenProjectMemories
	ScreenMemoryDetail
	ScreenTimeline
)

type Snapshot struct {
	DashboardState DashboardState
	DaemonURL      string
	Projects       []hiveclient.Project
	Memories       []hiveclient.Memory
	Health         []hiveclient.Health
	Warnings       []hiveclient.Warning
	LoadError      error
}

type Model struct {
	snapshot     Snapshot
	screen       Screen
	cursor       int
	projectIndex int
	memoryIndex  int
	detailReturn Screen
	message      string
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
	case key.Type == tea.KeyEsc || key.Type == tea.KeyBackspace:
		m = m.back()
	case runeKey(key, 't'):
		m.screen = ScreenTimeline
	case key.Type == tea.KeyDown || runeKey(key, 'j'):
		m = m.move(1)
	case key.Type == tea.KeyUp || runeKey(key, 'k'):
		m = m.move(-1)
	case key.Type == tea.KeyEnter:
		m = m.open()
	}
	return m, nil
}

func (m Model) Screen() Screen { return m.screen }

func (m Model) View() string {
	if m.snapshot.DashboardState == DashboardDaemonUnavailable || m.snapshot.LoadError != nil {
		return fmt.Sprintf("dashboard · offline\nCannot reach hive-daemon\nNo response from %s\nThe local Hive daemon is not running, so the TUI has nothing to read.\nprojects — memories — unsynced n/a warnings —\nq quit", m.snapshot.DaemonURL)
	}
	switch m.screen {
	case ScreenProjects:
		return m.projectsView()
	case ScreenProjectMemories:
		return m.projectMemoriesView()
	case ScreenMemoryDetail:
		return m.memoryDetailView()
	case ScreenTimeline:
		return m.timelineView()
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
	sb.WriteString("j/k move  enter open  q quit")
	return sb.String()
}

func (m Model) move(delta int) Model {
	switch m.screen {
	case ScreenProjects:
		m.projectIndex = wrapIndex(m.projectIndex+delta, len(m.snapshot.Projects))
	case ScreenProjectMemories, ScreenTimeline:
		m.memoryIndex = wrapIndex(m.memoryIndex+delta, len(m.projectMemories()))
	default:
		m.cursor = wrapIndex(m.cursor+delta, len(dashboardActions()))
	}
	return m
}

func (m Model) open() Model {
	if m.screen == ScreenProjects {
		if len(m.snapshot.Projects) == 0 {
			m.message = "No item is available to open."
			return m
		}
		m.screen = ScreenProjectMemories
		m.memoryIndex = 0
		return m
	}
	if m.screen == ScreenProjectMemories || m.screen == ScreenTimeline {
		if len(m.projectMemories()) == 0 {
			m.message = "No item is available to open."
			return m
		}
		m.detailReturn = m.screen
		m.screen = ScreenMemoryDetail
		return m
	}
	if m.screen != ScreenDashboard {
		return m
	}
	action := dashboardActions()[m.cursor]
	if action.disabled {
		m.message = action.label + " is disabled in this read-only TUI slice. No local Hive state was changed."
		return m
	}
	switch action.label {
	case "Project viewer":
		m.screen = ScreenProjects
	case "Project timeline":
		m.screen = ScreenTimeline
	default:
		m.message = action.label + " is not available in this navigation sub-slice. No local Hive state was changed."
	}
	return m
}

func (m Model) back() Model {
	switch m.screen {
	case ScreenMemoryDetail:
		if m.detailReturn == ScreenTimeline {
			m.screen = ScreenTimeline
		} else {
			m.screen = ScreenProjectMemories
		}
	case ScreenProjectMemories:
		m.screen = ScreenProjects
	case ScreenTimeline:
		m.screen = ScreenDashboard
	case ScreenProjects:
		m.screen = ScreenDashboard
	}
	return m
}

func (m Model) projectsView() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "dashboard / projects\t%d projects\n", len(m.snapshot.Projects))
	sb.WriteString("PROJECT MEMORIES UNSYNCED LAST\n")
	for i, project := range m.snapshot.Projects {
		mark := "  "
		if i == m.projectIndex {
			mark = "▌ "
		}
		fmt.Fprintf(&sb, "%s%s %d n/a %s\n", mark, project.Name, project.ActiveMemoryCount, relativeTime(project.LastActivityAt))
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("j/k move  enter open  t timeline  esc back  q quit")
	return sb.String()
}

func (m Model) projectMemoriesView() string {
	memories := m.projectMemories()
	project := m.selectedProject().Name
	var sb strings.Builder
	fmt.Fprintf(&sb, "dashboard / projects / %s\t%d memories\n", project, len(memories))
	if len(memories) == 0 {
		sb.WriteString("No local Hive memories found for this project\n")
	}
	for i, memory := range memories {
		mark := "  "
		if i == m.memoryIndex {
			mark = "▌ "
		}
		fmt.Fprintf(&sb, "%s%s  %s  %s\n", mark, emptyDash(memory.Category), memory.Title, relativeTime(memory.CreatedAt))
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("j/k move  enter open  t timeline  esc back  q quit")
	return sb.String()
}

func (m Model) memoryDetailView() string {
	memory := m.selectedMemory()
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s / %s\n", memory.Project, memoryKey(memory))
	fmt.Fprintf(&sb, "id %s  created %s\n", memoryKey(memory), formatDateTime(memory.CreatedAt))
	fmt.Fprintf(&sb, "project %s  sync %s\n", memory.Project, syncText(memory))
	fmt.Fprintf(&sb, "type %s  source %s\n", emptyDash(memory.Category), emptyDash(memory.CreatedBy))
	sb.WriteString("Content preview is not available from the read-only daemon snapshot.\n")
	sb.WriteString("esc back  q quit")
	return sb.String()
}

func (m Model) timelineView() string {
	memories := m.projectMemories()
	project := m.selectedProject().Name
	var sb strings.Builder
	fmt.Fprintf(&sb, "timeline / %s\t%d entries\n", project, len(memories))
	lastDay := ""
	for i, memory := range memories {
		day := timelineDateText(memory.CreatedAt)
		if day != lastDay {
			fmt.Fprintf(&sb, "┄ %s\n", day)
			lastDay = day
		}
		mark := "  "
		if i == m.memoryIndex {
			mark = "▌ "
		}
		fmt.Fprintf(&sb, "%s%s  %s  %s\n", mark, timelineTimeText(memory.CreatedAt), emptyDash(memory.Category), memory.Title)
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("j/k move  enter open  esc back  q quit")
	return sb.String()
}

type dashboardAction struct {
	label       string
	description string
	disabled    bool
}

func dashboardActions() []dashboardAction {
	return []dashboardAction{
		{"Project viewer", "browse projects and memories", false},
		{"Project timeline", "chronological project memories", false},
		{"Merge projects", "destructive operation deferred", true},
		{"Delete projects", "destructive operation deferred", true},
		{"Delete memories", "destructive operation deferred", true},
		{"Hive API config", "read-only state deferred", false},
		{"Hive API health", "read-only state deferred", false},
		{"Memory warnings", "read-only state deferred", false},
		{"Backup / Restore", "restore execution deferred", true},
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

func (m Model) selectedProject() hiveclient.Project {
	if len(m.snapshot.Projects) == 0 {
		return hiveclient.Project{Name: "-"}
	}
	return m.snapshot.Projects[wrapIndex(m.projectIndex, len(m.snapshot.Projects))]
}

func (m Model) projectMemories() []hiveclient.Memory {
	project := m.selectedProject().Name
	memories := make([]hiveclient.Memory, 0, len(m.snapshot.Memories))
	for _, memory := range m.snapshot.Memories {
		if memory.Project == project && !memory.Deleted {
			memories = append(memories, memory)
		}
	}
	return memories
}

func (m Model) selectedMemory() hiveclient.Memory {
	memories := m.projectMemories()
	if len(memories) == 0 {
		return hiveclient.Memory{Project: m.selectedProject().Name, SyncID: "-"}
	}
	return memories[wrapIndex(m.memoryIndex, len(memories))]
}

func wrapIndex(index, length int) int {
	if length == 0 {
		return 0
	}
	index %= length
	if index < 0 {
		index += length
	}
	return index
}

func memoryKey(memory hiveclient.Memory) string {
	if memory.SyncID != "" {
		return memory.SyncID
	}
	if memory.ID != 0 {
		return fmt.Sprintf("%d", memory.ID)
	}
	return "-"
}

func syncText(memory hiveclient.Memory) string {
	if memory.SyncID != "" {
		return "synced"
	}
	return "local"
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < 0 {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", max(1, int(d.Minutes())))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int((d+24*time.Hour-1)/(24*time.Hour)))
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04")
}

func timelineDateText(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return relativeTime(t)
}

func timelineTimeText(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return t.Format("15:04")
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runeKey(msg tea.KeyMsg, r rune) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}
