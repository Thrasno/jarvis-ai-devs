package hiveui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

func TestPrimaryListViewsKeepSelectionsVisibleAcrossNavigation(t *testing.T) {
	projects := make([]hiveclient.Project, 10)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
	}
	m := Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: projects}, screen: ScreenProjects}
	m = sizedModel(t, m, 6)

	for range 7 {
		m = sendRune(m, 'j')
	}
	if m.projectIndex != 7 || m.viewport.offset == 0 {
		t.Fatalf("after line navigation index/offset = %d/%d, want selected row with a positive physical offset", m.projectIndex, m.viewport.offset)
	}
	assertContains(t, m.View(), "project-07", "more")
	assertNotContains(t, m.View(), "project-00")

	m = sendRune(m, 'j')
	m = sendRune(m, 'j')
	m = sendRune(m, 'j')
	if m.projectIndex != 0 {
		t.Fatalf("after wraparound index = %d, want 0", m.projectIndex)
	}
	assertContains(t, m.View(), "project-00")

	m = sendKey(m, tea.KeyPgDown)
	if m.projectIndex == 0 {
		t.Fatal("page down did not advance the project selection")
	}
	m = sendKey(m, tea.KeyEnd)
	if m.projectIndex != 9 {
		t.Fatalf("after end index = %d, want 9", m.projectIndex)
	}
	assertContains(t, m.View(), "project-09")
	m = sendKey(m, tea.KeyHome)
	if m.projectIndex != 0 {
		t.Fatalf("after home index = %d, want 0", m.projectIndex)
	}
	assertContains(t, m.View(), "project-00")
}

func TestPrimaryMemoryCollectionsKeepActiveAndDeletedRowsVisible(t *testing.T) {
	memories := make([]hiveclient.Memory, 8)
	deleted := make([]hiveclient.Memory, 8)
	for i := range memories {
		memories[i] = hiveclient.Memory{Project: "alpha", Category: "note", Title: fmt.Sprintf("active-%02d", i)}
		deleted[i] = hiveclient.Memory{Project: "alpha", Category: "note", Title: fmt.Sprintf("deleted-%02d", i), Deleted: true}
	}
	m := Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "alpha"}}, Memories: memories, DeletedMemories: deleted}, screen: ScreenProjectMemories}
	m = sizedModel(t, m, 5)
	m = sendKey(m, tea.KeyEnd)
	assertContains(t, m.View(), "active-07", "more")
	assertNotContains(t, m.View(), "active-00")

	m = sendRune(m, 'x')
	m = sendKey(m, tea.KeyEnd)
	assertContains(t, m.View(), "deleted-07", "more")
	assertNotContains(t, m.View(), "deleted-00")
}

func TestPrimaryTimelineWarningsAndBackupsKeepRowsVisible(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	memories := make([]hiveclient.Memory, 7)
	warnings := make([]hiveclient.Warning, 7)
	backups := make([]hiveclient.Backup, 7)
	for i := range memories {
		memories[i] = hiveclient.Memory{Project: "alpha", Category: "decision", Title: fmt.Sprintf("timeline-%02d", i), CreatedAt: created.Add(time.Duration(i) * time.Hour)}
		warnings[i] = hiveclient.Warning{Source: fmt.Sprintf("warning-%02d", i), Message: "visible warning", Severity: "warning"}
		backups[i] = hiveclient.Backup{ID: fmt.Sprintf("backup-%02d", i)}
	}
	snapshot := Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "alpha"}}, TimelineMemories: memories, Warnings: warnings, Backups: backups}
	for _, screen := range []Screen{ScreenTimeline, ScreenWarnings, ScreenBackups} {
		t.Run(fmt.Sprintf("screen-%d", screen), func(t *testing.T) {
			m := sizedModel(t, Model{snapshot: snapshot, screen: screen}, 5)
			m = sendKey(m, tea.KeyEnd)
			view := m.View()
			assertContains(t, view, "more")
			switch screen {
			case ScreenTimeline:
				assertContains(t, view, "timeline-06")
				assertNotContains(t, view, "timeline-00")
			case ScreenWarnings:
				assertContains(t, view, "warning-06")
				assertNotContains(t, view, "warning-00")
			case ScreenBackups:
				assertContains(t, view, "backup-06")
				assertNotContains(t, view, "backup-00")
			}
		})
	}
}

