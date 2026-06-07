package hiveui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestDashboardStatesRenderReferenceCatalogStates(t *testing.T) {
	tests := []struct {
		name     string
		snapshot Snapshot
		want     []string
	}{
		{
			name: "healthy",
			snapshot: Snapshot{
				DashboardState: DashboardHealthy,
				Projects:       []hiveclient.Project{{Name: "core-api", ActiveMemoryCount: 3481}},
			},
			want: []string{"dashboard", "daemon running", "api healthy", "projects 1", "memories 3,481", "warnings 0"},
		},
		{
			name: "degraded cloud auth failed",
			snapshot: Snapshot{
				DashboardState: DashboardDegraded,
				Projects:       []hiveclient.Project{{Name: "core-api", ActiveMemoryCount: 3481}},
				Warnings:       []hiveclient.Warning{{Message: "37 memories waiting to sync"}, {Message: "Hive API rejected credentials"}},
			},
			want: []string{"dashboard · degraded", "Cloud sync is paused", "api auth failed", "unsynced n/a", "warnings 2"},
		},
		{
			name:     "local only",
			snapshot: Snapshot{DashboardState: DashboardLocalOnly, Projects: []hiveclient.Project{{Name: "local", ActiveMemoryCount: 612}}},
			want:     []string{"dashboard · local-only", "Running local-only", "api not configured", "sync disabled", "unsynced n/a"},
		},
		{
			name:     "daemon unavailable",
			snapshot: Snapshot{DashboardState: DashboardDaemonUnavailable, DaemonURL: "http://127.0.0.1:7438", LoadError: assertErr("connection refused")},
			want:     []string{"dashboard · offline", "Cannot reach hive-daemon", "No response from http://127.0.0.1:7438", "projects —", "q quit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, NewModelWithSnapshot(tt.snapshot).View(), tt.want...)
		})
	}
}

func TestDashboardShowsReadOnlyActionsAndBlocksDestructiveEntries(t *testing.T) {
	m := NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy})
	assertContains(t, m.View(), "Project viewer", "Memory warnings", "Backup snapshots", "Merge projects (disabled)", "Delete memories (disabled)")

	m = sendKey(m, tea.KeyDown)
	m = sendKey(m, tea.KeyDown)
	m = sendKey(m, tea.KeyEnter)

	if m.Screen() != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard", m.Screen())
	}
	assertContains(t, m.View(), "Merge projects is disabled in this read-only TUI slice", "No local Hive state was changed")
}

func TestDashboardHelpOnlyAdvertisesImplementedKeys(t *testing.T) {
	view := NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy}).View()
	assertContains(t, view, "j/k move", "enter open", "w warnings", "g health", "c config", "b backups", "q quit")
	assertNotContains(t, view, "r retry", "restore")
}

func TestAuxiliaryShortcutsOpenReadOnlyStates(t *testing.T) {
	tests := []struct {
		name string
		key  rune
		want Screen
		text []string
	}{
		{name: "warnings", key: 'w', want: ScreenWarnings, text: []string{"memory warnings", "CONFIG-401", "critical", "Hive API rejected credentials", "active", "esc back"}},
		{name: "backups", key: 'b', want: ScreenBackups, text: []string{"backup snapshots", "hive-20260606-143601", "4.1 MB", "checksum present", "enter inspect", "No restore action is available"}},
		{name: "api health", key: 'g', want: ScreenAPIHealth, text: []string{"hive api health", "core-api", "auth failed", "401 unauthorized", "consecutive failures 12", "backoff"}},
		{name: "api config", key: 'c', want: ScreenAPIConfig, text: []string{"hive api config", "Read-only snapshot", "API configuration endpoint is not available", "Secrets are never displayed"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sendRune(NewModelWithSnapshot(sampleNavigationSnapshot()), tt.key)
			if m.Screen() != tt.want {
				t.Fatalf("screen = %v, want %v", m.Screen(), tt.want)
			}
			assertContains(t, m.View(), tt.text...)
			assertNotContains(t, m.View(), "dismiss", "resolve", "save", "toggle", "R restore", "create backup")
		})
	}
}

func TestWarningsAndBackupsRenderHonestEmptyStates(t *testing.T) {
	m := sendRune(NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy}), 'w')
	assertContains(t, m.View(), "memory warnings", "No warnings are available in the current read-only snapshot")

	m = sendRune(NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy}), 'b')
	assertContains(t, m.View(), "backup snapshots", "No backups are available in the current read-only snapshot")
}

