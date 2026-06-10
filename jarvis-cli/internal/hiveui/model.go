package hiveui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	ScreenWarnings
	ScreenBackups
	ScreenBackupDetail
	ScreenAPIHealth
	ScreenAPIConfig
	ScreenMemoryGuard
	ScreenProjectArchive
	ScreenProjectMerge
)

type GuardExecutor interface {
	ExecuteGuard(context.Context, hiveclient.GuardRequest) (hiveclient.GuardResult, error)
}

type ProjectArchiveExecutor interface {
	ArchiveProject(context.Context, hiveclient.ProjectArchiveRequest) (hiveclient.ProjectArchiveResult, error)
}

type ProjectMergeExecutor interface {
	MergeProject(context.Context, hiveclient.ProjectMergeRequest) (hiveclient.ProjectMergeResult, error)
}

// ProjectMergeBatchExecutor executes a multi-source batch merge.
type ProjectMergeBatchExecutor interface {
	MergeProjects(context.Context, hiveclient.ProjectMergeBatchRequest) (hiveclient.ProjectMergeBatchResult, error)
}

// ProjectMergeImpact holds the per-source impact summary computed client-side
// from the snapshot before dispatching a batch merge.
type ProjectMergeImpact struct {
	Source          string
	Memories        int
	Sessions        int
	Prompts         int
	Unsynced        int
	HasSyncEvidence bool // true when project has any synced rows
}

type MemoryLoader interface {
	MemoryByID(context.Context, int64) (hiveclient.Memory, error)
}

type Snapshot struct {
	DashboardState DashboardState
	DaemonURL      string
	Projects       []hiveclient.Project
	Memories       []hiveclient.Memory
	Health         []hiveclient.Health
	Warnings       []hiveclient.Warning
	Backups        []hiveclient.Backup
	LoadError      error
}

type Model struct {
	// width holds the last known terminal width from tea.WindowSizeMsg.
	// Zero means no sizing message received yet; views apply an 80-col floor.
	width        int
	snapshot     Snapshot
	screen       Screen
	cursor       int
	projectIndex int
	memoryIndex  int
	warningIndex int
	backupIndex  int
	detailReturn Screen
	message      string

	guardExecutor     GuardExecutor
	guardOperation    string
	guardBackupID     string
	guardReason       string
	guardConfirmation string
	guardStep         memoryGuardStep
	guardSubmitting   bool
	guardMemory       hiveclient.Memory

	projectArchiveExecutor     ProjectArchiveExecutor
	projectArchiveProject      hiveclient.Project
	projectArchiveBackupID     string
	projectArchiveConfirmation string
	projectArchiveStep         memoryGuardStep
	projectArchiveSubmitting   bool

	projectMergeExecutor     ProjectMergeExecutor
	projectMergeSource       hiveclient.Project
	projectMergeTarget       string
	projectMergeBackupID     string
	projectMergeConfirmation string
	projectMergeStep         projectMergeStep
	projectMergeSubmitting   bool

	// Batch merge (multi-source) fields — used when projectMergeBatchExecutor is set.
	projectMergeBatchExecutor ProjectMergeBatchExecutor
	mergeStep                 batchMergeStep
	mergeSelectedSources      []string
	mergeTarget               string
	mergeTargetIsNew          bool
	mergeImpact               []ProjectMergeImpact
	mergeSyncEvidence         bool
	mergeBackupID             string
	mergeConfirmText          string
	mergeBatchResult          *hiveclient.ProjectMergeBatchResult
	mergeBatchSubmitting      bool

	memoryLoader  MemoryLoader
	memoryContent string
	memoryLoading bool
	memoryLoadErr error
}

type memoryGuardStep int

const (
	memoryGuardBackupID memoryGuardStep = iota
	memoryGuardReason
	memoryGuardConfirmation
)

type projectMergeStep int

const (
	projectMergeTarget projectMergeStep = iota
	projectMergeBackupID
	projectMergeConfirmation
)

// batchMergeStep is the step enum for the multi-source merge flow.
type batchMergeStep int

const (
	mergeStepSelectSources batchMergeStep = iota
	mergeStepPickTarget
	mergeStepImpact
	mergeStepBackupID
	mergeStepConfirm
	mergeStepExecuting
	mergeStepResult
)

type memoryGuardResultMsg struct {
	operation  string
	targetType string
	targetID   int64
	backupID   string
	result     hiveclient.GuardResult
	err        error
}

type projectArchiveResultMsg struct {
	project  string
	backupID string
	result   hiveclient.ProjectArchiveResult
	err      error
}

type projectMergeResultMsg struct {
	sourceProject string
	targetProject string
	backupID      string
	result        hiveclient.ProjectMergeResult
	err           error
}

type projectMergeBatchResultMsg struct {
	sources  []string
	target   string
	backupID string
	result   hiveclient.ProjectMergeBatchResult
	err      error
}

type memoryLoadResultMsg struct {
	id     int64
	memory hiveclient.Memory
	err    error
}

func NewModelWithSnapshot(snapshot Snapshot) Model {
	if snapshot.DaemonURL == "" {
		snapshot.DaemonURL = "http://127.0.0.1:7438"
	}
	return Model{snapshot: snapshot}
}

func NewModelWithSnapshotAndGuardExecutor(snapshot Snapshot, executor GuardExecutor) Model {
	m := NewModelWithSnapshot(snapshot)
	m.guardExecutor = executor
	return m
}

func NewModelWithSnapshotAndProjectArchiveExecutor(snapshot Snapshot, executor ProjectArchiveExecutor) Model {
	m := NewModelWithSnapshot(snapshot)
	m.projectArchiveExecutor = executor
	return m
}

func NewModelWithSnapshotAndProjectMergeExecutor(snapshot Snapshot, executor ProjectMergeExecutor) Model {
	m := NewModelWithSnapshot(snapshot)
	m.projectMergeExecutor = executor
	return m
}

func NewModelWithSnapshotAndProjectMergeBatchExecutor(snapshot Snapshot, executor ProjectMergeBatchExecutor) Model {
	m := NewModelWithSnapshot(snapshot)
	m.projectMergeBatchExecutor = executor
	return m
}