func TestPrimaryMultilineViewsScrollAndPreserveOffsetOnResize(t *testing.T) {
	contentLines := make([]string, 16)
	for i := range contentLines {
		contentLines[i] = fmt.Sprintf("detail-line-%02d", i)
	}
	m := Model{
		snapshot:      Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "alpha"}}, Memories: []hiveclient.Memory{{Project: "alpha", Title: "detail", SyncID: "detail-1"}}},
		screen:        ScreenMemoryDetail,
		memoryContent: strings.Join(contentLines, "\n"),
		memoryLoader:  &fakeMemoryLoader{},
	}
	m = sizedModel(t, m, 6)
	m = sendKey(m, tea.KeyEnd)
	if lines := renderedLineCount(m.View()); lines > 6 {
		t.Fatalf("memory detail rendered %d lines, want at most 6\n%s", lines, m.View())
	}
	assertContains(t, m.View(), "detail-line-15", "↑", "of")
	assertNotContains(t, m.View(), "detail-line-00")
	before := m.viewport.offset
	m = sizedModel(t, m, 5)
	if m.viewport.offset <= 0 || m.viewport.offset > before {
		t.Fatalf("detail offset after resize = %d, want a clamped positive offset no greater than %d", m.viewport.offset, before)
	}
	assertContains(t, m.View(), "detail-line-15")

	health := make([]hiveclient.Health, 4)
	for i := range health {
		health[i] = hiveclient.Health{Project: fmt.Sprintf("health-%02d", i), LastError: "unavailable"}
	}
	m = sizedModel(t, Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Health: health}, screen: ScreenAPIHealth}, 6)
	m = sendKey(m, tea.KeyEnd)
	if lines := renderedLineCount(m.View()); lines > 6 {
		t.Fatalf("API health rendered %d lines, want at most 6\n%s", lines, m.View())
	}
	assertContains(t, m.View(), "last success", "more")
	m = sendKey(m, tea.KeyHome)
	m = sendKey(m, tea.KeyPgDown)
	for range 4 {
		if strings.Contains(m.View(), "health-00") {
			break
		}
		m = sendRune(m, 'j')
	}
	assertContains(t, m.View(), "health-00", "↓", "of")
}

func TestPrimaryFramesFitKnownTerminalHeight(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	projects := make([]hiveclient.Project, 12)
	memories := make([]hiveclient.Memory, 12)
	warnings := make([]hiveclient.Warning, 12)
	backups := make([]hiveclient.Backup, 12)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
		memories[i] = hiveclient.Memory{Project: "project-00", Category: "decision", Title: fmt.Sprintf("timeline-%02d", i), CreatedAt: created.Add(time.Duration(i) * time.Hour)}
		warnings[i] = hiveclient.Warning{Source: fmt.Sprintf("warning-%02d", i), Message: strings.Repeat("wrapped warning text ", 8), Severity: "warning"}
		backups[i] = hiveclient.Backup{ID: fmt.Sprintf("backup-%02d", i)}
	}
	snapshot := Snapshot{DashboardState: DashboardHealthy, Projects: projects, Memories: memories, TimelineMemories: memories, Warnings: warnings, Backups: backups}
	for _, screen := range []Screen{ScreenProjects, ScreenProjectMemories, ScreenTimeline, ScreenWarnings, ScreenBackups} {
		t.Run(fmt.Sprintf("screen-%d", screen), func(t *testing.T) {
			m := sizedModel(t, Model{snapshot: snapshot, screen: screen}, 9)
			m = sendKey(m, tea.KeyEnd)
			view := m.View()
			if lines := renderedLineCount(view); lines > 9 {
				t.Fatalf("rendered lines = %d, want at most terminal height 9\n%s", lines, view)
			}
			assertContains(t, view, "q quit", "more")
		})
	}
}

func TestPrimaryViewportRetainsSelectedRowAcrossShrinkAndShortcutReentry(t *testing.T) {
	projects := make([]hiveclient.Project, 12)
	warnings := make([]hiveclient.Warning, 12)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
		warnings[i] = hiveclient.Warning{Source: fmt.Sprintf("warning-%02d", i), Message: "warning", Severity: "warning"}
	}
	snapshot := Snapshot{DashboardState: DashboardHealthy, Projects: projects, Warnings: warnings}
	m := sizedModel(t, Model{snapshot: snapshot, screen: ScreenProjects}, 18)
	m = sendKey(m, tea.KeyEnd)
	m = sizedModel(t, m, 6)
	assertContains(t, m.View(), "project-11")

	m = sizedModel(t, Model{snapshot: snapshot, screen: ScreenWarnings, warningIndex: 11}, 6)
	m = sendRune(m, 't')
	m = sendRune(m, 'w')
	if m.warningIndex != 11 {
		t.Fatalf("warning index after shortcut re-entry = %d, want 11", m.warningIndex)
	}
	assertContains(t, m.View(), "warning-11")
}