func TestBackupInspectIsReadOnlyAndDoesNotAdvertiseRestore(t *testing.T) {
	m := sendRune(NewModelWithSnapshot(sampleNavigationSnapshot()), 'b')
	m = sendKey(m, tea.KeyEnter)

	if m.Screen() != ScreenBackupDetail {
		t.Fatalf("screen = %v, want backup detail", m.Screen())
	}
	assertContains(t, m.View(), "backup detail", "hive-20260606-143601", "archive present (/tmp/hive-20260606-143601.db)", "checksum present (sha256:abc)", "status validity unknown", "Read-only inspection only")
	assertNotContains(t, m.View(), "valid\n", "restore", "delete", "create backup")
}

func TestBackupDetailEscReturnsToBackups(t *testing.T) {
	m := sendRune(NewModelWithSnapshot(sampleNavigationSnapshot()), 'b')
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEsc)

	if m.Screen() != ScreenBackups {
		t.Fatalf("screen = %v, want backups", m.Screen())
	}
	assertContains(t, m.View(), "backup snapshots", "hive-20260606-143601")
}

func TestQuitKeysReturnTeaQuit(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{name: "q", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
		{name: "ctrl-c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cmd := NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy}).Update(tt.msg)
			if cmd == nil {
				t.Fatal("cmd is nil, want tea.Quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("cmd() = %T, want tea.QuitMsg", cmd())
			}
		})
	}
}

func TestBackReturnsToThePreviousReadOnlyState(t *testing.T) {
	m := NewModelWithSnapshot(sampleNavigationSnapshot())
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)

	m = sendKey(m, tea.KeyEsc)
	if m.Screen() != ScreenProjectMemories {
		t.Fatalf("screen = %v, want project memories", m.Screen())
	}
	m = sendKey(m, tea.KeyEsc)
	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want projects", m.Screen())
	}
	m = sendKey(m, tea.KeyEsc)
	if m.Screen() != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard", m.Screen())
	}
}

func TestReadOnlyNavigationOpensProjectsMemoriesDetailAndTimeline(t *testing.T) {
	m := NewModelWithSnapshot(sampleNavigationSnapshot())

	m = sendKey(m, tea.KeyEnter)
	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want projects", m.Screen())
	}
	assertContains(t, m.View(), "dashboard / projects", "core-api", "web-client", "3481", "n/a", "2m ago")
	assertNotContains(t, m.View(), "merge", "archive", "delete")

	m = sendKey(m, tea.KeyEnter)
	if m.Screen() != ScreenProjectMemories {
		t.Fatalf("screen = %v, want project memories", m.Screen())
	}
	assertContains(t, m.View(), "dashboard / projects / core-api", "Use exponential backoff", "decision", "2d")
	assertNotContains(t, m.View(), "delete")

	m = sendKey(m, tea.KeyEnter)
	if m.Screen() != ScreenMemoryDetail {
		t.Fatalf("screen = %v, want memory detail", m.Screen())
	}
	assertContains(t, m.View(), "core-api / mem_8f3a91c0", "sync synced", "Content preview is not available from the read-only daemon snapshot")
	assertNotContains(t, m.View(), "copy body", "delete")

	m = sendKey(m, tea.KeyEsc)
	m = sendKey(m, tea.KeyEsc)
	m = sendRune(m, 't')
	if m.Screen() != ScreenTimeline {
		t.Fatalf("screen = %v, want timeline", m.Screen())
	}
	assertContains(t, m.View(), "timeline / core-api", "2d ago", "Use exponential backoff")
}

func TestDestructiveEntriesAreDisabledAndDoNotMutate(t *testing.T) {
	wantDisabled := map[string]bool{
		"Merge projects":  true,
		"Delete projects": true,
		"Delete memories": true,
	}

	for index, action := range dashboardActions() {
		if !wantDisabled[action.label] {
			continue
		}
		t.Run(action.label, func(t *testing.T) {
			if !action.disabled {
				t.Fatalf("%s disabled = false, want true", action.label)
			}

			m := NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy})
			m.cursor = index
			m = sendKey(m, tea.KeyEnter)

			if m.Screen() != ScreenDashboard {
				t.Fatalf("screen = %v, want dashboard", m.Screen())
			}
			if m.cursor != index {
				t.Fatalf("cursor = %d, want %d", m.cursor, index)
			}
			assertContains(t, m.View(), action.label+" is disabled in this read-only TUI slice", "No local Hive state was changed")
		})
	}
}

func TestTimelineDetailBackReturnsToTimeline(t *testing.T) {
	m := NewModelWithSnapshot(sampleNavigationSnapshot())
	m = sendRune(m, 't')
	m = sendKey(m, tea.KeyEnter)

	if m.Screen() != ScreenMemoryDetail {
		t.Fatalf("screen = %v, want memory detail", m.Screen())
	}

	m = sendKey(m, tea.KeyEsc)
	if m.Screen() != ScreenTimeline {
		t.Fatalf("screen = %v, want timeline", m.Screen())
	}
}