func NewModelWithAllExecutors(snapshot Snapshot, guard GuardExecutor, archive ProjectArchiveExecutor, merge ProjectMergeExecutor, memory MemoryLoader) Model {
	m := NewModelWithSnapshotAndGuardExecutor(snapshot, guard)
	m.projectArchiveExecutor = archive
	m.projectMergeExecutor = merge
	m.memoryLoader = memory
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(memoryGuardResultMsg); ok {
		return m.applyMemoryGuardResult(result), nil
	}
	if result, ok := msg.(projectArchiveResultMsg); ok {
		return m.applyProjectArchiveResult(result), nil
	}
	if result, ok := msg.(projectMergeResultMsg); ok {
		return m.applyProjectMergeResult(result), nil
	}
	if result, ok := msg.(projectMergeBatchResultMsg); ok {
		return m.applyProjectMergeBatchResult(result), nil
	}
	if result, ok := msg.(memoryLoadResultMsg); ok {
		return m.applyMemoryLoadResult(result), nil
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sz.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.screen == ScreenProjectMerge && m.projectMergeBatchExecutor != nil {
		if key.Type == tea.KeyCtrlC && !m.mergeBatchSubmitting {
			return m, tea.Quit
		}
		return m.updateBatchProjectMerge(key)
	}
	if m.screen == ScreenProjectMerge {
		if key.Type == tea.KeyCtrlC && !m.projectMergeSubmitting {
			return m, tea.Quit
		}
		return m.updateProjectMerge(key)
	}
	if m.screen == ScreenProjectArchive && m.projectArchiveSubmitting {
		return m.updateProjectArchive(key)
	}
	if m.screen == ScreenProjectArchive {
		if key.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m.updateProjectArchive(key)
	}
	if m.screen == ScreenMemoryGuard {
		if key.Type == tea.KeyCtrlC && !m.guardSubmitting {
			return m, tea.Quit
		}
		return m.updateMemoryGuard(key)
	}
	if key.Type == tea.KeyCtrlC || runeKey(key, 'q') {
		return m, tea.Quit
	}
	m.message = ""
	switch {
	case key.Type == tea.KeyEsc || key.Type == tea.KeyBackspace:
		m = m.back()
	case m.screen == ScreenMemoryDetail && runeKey(key, 'd') && !m.selectedMemory().Deleted:
		m = m.startMemoryGuard("delete")
	case m.screen == ScreenMemoryDetail && runeKey(key, 'r') && m.selectedMemory().Deleted:
		m = m.startMemoryGuard("restore")
	case m.screen == ScreenProjects && runeKey(key, 'a') && m.projectArchiveExecutor != nil:
		m = m.startProjectArchive()
	case m.screen == ScreenProjects && runeKey(key, 'm') && m.projectMergeBatchExecutor != nil:
		m = m.startBatchProjectMerge()
	case m.screen == ScreenProjects && runeKey(key, 'm') && m.projectMergeExecutor != nil:
		m = m.startProjectMerge()
	case runeKey(key, 't'):
		m.screen = ScreenTimeline
	case runeKey(key, 'w'):
		m.screen = ScreenWarnings
	case runeKey(key, 'b'):
		m.screen = ScreenBackups
	case runeKey(key, 'g'):
		m.screen = ScreenAPIHealth
	case runeKey(key, 'c'):
		m.screen = ScreenAPIConfig
	case key.Type == tea.KeyDown || runeKey(key, 'j'):
		m = m.move(1)
	case key.Type == tea.KeyUp || runeKey(key, 'k'):
		m = m.move(-1)
	case key.Type == tea.KeyEnter:
		m = m.open()
		if m.screen == ScreenMemoryDetail && m.memoryLoader != nil {
			return m.startMemoryLoad()
		}
	}
	return m, nil
}

func (m Model) Screen() Screen { return m.screen }

func (m Model) View() string {
	if m.snapshot.DashboardState == DashboardDaemonUnavailable || m.snapshot.LoadError != nil {
		w := max(m.width, 80)
		panelW := panelWidth(w)
		var sb strings.Builder
		crumb := breadcrumbStyle.Render("~/.jarvis · hive tui — ") + breadcrumbCurrent.Render("dashboard · offline")
		sb.WriteString(headerRow(crumb, modeBadge("offline"), w))
		sb.WriteString("\n")
		errorContent := statusDot("failed") + " " + dimTextStyle.Render("Cannot reach hive-daemon") + "\n" +
			dimTextStyle.Render(fmt.Sprintf("No response from %s", m.snapshot.DaemonURL)) + "\n" +
			dimTextStyle.Render("The local Hive daemon is not running, so the TUI has nothing to read.")
		sb.WriteString(borderedPanel(sectionHeader("CANNOT REACH HIVE-DAEMON", panelW)+errorContent, panelW))
		sb.WriteString("\n")
		glanceContent := dimTextStyle.Render("projects — memories — unsynced n/a warnings —")
		sb.WriteString(borderedPanel(sectionHeader("AT A GLANCE", panelW)+glanceContent, panelW))
		sb.WriteString("\n")
		sb.WriteString(helpBar([]KeyHint{{"q", "quit"}}, "offline", w))
		return sb.String()
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
	case ScreenWarnings:
		return m.warningsView()
	case ScreenBackups:
		return m.backupsView()
	case ScreenBackupDetail:
		return m.backupDetailView()
	case ScreenAPIHealth:
		return m.apiHealthView()
	case ScreenAPIConfig:
		return m.apiConfigView()
	case ScreenMemoryGuard:
		return m.memoryGuardView()
	case ScreenProjectArchive:
		return m.projectArchiveView()
	case ScreenProjectMerge:
		if m.projectMergeBatchExecutor != nil {
			return m.batchProjectMergeView()
		}
		return m.projectMergeView()
	}

	w := max(m.width, 80)
	panelW := panelWidth(w)

	// Determine mode for badge and breadcrumb suffix
	mode := "normal"
	switch m.snapshot.DashboardState {
	case DashboardDegraded:
		mode = "auth failed"
	case DashboardLocalOnly:
		mode = "local-only"
	}

	var sb strings.Builder
	crumb := breadcrumbStyle.Render("~/.jarvis · hive tui — ") + breadcrumbCurrent.Render(dashboardTitle(m.snapshot.DashboardState))
	sb.WriteString(headerRow(crumb, modeBadge(mode), w))
	sb.WriteString("\n")

	// Notice panel for degraded / local-only
	if notice := dashboardNotice(m.snapshot); notice != "" {
		panelLabel := "NOTICE"
		if m.snapshot.DashboardState == DashboardLocalOnly {
			panelLabel = "NOTE"
		}
		sb.WriteString(borderedPanel(sectionHeader(panelLabel, panelW)+dimTextStyle.Render(notice), panelW))
		sb.WriteString("\n")
	}

	// STATUS panel
	statusContent := fmt.Sprintf("daemon running · %s · %s", apiStatus(m.snapshot), syncStatus(m.snapshot))
	sb.WriteString(borderedPanel(sectionHeader("STATUS", panelW)+statusContent, panelW))
	sb.WriteString("\n")

	// AT A GLANCE panel
	glanceLine := dimTextStyle.Render(fmt.Sprintf(
		"projects %d  memories %s  unsynced %s  warnings %d  last sync %s",
		len(m.snapshot.Projects),
		comma(totalMemories(m.snapshot.Projects)),
		unsyncedText(m.snapshot),
		len(m.snapshot.Warnings),
		lastSyncText(m.snapshot),
	))
	sb.WriteString(borderedPanel(sectionHeader("AT A GLANCE", panelW)+glanceLine, panelW))
	sb.WriteString("\n")

	// ACTIONS panel
	var actionsBlock strings.Builder
	for i, action := range dashboardActions() {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("▌") + " "
		}
		state := ""
		if action.disabled {
			state = dimTextStyle.Render(" (disabled)")
		}
		var row string
		if i == m.cursor {
			row = selectedRow(action.label+state+" — "+dimTextStyle.Render(action.description), panelW-4)
		} else {
			row = titleStyle.Render(action.label) + state + " — " + dimTextStyle.Render(action.description)
		}
		actionsBlock.WriteString(cursor + row + "\n")
	}
	sb.WriteString(borderedPanel(sectionHeader("ACTIONS", panelW)+actionsBlock.String(), panelW))

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"j/k", "move"}, {"enter", "open"}, {"w", "warnings"}, {"g", "health"}, {"c", "config"}, {"b", "backups"}, {"q", "quit"}}, mode, w))
	return sb.String()
}

