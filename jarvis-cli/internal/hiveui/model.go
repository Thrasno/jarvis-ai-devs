package hiveui

import (
	"context"
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
	ScreenWarnings
	ScreenBackups
	ScreenBackupDetail
	ScreenAPIHealth
	ScreenAPIConfig
	ScreenMemoryGuard
)

type GuardExecutor interface {
	ExecuteGuard(context.Context, hiveclient.GuardRequest) (hiveclient.GuardResult, error)
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
	guardConfirmation string
	guardStep         memoryGuardStep
	guardSubmitting   bool
	guardMemory       hiveclient.Memory
}

type memoryGuardStep int

const (
	memoryGuardBackupID memoryGuardStep = iota
	memoryGuardConfirmation
)

type memoryGuardResultMsg struct {
	operation  string
	targetType string
	targetID   int64
	backupID   string
	result     hiveclient.GuardResult
	err        error
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

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(memoryGuardResultMsg); ok {
		return m.applyMemoryGuardResult(result), nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyCtrlC || runeKey(key, 'q') {
		return m, tea.Quit
	}
	if m.screen == ScreenMemoryGuard {
		return m.updateMemoryGuard(key)
	}
	m.message = ""
	switch {
	case key.Type == tea.KeyEsc || key.Type == tea.KeyBackspace:
		m = m.back()
	case m.screen == ScreenMemoryDetail && runeKey(key, 'd'):
		m = m.startMemoryGuard("delete")
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
	sb.WriteString("j/k move  enter open  w warnings  g health  c config  b backups  q quit")
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
		if m.detailReturn == ScreenTimeline {
			m.screen = ScreenTimeline
		} else {
			m.screen = ScreenProjectMemories
		}
	case ScreenMemoryGuard:
		m.screen = ScreenMemoryDetail
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
	if m.guardExecutor != nil && memory.ID != 0 {
		sb.WriteString("d delete guarded by backup ID and exact confirmation\n")
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "%s\n", m.message)
	}
	sb.WriteString("esc back  q quit")
	return sb.String()
}

func (m Model) startMemoryGuard(operation string) Model {
	if m.guardExecutor == nil {
		return m
	}
	if m.guardSubmitting {
		m.screen = ScreenMemoryGuard
		m.message = "Guarded memory delete is already pending through hive-daemon. Wait for the result before leaving or submitting again."
		return m
	}
	memory := m.selectedMemory()
	if memory.ID == 0 {
		m.message = "Memory numeric ID is required before guarded delete can be dispatched."
		return m
	}
	m.screen = ScreenMemoryGuard
	m.guardOperation = operation
	m.guardMemory = memory
	m.guardStep = memoryGuardBackupID
	m.guardBackupID = ""
	m.guardConfirmation = ""
	m.guardSubmitting = false
	m.message = ""
	return m
}

func (m Model) updateMemoryGuard(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.guardSubmitting {
		m.message = "Guarded memory delete is already pending through hive-daemon. Wait for the result before leaving or submitting again."
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
		m.message = "Guarded memory delete is already pending through hive-daemon."
		return m, nil
	}
	if m.guardStep == memoryGuardBackupID {
		if strings.TrimSpace(m.guardBackupID) == "" {
			m.message = "Backup ID is required before guarded delete."
			return m, nil
		}
		m.guardStep = memoryGuardConfirmation
		return m, nil
	}
	expected := memoryGuardConfirmationPhrase(m.guardOperation, m.guardMemory)
	if m.guardConfirmation != expected {
		m.message = "Confirmation mismatch. Type the phrase exactly; input is not trimmed."
		return m, nil
	}
	if m.guardExecutor == nil {
		m.message = "Guarded memory delete is unavailable without a daemon command boundary."
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
	return m
}

func (m Model) appendGuardText(text string) Model {
	if m.guardStep == memoryGuardBackupID {
		m.guardBackupID += text
		return m
	}
	m.guardConfirmation += text
	return m
}

func (m Model) removeGuardRune() Model {
	if m.guardStep == memoryGuardBackupID {
		m.guardBackupID = trimLastRune(m.guardBackupID)
		return m
	}
	m.guardConfirmation = trimLastRune(m.guardConfirmation)
	return m
}

func (m Model) memoryGuardView() string {
	memory := m.guardMemory
	var sb strings.Builder
	fmt.Fprintf(&sb, "guarded memory %s\n", m.guardOperation)
	fmt.Fprintf(&sb, "target %s\n", memoryKey(memory))
	fmt.Fprintf(&sb, "Backup ID is required: %s\n", visibleInput(m.guardBackupID))
	fmt.Fprintf(&sb, "Confirmation must match exactly. Type exactly: %s\n", memoryGuardConfirmationPhrase(m.guardOperation, memory))
	if m.guardStep == memoryGuardConfirmation {
		fmt.Fprintf(&sb, "confirmation: %s\n", visibleInput(m.guardConfirmation))
	}
	if m.guardSubmitting {
		sb.WriteString("Guarded memory delete is pending through hive-daemon. Submit is disabled until the result returns.\n")
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "%s\n", m.message)
	}
	sb.WriteString("No delete will run until both fields pass guards. Dispatch uses hive-daemon only; no direct SQLite or cloud mutation.\n")
	if m.guardSubmitting {
		sb.WriteString("q quit")
	} else {
		sb.WriteString("esc back  q quit")
	}
	return sb.String()
}

func memoryGuardConfirmationPhrase(operation string, memory hiveclient.Memory) string {
	return fmt.Sprintf("%s memory %d", strings.ToUpper(operation), memory.ID)
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

func (m Model) warningsView() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "memory warnings\t%d active\n", activeWarnings(m.snapshot.Warnings))
	if len(m.snapshot.Warnings) == 0 {
		sb.WriteString("No warnings are available in the current read-only snapshot.\n")
	}
	for i, warning := range m.snapshot.Warnings {
		mark := "  "
		if i == m.warningIndex {
			mark = "▌ "
		}
		fmt.Fprintf(&sb, "%s%s  %s  %s  %s  %s\n", mark, emptyDash(warning.Severity), emptyDash(warning.Source), warning.Message, emptyDash(warning.ResolutionState), formatDateTime(warning.CreatedAt))
	}
	sb.WriteString("j/k move  esc back  q quit")
	return sb.String()
}