func TestPrimaryShortcutReentryFollowsRetainedSelections(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	memories := make([]hiveclient.Memory, 12)
	warnings := make([]hiveclient.Warning, 12)
	backups := make([]hiveclient.Backup, 12)
	for i := range memories {
		memories[i] = hiveclient.Memory{Project: "alpha", Category: "decision", Title: fmt.Sprintf("timeline-%02d", i), CreatedAt: created.AddDate(0, 0, i)}
		warnings[i] = hiveclient.Warning{Source: fmt.Sprintf("warning-%02d", i), Message: "warning", Severity: "warning"}
		backups[i] = hiveclient.Backup{ID: fmt.Sprintf("backup-%02d", i)}
	}
	snapshot := Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "alpha"}}, TimelineMemories: memories, Warnings: warnings, Backups: backups}

	m := sizedModel(t, Model{snapshot: snapshot, screen: ScreenTimeline, memoryIndex: 11}, 7)
	m = sendRune(m, 'w')
	m = sendRune(m, 't')
	view := m.View()
	assertContains(t, view, "timeline-11", "┄")
	if lines := renderedLineCount(view); lines > 7 {
		t.Fatalf("timeline with separators rendered %d lines, want at most 7\n%s", lines, view)
	}

	m = sizedModel(t, Model{snapshot: snapshot, screen: ScreenBackups, backupIndex: 11}, 7)
	m = sendRune(m, 'w')
	m = sendRune(m, 'b')
	assertContains(t, m.View(), "backup-11")
}

func TestDashboardEntriesFollowRetainedPrimarySelections(t *testing.T) {
	created := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	projects := make([]hiveclient.Project, 12)
	memories := make([]hiveclient.Memory, 12)
	warnings := make([]hiveclient.Warning, 12)
	backups := make([]hiveclient.Backup, 12)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
		memories[i] = hiveclient.Memory{Project: "project-00", Title: fmt.Sprintf("timeline-%02d", i), CreatedAt: created}
		warnings[i] = hiveclient.Warning{Source: fmt.Sprintf("warning-%02d", i), Message: "warning"}
		backups[i] = hiveclient.Backup{ID: fmt.Sprintf("backup-%02d", i)}
	}
	snapshot := Snapshot{DashboardState: DashboardHealthy, Projects: projects, TimelineMemories: memories, Warnings: warnings, Backups: backups}
	for _, tt := range []struct {
		action string
		want   Screen
		row    string
	}{
		{action: "Project viewer", want: ScreenProjects, row: "project-11"},
		{action: "Project timeline", want: ScreenTimeline, row: "timeline-11"},
		{action: "Memory warnings", want: ScreenWarnings, row: "warning-11"},
		{action: "Backup snapshots", want: ScreenBackups, row: "backup-11"},
	} {
		t.Run(tt.action, func(t *testing.T) {
			m := sizedModel(t, Model{snapshot: snapshot, screen: ScreenDashboard, projectIndex: 11, memoryIndex: 11, warningIndex: 11, backupIndex: 11}, 7)
			m.cursor = dashboardActionIndex(t, tt.action)
			m = sendKey(m, tea.KeyEnter)
			if m.Screen() != tt.want {
				t.Fatalf("screen = %v, want %v", m.Screen(), tt.want)
			}
			assertContains(t, m.View(), tt.row)
		})
	}
}