func (m Model) move(delta int) Model {
	switch m.screen {
	case ScreenProjects:
		m.projectIndex = wrapIndex(m.projectIndex+delta, len(m.snapshot.Projects))
	case ScreenProjectMemories, ScreenTimeline:
		m.memoryIndex = wrapIndex(m.memoryIndex+delta, len(m.projectMemories()))
	case ScreenWarnings:
		m.warningIndex = wrapIndex(m.warningIndex+delta, len(m.snapshot.Warnings))
	case ScreenBackups:
		m.backupIndex = wrapIndex(m.backupIndex+delta, len(m.snapshot.Backups))
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
	if m.screen == ScreenBackups {
		if len(m.snapshot.Backups) == 0 {
			m.message = "No item is available to inspect."
			return m
		}
		m.screen = ScreenBackupDetail
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
	case "Hive API config":
		m.screen = ScreenAPIConfig
	case "Hive API health":
		m.screen = ScreenAPIHealth
	case "Memory warnings":
		m.screen = ScreenWarnings
	case "Backup snapshots":
		m.screen = ScreenBackups
	default:
		m.message = action.label + " is not available in this navigation sub-slice. No local Hive state was changed."
	}
	return m
}

func (m Model) back() Model {
	switch m.screen {
	case ScreenMemoryDetail:
		m.memoryContent = ""
		m.memoryLoading = false
		m.memoryLoadErr = nil
		if m.detailReturn == ScreenTimeline {
			m.screen = ScreenTimeline
		} else {
			m.screen = ScreenProjectMemories
		}
	case ScreenMemoryGuard:
		m.screen = ScreenMemoryDetail
	case ScreenProjectArchive:
		m.screen = ScreenProjects
	case ScreenProjectMerge:
		m.screen = ScreenProjects
		m.mergeSelectedSources = nil
		m.mergeBatchResult = nil
		m.mergeBatchSubmitting = false
	case ScreenProjectMemories:
		m.screen = ScreenProjects
	case ScreenTimeline:
		m.screen = ScreenDashboard
	case ScreenBackupDetail:
		m.screen = ScreenBackups
	case ScreenWarnings, ScreenBackups, ScreenAPIHealth, ScreenAPIConfig:
		m.screen = ScreenDashboard
	case ScreenProjects:
		m.screen = ScreenDashboard
	}
	return m
}

func (m Model) projectsView() string {
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbStyle.Render("dashboard / ") + breadcrumbCurrent.Render("projects")
	countBadge := dimTextStyle.Render(fmt.Sprintf("%d projects", len(m.snapshot.Projects)))

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, countBadge, w))
	sb.WriteString("\n")

	var tableContent strings.Builder
	tableContent.WriteString(columnHeaderStyle.Render("PROJECT  MEMORIES  UNSYNCED  WARNINGS  LAST") + "\n")
	for i, project := range m.snapshot.Projects {
		cursor := "  "
		if i == m.projectIndex {
			cursor = cursorStyle.Render("▌") + " "
		}
		rowText := fmt.Sprintf("%s  %d  %d  %d  %s",
			project.Name,
			project.ActiveMemoryCount,
			project.UnsyncedCount,
			projectWarningCount(m.snapshot, project.Name),
			relativeTime(project.LastActivityAt),
		)
		var row string
		if i == m.projectIndex {
			row = selectedRow(rowText, panelW-4)
		} else {
			row = rowText
		}
		tableContent.WriteString(cursor + row + "\n")
	}
	sb.WriteString(borderedPanel(sectionHeader("PROJECTS", panelW)+tableContent.String(), panelW))

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	if m.projectArchiveExecutor != nil && len(m.snapshot.Projects) > 0 {
		sb.WriteString("\na archive guarded by backup ID and exact confirmation\n")
	}
	if m.projectMergeExecutor != nil && len(m.snapshot.Projects) > 0 {
		sb.WriteString("m merge guarded by backup ID and exact confirmation\n")
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"j/k", "move"}, {"⏎", "open"}, {"t", "timeline"}, {"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func (m Model) projectMemoriesView() string {
	memories := m.projectMemories()
	project := m.selectedProject().Name
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbStyle.Render("dashboard / projects / ") + breadcrumbCurrent.Render(project)
	countBadge := dimTextStyle.Render(fmt.Sprintf("%d memories", len(memories)))

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, countBadge, w))
	sb.WriteString("\n")

	var listContent strings.Builder
	if len(memories) == 0 {
		listContent.WriteString(dimTextStyle.Render("No local Hive memories found for this project") + "\n")
	}
	for i, memory := range memories {
		cursor := "  "
		if i == m.memoryIndex {
			cursor = cursorStyle.Render("▌") + " "
		}
		deleted := ""
		if memory.Deleted {
			deleted = " [deleted]"
		}
		rowText := typeBadge(emptyDash(memory.Category)) + "  " + memory.Title + deleted + "  " + dimTextStyle.Render(relativeTime(memory.CreatedAt))
		var row string
		if i == m.memoryIndex {
			row = selectedRow(rowText, panelW-4)
		} else {
			row = rowText
		}
		listContent.WriteString(cursor + row + "\n")
	}
	sb.WriteString(borderedPanel(sectionHeader("MEMORIES", panelW)+listContent.String(), panelW))

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"j/k", "move"}, {"⏎", "open"}, {"t", "timeline"}, {"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func (m Model) memoryDetailView() string {
	memory := m.selectedMemory()
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbStyle.Render("… / "+memory.Project+" / ") + breadcrumbCurrent.Render(memoryKey(memory))

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, typeBadge(emptyDash(memory.Category)), w))
	sb.WriteString("\n")

	// METADATA panel
	metaContent := fmt.Sprintf("%s %s  %s %s\n%s %s  %s %s\n%s %s  %s %s",
		dimTextStyle.Render("id"), memoryKey(memory),
		dimTextStyle.Render("created"), formatDateTime(memory.CreatedAt),
		dimTextStyle.Render("project"), memory.Project,
		dimTextStyle.Render("sync"), syncText(memory),
		dimTextStyle.Render("type"), emptyDash(memory.Category),
		dimTextStyle.Render("source"), emptyDash(memory.CreatedBy),
	)
	if memory.Deleted {
		metaContent += "\n" + dimTextStyle.Render("status") + " deleted"
	}
	sb.WriteString(borderedPanel(sectionHeader("METADATA", panelW)+metaContent, panelW))
	sb.WriteString("\n")

	// CONTENT panel
	var contentBody string
	switch {
	case m.memoryLoader == nil:
		contentBody = readOnlyBanner.Render("Content preview is not available from the read-only daemon snapshot.")
	case m.memoryLoading:
		contentBody = "Loading content from hive-daemon..."
	case m.memoryLoadErr != nil:
		contentBody = fmt.Sprintf("Content failed to load through hive-daemon: %v", m.memoryLoadErr)
	case strings.TrimSpace(m.memoryContent) == "":
		contentBody = dimTextStyle.Render("No content recorded for this memory.")
	default:
		contentBody = m.memoryContent
	}
	sb.WriteString(borderedPanel(sectionHeader("CONTENT", panelW)+contentBody, panelW))

	if m.guardExecutor != nil && memory.ID != 0 && memory.Deleted {
		sb.WriteString("\nr restore guarded by backup ID and exact confirmation\n")
	} else if m.guardExecutor != nil && memory.ID != 0 {
		sb.WriteString("\nd delete guarded by backup ID, delete reason, and exact confirmation\n")
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "%s\n", m.message)
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func (m Model) startMemoryGuard(operation string) Model {
	if m.guardExecutor == nil {
		return m
	}
	if m.guardSubmitting {
		m.screen = ScreenMemoryGuard
		m.message = fmt.Sprintf("Guarded memory %s is already pending through hive-daemon. Wait for the result before leaving or submitting again.", m.guardOperation)
		return m
	}
	memory := m.selectedMemory()
	if memory.ID == 0 {
		m.message = fmt.Sprintf("Memory numeric ID is required before guarded %s can be dispatched.", operation)
		return m
	}
	m.screen = ScreenMemoryGuard
	m.guardOperation = operation
	m.guardMemory = memory
	m.guardStep = memoryGuardBackupID
	m.guardBackupID = ""
	m.guardReason = ""
	m.guardConfirmation = ""
	m.guardSubmitting = false
	m.message = ""
	return m
}

func (m Model) updateMemoryGuard(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.guardSubmitting {
		m.message = fmt.Sprintf("Guarded memory %s is already pending through hive-daemon. Wait for the result before leaving or submitting again.", m.guardOperation)
		return m, nil
	}
	switch {
	case key.Type == tea.KeyEsc:
		m = m.back()
		m.message = ""
		return m, nil
	case key.Type == tea.KeyBackspace:
		m = m.removeGuardRune()
		return m, nil
	case key.Type == tea.KeyEnter:
		return m.submitMemoryGuard()
	case key.Type == tea.KeySpace:
		m = m.appendGuardText(" ")
		return m, nil
	case key.Type == tea.KeyRunes:
		m = m.appendGuardText(string(key.Runes))
		return m, nil
	}
	return m, nil
}

func (m Model) submitMemoryGuard() (tea.Model, tea.Cmd) {
	m.message = ""
	if m.guardSubmitting {
		m.message = fmt.Sprintf("Guarded memory %s is already pending through hive-daemon.", m.guardOperation)
		return m, nil
	}
	if m.guardStep == memoryGuardBackupID {
		if strings.TrimSpace(m.guardBackupID) == "" {
			m.message = fmt.Sprintf("Backup ID is required before guarded %s.", m.guardOperation)
			return m, nil
		}
		if m.guardOperation == "delete" {
			m.guardStep = memoryGuardReason
			return m, nil
		}
		m.guardStep = memoryGuardConfirmation
		return m, nil
	}
	if m.guardStep == memoryGuardReason {
		reason := strings.TrimSpace(m.guardReason)
		if reason == "" {
			m.message = "Delete reason is required before guarded delete."
			return m, nil
		}
		m.guardReason = reason
		m.guardStep = memoryGuardConfirmation
		return m, nil
	}
	expected := memoryGuardConfirmationPhrase(m.guardOperation, m.guardMemory)
	if m.guardConfirmation != expected {
		m.message = "Confirmation mismatch. Type the phrase exactly; input is not trimmed."
		return m, nil
	}
	if m.guardExecutor == nil {
		m.message = fmt.Sprintf("Guarded memory %s is unavailable without a daemon command boundary.", m.guardOperation)
		return m, nil
	}
	executor := m.guardExecutor
	request := hiveclient.GuardRequest{
		Operation:    m.guardOperation,
		TargetType:   "memory",
		TargetID:     m.guardMemory.ID,
		BackupID:     strings.TrimSpace(m.guardBackupID),
		Confirmation: m.guardConfirmation,
	}
	if m.guardOperation == "delete" {
		request.Reason = strings.TrimSpace(m.guardReason)
	}
	m.guardSubmitting = true
	return m, func() tea.Msg {
		result, err := executor.ExecuteGuard(context.Background(), request)
		return memoryGuardResultMsg{operation: request.Operation, targetType: request.TargetType, targetID: request.TargetID, backupID: request.BackupID, result: result, err: err}
	}
}