func (m Model) backupsView() string {
	var sb strings.Builder
	sb.WriteString("backup snapshots\n")
	if len(m.snapshot.Backups) == 0 {
		sb.WriteString("No backups are available in the current read-only snapshot.\n")
	}
	for i, backup := range m.snapshot.Backups {
		mark := "  "
		if i == m.backupIndex {
			mark = "▌ "
		}
		fmt.Fprintf(&sb, "%s%s  %s  %s  %s\n", mark, backup.ID, relativeTime(backup.CreatedAt), byteSize(backup.SizeBytes), backupMetadataStatus(backup))
	}
	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("enter inspect  esc back  q quit\nNo restore action is available in this read-only TUI slice.")
	return sb.String()
}

func (m Model) backupDetailView() string {
	backup := m.selectedBackup()
	var sb strings.Builder
	fmt.Fprintf(&sb, "backup detail\n%s\n", backup.ID)
	fmt.Fprintf(&sb, "created %s\n", formatDateTime(backup.CreatedAt))
	fmt.Fprintf(&sb, "archive %s\n", presentValue(backup.ArchivePath, "archive"))
	fmt.Fprintf(&sb, "manifest %s\n", presentValue(backup.ManifestPath, "metadata"))
	fmt.Fprintf(&sb, "checksum %s\n", presentValue(backup.Checksum, "checksum"))
	sb.WriteString("status validity unknown\n")
	fmt.Fprintf(&sb, "size %s\n", byteSize(backup.SizeBytes))
	sb.WriteString("Read-only inspection only.\nesc back  q quit")
	return sb.String()
}

func (m Model) apiHealthView() string {
	var sb strings.Builder
	sb.WriteString("hive api health\n")
	if len(m.snapshot.Health) == 0 {
		sb.WriteString("Health details are not available in the current read-only snapshot.\n")
	}
	for _, health := range m.snapshot.Health {
		fmt.Fprintf(&sb, "%s  %s\n", emptyDash(health.Project), healthState(health))
		fmt.Fprintf(&sb, "last error %s\n", emptyDash(health.LastError))
		fmt.Fprintf(&sb, "consecutive failures %d\n", health.ConsecutiveFailures)
		fmt.Fprintf(&sb, "backoff %s\n", formatDateTime(health.BackoffUntil))
		fmt.Fprintf(&sb, "last success %s  last failure %s\n", formatDateTime(health.LastSuccessAt), formatDateTime(health.LastFailureAt))
	}
	sb.WriteString("w warnings  c config  esc back  q quit")
	return sb.String()
}

func (m Model) apiConfigView() string {
	return "hive api config\nRead-only snapshot\nAPI configuration endpoint is not available from the current daemon client contract.\nSecrets are never displayed, echoed, or inferred by this TUI.\nesc back  q quit"
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runeKey(msg tea.KeyMsg, r rune) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}