func TestEnterOnEmptyListsDoesNotOpenFakeDetail(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		want  Screen
	}{
		{
			name:  "empty projects stays on projects",
			model: Model{snapshot: Snapshot{DashboardState: DashboardHealthy}, screen: ScreenProjects},
			want:  ScreenProjects,
		},
		{
			name: "empty memories stays on project memories",
			model: Model{
				snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "empty-project"}}},
				screen:   ScreenProjectMemories,
			},
			want: ScreenProjectMemories,
		},
		{
			name: "empty timeline stays on timeline",
			model: Model{
				snapshot: Snapshot{DashboardState: DashboardHealthy, Projects: []hiveclient.Project{{Name: "empty-project"}}},
				screen:   ScreenTimeline,
			},
			want: ScreenTimeline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sendKey(tt.model, tea.KeyEnter)
			if m.Screen() != tt.want {
				t.Fatalf("screen = %v, want %v", m.Screen(), tt.want)
			}
			assertContains(t, m.View(), "No item is available to open")
			assertNotContains(t, m.View(), "Content preview is not available from the read-only daemon snapshot")
		})
	}
}

func TestTimelineZeroCreatedAtRendersUnavailable(t *testing.T) {
	m := NewModelWithSnapshot(Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "core-api", ActiveMemoryCount: 1}},
		Memories:       []hiveclient.Memory{{SyncID: "mem_zero", Project: "core-api", Category: "decision", Title: "Missing timestamp"}},
	})
	m = sendRune(m, 't')

	view := m.View()
	assertContains(t, view, "┄ n/a", "n/a  decision  Missing timestamp")
	assertNotContains(t, view, "00:00")
}

func TestUnsyncedCountDoesNotUseWarningTextOrConsecutiveFailures(t *testing.T) {
	m := NewModelWithSnapshot(Snapshot{
		DashboardState: DashboardDegraded,
		Warnings:       []hiveclient.Warning{{Message: "37 memories waiting to sync"}},
		Health:         []hiveclient.Health{{ConsecutiveFailures: 9}},
	})

	view := m.View()
	assertContains(t, view, "unsynced n/a")
	assertNotContains(t, view, "unsynced 37", "sync 9 behind")
}

func sendKey(m Model, key tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	return updated.(Model)
}

func sendRune(m Model, r rune) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return updated.(Model)
}

func sampleNavigationSnapshot() Snapshot {
	base := time.Now().Add(-47 * time.Hour).UTC()
	return Snapshot{
		DashboardState: DashboardDegraded,
		Projects: []hiveclient.Project{
			{Name: "core-api", ActiveMemoryCount: 3481, DeletedMemoryCount: 4, LastActivityAt: time.Now().Add(-2 * time.Minute)},
			{Name: "web-client", ActiveMemoryCount: 642, LastActivityAt: base.Add(-time.Hour)},
		},
		Memories: []hiveclient.Memory{
			{SyncID: "mem_8f3a91c0", Project: "core-api", Category: "decision", Title: "Use exponential backoff", CreatedBy: "mcp", CreatedAt: base, Deleted: false},
			{SyncID: "mem_503", Project: "core-api", Category: "bugfix", Title: "Fix retry storm", CreatedBy: "mcp", CreatedAt: base.Add(-7 * time.Minute), Deleted: false},
			{SyncID: "mem_web", Project: "web-client", Category: "pattern", Title: "Use presenter boundary", CreatedAt: base.Add(-time.Hour), Deleted: false},
		},
		Health:   []hiveclient.Health{{Project: "core-api", LastSuccessAt: base.Add(-2 * time.Hour), LastFailureAt: base.Add(-6 * time.Minute), BackoffUntil: base.Add(time.Hour), ConsecutiveFailures: 12, LastError: "401 unauthorized"}},
		Warnings: []hiveclient.Warning{{Severity: "critical", Source: "CONFIG-401", Message: "Hive API rejected credentials", ResolutionState: "active", CreatedAt: base}},
		Backups:  []hiveclient.Backup{{ID: "hive-20260606-143601", CreatedAt: time.Now().Add(-2 * time.Minute), ArchivePath: "/tmp/hive-20260606-143601.db", Checksum: "sha256:abc", SizeBytes: 4_100_000}},
	}
}

func assertContains(t *testing.T, view string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(view, want) {
			t.Fatalf("view =\n%s\nmissing %q", view, want)
		}
	}
}

func assertNotContains(t *testing.T, view string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if strings.Contains(view, want) {
			t.Fatalf("view =\n%s\ncontains %q", view, want)
		}
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