func (m Model) applyMemoryGuardResult(msg memoryGuardResultMsg) Model {
	if !m.guardSubmitting || msg.operation != m.guardOperation || msg.targetType != "memory" || msg.targetID != m.guardMemory.ID || msg.backupID != strings.TrimSpace(m.guardBackupID) {
		return m
	}
	m.screen = ScreenMemoryDetail
	m.guardSubmitting = false
	if msg.err != nil {
		m.message = fmt.Sprintf("Guarded memory %s failed through hive-daemon: %v", msg.operation, msg.err)
		return m
	}
	m.message = fmt.Sprintf("Guarded memory %s dispatched through hive-daemon. No direct SQLite or cloud mutation was performed by the TUI.", msg.operation)
	if msg.operation == "delete" {
		m = m.removeMemoryFromNormalSnapshot(msg.targetID)
		if m.detailReturn == ScreenTimeline {
			m.screen = ScreenTimeline
		} else {
			m.screen = ScreenProjectMemories
		}
		m.memoryContent = ""
		m.memoryLoading = false
		m.memoryLoadErr = nil
	}
	return m
}

func (m Model) removeMemoryFromNormalSnapshot(id int64) Model {
	var deletedProject string
	memories := m.snapshot.Memories[:0]
	for _, memory := range m.snapshot.Memories {
		if memory.ID == id {
			deletedProject = memory.Project
			continue
		}
		memories = append(memories, memory)
	}
	m.snapshot.Memories = memories
	if deletedProject != "" {
		for i := range m.snapshot.Projects {
			if m.snapshot.Projects[i].Name != deletedProject {
				continue
			}
			if m.snapshot.Projects[i].ActiveMemoryCount > 0 {
				m.snapshot.Projects[i].ActiveMemoryCount--
			}
			m.snapshot.Projects[i].DeletedMemoryCount++
			break
		}
	}
	if len(m.projectMemories()) == 0 {
		m.memoryIndex = 0
		return m
	}
	m.memoryIndex = wrapIndex(m.memoryIndex, len(m.projectMemories()))
	return m
}

func (m Model) startMemoryLoad() (tea.Model, tea.Cmd) {
	memory := m.selectedMemory()
	if memory.ID == 0 {
		m.memoryLoading = false
		m.memoryContent = ""
		m.memoryLoadErr = nil
		return m, nil
	}
	loader := m.memoryLoader
	id := memory.ID
	m.memoryLoading = true
	m.memoryContent = ""
	m.memoryLoadErr = nil
	return m, func() tea.Msg {
		loaded, err := loader.MemoryByID(context.Background(), id)
		return memoryLoadResultMsg{id: id, memory: loaded, err: err}
	}
}

func (m Model) applyMemoryLoadResult(msg memoryLoadResultMsg) Model {
	if !m.memoryLoading || msg.id != m.selectedMemory().ID {
		return m
	}
	m.memoryLoading = false
	if msg.err != nil {
		m.memoryLoadErr = msg.err
		m.memoryContent = ""
		return m
	}
	m.memoryLoadErr = nil
	m.memoryContent = msg.memory.Content
	return m
}

func (m Model) appendGuardText(text string) Model {
	switch m.guardStep {
	case memoryGuardBackupID:
		m.guardBackupID += text
	case memoryGuardReason:
		m.guardReason += text
	default:
		m.guardConfirmation += text
	}
	return m
}

func (m Model) removeGuardRune() Model {
	switch m.guardStep {
	case memoryGuardBackupID:
		m.guardBackupID = trimLastRune(m.guardBackupID)
	case memoryGuardReason:
		m.guardReason = trimLastRune(m.guardReason)
	default:
		m.guardConfirmation = trimLastRune(m.guardConfirmation)
	}
	return m
}

func (m Model) memoryGuardView() string {
	memory := m.guardMemory
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render(fmt.Sprintf("guarded memory %s", m.guardOperation))

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, modeBadge("destructive"), w))
	sb.WriteString("\n")

	// IMPACT panel
	impactContent := fmt.Sprintf("%s %s", dimTextStyle.Render("target"), memoryKey(memory))
	sb.WriteString(borderedPanel(sectionHeader("IMPACT", panelW)+impactContent, panelW))
	sb.WriteString("\n")

	// SAFETY panel
	safetyContent := fmt.Sprintf("Backup ID is required: %s\n", visibleInput(m.guardBackupID))
	if m.guardOperation == "delete" {
		safetyContent += fmt.Sprintf("Delete reason is required: %s\n", visibleInput(m.guardReason))
	}
	if m.guardOperation == "delete" && m.guardStep != memoryGuardConfirmation {
		safetyContent += "Confirmation must match exactly after backup ID and delete reason are provided.\n"
	} else {
		safetyContent += fmt.Sprintf("Confirmation must match exactly. Type exactly: %s\n", memoryGuardConfirmationPhrase(m.guardOperation, memory))
	}
	if m.guardStep == memoryGuardConfirmation {
		safetyContent += fmt.Sprintf("confirmation: %s\n", visibleInput(m.guardConfirmation))
	}
	fieldScope := "both fields"
	if m.guardOperation == "delete" {
		fieldScope = "all fields"
	}
	safetyContent += fmt.Sprintf("No %s will run until %s pass guards. Dispatch uses hive-daemon only; no direct SQLite or cloud mutation.", m.guardOperation, fieldScope)
	sb.WriteString(borderedPanel(sectionHeader("SAFETY", panelW)+safetyContent, panelW))

	if m.guardSubmitting {
		fmt.Fprintf(&sb, "\n%s\n", guardPending.Render(fmt.Sprintf("Guarded memory %s is pending through hive-daemon. Wait for the result before leaving or submitting again.", m.guardOperation)))
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	if !m.guardSubmitting {
		sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"ctrl-c", "quit"}}, "destructive", w))
	}
	return sb.String()
}

func memoryGuardConfirmationPhrase(operation string, memory hiveclient.Memory) string {
	return fmt.Sprintf("%s memory %d", strings.ToUpper(operation), memory.ID)
}

func (m Model) startProjectArchive() Model {
	if m.projectArchiveExecutor == nil || len(m.snapshot.Projects) == 0 {
		return m
	}
	m.screen = ScreenProjectArchive
	m.projectArchiveProject = m.selectedProject()
	m.projectArchiveBackupID = ""
	m.projectArchiveConfirmation = ""
	m.projectArchiveStep = memoryGuardBackupID
	m.projectArchiveSubmitting = false
	m.message = ""
	return m
}

func (m Model) updateProjectArchive(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.projectArchiveSubmitting {
		m.message = "Guarded project archive is already pending through hive-daemon. Wait for the result before leaving or submitting again."
		return m, nil
	}
	switch {
	case key.Type == tea.KeyEsc:
		m = m.back()
		m.message = ""
		return m, nil
	case key.Type == tea.KeyBackspace:
		m = m.removeProjectArchiveRune()
		return m, nil
	case key.Type == tea.KeyEnter:
		return m.submitProjectArchive()
	case key.Type == tea.KeySpace:
		m = m.appendProjectArchiveText(" ")
		return m, nil
	case key.Type == tea.KeyRunes:
		m = m.appendProjectArchiveText(string(key.Runes))
		return m, nil
	}
	return m, nil
}

func (m Model) submitProjectArchive() (tea.Model, tea.Cmd) {
	if m.projectArchiveStep == memoryGuardBackupID {
		if strings.TrimSpace(m.projectArchiveBackupID) == "" {
			m.message = "Backup ID is required before guarded project archive."
			return m, nil
		}
		m.projectArchiveStep = memoryGuardConfirmation
		m.message = ""
		return m, nil
	}
	expected := projectArchiveConfirmationPhrase(m.projectArchiveProject.Name)
	if m.projectArchiveConfirmation != expected {
		m.message = "Confirmation mismatch. Type the phrase exactly; input is not trimmed."
		return m, nil
	}
	if m.projectArchiveExecutor == nil {
		m.message = "Guarded project archive is unavailable without a daemon command boundary."
		return m, nil
	}
	executor := m.projectArchiveExecutor
	request := hiveclient.ProjectArchiveRequest{Project: m.projectArchiveProject.Name, BackupID: strings.TrimSpace(m.projectArchiveBackupID), Confirmation: m.projectArchiveConfirmation}
	m.projectArchiveSubmitting = true
	return m, func() tea.Msg {
		result, err := executor.ArchiveProject(context.Background(), request)
		return projectArchiveResultMsg{project: request.Project, backupID: request.BackupID, result: result, err: err}
	}
}

func (m Model) applyProjectArchiveResult(msg projectArchiveResultMsg) Model {
	if !m.projectArchiveSubmitting || msg.project != m.projectArchiveProject.Name || msg.backupID != strings.TrimSpace(m.projectArchiveBackupID) {
		return m
	}
	m.screen = ScreenProjects
	m.projectArchiveSubmitting = false
	if msg.err != nil {
		m.message = fmt.Sprintf("Project %s archive failed through hive-daemon: %v", msg.project, msg.err)
		return m
	}
	status := "already archived locally"
	if msg.result.Mutated {
		status = "archive completed locally"
	}
	m.message = fmt.Sprintf("Project %s %s with backup %s.", msg.project, status, msg.backupID)
	if strings.TrimSpace(msg.result.CloudHandoffNote) != "" {
		m.message += " Cloud handoff: " + msg.result.CloudHandoffNote
	}
	return m
}