func TestPrimarySelectionMetadataIgnoresCursorGlyphInUserContent(t *testing.T) {
	projects := make([]hiveclient.Project, 12)
	memories := make([]hiveclient.Memory, 12)
	warnings := make([]hiveclient.Warning, 12)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
		memories[i] = hiveclient.Memory{Project: "alpha", Title: fmt.Sprintf("memory-%02d", i)}
		warnings[i] = hiveclient.Warning{Source: fmt.Sprintf("warning-%02d", i), Message: "warning"}
	}
	projects[0].Name = "▌ user project"
	memories[0].Title = "▌ user memory"
	warnings[0].Message = "▌ user warning"
	snapshot := Snapshot{DashboardState: DashboardHealthy, Projects: append([]hiveclient.Project{{Name: "alpha"}}, projects...), Memories: memories, Warnings: warnings}
	for _, tt := range []struct {
		screen Screen
		row    string
		model  Model
	}{
		{screen: ScreenProjects, row: "project-11", model: Model{snapshot: snapshot, screen: ScreenProjects, projectIndex: 12}},
		{screen: ScreenProjectMemories, row: "memory-11", model: Model{snapshot: snapshot, screen: ScreenProjectMemories, projectIndex: 0, memoryIndex: 11}},
		{screen: ScreenWarnings, row: "warning-11", model: Model{snapshot: snapshot, screen: ScreenWarnings, warningIndex: 11}},
	} {
		t.Run(fmt.Sprintf("screen-%d", tt.screen), func(t *testing.T) {
			m := sizedModel(t, tt.model, 6)
			assertContains(t, m.View(), tt.row)
		})
	}
}

func TestPrimaryFramesFitActualNarrowTerminalWidth(t *testing.T) {
	projects := make([]hiveclient.Project, 12)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
	}

	m := sizedModelAt(t, Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: projects}, screen: ScreenProjects}, 40, 9)
	m = sendKey(m, tea.KeyEnd)
	assertPhysicalFrameFits(t, m.View(), 40, 9)
	assertContains(t, m.View(), "project-11", "more")
}

func TestPrimaryViewportUsesDeterministicNarrowChromeFallback(t *testing.T) {
	m := sizedModelAt(t, Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "alpha"}}}, screen: ScreenProjects}, 8, 2)
	if view := m.View(); view != "!" {
		t.Fatalf("narrow terminal fallback = %q, want !", view)
	}
}

func TestPrimaryFramesFitPhysicalDisplayBoundsAtWidth100(t *testing.T) {
	health := make([]hiveclient.Health, 6)
	projects := make([]hiveclient.Project, 12)
	for i := range health {
		health[i] = hiveclient.Health{Project: fmt.Sprintf("health-%02d", i), LastError: strings.Repeat("wrapped error ", 8)}
	}
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
	}
	for _, m := range []Model{
		{snapshot: Snapshot{DashboardState: DashboardHealthy, Health: health}, screen: ScreenAPIHealth},
		{snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: projects}, screen: ScreenProjects},
	} {
		m = sizedModel(t, m, 9)
		m = sendKey(m, tea.KeyEnd)
		assertPhysicalFrameFits(t, m.View(), 100, 9)
	}
}

func TestPrimaryViewportUsesTinyTerminalFallback(t *testing.T) {
	m := sizedModel(t, Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "alpha"}}}, screen: ScreenProjects}, 1)
	view := m.View()
	if lines := renderedLineCount(view); lines > 1 {
		t.Fatalf("tiny terminal rendered %d lines, want at most 1\n%s", lines, view)
	}
	assertContains(t, view, "Terminal too small")
}

func TestPrimaryViewportKeepsEmptyStatesAndFixedChrome(t *testing.T) {
	m := sizedModel(t, Model{snapshot: Snapshot{DashboardState: DashboardHealthy}, screen: ScreenProjects}, 5)
	view := m.View()
	assertContains(t, view, "projects", "0 projects", "q quit")
	assertNotContains(t, view, "0 of 0", "▌")
}

func assertPhysicalFrameFits(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := terminalui.PhysicalLines(view, width)
	if len(lines) > height {
		t.Fatalf("physical lines = %d, want at most height %d\n%s", len(lines), height, view)
	}
	for _, line := range lines {
		if line != "" && lipgloss.Width(line) > width {
			t.Fatalf("line width = %d, want at most %d: %q", lipgloss.Width(line), width, line)
		}
	}
}

func renderedLineCount(view string) int {
	if view == "" {
		return 0
	}
	return len(strings.Split(view, "\n"))
}

func sizedModel(t *testing.T, m Model, height int) Model {
	t.Helper()
	return sizedModelAt(t, m, 100, height)
}

func sizedModelAt(t *testing.T, m Model, width, height int) Model {
	t.Helper()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	return got
}