func (m Model) appendProjectArchiveText(text string) Model {
	if m.projectArchiveStep == memoryGuardBackupID {
		m.projectArchiveBackupID += text
		return m
	}
	m.projectArchiveConfirmation += text
	return m
}

func (m Model) removeProjectArchiveRune() Model {
	if m.projectArchiveStep == memoryGuardBackupID {
		m.projectArchiveBackupID = trimLastRune(m.projectArchiveBackupID)
		return m
	}
	m.projectArchiveConfirmation = trimLastRune(m.projectArchiveConfirmation)
	return m
}

func (m Model) projectArchiveView() string {
	project := m.projectArchiveProject.Name
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("guarded project archive")

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, badgeDestructive.Render("destructive"), w))
	sb.WriteString("\n")

	// IMPACT panel
	impactContent := fmt.Sprintf("%s %s", dimTextStyle.Render("target"), project)
	sb.WriteString(borderedPanel(sectionHeader("IMPACT", panelW)+impactContent, panelW))
	sb.WriteString("\n")

	// REASON panel
	reasonContent := fmt.Sprintf("Backup ID is required: %s\n", visibleInput(m.projectArchiveBackupID)) +
		fmt.Sprintf("Confirmation must match exactly. Type exactly: %s\n", projectArchiveConfirmationPhrase(project))
	if m.projectArchiveStep == memoryGuardConfirmation {
		reasonContent += fmt.Sprintf("confirmation: %s\n", visibleInput(m.projectArchiveConfirmation))
	}
	reasonContent += "No archive will run until both fields pass guards. Dispatch uses hive-daemon only; no direct SQLite or cloud mutation."
	sb.WriteString(borderedPanel(sectionHeader("REASON — REQUIRED", panelW)+reasonContent, panelW))

	if m.projectArchiveSubmitting {
		fmt.Fprintf(&sb, "\n%s\n", guardPending.Render("Guarded project archive is pending through hive-daemon. Wait for the result before leaving or submitting again."))
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	if !m.projectArchiveSubmitting {
		sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"ctrl-c", "quit"}}, "destructive", w))
	}
	return sb.String()
}

func projectArchiveConfirmationPhrase(project string) string {
	return "ARCHIVE project " + project
}

func (m Model) startProjectMerge() Model {
	if m.projectMergeExecutor == nil || len(m.snapshot.Projects) == 0 {
		return m
	}
	m.screen = ScreenProjectMerge
	m.projectMergeSource = m.selectedProject()
	m.projectMergeTarget = ""
	m.projectMergeBackupID = ""
	m.projectMergeConfirmation = ""
	m.projectMergeStep = projectMergeTarget
	m.projectMergeSubmitting = false
	m.message = ""
	return m
}

func (m Model) updateProjectMerge(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.projectMergeSubmitting {
		m.message = "Guarded project merge is already pending through hive-daemon. Wait for the result before leaving or submitting again."
		return m, nil
	}
	switch {
	case key.Type == tea.KeyEsc:
		m = m.back()
		m.message = ""
		return m, nil
	case key.Type == tea.KeyBackspace:
		m = m.removeProjectMergeRune()
		return m, nil
	case key.Type == tea.KeyEnter:
		return m.submitProjectMerge()
	case key.Type == tea.KeySpace:
		m = m.appendProjectMergeText(" ")
		return m, nil
	case key.Type == tea.KeyRunes:
		m = m.appendProjectMergeText(string(key.Runes))
		return m, nil
	}
	return m, nil
}

func (m Model) submitProjectMerge() (tea.Model, tea.Cmd) {
	source := strings.TrimSpace(m.projectMergeSource.Name)
	target := strings.TrimSpace(m.projectMergeTarget)
	switch m.projectMergeStep {
	case projectMergeTarget:
		if source == "" || source == "-" {
			m.message = "Source project is required before guarded project merge."
			return m, nil
		}
		if target == "" {
			m.message = "Target project is required before guarded project merge."
			return m, nil
		}
		if source == target {
			m.message = "Source and target project must be different before guarded project merge."
			return m, nil
		}
		if !m.snapshotHasProject(target) {
			m.message = fmt.Sprintf("Target project %s is not in the current snapshot before guarded project merge.", target)
			return m, nil
		}
		m.projectMergeTarget = target
		m.projectMergeStep = projectMergeBackupID
		m.message = ""
		return m, nil
	case projectMergeBackupID:
		backupID := strings.TrimSpace(m.projectMergeBackupID)
		if backupID == "" {
			m.message = "Backup ID is required before guarded project merge."
			return m, nil
		}
		if !m.snapshotHasBackup(backupID) {
			m.message = fmt.Sprintf("Backup ID %s is not in the current snapshot before guarded project merge.", backupID)
			return m, nil
		}
		m.projectMergeStep = projectMergeConfirmation
		m.message = ""
		return m, nil
	}
	expected := projectMergeConfirmationPhrase(source, target)
	if m.projectMergeConfirmation != expected {
		m.message = "Confirmation mismatch. Type the phrase exactly; input is not trimmed."
		return m, nil
	}
	if m.projectMergeExecutor == nil {
		m.message = "Guarded project merge is unavailable without a daemon command boundary."
		return m, nil
	}
	executor := m.projectMergeExecutor
	request := hiveclient.ProjectMergeRequest{SourceProject: source, TargetProject: target, BackupID: strings.TrimSpace(m.projectMergeBackupID), Confirmation: m.projectMergeConfirmation}
	m.projectMergeSubmitting = true
	return m, func() tea.Msg {
		result, err := executor.MergeProject(context.Background(), request)
		return projectMergeResultMsg{sourceProject: request.SourceProject, targetProject: request.TargetProject, backupID: request.BackupID, result: result, err: err}
	}
}

func (m Model) applyProjectMergeResult(msg projectMergeResultMsg) Model {
	if !m.projectMergeSubmitting || msg.sourceProject != strings.TrimSpace(m.projectMergeSource.Name) || msg.targetProject != strings.TrimSpace(m.projectMergeTarget) || msg.backupID != strings.TrimSpace(m.projectMergeBackupID) {
		return m
	}
	m.screen = ScreenProjects
	m.projectMergeSubmitting = false
	if msg.err != nil {
		m.message = fmt.Sprintf("Project %s merge into %s failed through hive-daemon: %v", msg.sourceProject, msg.targetProject, msg.err)
		return m
	}
	status := "already recorded locally"
	if msg.result.Mutated {
		status = "recorded locally"
	}
	m.message = fmt.Sprintf("Project %s merge into %s %s with backup %s.", msg.sourceProject, msg.targetProject, status, msg.backupID)
	if strings.TrimSpace(msg.result.CloudHandoffNote) != "" {
		m.message += " Cloud handoff: " + msg.result.CloudHandoffNote
	}
	return m
}

func (m Model) appendProjectMergeText(text string) Model {
	switch m.projectMergeStep {
	case projectMergeTarget:
		m.projectMergeTarget += text
	case projectMergeBackupID:
		m.projectMergeBackupID += text
	default:
		m.projectMergeConfirmation += text
	}
	return m
}

func (m Model) removeProjectMergeRune() Model {
	switch m.projectMergeStep {
	case projectMergeTarget:
		m.projectMergeTarget = trimLastRune(m.projectMergeTarget)
	case projectMergeBackupID:
		m.projectMergeBackupID = trimLastRune(m.projectMergeBackupID)
	default:
		m.projectMergeConfirmation = trimLastRune(m.projectMergeConfirmation)
	}
	return m
}

func (m Model) projectMergeView() string {
	source := strings.TrimSpace(m.projectMergeSource.Name)
	target := strings.TrimSpace(m.projectMergeTarget)
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("guarded project merge")

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, badgeDestructive.Render("destructive"), w))
	sb.WriteString("\n")

	// IMPACT PREVIEW panel
	impactContent := fmt.Sprintf("%s %s", dimTextStyle.Render("source"), source)
	sb.WriteString(borderedPanel(sectionHeader("IMPACT PREVIEW", panelW)+impactContent, panelW))
	sb.WriteString("\n")

	// SAFETY panel — multi-step inputs
	safetyContent := fmt.Sprintf("Target project is required: %s\nBackup ID is required: %s\n",
		visibleInput(m.projectMergeTarget),
		visibleInput(m.projectMergeBackupID),
	)
	if target == "" {
		safetyContent += "Confirmation must match exactly after target is provided.\n"
	} else {
		safetyContent += "Confirmation must match exactly.\n"
		safetyContent += fmt.Sprintf("Type exactly: %s\n", projectMergeConfirmationPhrase(source, target))
	}
	if m.projectMergeStep == projectMergeConfirmation {
		safetyContent += fmt.Sprintf("confirmation: %s\n", visibleInput(m.projectMergeConfirmation))
	}
	safetyContent += "No merge will run until all fields pass guards. Dispatch uses hive-daemon only; no direct SQLite or cloud mutation."
	sb.WriteString(borderedPanel(sectionHeader("SAFETY", panelW)+safetyContent, panelW))

	if m.projectMergeSubmitting {
		fmt.Fprintf(&sb, "\n%s\n", guardPending.Render("Guarded project merge is pending through hive-daemon. Wait for the result before leaving or submitting again."))
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	if !m.projectMergeSubmitting {
		sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"ctrl-c", "quit"}}, "destructive", w))
	}
	return sb.String()
}

func projectMergeConfirmationPhrase(source, target string) string {
	return "MERGE project " + source + " INTO " + target
}

// ─── Batch multi-source merge flow ──────────────────────────────────────────

func (m Model) startBatchProjectMerge() Model {
	if m.projectMergeBatchExecutor == nil || len(m.snapshot.Projects) == 0 {
		return m
	}
	m.screen = ScreenProjectMerge
	m.mergeStep = mergeStepSelectSources
	m.mergeSelectedSources = nil
	m.mergeTarget = ""
	m.mergeTargetIsNew = false
	m.mergeImpact = nil
	m.mergeSyncEvidence = false
	m.mergeBackupID = ""
	m.mergeConfirmText = ""
	m.mergeBatchResult = nil
	m.mergeBatchSubmitting = false
	m.message = ""
	return m
}

// updateBatchProjectMerge handles key events for the multi-source merge flow.
func (m Model) updateBatchProjectMerge(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mergeBatchSubmitting {
		m.message = "Batch merge is already pending through hive-daemon. Wait for the result."
		return m, nil
	}
	switch {
	case key.Type == tea.KeyEsc:
		if m.mergeStep == mergeStepResult {
			m = m.back()
			m.message = ""
			return m, nil
		}
		m = m.back()
		m.message = ""
		return m, nil
	case key.Type == tea.KeyEnter:
		return m.submitBatchMergeStep()
	case key.Type == tea.KeySpace:
		if m.mergeStep == mergeStepSelectSources {
			m = m.toggleMergeSource()
			return m, nil
		}
		// text-input steps — append space
		m = m.appendBatchMergeText(" ")
		return m, nil
	case key.Type == tea.KeyDown:
		if m.mergeStep == mergeStepSelectSources {
			m.projectIndex = wrapIndex(m.projectIndex+1, len(m.snapshot.Projects))
		}
		return m, nil
	case key.Type == tea.KeyUp:
		if m.mergeStep == mergeStepSelectSources {
			m.projectIndex = wrapIndex(m.projectIndex-1, len(m.snapshot.Projects))
		}
		return m, nil
	case key.Type == tea.KeyBackspace:
		m = m.removeBatchMergeRune()
		return m, nil
	case key.Type == tea.KeyRunes:
		m = m.appendBatchMergeText(string(key.Runes))
		return m, nil
	}
	return m, nil
}

func (m Model) toggleMergeSource() Model {
	project := m.selectedProject().Name
	for i, s := range m.mergeSelectedSources {
		if s == project {
			m.mergeSelectedSources = append(m.mergeSelectedSources[:i], m.mergeSelectedSources[i+1:]...)
			return m
		}
	}
	m.mergeSelectedSources = append(m.mergeSelectedSources, project)
	return m
}

func (m Model) submitBatchMergeStep() (tea.Model, tea.Cmd) {
	m.message = ""
	switch m.mergeStep {
	case mergeStepSelectSources:
		if len(m.mergeSelectedSources) == 0 {
			m.message = "Select at least one source project before proceeding."
			return m, nil
		}
		m.mergeStep = mergeStepPickTarget
		return m, nil

	case mergeStepPickTarget:
		target := strings.TrimSpace(m.mergeTarget)
		if target == "" {
			m.message = "Target project name is required."
			return m, nil
		}
		for _, src := range m.mergeSelectedSources {
			if src == target {
				m.message = "Target must not be one of the selected sources."
				return m, nil
			}
		}
		m.mergeTarget = target
		m.mergeTargetIsNew = !m.snapshotHasProject(target)
		m.mergeImpact = computeMergeImpact(m.snapshot, m.mergeSelectedSources)
		m.mergeSyncEvidence = anyImpactHasSyncEvidence(m.mergeImpact) || targetHasSyncEvidence(m.snapshot, target)
		m.mergeStep = mergeStepImpact
		return m, nil

	case mergeStepImpact:
		m.mergeStep = mergeStepBackupID
		return m, nil

	case mergeStepBackupID:
		backupID := strings.TrimSpace(m.mergeBackupID)
		if backupID == "" {
			m.message = "Backup ID is required before batch merge."
			return m, nil
		}
		m.mergeStep = mergeStepConfirm
		return m, nil

	case mergeStepConfirm:
		expected := mergeBatchConfirmationPhrase(m.mergeSelectedSources, m.mergeTarget)
		if m.mergeConfirmText != expected {
			m.message = "Confirmation mismatch. Type the phrase exactly; input is not trimmed."
			return m, nil
		}
		if m.projectMergeBatchExecutor == nil {
			m.message = "Batch merge executor is not available."
			return m, nil
		}
		executor := m.projectMergeBatchExecutor
		req := hiveclient.ProjectMergeBatchRequest{
			Sources:      m.mergeSelectedSources,
			Target:       m.mergeTarget,
			BackupID:     strings.TrimSpace(m.mergeBackupID),
			Confirmation: m.mergeConfirmText,
		}
		m.mergeStep = mergeStepExecuting
		m.mergeBatchSubmitting = true
		return m, func() tea.Msg {
			result, err := executor.MergeProjects(context.Background(), req)
			return projectMergeBatchResultMsg{sources: req.Sources, target: req.Target, backupID: req.BackupID, result: result, err: err}
		}

	case mergeStepResult:
		// enter/esc → return to projects + reload snapshot
		m = m.back()
		return m, nil
	}
	return m, nil
}

func (m Model) applyProjectMergeBatchResult(msg projectMergeBatchResultMsg) Model {
	if !m.mergeBatchSubmitting {
		return m
	}
	m.mergeBatchSubmitting = false
	if msg.err != nil {
		m.mergeStep = mergeStepConfirm
		m.message = fmt.Sprintf("Batch merge failed: %v", msg.err)
		return m
	}
	result := msg.result
	m.mergeBatchResult = &result
	m.mergeStep = mergeStepResult
	return m
}

func (m Model) appendBatchMergeText(text string) Model {
	switch m.mergeStep {
	case mergeStepPickTarget:
		m.mergeTarget += text
	case mergeStepBackupID:
		m.mergeBackupID += text
	case mergeStepConfirm:
		m.mergeConfirmText += text
	}
	return m
}

func (m Model) removeBatchMergeRune() Model {
	switch m.mergeStep {
	case mergeStepPickTarget:
		m.mergeTarget = trimLastRune(m.mergeTarget)
	case mergeStepBackupID:
		m.mergeBackupID = trimLastRune(m.mergeBackupID)
	case mergeStepConfirm:
		m.mergeConfirmText = trimLastRune(m.mergeConfirmText)
	}
	return m
}

// computeMergeImpact builds the impact table from the snapshot.
func computeMergeImpact(snapshot Snapshot, sources []string) []ProjectMergeImpact {
	impact := make([]ProjectMergeImpact, 0, len(sources))
	for _, src := range sources {
		var proj hiveclient.Project
		for _, p := range snapshot.Projects {
			if p.Name == src {
				proj = p
				break
			}
		}
		synced := proj.ActiveMemoryCount - proj.UnsyncedCount
		if synced < 0 {
			synced = 0
		}
		impact = append(impact, ProjectMergeImpact{
			Source:          src,
			Memories:        proj.ActiveMemoryCount,
			Sessions:        proj.SessionCount,
			Prompts:         proj.PromptCount,
			Unsynced:        proj.UnsyncedCount,
			HasSyncEvidence: synced > 0,
		})
	}
	return impact
}

func anyImpactHasSyncEvidence(impacts []ProjectMergeImpact) bool {
	for _, imp := range impacts {
		if imp.HasSyncEvidence {
			return true
		}
	}
	return false
}

func targetHasSyncEvidence(snapshot Snapshot, target string) bool {
	for _, p := range snapshot.Projects {
		if p.Name == target {
			synced := p.ActiveMemoryCount - p.UnsyncedCount
			return synced > 0
		}
	}
	return false
}

// mergeBatchConfirmationPhrase returns the exact phrase the user must type.
func mergeBatchConfirmationPhrase(sources []string, target string) string {
	return "MERGE projects INTO " + target
}

// batchProjectMergeView renders the multi-source merge screen.
func (m Model) batchProjectMergeView() string {
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("batch project merge")
	var sb strings.Builder
	sb.WriteString(headerRow(crumb, badgeDestructive.Render("destructive"), w))
	sb.WriteString("\n")

	switch m.mergeStep {
	case mergeStepSelectSources:
		m.renderSelectSourcesPanel(&sb, panelW)
	case mergeStepPickTarget:
		m.renderPickTargetPanel(&sb, panelW)
	case mergeStepImpact:
		m.renderImpactPanel(&sb, panelW)
	case mergeStepBackupID:
		m.renderBatchBackupIDPanel(&sb, panelW)
	case mergeStepConfirm:
		m.renderBatchConfirmPanel(&sb, panelW)
	case mergeStepExecuting:
		sb.WriteString(borderedPanel(sectionHeader("STATUS", panelW)+guardPending.Render("Batch merge is running through hive-daemon. Please wait."), panelW))
	case mergeStepResult:
		m.renderBatchResultPanel(&sb, panelW)
	}

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	if !m.mergeBatchSubmitting && m.mergeStep != mergeStepResult {
		sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"ctrl-c", "quit"}}, "destructive", w))
	}
	if m.mergeStep == mergeStepResult {
		sb.WriteString(helpBar([]KeyHint{{"enter/esc", "done"}}, "destructive", w))
	}
	return sb.String()
}

func (m Model) renderSelectSourcesPanel(sb *strings.Builder, panelW int) {
	var content strings.Builder
	content.WriteString("Select source projects to merge. Space to toggle, enter to confirm.\n\n")
	for i, project := range m.snapshot.Projects {
		cursor := "  "
		if i == m.projectIndex {
			cursor = cursorStyle.Render("▌") + " "
		}
		selected := "[ ] "
		if containsString(m.mergeSelectedSources, project.Name) {
			selected = "[x] "
		}
		content.WriteString(cursor + selected + project.Name + "\n")
	}
	sb.WriteString(borderedPanel(sectionHeader("SELECT SOURCES", panelW)+content.String(), panelW))
}

func (m Model) renderPickTargetPanel(sb *strings.Builder, panelW int) {
	selected := strings.Join(m.mergeSelectedSources, ", ")
	content := fmt.Sprintf("Sources selected: %s\n\nTarget project name: %s\n", selected, visibleInput(m.mergeTarget))
	sb.WriteString(borderedPanel(sectionHeader("PICK TARGET", panelW)+content, panelW))
}

func (m Model) renderImpactPanel(sb *strings.Builder, panelW int) {
	var content strings.Builder
	content.WriteString(fmt.Sprintf("Target: %s", m.mergeTarget))
	if m.mergeTargetIsNew {
		content.WriteString(" (new project — will be created)")
	}
	content.WriteString("\n\n")
	content.WriteString(columnHeaderStyle.Render("SOURCE  MEMORIES  SESSIONS  PROMPTS  UNSYNCED") + "\n")
	for _, imp := range m.mergeImpact {
		content.WriteString(fmt.Sprintf("%s  %d  %d  %d  %d\n",
			imp.Source, imp.Memories, imp.Sessions, imp.Prompts, imp.Unsynced))
	}
	sb.WriteString(borderedPanel(sectionHeader("IMPACT", panelW)+content.String(), panelW))

	if m.mergeSyncEvidence {
		sb.WriteString("\n")
		guardContent := "One or more source projects contain synced data.\n" +
			"Before proceeding, notify your admin to handle cloud-side cleanup.\n\n" +
			dimTextStyle.Render("admin note: The following projects were merged locally and their cloud entries must be reconciled: ") +
			strings.Join(m.mergeSelectedSources, ", ") + " → " + m.mergeTarget
		sb.WriteString(borderedPanel(sectionHeader("CLOUD SYNC NOTICE", panelW)+guardContent, panelW))
	}
}

func (m Model) renderBatchBackupIDPanel(sb *strings.Builder, panelW int) {
	content := fmt.Sprintf("Sources: %s → %s\n\nBackup ID required: %s\n",
		strings.Join(m.mergeSelectedSources, ", "),
		m.mergeTarget,
		visibleInput(m.mergeBackupID),
	)
	sb.WriteString(borderedPanel(sectionHeader("BACKUP ID", panelW)+content, panelW))
}

func (m Model) renderBatchConfirmPanel(sb *strings.Builder, panelW int) {
	phrase := mergeBatchConfirmationPhrase(m.mergeSelectedSources, m.mergeTarget)
	content := fmt.Sprintf("Type exactly to confirm: %s\n\nconfirmation: %s\n",
		phrase,
		visibleInput(m.mergeConfirmText),
	)
	sb.WriteString(borderedPanel(sectionHeader("CONFIRM", panelW)+content, panelW))
}

func (m Model) renderBatchResultPanel(sb *strings.Builder, panelW int) {
	var content strings.Builder
	if m.mergeBatchResult == nil {
		content.WriteString("No result available.\n")
	} else {
		content.WriteString(fmt.Sprintf("Target: %s  Backup: %s\n\n", m.mergeBatchResult.Target, m.mergeBatchResult.BackupID))
		content.WriteString(columnHeaderStyle.Render("SOURCE  TARGET  MUTATED  STATUS") + "\n")
		for _, r := range m.mergeBatchResult.Results {
			status := "ok"
			if r.ErrMsg != "" {
				status = "error: " + r.ErrMsg
			} else if r.AlreadyMerged {
				status = "already merged"
			}
			content.WriteString(fmt.Sprintf("%s  %s  %v  %s\n", r.Source, r.Target, r.Mutated, status))
		}
		if m.mergeBatchResult.HasSyncEvidence && strings.TrimSpace(m.mergeBatchResult.CloudHandoffNote) != "" {
			content.WriteString("\n" + dimTextStyle.Render("Cloud handoff: "+m.mergeBatchResult.CloudHandoffNote) + "\n")
		}
	}
	sb.WriteString(borderedPanel(sectionHeader("RESULT", panelW)+content.String(), panelW))
}

// containsString checks if a slice contains the given string.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func (m Model) snapshotHasProject(name string) bool {
	for _, project := range m.snapshot.Projects {
		if project.Name == name {
			return true
		}
	}
	return false
}

func (m Model) snapshotHasBackup(id string) bool {
	for _, backup := range m.snapshot.Backups {
		if backup.ID == id {
			return true
		}
	}
	return false
}

func visibleInput(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func (m Model) timelineView() string {
	memories := m.projectMemories()
	project := m.selectedProject().Name
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbStyle.Render("timeline / ") + breadcrumbCurrent.Render(project)
	countBadge := dimTextStyle.Render(fmt.Sprintf("%d entries", len(memories)))

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, countBadge, w))
	sb.WriteString("\n")

	var timelineContent strings.Builder
	lastDay := ""
	for i, memory := range memories {
		day := timelineDateText(memory.CreatedAt)
		if day != lastDay {
			timelineContent.WriteString(separatorStyle.Render(fmt.Sprintf("┄ %s", day)) + "\n")
			lastDay = day
		}
		cursor := "  "
		if i == m.memoryIndex {
			cursor = cursorStyle.Render("▌") + " "
		}
		deleted := ""
		if memory.Deleted {
			deleted = " [deleted]"
		}
		rowText := dimTextStyle.Render(timelineTimeText(memory.CreatedAt)) + "  " + dimTextStyle.Render(emptyDash(memory.Category)) + "  " + memory.Title + deleted
		var row string
		if i == m.memoryIndex {
			row = selectedRow(rowText, panelW-4)
		} else {
			row = rowText
		}
		timelineContent.WriteString(cursor + row + "\n")
	}
	sb.WriteString(borderedPanel(sectionHeader("TIMELINE", panelW)+timelineContent.String(), panelW))

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"j/k", "move"}, {"⏎", "open"}, {"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func (m Model) warningsView() string {
	w := max(m.width, 80)
	panelW := panelWidth(w)

	activeCount := activeWarnings(m.snapshot.Warnings)
	crumb := breadcrumbCurrent.Render("memory warnings")
	var countBadge string
	if activeCount > 0 {
		countBadge = lipgloss.NewStyle().Foreground(colorRed).Render(fmt.Sprintf("%d active", activeCount))
	} else {
		countBadge = dimTextStyle.Render(fmt.Sprintf("%d active", activeCount))
	}

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, countBadge, w))
	sb.WriteString("\n")

	var listContent strings.Builder
	if len(m.snapshot.Warnings) == 0 {
		listContent.WriteString(dimTextStyle.Render("No warnings are available in the current read-only snapshot.") + "\n")
	}
	for i, warning := range m.snapshot.Warnings {
		cursor := "  "
		if i == m.warningIndex {
			cursor = cursorStyle.Render("▌") + " "
		}
		severity := warningSeverityBadge(warning.Severity).Render(emptyDash(warning.Severity))
		state := warningStateBadge(warning.ResolutionState).Render(emptyDash(warning.ResolutionState))
		rowText := severity + "  " + emptyDash(warning.Source) + "  " + warning.Message + "  " + state + "  " + dimTextStyle.Render(formatDateTime(warning.CreatedAt))
		listContent.WriteString(cursor + rowText + "\n")
	}
	sb.WriteString(borderedPanel(sectionHeader("WARNINGS", panelW)+listContent.String(), panelW))

	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"j/k", "move"}, {"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func warningSeverityBadge(severity string) lipgloss.Style {
	switch strings.ToLower(severity) {
	case "critical":
		return badgeCritical
	case "warning":
		return badgeWarning
	default:
		return dimTextStyle
	}
}

func warningStateBadge(state string) lipgloss.Style {
	switch strings.ToLower(state) {
	case "resolved":
		return badgeOffline
	case "active":
		return badgeWarning
	case "":
		return dimTextStyle
	default:
		return dimTextStyle
	}
}

func (m Model) backupsView() string {
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("backup snapshots")
	pathBadge := dimTextStyle.Render("read-only")

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, pathBadge, w))
	sb.WriteString("\n")

	var tableContent strings.Builder
	tableContent.WriteString(columnHeaderStyle.Render("SNAPSHOT  WHEN  SIZE  VALID") + "\n")
	if len(m.snapshot.Backups) == 0 {
		tableContent.WriteString(dimTextStyle.Render("No backups are available in the current read-only snapshot.") + "\n")
	}
	for i, backup := range m.snapshot.Backups {
		cursor := "  "
		if i == m.backupIndex {
			cursor = cursorStyle.Render("▌") + " "
		}
		valid := backupMetadataStatus(backup)
		rowText := fmt.Sprintf("%s  %s  %s  %s",
			backup.ID,
			relativeTime(backup.CreatedAt),
			byteSize(backup.SizeBytes),
			valid,
		)
		var row string
		if i == m.backupIndex {
			row = selectedRow(rowText, panelW-4)
		} else {
			row = rowText
		}
		tableContent.WriteString(cursor + row + "\n")
	}
	tableContent.WriteString(readOnlyBanner.Render("No restore action is available in this read-only TUI slice."))
	sb.WriteString(borderedPanel(sectionHeader("BACKUPS", panelW)+tableContent.String(), panelW))

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"enter", "inspect"}, {"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func (m Model) backupDetailView() string {
	backup := m.selectedBackup()
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("backup detail")

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, modeBadge("normal"), w))
	sb.WriteString("\n")

	detailContent := fmt.Sprintf("%s %s\n%s %s\n%s\n%s\n%s\nstatus validity unknown\n%s %s\n%s",
		dimTextStyle.Render("id"), backup.ID,
		dimTextStyle.Render("created"), formatDateTime(backup.CreatedAt),
		fmt.Sprintf("archive %s", presentValue(backup.ArchivePath, "archive")),
		fmt.Sprintf("manifest %s", presentValue(backup.ManifestPath, "metadata")),
		fmt.Sprintf("checksum %s", presentValue(backup.Checksum, "checksum")),
		dimTextStyle.Render("size"), byteSize(backup.SizeBytes),
		readOnlyBanner.Render("Read-only inspection only."),
	)
	sb.WriteString(borderedPanel(sectionHeader("BACKUP DETAIL", panelW)+detailContent, panelW))

	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func (m Model) apiHealthView() string {
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("hive api health")

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, modeBadge("normal"), w))
	sb.WriteString("\n")

	if len(m.snapshot.Health) == 0 {
		emptyContent := dimTextStyle.Render("Health details are not available in the current read-only snapshot.")
		sb.WriteString(borderedPanel(sectionHeader("CONNECTIVITY", panelW)+emptyContent, panelW))
	} else {
		for _, health := range m.snapshot.Health {
			state := healthState(health)
			badge := healthStateBadge(state)

			connectContent := fmt.Sprintf("%s  %s\n%s %s\n%s %d\n%s %s",
				emptyDash(health.Project), badge.Render(state),
				dimTextStyle.Render("last error"), emptyDash(health.LastError),
				dimTextStyle.Render("consecutive failures"), health.ConsecutiveFailures,
				dimTextStyle.Render("backoff"), formatDateTime(health.BackoffUntil),
			)
			sb.WriteString(borderedPanel(sectionHeader("CONNECTIVITY", panelW)+connectContent, panelW))
			sb.WriteString("\n")

			historyContent := fmt.Sprintf("last success %s  last failure %s",
				formatDateTime(health.LastSuccessAt),
				formatDateTime(health.LastFailureAt),
			)
			sb.WriteString(borderedPanel(sectionHeader("HISTORY", panelW)+historyContent, panelW))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"w", "warnings"}, {"c", "config"}, {"esc", "back"}, {"q", "quit"}}, "normal", w))
	return sb.String()
}

func healthStateBadge(state string) lipgloss.Style {
	switch state {
	case "healthy":
		return badgeHealthy
	case "degraded":
		return badgeDegraded
	case "auth failed":
		return badgeCritical
	default:
		return badgeOffline
	}
}

func (m Model) apiConfigView() string {
	w := max(m.width, 80)
	panelW := panelWidth(w)

	crumb := breadcrumbCurrent.Render("hive api config")

	var sb strings.Builder
	sb.WriteString(headerRow(crumb, modeBadge("secrets"), w))
	sb.WriteString("\n")

	endpointContent := "Read-only snapshot\n" +
		dimTextStyle.Render("API configuration endpoint is not available from the current daemon client contract.")
	sb.WriteString(borderedPanel(sectionHeader("ENDPOINT", panelW)+endpointContent, panelW))
	sb.WriteString("\n")

	credContent := readOnlyBanner.Render("Secrets are never displayed, echoed, or inferred by this TUI.")
	sb.WriteString(borderedPanel(sectionHeader("CREDENTIALS — NEVER SHOWN OR LOGGED", panelW)+credContent, panelW))

	sb.WriteString("\n")
	sb.WriteString(helpBar([]KeyHint{{"esc", "back"}, {"q", "quit"}}, "secrets", w))
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
		{"Hive API config", "read-only configuration state", false},
		{"Hive API health", "connectivity and sync health", false},
		{"Memory warnings", "triage active warnings", false},
		{"Backup snapshots", "inspect snapshots of memory.db", false},
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
	total := 0
	for _, p := range snapshot.Projects {
		total += p.UnsyncedCount
	}
	return comma(total)
}

func lastSyncText(snapshot Snapshot) string {
	var latest time.Time
	for _, h := range snapshot.Health {
		if h.LastSuccessAt.After(latest) {
			latest = h.LastSuccessAt
		}
	}
	if latest.IsZero() {
		return "never"
	}
	return relativeTime(latest)
}

func projectWarningCount(snapshot Snapshot, projectName string) int {
	count := 0
	for _, w := range snapshot.Warnings {
		if w.Source == projectName {
			count++
		}
	}
	return count
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
		if memory.Project == project {
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

func (m Model) selectedBackup() hiveclient.Backup {
	if len(m.snapshot.Backups) == 0 {
		return hiveclient.Backup{ID: "-"}
	}
	return m.snapshot.Backups[wrapIndex(m.backupIndex, len(m.snapshot.Backups))]
}

func activeWarnings(warnings []hiveclient.Warning) int {
	active := 0
	for _, warning := range warnings {
		if strings.EqualFold(warning.ResolutionState, "active") || warning.ResolutionState == "" {
			active++
		}
	}
	return active
}

func backupMetadataStatus(backup hiveclient.Backup) string {
	if strings.TrimSpace(backup.Checksum) != "" {
		return "checksum present"
	}
	if strings.TrimSpace(backup.ArchivePath) != "" {
		return "archive present"
	}
	if strings.TrimSpace(backup.ManifestPath) != "" {
		return "metadata present"
	}
	return "validity unknown"
}

func presentValue(value, label string) string {
	if strings.TrimSpace(value) == "" {
		return label + " missing"
	}
	return label + " present (" + value + ")"
}

func healthState(health hiveclient.Health) string {
	if strings.Contains(strings.ToLower(health.LastError), "401") {
		return "auth failed"
	}
	if strings.TrimSpace(health.LastError) != "" || health.ConsecutiveFailures > 0 {
		return "degraded"
	}
	return "healthy"
}

func byteSize(bytes int64) string {
	if bytes <= 0 {
		return "n/a"
	}
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KB", "MB", "GB"}
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB", value/unit)
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

func runeKey(msg tea.KeyMsg, r rune) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}
