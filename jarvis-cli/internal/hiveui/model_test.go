package hiveui

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func TestNewModelWithAllExecutors_WiresAllThreeExecutors(t *testing.T) {
	guard := &fakeGuardExecutor{}
	archive := &fakeProjectArchiveExecutor{note: "test"}
	merge := &fakeProjectMergeExecutor{note: "test"}

	// Build a snapshot that has non-zero memory IDs (required for guard hotkey),
	// at least two projects (required for merge target), and a backup (required for merge guard).
	snapshot := projectMergeSnapshot() // has alpha+beta projects, backup-merge, memories with ID 7
	snapshot.Memories[0].ID = 7

	m := NewModelWithAllExecutors(snapshot, guard, archive, merge, nil, nil, nil)

	// Assert guard executor is wired: guard hotkey appears in memory detail.
	detail := openMemoryDetail(m)
	assertContains(t, detail.View(), "d delete guarded by backup ID, delete reason, and exact confirmation")

	// Assert archive executor is wired: 'a' hotkey appears in projects view.
	projects := sendKey(m, tea.KeyEnter)
	assertContains(t, projects.View(), "a archive guarded by backup ID and exact confirmation")

	// Assert merge executor is wired: 'm' hotkey appears in projects view.
	assertContains(t, projects.View(), "m merge guarded by backup ID and exact confirmation")
}

func TestNewModelWithAllExecutors_EmptyDaemonURLDefaults(t *testing.T) {
	snapshot := Snapshot{DaemonURL: ""}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, nil, nil, nil)
	if m.snapshot.DaemonURL != "http://127.0.0.1:7438" {
		t.Fatalf("DaemonURL = %q, want %q", m.snapshot.DaemonURL, "http://127.0.0.1:7438")
	}
}

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
			want: []string{"dashboard · degraded", "Cloud sync is paused", "api auth failed", "unsynced 0", "warnings 2"},
		},
		{
			name:     "local only",
			snapshot: Snapshot{DashboardState: DashboardLocalOnly, Projects: []hiveclient.Project{{Name: "local", ActiveMemoryCount: 612}}},
			want:     []string{"dashboard · local-only", "Running local-only", "api not configured", "sync disabled", "unsynced 0"},
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

func TestWarningsViewRendersPopulatedActiveAndResolvedRows(t *testing.T) {
	created := time.Date(2026, 6, 11, 13, 45, 0, 0, time.UTC)
	resolvedAt := created.Add(2 * time.Hour)
	m := Model{
		snapshot: Snapshot{
			DashboardState: DashboardHealthy,
			Warnings: []hiveclient.Warning{
				{Severity: "critical", Source: "CONFIG-401", Message: "Hive API rejected credentials", ResolutionState: "active", CreatedAt: created},
				{Severity: "warning", Source: "core-api", Message: "37 memories waiting to sync", ResolutionState: "resolved", CreatedAt: created.Add(-time.Hour), ResolvedAt: &resolvedAt},
			},
		},
		screen: ScreenWarnings,
		width:  140,
	}

	view := m.View()
	assertContains(t, view,
		"memory warnings",
		"1 active",
		"critical",
		"active",
		"CONFIG-401",
		"Hive API rejected credentials",
		formatDateTime(created),
		"warning",
		"resolved",
		"core-api",
		"37 memories waiting to sync",
		formatDateTime(created.Add(-time.Hour)),
	)
}

func TestWarningsViewUsesNeutralReadOnlySourceSemantics(t *testing.T) {
	m := Model{
		snapshot: Snapshot{
			DashboardState: DashboardHealthy,
			Warnings:       []hiveclient.Warning{{Severity: "warning", Source: "core-api", Message: "warning from source", ResolutionState: "active", CreatedAt: time.Date(2026, 6, 11, 13, 45, 0, 0, time.UTC)}},
		},
		screen: ScreenWarnings,
		width:  120,
	}

	view := m.View()
	assertContains(t, view, "source", "core-api", "warning from source")
	assertNotContains(t, view,
		"current project",
		"Current project",
		"resolve warning",
		"dismiss warning",
		"save warning",
		"edit warning",
		"deduplicate",
		"mutation",
	)
}

func TestWarningsViewUsesHiveVisualSystemForSelectedRows(t *testing.T) {
	created := time.Date(2026, 6, 11, 13, 45, 0, 0, time.UTC)
	warnings := []hiveclient.Warning{
		{Severity: "critical", Source: "CONFIG-401", Message: "Hive API rejected credentials", ResolutionState: "active", CreatedAt: created},
		{Severity: "warning", Source: "sync", Message: "37 memories waiting to sync", ResolutionState: "resolved", CreatedAt: created.Add(-time.Hour)},
	}
	m := Model{snapshot: Snapshot{DashboardState: DashboardHealthy, Warnings: warnings}, screen: ScreenWarnings, warningIndex: 1, width: 100}

	view := m.View()
	panelW := panelWidth(100)
	expectedSelected := selectedRow(warningRowText(warnings[1]), panelW-4)
	assertContains(t, view,
		"memory warnings",
		"1 active",
		"WARNINGS",
		"SEVERITY  STATE  SOURCE  MESSAGE  CREATED",
		"▌ "+expectedSelected,
		"j/k move",
		"esc back",
		"q quit",
	)
}

func TestWarningsUpdateWrapsSelectionAcrossRows(t *testing.T) {
	m := Model{
		snapshot: Snapshot{DashboardState: DashboardHealthy, Warnings: []hiveclient.Warning{
			{Severity: "critical", Source: "CONFIG-401", Message: "first", ResolutionState: "active"},
			{Severity: "warning", Source: "sync", Message: "second", ResolutionState: "resolved"},
		}},
		screen: ScreenWarnings,
	}

	m = sendRune(m, 'k')
	if m.warningIndex != 1 {
		t.Fatalf("warningIndex after k from first row = %d, want 1", m.warningIndex)
	}
	m = sendRune(m, 'j')
	if m.warningIndex != 0 {
		t.Fatalf("warningIndex after j wraps from last row = %d, want 0", m.warningIndex)
	}
	m = sendKey(m, tea.KeyDown)
	if m.warningIndex != 1 {
		t.Fatalf("warningIndex after down = %d, want 1", m.warningIndex)
	}
	m = sendKey(m, tea.KeyUp)
	if m.warningIndex != 0 {
		t.Fatalf("warningIndex after up = %d, want 0", m.warningIndex)
	}
}

func TestWarningsEnterIsReadOnlyAndLeavesSnapshotUnchanged(t *testing.T) {
	snapshot := Snapshot{DashboardState: DashboardHealthy, Warnings: []hiveclient.Warning{
		{ID: 1, Severity: "critical", Source: "CONFIG-401", Message: "Hive API rejected credentials", ResolutionState: "active", CreatedAt: time.Date(2026, 6, 11, 13, 45, 0, 0, time.UTC)},
		{ID: 2, Severity: "warning", Source: "sync", Message: "37 memories waiting to sync", ResolutionState: "resolved", CreatedAt: time.Date(2026, 6, 11, 12, 45, 0, 0, time.UTC)},
	}}
	m := Model{snapshot: snapshot, screen: ScreenWarnings, warningIndex: 1}

	updated := sendKey(m, tea.KeyEnter)
	if updated.Screen() != ScreenWarnings {
		t.Fatalf("screen after enter = %v, want ScreenWarnings", updated.Screen())
	}
	if updated.warningIndex != 1 {
		t.Fatalf("warningIndex after enter = %d, want 1", updated.warningIndex)
	}
	if !reflect.DeepEqual(updated.snapshot.Warnings, snapshot.Warnings) {
		t.Fatalf("warnings changed after enter: got %#v, want %#v", updated.snapshot.Warnings, snapshot.Warnings)
	}
	assertContains(t, updated.View(), "j/k move", "esc back", "q quit")
	assertNotContains(t, updated.View(), "resolve warning", "dismiss warning", "save warning")
}

func TestWarningsEmptyStateIsHonestAndHasNoWarningRows(t *testing.T) {
	m := Model{snapshot: Snapshot{DashboardState: DashboardHealthy}, screen: ScreenWarnings}

	view := m.View()
	assertContains(t, view, "memory warnings", "No warnings are available in the current read-only snapshot")
	assertNotContains(t, view,
		"▌",
		"resolved",
		"hidden",
		"dismissed",
		"no active warnings",
	)
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

func TestProjectArchiveCtrlCReturnsTeaQuitBeforeSubmit(t *testing.T) {
	executor := &fakeProjectArchiveExecutor{note: "No cloud project mutation was performed."}
	m := NewModelWithSnapshotAndProjectArchiveExecutor(projectArchiveSnapshot(), executor)
	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'a')

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("cmd is nil, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd() = %T, want tea.QuitMsg", cmd())
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
	assertContains(t, m.View(), "dashboard / projects", "core-api", "web-client", "3481", "2m ago")
	assertNotContains(t, m.View(), "merge", "merge guarded", "guarded project merge", "archive", "delete")
	if m = sendRune(m, 'm'); m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want projects", m.Screen())
	}

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

func TestDeletedMemoriesAreVisuallyDistinguishedInListsAndDetail(t *testing.T) {
	m := NewModelWithSnapshot(deletedGuardedMemorySnapshot())
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)

	assertContains(t, m.View(), "Use exponential backoff [deleted]")

	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "status deleted")

	m = sendKey(m, tea.KeyEsc)
	m = sendKey(m, tea.KeyEsc)
	m = sendRune(m, 't')
	assertContains(t, m.View(), "Use exponential backoff [deleted]")
}

func TestMemoryDetailAdvertisesCorrectGuardActionForMemoryStatus(t *testing.T) {
	tests := []struct {
		name       string
		snapshot   Snapshot
		wantAction rune
		wantText   string
		guardText  string
		blockKey   rune
		blockText  string
	}{
		{
			name:       "active memory advertises delete only",
			snapshot:   guardedMemorySnapshot(),
			wantAction: 'd',
			wantText:   "d delete guarded by backup ID, delete reason, and exact confirmation",
			guardText:  "guarded memory delete",
			blockKey:   'r',
			blockText:  "r restore",
		},
		{
			name:       "deleted memory advertises restore only",
			snapshot:   deletedGuardedMemorySnapshot(),
			wantAction: 'r',
			wantText:   "r restore guarded by backup ID and exact confirmation",
			guardText:  "guarded memory restore",
			blockKey:   'd',
			blockText:  "d delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeGuardExecutor{}
			m := openMemoryDetail(NewModelWithSnapshotAndGuardExecutor(tt.snapshot, executor))

			assertContains(t, m.View(), tt.wantText)
			assertNotContains(t, m.View(), tt.blockText)

			blocked := sendRune(m, tt.blockKey)
			if blocked.Screen() != ScreenMemoryDetail {
				t.Fatalf("screen after blocked key = %v, want memory detail", blocked.Screen())
			}

			opened := sendRune(m, tt.wantAction)
			if opened.Screen() != ScreenMemoryGuard {
				t.Fatalf("screen after action key = %v, want memory guard", opened.Screen())
			}
			assertContains(t, opened.View(), tt.guardText)
		})
	}
}

func TestDestructiveEntriesAreDisabledAndDoNotMutate(t *testing.T) {
	wantDisabled := map[string]bool{
		"Merge projects":  true,
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

func TestGuardedMemoryDeleteRequiresBackupAndExactConfirmationBeforeDispatch(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor)
	m = openGuardedMemoryDelete(m)

	assertContains(t, m.View(), "guarded memory delete", "target mem_8f3a91c0", "Backup ID is required", "Delete reason is required", "No delete will run until all fields pass guards")
	assertContains(t, m.View(), "ctrl-c quit")
	assertNotContains(t, m.View(), "q quit")

	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 without backup", len(executor.requests))
	}
	assertContains(t, m.View(), "Backup ID is required before guarded delete")

	m = sendText(m, "backup-1")
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Delete reason is required")

	m = sendText(m, "  stale cleanup  ")
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Type exactly: DELETE memory 7")

	m = sendText(m, "DELETE memory 7 ")
	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 when confirmation has trailing space", len(executor.requests))
	}
	assertContains(t, m.View(), "Confirmation mismatch. Type the phrase exactly; input is not trimmed")

	m = sendKey(m, tea.KeyBackspace)
	m = submitGuardAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	request := executor.requests[0]
	if request.Operation != "delete" || request.TargetType != "memory" || request.TargetID != 7 || request.BackupID != "backup-1" || request.Confirmation != "DELETE memory 7" || request.Reason != "stale cleanup" {
		t.Fatalf("request = %#v, want guarded memory delete with exact confirmation", request)
	}
	assertContains(t, m.View(), "Guarded memory delete dispatched through hive-daemon", "No direct SQLite or cloud mutation was performed by the TUI")
}

func TestGuardedMemoryDeleteRejectsMissingReasonBeforeConfirmation(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := openGuardedMemoryDelete(NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor))
	m = sendText(m, "backup-1")
	m = sendKey(m, tea.KeyEnter)

	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 without delete reason", len(executor.requests))
	}
	assertContains(t, m.View(), "Delete reason is required before guarded delete")
	assertNotContains(t, m.View(), "Type exactly: DELETE memory 7")

	m = sendText(m, "   ")
	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 with whitespace-only delete reason", len(executor.requests))
	}
	assertContains(t, m.View(), "Delete reason is required before guarded delete")
}

func TestGuardedMemoryDeleteSuccessRemovesMemoryFromNormalSnapshot(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := readyGuardedMemoryDelete(executor)
	m = submitGuardAndApplyResult(t, m)

	if m.Screen() != ScreenProjectMemories {
		t.Fatalf("screen = %v, want project memories after delete", m.Screen())
	}
	if len(m.snapshot.Memories) != 2 {
		t.Fatalf("snapshot memories = %d, want 2 after removing deleted memory", len(m.snapshot.Memories))
	}
	for _, memory := range m.snapshot.Memories {
		if memory.ID == 7 || memory.SyncID == "mem_8f3a91c0" {
			t.Fatalf("deleted memory still present in normal snapshot: %#v", memory)
		}
	}
	if m.snapshot.Projects[0].ActiveMemoryCount != 3480 || m.snapshot.Projects[0].DeletedMemoryCount != 5 {
		t.Fatalf("project counts = active %d deleted %d, want active 3480 deleted 5", m.snapshot.Projects[0].ActiveMemoryCount, m.snapshot.Projects[0].DeletedMemoryCount)
	}
	if totalMemories(m.snapshot.Projects) != 4122 {
		t.Fatalf("dashboard memory total = %d, want 4122", totalMemories(m.snapshot.Projects))
	}
	assertContains(t, m.View(), "Fix retry storm", "Guarded memory delete dispatched through hive-daemon")
	assertNotContains(t, m.View(), "Use exponential backoff", "[deleted]")
}

func TestGuardedMemoryDeleteViewUsesHiveVisualSystem(t *testing.T) {
	m := openGuardedMemoryDelete(NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), &fakeGuardExecutor{}))
	view := m.View()

	assertContains(t, view,
		"guarded memory delete",
		"destructive",
		"╭",
		"IMPACT",
		"SAFETY",
		"Backup ID is required: -",
		"Delete reason is required: -",
		"esc back",
		"ctrl-c quit",
	)
	assertNotContains(t, view, "q quit")
}

func TestGuardedMemoryDeletePendingBlocksDuplicateEscAndReopen(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := readyGuardedMemoryDelete(executor)

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("first cmd is nil, want guarded delete dispatch")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'d'}}, {Type: tea.KeyCtrlC}, {Type: tea.KeyRunes, Runes: []rune{'q'}}} {
		var cmd tea.Cmd
		updated, cmd = updated.Update(key)
		if cmd != nil {
			t.Fatalf("pending key %v returned cmd, want blocked", key.Type)
		}
	}
	m = updated.(Model)
	if m.Screen() != ScreenMemoryGuard {
		t.Fatalf("screen = %v, want memory guard while delete is pending", m.Screen())
	}
	assertContains(t, m.View(), "Wait for the result before leaving or submitting again")
	assertNotContains(t, m.View(), "esc back", "ctrl-c quit", "q quit")

	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count before async command runs = %d, want 0", len(executor.requests))
	}

	updated, _ = updated.Update(firstCmd())
	m = updated.(Model)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	assertContains(t, m.View(), "Guarded memory delete dispatched through hive-daemon")
}

func TestStaleMemoryGuardResultDoesNotClearCurrentPendingDelete(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := readyGuardedMemoryDelete(executor)

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("first cmd is nil, want guarded delete dispatch")
	}

	stale := memoryGuardResultMsg{operation: "delete", targetType: "memory", targetID: 99, backupID: "backup-older"}
	updated, _ = updated.Update(stale)
	m = updated.(Model)
	if m.Screen() != ScreenMemoryGuard {
		t.Fatalf("screen = %v, want memory guard after stale result", m.Screen())
	}
	if !m.guardSubmitting {
		t.Fatal("guardSubmitting = false, want current pending delete to remain pending")
	}

	updated, _ = updated.Update(firstCmd())
	m = updated.(Model)
	if m.guardSubmitting {
		t.Fatal("guardSubmitting = true, want matching result to clear pending delete")
	}
	assertContains(t, m.View(), "Guarded memory delete dispatched through hive-daemon")
}

func TestGuardedMemoryRestoreRequiresBackupAndExactConfirmationBeforeDispatch(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := NewModelWithSnapshotAndGuardExecutor(deletedGuardedMemorySnapshot(), executor)
	m = openGuardedMemoryRestore(m)

	assertContains(t, m.View(), "guarded memory restore", "target mem_8f3a91c0", "Backup ID is required", "Confirmation must match exactly", "No restore will run until both fields pass guards")
	assertContains(t, m.View(), "ctrl-c quit")
	assertNotContains(t, m.View(), "q quit")

	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 without backup", len(executor.requests))
	}
	assertContains(t, m.View(), "Backup ID is required before guarded restore")

	m = sendText(m, "backup-2")
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Type exactly: RESTORE memory 7")

	m = sendText(m, "RESTORE memory 7 ")
	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 when confirmation has trailing space", len(executor.requests))
	}
	assertContains(t, m.View(), "Confirmation mismatch. Type the phrase exactly; input is not trimmed")

	m = sendKey(m, tea.KeyBackspace)
	m = submitGuardAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	request := executor.requests[0]
	if request.Operation != "restore" || request.TargetType != "memory" || request.TargetID != 7 || request.BackupID != "backup-2" || request.Confirmation != "RESTORE memory 7" {
		t.Fatalf("request = %#v, want guarded memory restore with exact confirmation", request)
	}
	assertContains(t, m.View(), "Guarded memory restore dispatched through hive-daemon", "No direct SQLite or cloud mutation was performed by the TUI")
}

func TestGuardedMemoryRestorePendingBlocksDuplicateEscReopenAndStaleResult(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := readyGuardedMemoryRestore(executor)

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("first cmd is nil, want guarded restore dispatch")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'r'}}, {Type: tea.KeyCtrlC}, {Type: tea.KeyRunes, Runes: []rune{'q'}}} {
		var cmd tea.Cmd
		updated, cmd = updated.Update(key)
		if cmd != nil {
			t.Fatalf("pending key %v returned cmd, want blocked", key.Type)
		}
	}
	m = updated.(Model)
	if m.Screen() != ScreenMemoryGuard {
		t.Fatalf("screen = %v, want memory guard while restore is pending", m.Screen())
	}
	assertContains(t, m.View(), "Wait for the result before leaving or submitting again")
	assertNotContains(t, m.View(), "esc back", "ctrl-c quit", "q quit")

	stale := memoryGuardResultMsg{operation: "delete", targetType: "memory", targetID: 7, backupID: "backup-2"}
	updated, _ = updated.Update(stale)
	m = updated.(Model)
	if !m.guardSubmitting {
		t.Fatal("guardSubmitting = false, want current pending restore to remain pending")
	}

	updated, _ = updated.Update(firstCmd())
	m = updated.(Model)
	if m.guardSubmitting {
		t.Fatal("guardSubmitting = true, want matching result to clear pending restore")
	}
	assertContains(t, m.View(), "Guarded memory restore dispatched through hive-daemon")
}

func TestMemoryGuardEscWithPartialInputDoesNotDispatch(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor)
	m = openGuardedMemoryDelete(m)
	m = sendText(m, "partial-backup")
	m = sendKey(m, tea.KeyEsc)

	if m.Screen() != ScreenMemoryDetail {
		t.Fatalf("screen = %v, want memory detail", m.Screen())
	}
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0", len(executor.requests))
	}
}

func TestMemoryDetailDoesNotAdvertiseDeleteWithoutGuardExecutor(t *testing.T) {
	tests := []struct {
		name     string
		snapshot Snapshot
	}{
		{name: "active memory does not advertise delete", snapshot: guardedMemorySnapshot()},
		{name: "deleted memory does not advertise restore", snapshot: deletedGuardedMemorySnapshot()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModelWithSnapshot(tt.snapshot)
			m = openMemoryDetail(m)

			assertNotContains(t, m.View(), "d delete", "r restore", "guarded memory delete", "guarded memory restore")
			m = sendRune(m, 'd')
			if m.Screen() != ScreenMemoryDetail {
				t.Fatalf("screen = %v, want memory detail", m.Screen())
			}
			m = sendRune(m, 'r')
			if m.Screen() != ScreenMemoryDetail {
				t.Fatalf("screen = %v, want memory detail after restore key without executor", m.Screen())
			}
		})
	}
}

func TestProjectArchiveRequiresBackupAndExactConfirmationBeforeDispatch(t *testing.T) {
	executor := &fakeProjectArchiveExecutor{note: "No cloud project mutation was performed."}
	m := NewModelWithSnapshotAndProjectArchiveExecutor(projectArchiveSnapshot(), executor)
	m = sendKey(m, tea.KeyEnter)

	assertContains(t, m.View(), "a archive guarded by backup ID and exact confirmation")
	m = sendRune(m, 'a')
	assertContains(t, m.View(), "guarded project archive", "target alpha", "Backup ID is required", "Type exactly: ARCHIVE project alpha", "esc back", "ctrl-c quit")
	assertNotContains(t, m.View(), "q quit")

	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 without backup", len(executor.requests))
	}
	assertContains(t, m.View(), "Backup ID is required before guarded project archive")

	m = sendText(m, "backup-archive")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "ARCHIVE project alpha ")
	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 when confirmation has trailing space", len(executor.requests))
	}
	assertContains(t, m.View(), "Confirmation mismatch. Type the phrase exactly; input is not trimmed")

	m = sendKey(m, tea.KeyBackspace)
	m = submitProjectArchiveAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	request := executor.requests[0]
	if request.Project != "alpha" || request.BackupID != "backup-archive" || request.Confirmation != "ARCHIVE project alpha" {
		t.Fatalf("request = %#v, want guarded project archive with exact confirmation", request)
	}
	assertContains(t, m.View(), "Project alpha archive completed locally with backup backup-archive", "Cloud handoff: No cloud project mutation was performed.")
}

func TestProjectArchiveConfirmationCanContainLowercaseQ(t *testing.T) {
	executor := &fakeProjectArchiveExecutor{note: "No cloud project mutation was performed."}
	snapshot := projectArchiveSnapshot()
	snapshot.Projects[0].Name = "query-service"
	snapshot.Memories[0].Project = "query-service"
	snapshot.Memories[1].Project = "query-service"
	m := NewModelWithSnapshotAndProjectArchiveExecutor(snapshot, executor)

	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'a')
	m = sendText(m, "backup-archive")
	m = sendKey(m, tea.KeyEnter)

	for _, r := range "ARCHIVE project query-service" {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			t.Fatalf("typing confirmation rune %q returned cmd, want form input without global quit", r)
		}
		m = updated.(Model)
	}
	m = submitProjectArchiveAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	request := executor.requests[0]
	if request.Project != "query-service" || request.BackupID != "backup-archive" || request.Confirmation != "ARCHIVE project query-service" {
		t.Fatalf("request = %#v, want guarded project archive with lowercase q in confirmation", request)
	}
	assertContains(t, m.View(), "Project query-service archive completed locally")
}

func TestProjectArchivePendingBlocksDuplicateEscReopenAndStaleResult(t *testing.T) {
	executor := &fakeProjectArchiveExecutor{note: "No cloud project mutation was performed."}
	m := readyProjectArchive(executor)

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("first cmd is nil, want project archive dispatch")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'a'}}, {Type: tea.KeyRunes, Runes: []rune{'q'}}, {Type: tea.KeyCtrlC}} {
		var cmd tea.Cmd
		updated, cmd = updated.Update(key)
		if cmd != nil {
			t.Fatalf("pending key %v returned cmd, want blocked", key.Type)
		}
	}
	m = updated.(Model)
	if m.Screen() != ScreenProjectArchive {
		t.Fatalf("screen = %v, want project archive while pending", m.Screen())
	}
	assertContains(t, m.View(), "Wait for the result before leaving or submitting again")
	assertNotContains(t, m.View(), "esc back", "q quit", "j/k move", "enter open")

	updated, _ = updated.Update(projectArchiveResultMsg{project: "beta", backupID: "backup-archive"})
	m = updated.(Model)
	if !m.projectArchiveSubmitting {
		t.Fatal("projectArchiveSubmitting = false, want current pending archive to remain pending")
	}

	updated, _ = updated.Update(firstCmd())
	m = updated.(Model)
	if m.projectArchiveSubmitting {
		t.Fatal("projectArchiveSubmitting = true, want matching result to clear pending archive")
	}
	assertContains(t, m.View(), "Project alpha archive completed locally")
}

func TestProjectArchiveDoesNotAdvertiseWithoutExecutor(t *testing.T) {
	m := sendKey(NewModelWithSnapshot(projectArchiveSnapshot()), tea.KeyEnter)
	assertNotContains(t, m.View(), "archive guarded", "guarded project archive")

	m = sendRune(m, 'a')
	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want projects", m.Screen())
	}
}

func TestProjectMergeRequiresTargetBackupAndExactConfirmationBeforeDispatch(t *testing.T) {
	executor := &fakeProjectMergeExecutor{note: "No direct cloud project mutation was performed."}
	m := openProjectMerge(executor)

	assertContains(t, m.View(), "m merge guarded by backup ID and exact confirmation")
	m = sendRune(m, 'm')
	assertContains(t, m.View(), "guarded project merge", "source alpha", "Target project is required", "Backup ID is required", "No merge will run until all fields pass guards")

	m = sendKey(m, tea.KeyEnter)
	assertNoProjectMergeDispatch(t, executor, "without target")
	assertContains(t, m.View(), "Target project is required before guarded project merge")

	m = sendText(m, "beta")
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	assertNoProjectMergeDispatch(t, executor, "without backup")
	assertContains(t, m.View(), "Backup ID is required before guarded project merge")

	m = sendText(m, "backup-merge")
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Type exactly: MERGE project alpha INTO beta")

	m = sendText(m, "MERGE project alpha INTO beta ")
	m = sendKey(m, tea.KeyEnter)
	assertNoProjectMergeDispatch(t, executor, "when confirmation has trailing space")
	assertContains(t, m.View(), "Confirmation mismatch. Type the phrase exactly; input is not trimmed")

	m = sendKey(m, tea.KeyBackspace)
	m = submitProjectMergeAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	request := executor.requests[0]
	if request.SourceProject != "alpha" || request.TargetProject != "beta" || request.BackupID != "backup-merge" || request.Confirmation != "MERGE project alpha INTO beta" {
		t.Fatalf("request = %#v, want guarded project merge with exact confirmation", request)
	}
	assertContains(t, m.View(), "Project alpha merge into beta recorded locally with backup backup-merge", "Cloud handoff: No direct cloud project mutation was performed.")
}

func TestProjectMergePreflightRejectsInvalidTargetOrBackupBeforeDispatch(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		backup     string
		wantReason string
	}{
		{"same source and target", "alpha", "", "Source and target project must be different before guarded project merge"},
		{"unknown target", "gamma", "", "Target project gamma is not in the current snapshot before guarded project merge"},
		{"unknown backup", "beta", "missing-backup", "Backup ID missing-backup is not in the current snapshot before guarded project merge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeProjectMergeExecutor{}
			m := sendText(sendRune(openProjectMerge(executor), 'm'), tt.target)
			m = sendKey(m, tea.KeyEnter)
			if tt.backup != "" {
				m = sendKey(sendText(m, tt.backup), tea.KeyEnter)
			}
			assertNoProjectMergeDispatch(t, executor, tt.name)
			assertContains(t, m.View(), tt.wantReason)
		})
	}
}

func TestProjectMergePendingBlocksDuplicateEscReopenQuitAndStaleResult(t *testing.T) {
	executor := &fakeProjectMergeExecutor{note: "No direct cloud project mutation was performed."}
	m := readyProjectMerge(executor)

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("first cmd is nil, want project merge dispatch")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'m'}}, {Type: tea.KeyRunes, Runes: []rune{'q'}}, {Type: tea.KeyCtrlC}} {
		var cmd tea.Cmd
		updated, cmd = updated.Update(key)
		if cmd != nil {
			t.Fatalf("pending key %v returned cmd, want blocked", key.Type)
		}
	}
	m = updated.(Model)
	if m.Screen() != ScreenProjectMerge {
		t.Fatalf("screen = %v, want project merge while pending", m.Screen())
	}
	assertContains(t, m.View(), "Wait for the result before leaving or submitting again")
	assertNotContains(t, m.View(), "esc back", "q quit", "j/k move", "enter open")
	updated, _ = updated.Update(projectMergeResultMsg{sourceProject: "alpha", targetProject: "gamma", backupID: "backup-merge"})
	m = updated.(Model)
	if !m.projectMergeSubmitting {
		t.Fatal("projectMergeSubmitting = false, want current pending merge to remain pending")
	}
	updated, _ = updated.Update(firstCmd())
	m = updated.(Model)
	if m.projectMergeSubmitting {
		t.Fatal("projectMergeSubmitting = true, want matching result to clear pending merge")
	}
	assertContains(t, m.View(), "Project alpha merge into beta recorded locally")
}

func TestUnsyncedCountDoesNotUseWarningTextOrConsecutiveFailures(t *testing.T) {
	// Unsynced count comes from project UnsyncedCount, NOT from warning messages
	// or health consecutive failures. Zero projects → unsynced 0.
	m := NewModelWithSnapshot(Snapshot{
		DashboardState: DashboardDegraded,
		Warnings:       []hiveclient.Warning{{Message: "37 memories waiting to sync"}},
		Health:         []hiveclient.Health{{ConsecutiveFailures: 9}},
	})

	view := m.View()
	assertContains(t, view, "unsynced 0")
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

func sendText(m Model, text string) Model {
	for _, r := range text {
		m = sendRune(m, r)
	}
	return m
}

func openProjectMerge(executor *fakeProjectMergeExecutor) Model {
	return sendKey(NewModelWithSnapshotAndProjectMergeExecutor(projectMergeSnapshot(), executor), tea.KeyEnter)
}

func assertNoProjectMergeDispatch(t *testing.T, executor *fakeProjectMergeExecutor, reason string) {
	t.Helper()
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 %s", len(executor.requests), reason)
	}
}

func openGuardedMemoryDelete(m Model) Model {
	m = openMemoryDetail(m)
	m = sendRune(m, 'd')
	return m
}

func openGuardedMemoryRestore(m Model) Model {
	m = openMemoryDetail(m)
	m = sendRune(m, 'r')
	return m
}

func openMemoryDetail(m Model) Model {
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	return m
}

func readyProjectArchive(executor *fakeProjectArchiveExecutor) Model {
	m := NewModelWithSnapshotAndProjectArchiveExecutor(projectArchiveSnapshot(), executor)
	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'a')
	m = sendText(m, "backup-archive")
	m = sendKey(m, tea.KeyEnter)
	return sendText(m, "ARCHIVE project alpha")
}

func readyProjectMerge(executor *fakeProjectMergeExecutor) Model {
	m := openProjectMerge(executor)
	m = sendRune(m, 'm')
	m = sendText(m, "beta")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "backup-merge")
	m = sendKey(m, tea.KeyEnter)
	return sendText(m, "MERGE project alpha INTO beta")
}

func submitProjectArchiveAndApplyResult(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want project archive dispatch")
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

func submitProjectMergeAndApplyResult(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want project merge dispatch")
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

func readyGuardedMemoryDelete(executor *fakeGuardExecutor) Model {
	m := openGuardedMemoryDelete(NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor))
	m = sendText(m, "backup-1")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "stale cleanup")
	m = sendKey(m, tea.KeyEnter)
	return sendText(m, "DELETE memory 7")
}

func readyGuardedMemoryRestore(executor *fakeGuardExecutor) Model {
	m := openGuardedMemoryRestore(NewModelWithSnapshotAndGuardExecutor(deletedGuardedMemorySnapshot(), executor))
	m = sendText(m, "backup-2")
	m = sendKey(m, tea.KeyEnter)
	return sendText(m, "RESTORE memory 7")
}

func submitGuardAndApplyResult(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want guarded delete dispatch")
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

func guardedMemorySnapshot() Snapshot {
	snapshot := sampleNavigationSnapshot()
	snapshot.Memories[0].ID = 7
	return snapshot
}

func projectArchiveSnapshot() Snapshot {
	snapshot := sampleNavigationSnapshot()
	snapshot.Projects[0].Name = "alpha"
	snapshot.Memories[0].Project = "alpha"
	snapshot.Memories[1].Project = "alpha"
	return snapshot
}

func projectMergeSnapshot() Snapshot {
	snapshot := projectArchiveSnapshot()
	snapshot.Projects[1].Name = "beta"
	snapshot.Memories[2].Project = "beta"
	snapshot.Backups = append(snapshot.Backups, hiveclient.Backup{ID: "backup-merge"})
	return snapshot
}

func deletedGuardedMemorySnapshot() Snapshot {
	snapshot := guardedMemorySnapshot()
	snapshot.Memories[0].Deleted = true
	return snapshot
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

type fakeGuardExecutor struct {
	requests []hiveclient.GuardRequest
}

func (f *fakeGuardExecutor) ExecuteGuard(_ context.Context, request hiveclient.GuardRequest) (hiveclient.GuardResult, error) {
	f.requests = append(f.requests, request)
	return hiveclient.GuardResult{Operation: request.Operation, TargetType: request.TargetType, TargetID: request.TargetID, BackupID: request.BackupID, Mutated: true}, nil
}

type fakeProjectArchiveExecutor struct {
	requests []hiveclient.ProjectArchiveRequest
	note     string
}

func (f *fakeProjectArchiveExecutor) ArchiveProject(_ context.Context, request hiveclient.ProjectArchiveRequest) (hiveclient.ProjectArchiveResult, error) {
	f.requests = append(f.requests, request)
	return hiveclient.ProjectArchiveResult{Operation: "archive", TargetType: "project", Project: request.Project, BackupID: request.BackupID, Mutated: true, CloudHandoffNote: f.note}, nil
}

type fakeProjectMergeExecutor struct {
	requests []hiveclient.ProjectMergeRequest
	note     string
}

func (f *fakeProjectMergeExecutor) MergeProject(_ context.Context, request hiveclient.ProjectMergeRequest) (hiveclient.ProjectMergeResult, error) {
	f.requests = append(f.requests, request)
	return hiveclient.ProjectMergeResult{Operation: "merge", TargetType: "project", SourceProject: request.SourceProject, TargetProject: request.TargetProject, BackupID: request.BackupID, Mutated: true, CloudHandoffNote: f.note}, nil
}

// Task 5.1 — unsyncedText, lastSyncText, projectWarningCount helpers

func TestUnsyncedText_Sum(t *testing.T) {
	snapshot := Snapshot{
		Projects: []hiveclient.Project{
			{Name: "a", UnsyncedCount: 2},
			{Name: "b", UnsyncedCount: 0},
			{Name: "c", UnsyncedCount: 3},
		},
	}
	got := unsyncedText(snapshot)
	if got != "5" {
		t.Fatalf("unsyncedText = %q, want %q", got, "5")
	}
}

func TestLastSyncText_MaxValue(t *testing.T) {
	t1 := time.Now().Add(-48 * time.Hour)
	t2 := time.Now().Add(-24 * time.Hour)
	snapshot := Snapshot{
		Health: []hiveclient.Health{
			{Project: "a", LastSuccessAt: t1},
			{Project: "b", LastSuccessAt: t2},
		},
	}
	// t2 is the max; assert the exact value is selected
	// want is computed first to minimise the TOCTOU window between the two time.Since calls
	want := relativeTime(t2)
	got := lastSyncText(snapshot)
	if got != want {
		t.Fatalf("lastSyncText = %q, want %q (relativeTime of t2, the maximum)", got, want)
	}
}

func TestLastSyncText_EmptyHealth(t *testing.T) {
	snapshot := Snapshot{}
	got := lastSyncText(snapshot)
	if got != "never" {
		t.Fatalf("lastSyncText = %q, want %q", got, "never")
	}
}

func TestLastSyncText_AllZero(t *testing.T) {
	snapshot := Snapshot{
		Health: []hiveclient.Health{
			{Project: "a", LastSuccessAt: time.Time{}},
		},
	}
	got := lastSyncText(snapshot)
	if got != "never" {
		t.Fatalf("lastSyncText = %q, want %q", got, "never")
	}
}

func TestProjectWarningCount_Match(t *testing.T) {
	snapshot := Snapshot{
		Warnings: []hiveclient.Warning{
			{Source: "proj-a", Message: "w1"},
			{Source: "proj-a", Message: "w2"},
			{Source: "proj-b", Message: "w3"},
		},
	}
	got := projectWarningCount(snapshot, "proj-a")
	if got != 2 {
		t.Fatalf("projectWarningCount(proj-a) = %d, want 2", got)
	}
}

func TestProjectWarningCount_NoMatch(t *testing.T) {
	snapshot := Snapshot{
		Warnings: []hiveclient.Warning{
			{Source: "proj-a", Message: "w1"},
		},
	}
	got := projectWarningCount(snapshot, "proj-c")
	if got != 0 {
		t.Fatalf("projectWarningCount(proj-c) = %d, want 0", got)
	}
}

// Task 5.2 — projectsView enrichment

func TestProjectsView_ShowsRealUnsyncedCount(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects: []hiveclient.Project{
			{Name: "proj-a", ActiveMemoryCount: 5, UnsyncedCount: 4},
		},
	}
	m := NewModelWithSnapshot(snapshot)
	m = sendKey(m, tea.KeyEnter) // open projects
	view := m.View()
	assertContains(t, view, "4")
	assertNotContains(t, view, "n/a")
}

func TestProjectsView_ShowsWarningCount(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects: []hiveclient.Project{
			{Name: "proj-a", ActiveMemoryCount: 5, UnsyncedCount: 0},
		},
		Warnings: []hiveclient.Warning{
			{Source: "proj-a", Message: "w1"},
			{Source: "proj-a", Message: "w2"},
		},
	}
	m := NewModelWithSnapshot(snapshot)
	m = sendKey(m, tea.KeyEnter) // open projects
	view := m.View()
	// Assert the row contains proj-a and the column values in order: activeMemories=5, unsynced=0, warningCount=2
	assertContains(t, view, "proj-a")
	assertContains(t, view, "5  0  2")
}

func TestProjectsView_NoNAString(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects: []hiveclient.Project{
			{Name: "proj-a", ActiveMemoryCount: 3, UnsyncedCount: 0},
		},
	}
	m := NewModelWithSnapshot(snapshot)
	m = sendKey(m, tea.KeyEnter) // open projects
	assertNotContains(t, m.View(), "n/a")
}

// Task 5.3 — Dashboard header last-sync line

func TestDashboardView_LastSync_WithHealth(t *testing.T) {
	lastSuccess := time.Now().Add(-5 * time.Minute)
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Health:         []hiveclient.Health{{Project: "a", LastSuccessAt: lastSuccess}},
	}
	view := NewModelWithSnapshot(snapshot).View()
	assertContains(t, view, "last sync")
	assertNotContains(t, view, "last sync never")
}

func TestDashboardView_LastSync_EmptyHealth(t *testing.T) {
	snapshot := Snapshot{DashboardState: DashboardHealthy}
	view := NewModelWithSnapshot(snapshot).View()
	assertContains(t, view, "last sync never")
}

// Task 5.5 — NewModelWithAllExecutors signature change (5th param: MemoryLoader)

func TestNewModelWithAllExecutors_WiresMemoryLoader(t *testing.T) {
	loader := &fakeMemoryLoader{}
	snapshot := projectMergeSnapshot()
	snapshot.Memories[0].ID = 10

	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, loader, nil, nil)
	if m.memoryLoader == nil {
		t.Fatal("memoryLoader should be wired, got nil")
	}
}

// Task 5.6 — startMemoryLoad cmd, applyMemoryLoadResult, memoryDetailView states

func TestStartMemoryLoad_EmitsCmd(t *testing.T) {
	loader := &fakeMemoryLoader{content: "loaded content"}
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories: []hiveclient.Memory{
			{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"},
		},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, loader, nil, nil)
	// Navigate: dashboard -> projects -> project memories -> memory detail
	m = sendKey(m, tea.KeyEnter) // open projects
	m = sendKey(m, tea.KeyEnter) // open project memories
	// Now send enter to open memory detail — the enter case should emit a cmd
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	if m.Screen() != ScreenMemoryDetail {
		t.Fatalf("screen = %v, want ScreenMemoryDetail", m.Screen())
	}
	if !m.memoryLoading {
		t.Fatal("memoryLoading should be true after enter on memory detail")
	}
	if cmd == nil {
		t.Fatal("expected a tea.Cmd to be returned for async load")
	}
}

func TestApplyMemoryLoadResult_SetsContent(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories: []hiveclient.Memory{
			{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"},
		},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, &fakeMemoryLoader{}, nil, nil)
	m.screen = ScreenMemoryDetail
	m.memoryLoading = true

	result := memoryLoadResultMsg{id: 10, memory: hiveclient.Memory{ID: 10, Content: "fetched content"}, err: nil}
	m = m.applyMemoryLoadResult(result)

	if m.memoryLoading {
		t.Fatal("memoryLoading should be false after result applied")
	}
	if m.memoryContent != "fetched content" {
		t.Fatalf("memoryContent = %q, want %q", m.memoryContent, "fetched content")
	}
	if m.memoryLoadErr != nil {
		t.Fatalf("memoryLoadErr = %v, want nil", m.memoryLoadErr)
	}
}

func TestApplyMemoryLoadResult_IgnoresStale(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories: []hiveclient.Memory{
			{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"},
		},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, &fakeMemoryLoader{}, nil, nil)
	m.screen = ScreenMemoryDetail
	m.memoryLoading = true
	m.memoryContent = ""

	// Result for a different ID (stale)
	result := memoryLoadResultMsg{id: 99, memory: hiveclient.Memory{ID: 99, Content: "stale content"}, err: nil}
	m = m.applyMemoryLoadResult(result)

	// Should be ignored — content stays empty
	if m.memoryContent != "" {
		t.Fatalf("stale result should be ignored, memoryContent = %q", m.memoryContent)
	}
}

func TestApplyMemoryLoadResult_SetsError(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories: []hiveclient.Memory{
			{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"},
		},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, &fakeMemoryLoader{}, nil, nil)
	m.screen = ScreenMemoryDetail
	m.memoryLoading = true

	loadErr := assertErr("load failed")
	result := memoryLoadResultMsg{id: 10, err: loadErr}
	m = m.applyMemoryLoadResult(result)

	if m.memoryLoading {
		t.Fatal("memoryLoading should be false after error")
	}
	if m.memoryLoadErr == nil {
		t.Fatal("memoryLoadErr should be set")
	}
	if m.memoryContent != "" {
		t.Fatalf("memoryContent should be empty on error, got %q", m.memoryContent)
	}
}

func TestMemoryDetailView_Loading(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories:       []hiveclient.Memory{{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"}},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, &fakeMemoryLoader{}, nil, nil)
	m.screen = ScreenMemoryDetail
	m.memoryLoading = true

	view := m.memoryDetailView()
	assertContains(t, view, "Loading")
}

func TestMemoryDetailView_Content(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories:       []hiveclient.Memory{{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"}},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, &fakeMemoryLoader{}, nil, nil)
	m.screen = ScreenMemoryDetail
	m.memoryLoading = false
	m.memoryContent = "the full content"

	view := m.memoryDetailView()
	assertContains(t, view, "the full content")
}

func TestMemoryDetailView_Error(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories:       []hiveclient.Memory{{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"}},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, &fakeMemoryLoader{}, nil, nil)
	m.screen = ScreenMemoryDetail
	m.memoryLoading = false
	m.memoryLoadErr = assertErr("connection refused")

	view := m.memoryDetailView()
	assertContains(t, view, "Content failed to load")
	assertContains(t, view, "connection refused")
}

func TestMemoryDetailView_NoLoader(t *testing.T) {
	snapshot := Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha"}},
		Memories:       []hiveclient.Memory{{ID: 10, Project: "alpha", Title: "mem", SyncID: "s-1"}},
	}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil, nil, nil, nil) // no loader
	m.screen = ScreenMemoryDetail

	view := m.memoryDetailView()
	assertContains(t, view, "Content preview is not available from the read-only daemon snapshot.")
}

type fakeMemoryLoader struct {
	content string
	err     error
}

func (f *fakeMemoryLoader) MemoryByID(_ context.Context, id int64) (hiveclient.Memory, error) {
	if f.err != nil {
		return hiveclient.Memory{}, f.err
	}
	return hiveclient.Memory{ID: id, Content: f.content}, nil
}

// TestWindowSizeMsgUpdatesWidth verifies that Update handles tea.WindowSizeMsg
// and stores the terminal width in m.width.
func TestWindowSizeMsgUpdatesWidth(t *testing.T) {
	m := NewModelWithSnapshot(Snapshot{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned non-Model type %T", updated)
	}
	if got.width != 120 {
		t.Fatalf("width = %d, want 120", got.width)
	}
}

// TestTypeBadgeAllCategories verifies that typeBadge returns a non-empty string
// for each of the 7 defined memory observation types.
func TestTypeBadgeAllCategories(t *testing.T) {
	categories := []string{"decision", "bugfix", "pattern", "architecture", "config", "preference", "discovery"}
	for _, cat := range categories {
		got := typeBadge(cat)
		if got == "" {
			t.Errorf("typeBadge(%q) returned empty string", cat)
		}
	}
}

// TestTypeBadgeCaseInsensitive verifies that typeBadge normalizes input to
// lowercase before map lookup.
func TestTypeBadgeCaseInsensitive(t *testing.T) {
	lower := typeBadge("decision")
	upper := typeBadge("Decision")
	if lower != upper {
		t.Errorf("typeBadge case-sensitivity: typeBadge(\"decision\") = %q, typeBadge(\"Decision\") = %q", lower, upper)
	}
}

// TestBorderedPanelMinWidth verifies that borderedPanel does not panic when
// called with width=0 and returns a non-empty string containing the content.
func TestBorderedPanelMinWidth(t *testing.T) {
	result := borderedPanel("x", 0)
	if result == "" {
		t.Error("borderedPanel(\"x\", 0) returned empty string")
	}
	if !strings.Contains(result, "x") {
		t.Errorf("borderedPanel(\"x\", 0) = %q, want string containing \"x\"", result)
	}
}

func TestMemoryGuardCtrlCReturnsTeaQuitBeforeSubmit(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor)
	m = openGuardedMemoryDelete(m)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("cmd is nil, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

func TestMemoryGuardInputCanContainLowercaseQ(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor)
	m = openGuardedMemoryDelete(m)

	for _, r := range "backup-q99" {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			t.Fatalf("typing backup ID rune %q returned cmd, want form input without global quit", r)
		}
		m = updated.(Model)
	}
	if m.guardBackupID != "backup-q99" {
		t.Fatalf("guardBackupID = %q, want %q", m.guardBackupID, "backup-q99")
	}
}

// ─── Phase 5 RED tests ──────────────────────────────────────────────────────

// Task 5.1 — space toggles source in mergeStepSelectSources
func TestBatchMergeSelectSources_SpaceTogglesAndEnterAdvances(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMerge(executor)

	// should be on mergeStepSelectSources; alpha is cursor row
	if m.mergeStep != mergeStepSelectSources {
		t.Fatalf("mergeStep = %v, want mergeStepSelectSources", m.mergeStep)
	}

	// space selects alpha
	m = sendSpace(m)
	if !containsStr(m.mergeSelectedSources, "alpha") {
		t.Fatalf("mergeSelectedSources = %v, want alpha selected", m.mergeSelectedSources)
	}
	assertContains(t, m.View(), "[x] alpha")

	// space again deselects alpha
	m = sendSpace(m)
	if containsStr(m.mergeSelectedSources, "alpha") {
		t.Fatalf("mergeSelectedSources = %v, want alpha deselected", m.mergeSelectedSources)
	}
	assertNotContains(t, m.View(), "[x] alpha")

	// select alpha, then move down and select beta
	m = sendSpace(m) // re-select alpha
	m = sendKey(m, tea.KeyDown)
	m = sendSpace(m) // select beta
	if !containsStr(m.mergeSelectedSources, "alpha") || !containsStr(m.mergeSelectedSources, "beta") {
		t.Fatalf("mergeSelectedSources = %v, want both alpha and beta", m.mergeSelectedSources)
	}

	// enter with ≥1 source advances to mergeStepPickTarget
	m = sendKey(m, tea.KeyEnter)
	if m.mergeStep != mergeStepPickTarget {
		t.Fatalf("mergeStep = %v, want mergeStepPickTarget after enter with selections", m.mergeStep)
	}
}

// Task 5.2 — enter with zero selections shows validation, does NOT advance
func TestBatchMergeSelectSources_EnterWithZeroSelectionShowsError(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMerge(executor)

	m = sendKey(m, tea.KeyEnter)
	if m.mergeStep != mergeStepSelectSources {
		t.Fatalf("mergeStep = %v, want mergeStepSelectSources (no advance with 0 selections)", m.mergeStep)
	}
	assertContains(t, m.View(), "Select at least one source project")
}

// Task 5.3 — mergeStepPickTarget blocks when target equals a selected source
func TestBatchMergePickTarget_BlocksWhenTargetEqualsSelectedSource(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtPickTarget(executor)

	// type "alpha" (which is in selected sources)
	m = sendText(m, "alpha")
	m = sendKey(m, tea.KeyEnter)

	if m.mergeStep != mergeStepPickTarget {
		t.Fatalf("mergeStep = %v, want mergeStepPickTarget (blocked)", m.mergeStep)
	}
	assertContains(t, m.View(), "Target must not be one of the selected sources")
}

// Task 5.4 — mergeStepImpact renders per-source row + cloud guardrail panel when sync evidence
func TestBatchMergeImpact_RendersPerSourceRowsAndGuardrailWhenSyncEvidence(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtImpact(executor, true /* syncEvidence */)

	view := m.View()
	// per-source rows
	assertContains(t, view, "alpha")
	// cloud guardrail panel present
	assertContains(t, view, "CLOUD SYNC NOTICE")
	assertContains(t, view, "admin note")
}

func TestBatchMergeImpact_NoGuardrailWhenNoSyncEvidence(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtImpact(executor, false /* no syncEvidence */)

	view := m.View()
	assertContains(t, view, "alpha")
	assertNotContains(t, view, "CLOUD SYNC NOTICE")
}

func TestBatchMergeImpact_EnterAdvancesToBackupID(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtImpact(executor, false)

	m = sendKey(m, tea.KeyEnter)
	if m.mergeStep != mergeStepBackupID {
		t.Fatalf("mergeStep = %v, want mergeStepBackupID", m.mergeStep)
	}
}

// Task 5.5 — mergeStepConfirm wrong phrase does NOT advance; correct phrase advances
func TestBatchMergeConfirm_WrongPhraseDoesNotAdvance(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtConfirm(executor)

	m = sendText(m, "wrong phrase")
	m = sendKey(m, tea.KeyEnter)

	if m.mergeStep != mergeStepConfirm {
		t.Fatalf("mergeStep = %v, want mergeStepConfirm (blocked)", m.mergeStep)
	}
	if executor.callCount != 0 {
		t.Fatalf("callCount = %d, want 0 (no dispatch with wrong phrase)", executor.callCount)
	}
	assertContains(t, m.View(), "Confirmation mismatch")
}

func TestBatchMergeConfirm_CorrectPhraseDispatchesAndAdvancesToExecuting(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtConfirm(executor)

	phrase := mergeBatchConfirmationPhrase(m.mergeTarget)
	m = sendText(m, phrase)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want batch merge dispatch")
	}
}

// Task 5.6 — mergeStepResult renders per-source outcomes; enter → ScreenProjects
func TestBatchMergeResult_RendersOutcomesAndEnterReturnsToProjects(t *testing.T) {
	executor := &fakeProjectMergeBatchExecutor{}
	m := openBatchProjectMergeAtResult(executor)

	view := m.View()
	assertContains(t, view, "alpha")
	assertContains(t, view, "beta")

	// enter/esc returns to ScreenProjects
	m = sendKey(m, tea.KeyEnter)
	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want ScreenProjects after enter on result", m.Screen())
	}
}

// ─── Phase 5 helpers ────────────────────────────────────────────────────────

func openBatchProjectMerge(executor *fakeProjectMergeBatchExecutor) Model {
	m := NewModelWithSnapshotAndProjectMergeBatchExecutor(batchMergeSnapshot(), executor)
	m = sendKey(m, tea.KeyEnter) // open projects
	m = sendRune(m, 'm')         // start merge flow
	return m
}

// openBatchProjectMergeAtPickTarget positions the model at mergeStepPickTarget
// with alpha already selected as a source.
func openBatchProjectMergeAtPickTarget(executor *fakeProjectMergeBatchExecutor) Model {
	m := openBatchProjectMerge(executor)
	m = sendSpace(m)             // select alpha
	m = sendKey(m, tea.KeyEnter) // advance to pick target
	return m
}

// openBatchProjectMergeAtImpact positions the model at mergeStepImpact with
// alpha selected as source and "beta" as target. syncEvidence controls whether
// the snapshot contains sync evidence for alpha.
func openBatchProjectMergeAtImpact(executor *fakeProjectMergeBatchExecutor, syncEvidence bool) Model {
	snap := batchMergeSnapshot()
	if syncEvidence {
		// give alpha an unsynced=0 out of 2 total → synced rows exist
		for i, p := range snap.Projects {
			if p.Name == "alpha" {
				snap.Projects[i].UnsyncedCount = 0
				snap.Projects[i].ActiveMemoryCount = 2
			}
		}
		// add a memory with SyncID for alpha so evidence is non-zero
		snap.Memories = append(snap.Memories, hiveclient.Memory{
			ID: 99, Project: "alpha", SyncID: "s-99", Title: "synced mem",
		})
	}
	m := NewModelWithSnapshotAndProjectMergeBatchExecutor(snap, executor)
	m = sendKey(m, tea.KeyEnter) // open projects
	m = sendRune(m, 'm')         // start merge
	m = sendSpace(m)             // select alpha
	m = sendKey(m, tea.KeyEnter) // advance to pick target
	m = sendText(m, "beta")      // type target
	m = sendKey(m, tea.KeyEnter) // advance to impact
	return m
}

// openBatchProjectMergeAtConfirm positions the model at mergeStepConfirm.
func openBatchProjectMergeAtConfirm(executor *fakeProjectMergeBatchExecutor) Model {
	m := openBatchProjectMergeAtImpact(executor, false)
	m = sendKey(m, tea.KeyEnter) // advance past impact → backup ID
	m = sendText(m, "backup-batch")
	m = sendKey(m, tea.KeyEnter) // advance backup ID → confirm
	return m
}

// openBatchProjectMergeAtResult positions the model at mergeStepResult by
// dispatching and applying a fake result.
func openBatchProjectMergeAtResult(executor *fakeProjectMergeBatchExecutor) Model {
	m := openBatchProjectMergeAtConfirm(executor)
	phrase := mergeBatchConfirmationPhrase(m.mergeTarget)
	m = sendText(m, phrase)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		return updated.(Model)
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

func sendSpace(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	return updated.(Model)
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func batchMergeSnapshot() Snapshot {
	snap := projectMergeSnapshot()
	snap.Projects[0].Name = "alpha"
	snap.Projects[1].Name = "beta"
	snap.Backups = []hiveclient.Backup{{ID: "backup-batch"}}
	// Default: all memories unsynced → no sync evidence. This keeps the
	// no-guardrail test path clean. The sync-evidence test path overrides.
	snap.Projects[0].UnsyncedCount = snap.Projects[0].ActiveMemoryCount
	snap.Projects[1].UnsyncedCount = snap.Projects[1].ActiveMemoryCount
	return snap
}

type fakeProjectMergeBatchExecutor struct {
	callCount int
	result    hiveclient.ProjectMergeBatchResult
	err       error
}

func (f *fakeProjectMergeBatchExecutor) MergeProjects(_ context.Context, req hiveclient.ProjectMergeBatchRequest) (hiveclient.ProjectMergeBatchResult, error) {
	f.callCount++
	if f.err != nil {
		return hiveclient.ProjectMergeBatchResult{}, f.err
	}
	results := make([]hiveclient.MergeResult, len(req.Sources))
	for i, src := range req.Sources {
		results[i] = hiveclient.MergeResult{Source: src, Target: req.Target, Mutated: true}
	}
	return hiveclient.ProjectMergeBatchResult{
		Operation: "batch_merge",
		Target:    req.Target,
		BackupID:  req.BackupID,
		Results:   results,
	}, nil
}

// Phase 5 — ScreenProjectPurge tests

// 5.2 RED: empty archived list → "Purge archived" entry → empty-state message, no crash.
func TestScreenProjectPurge_EmptyList(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "manual cloud cleanup required"}
	// No projects → purge screen should show empty-state message.
	snapshot := Snapshot{DashboardState: DashboardHealthy}
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snapshot, executor)
	m = activatePurgeFromDashboard(m)

	if m.Screen() != ScreenProjectPurge {
		t.Fatalf("screen = %v, want ScreenProjectPurge", m.Screen())
	}
	assertContains(t, m.View(), "No projects available to purge")
}

// 5.3 RED: project present → "Purge archived" → project in list → Enter advances to backupID step.
func TestScreenProjectPurge_SelectStep(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "manual cloud cleanup required"}
	snapshot := projectPurgeSnapshot()
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snapshot, executor)
	m = activatePurgeFromDashboard(m)

	if m.Screen() != ScreenProjectPurge {
		t.Fatalf("screen = %v, want ScreenProjectPurge", m.Screen())
	}
	// Project should appear in the purge view.
	assertContains(t, m.View(), "alpha")

	// Pressing Enter at the select step should advance to the backupID step.
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Backup ID is required")
}

// 5.4 RED: reach confirmation step → wrong phrase → error shown, executor not called.
func TestScreenProjectPurge_ConfirmMismatch(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "manual cloud cleanup required"}
	snapshot := projectPurgeSnapshot()
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snapshot, executor)
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter) // advance to backupID step
	m = sendText(m, "backup-purge")
	m = sendKey(m, tea.KeyEnter) // advance to confirmation step
	m = sendText(m, "WRONG phrase")
	m = sendKey(m, tea.KeyEnter) // attempt submit

	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 on mismatch", len(executor.requests))
	}
	assertContains(t, m.View(), "Confirmation mismatch")
}

// 5.5 RED: correct phrase → executor called → CloudHandoffNote visible in result view.
func TestScreenProjectPurge_Success(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "manual cloud cleanup required"}
	snapshot := projectPurgeSnapshot()
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snapshot, executor)
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter) // select → backupID
	m = sendText(m, "backup-purge")
	m = sendKey(m, tea.KeyEnter) // backupID → confirm
	m = sendText(m, "PURGE project alpha")
	m = submitProjectPurgeAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	req := executor.requests[0]
	if req.Project != "alpha" || req.BackupID != "backup-purge" || req.Confirmation != "PURGE project alpha" {
		t.Fatalf("request = %#v, want purge request for alpha", req)
	}
	assertContains(t, m.View(), "manual cloud cleanup required")
}

// 5.6 RED: activate "Delete projects" dashboard entry → screen == ScreenProjects.
func TestDeleteProjectsDashboardEntryRoutesToScreenProjects(t *testing.T) {
	m := NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy})
	// Find the "Delete projects" action index.
	deleteIndex := -1
	for i, a := range dashboardActions() {
		if a.label == "Delete projects" {
			deleteIndex = i
			break
		}
	}
	if deleteIndex < 0 {
		t.Fatal("Delete projects action not found in dashboardActions")
	}
	m.cursor = deleteIndex
	m = sendKey(m, tea.KeyEnter)

	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want ScreenProjects after activating Delete projects", m.Screen())
	}
}

// Phase 5 helpers

type fakeProjectDeleteExecutor struct {
	requests []hiveclient.ProjectDeleteRequest
	note     string
}

func (f *fakeProjectDeleteExecutor) DeleteProject(_ context.Context, req hiveclient.ProjectDeleteRequest) (hiveclient.ProjectDeleteResult, error) {
	f.requests = append(f.requests, req)
	return hiveclient.ProjectDeleteResult{
		Operation:        "delete",
		TargetType:       "project",
		Project:          req.Project,
		BackupID:         req.BackupID,
		RowsDeleted:      42,
		Mutated:          true,
		CloudHandoffNote: f.note,
	}, nil
}

func NewModelWithSnapshotAndProjectDeleteExecutor(snapshot Snapshot, executor ProjectDeleteExecutor) Model {
	m := NewModelWithSnapshot(snapshot)
	m.projectDeleteExecutor = executor
	return m
}

func projectPurgeSnapshot() Snapshot {
	snap := projectArchiveSnapshot()
	// alpha is the selected project; treat it as purgeable (archived-only guard is backend's job).
	return snap
}

// activatePurgeFromDashboard navigates from the dashboard to ScreenProjectPurge
// by finding and activating the "Purge archived" dashboard entry.
func activatePurgeFromDashboard(m Model) Model {
	purgeIndex := -1
	for i, a := range dashboardActions() {
		if a.label == "Purge archived" {
			purgeIndex = i
			break
		}
	}
	if purgeIndex < 0 {
		// Action not yet added; return model unchanged so test fails descriptively.
		return m
	}
	m.cursor = purgeIndex
	return sendKey(m, tea.KeyEnter)
}

func submitProjectPurgeAndApplyResult(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want project purge dispatch")
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

// C1 — typing at select step must not corrupt projectDeleteConfirmation.
func TestScreenProjectPurge_TypingAtSelectStepDoesNotCorruptConfirmation(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "test"}
	m := NewModelWithSnapshotAndProjectDeleteExecutor(projectPurgeSnapshot(), executor)
	m = activatePurgeFromDashboard(m)

	if m.projectDeleteStep != projectPurgeSelect {
		t.Fatalf("step = %v, want projectPurgeSelect", m.projectDeleteStep)
	}

	m = sendRune(m, 'X')
	m = sendRune(m, 'Y')
	m = sendKey(m, tea.KeySpace)

	if m.projectDeleteConfirmation != "" {
		t.Fatalf("projectDeleteConfirmation = %q, want empty — typing at select step must not corrupt confirmation", m.projectDeleteConfirmation)
	}
	if m.projectDeleteBackupID != "" {
		t.Fatalf("projectDeleteBackupID = %q, want empty — typing at select step must not corrupt backupID", m.projectDeleteBackupID)
	}
}

// C2 — j/k/Up/Down navigate the project list at the select step.
func TestScreenProjectPurge_NavigationAtSelectStep(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "test"}
	snap := projectPurgeSnapshot()
	// Ensure at least two projects for meaningful navigation.
	if len(snap.Projects) < 2 {
		snap.Projects = append(snap.Projects, hiveclient.Project{Name: "gamma"})
	}
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snap, executor)
	m = activatePurgeFromDashboard(m)

	initial := m.projectIndex

	// Down increments.
	m = sendKey(m, tea.KeyDown)
	if m.projectIndex != initial+1 {
		t.Fatalf("after Down: projectIndex = %d, want %d", m.projectIndex, initial+1)
	}

	// Up decrements back.
	m = sendKey(m, tea.KeyUp)
	if m.projectIndex != initial {
		t.Fatalf("after Up: projectIndex = %d, want %d", m.projectIndex, initial)
	}

	// 'j' increments.
	m = sendRune(m, 'j')
	if m.projectIndex != initial+1 {
		t.Fatalf("after j: projectIndex = %d, want %d", m.projectIndex, initial+1)
	}

	// 'k' decrements.
	m = sendRune(m, 'k')
	if m.projectIndex != initial {
		t.Fatalf("after k: projectIndex = %d, want %d", m.projectIndex, initial)
	}

	// Floor at 0.
	m = sendKey(m, tea.KeyUp)
	if m.projectIndex < 0 {
		t.Fatalf("projectIndex = %d, must not go below 0", m.projectIndex)
	}

	// Cap at len-1.
	for range snap.Projects {
		m = sendKey(m, tea.KeyDown)
	}
	if m.projectIndex >= len(snap.Projects) {
		t.Fatalf("projectIndex = %d, must not exceed len-1 (%d)", m.projectIndex, len(snap.Projects)-1)
	}
}

// C3 — 'p' hint appears in projectsView when executor is set.
func TestProjectsView_PurgeHintAppearsWhenExecutorSet(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{}
	snap := projectPurgeSnapshot()
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snap, executor)
	m = sendKey(m, tea.KeyEnter) // navigate to projects

	assertContains(t, m.View(), "p purge archived project guarded by backup ID and exact confirmation")
}

// C3 inverse — 'p' hint absent when executor is nil.
func TestProjectsView_PurgeHintAbsentWithoutExecutor(t *testing.T) {
	m := sendKey(NewModelWithSnapshot(projectPurgeSnapshot()), tea.KeyEnter)
	assertNotContains(t, m.View(), "p purge archived")
}

// C4 — startProjectPurge with nil executor sets message, does not change screen.
func TestStartProjectPurge_NilExecutorSetsMessage(t *testing.T) {
	m := NewModelWithSnapshot(projectPurgeSnapshot())
	m.screen = ScreenProjects

	before := m.Screen()
	m = m.startProjectPurge()

	if m.Screen() != before {
		t.Fatalf("screen changed to %v, want no change when executor is nil", m.Screen())
	}
	if !strings.Contains(m.message, "purge executor not available") {
		t.Fatalf("message = %q, want 'purge executor not available'", m.message)
	}
}

// C5 — executor error path: error message shown, screen returns to ScreenProjects.
func TestScreenProjectPurge_ExecutorError(t *testing.T) {
	executor := &fakeProjectDeleteExecutorError{err: assertErr("daemon rejected purge")}
	snap := projectPurgeSnapshot()
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snap, executor)
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter) // select → backupID
	m = sendText(m, "backup-purge")
	m = sendKey(m, tea.KeyEnter) // backupID → confirmation
	m = sendText(m, "PURGE project alpha")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want purge dispatch")
	}
	updated, _ = updated.Update(cmd())
	m = updated.(Model)

	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want ScreenProjects after executor error", m.Screen())
	}
	assertContains(t, m.View(), "purge failed")
}

type fakeProjectDeleteExecutorError struct {
	err error
}

func (f *fakeProjectDeleteExecutorError) DeleteProject(_ context.Context, req hiveclient.ProjectDeleteRequest) (hiveclient.ProjectDeleteResult, error) {
	return hiveclient.ProjectDeleteResult{}, f.err
}

// C6 — ESC at each purge step navigates back to ScreenProjects without calling executor.
func TestScreenProjectPurge_EscAtEachStep(t *testing.T) {
	tests := []struct {
		name    string
		advance func(m Model) Model
	}{
		{
			name:    "esc at select step",
			advance: func(m Model) Model { return m },
		},
		{
			name: "esc at backupID step",
			advance: func(m Model) Model {
				return sendKey(m, tea.KeyEnter) // select → backupID
			},
		},
		{
			name: "esc at confirmation step",
			advance: func(m Model) Model {
				m = sendKey(m, tea.KeyEnter) // select → backupID
				m = sendText(m, "backup-purge")
				return sendKey(m, tea.KeyEnter) // backupID → confirmation
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeProjectDeleteExecutor{note: "test"}
			m := NewModelWithSnapshotAndProjectDeleteExecutor(projectPurgeSnapshot(), executor)
			m = activatePurgeFromDashboard(m)
			m = tt.advance(m)

			m = sendKey(m, tea.KeyEsc)

			if m.Screen() != ScreenProjects {
				t.Fatalf("screen = %v, want ScreenProjects after ESC", m.Screen())
			}
			if len(executor.requests) != 0 {
				t.Fatalf("executor called %d times, want 0 after ESC", len(executor.requests))
			}
		})
	}
}

// C7 — pending blocks keys; stale result is ignored.
func TestScreenProjectPurge_PendingBlocksKeysAndStaleResult(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "test"}
	snap := projectPurgeSnapshot()
	m := NewModelWithSnapshotAndProjectDeleteExecutor(snap, executor)
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter) // select → backupID
	m = sendText(m, "backup-purge")
	m = sendKey(m, tea.KeyEnter) // backupID → confirmation
	m = sendText(m, "PURGE project alpha")

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("cmd is nil, want purge dispatch")
	}

	// While submitting, these keys must be blocked (no screen change, still submitting).
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyRunes, Runes: []rune{'p'}},
	} {
		var cmd tea.Cmd
		updated, cmd = updated.Update(key)
		if cmd != nil {
			t.Fatalf("pending key %v returned cmd, want blocked", key.Type)
		}
	}
	m = updated.(Model)
	if m.Screen() != ScreenProjectPurge {
		t.Fatalf("screen = %v, want ScreenProjectPurge while pending", m.Screen())
	}
	assertContains(t, m.View(), "Wait for the result before leaving or submitting again")

	// Stale result (different project name) must be ignored.
	updated, _ = updated.Update(projectDeleteResultMsg{project: "other-project", backupID: "backup-purge"})
	m = updated.(Model)
	if !m.projectDeleteSubmitting {
		t.Fatal("projectDeleteSubmitting = false after stale result, want still pending")
	}

	// Matching result clears pending state.
	updated, _ = updated.Update(firstCmd())
	m = updated.(Model)
	if m.projectDeleteSubmitting {
		t.Fatal("projectDeleteSubmitting = true after matching result, want cleared")
	}
	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want ScreenProjects after successful purge", m.Screen())
	}
}

// C8 — back() resets purge state fields.
func TestScreenProjectPurge_BackResetsState(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "test"}
	m := NewModelWithSnapshotAndProjectDeleteExecutor(projectPurgeSnapshot(), executor)
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter) // select → backupID
	m = sendText(m, "backup-purge")
	m = sendKey(m, tea.KeyEnter) // backupID → confirmation
	m = sendText(m, "partial confirm")

	m = sendKey(m, tea.KeyEsc) // back to projects

	if m.Screen() != ScreenProjects {
		t.Fatalf("screen = %v, want ScreenProjects", m.Screen())
	}
	if m.projectDeleteStep != projectPurgeSelect {
		t.Fatalf("projectDeleteStep = %v, want projectPurgeSelect", m.projectDeleteStep)
	}
	if m.projectDeleteBackupID != "" {
		t.Fatalf("projectDeleteBackupID = %q, want empty", m.projectDeleteBackupID)
	}
	if m.projectDeleteConfirmation != "" {
		t.Fatalf("projectDeleteConfirmation = %q, want empty", m.projectDeleteConfirmation)
	}
	if m.projectDeleteSubmitting {
		t.Fatalf("projectDeleteSubmitting = true, want false")
	}
	if m.projectDeleteProject != (hiveclient.Project{}) {
		t.Fatalf("projectDeleteProject = %+v, want zero value", m.projectDeleteProject)
	}
}

// C10 — ctrl-c returns tea.Quit when not submitting.
func TestScreenProjectPurge_CtrlCReturnsTeaQuitBeforeSubmit(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "test"}
	m := NewModelWithSnapshotAndProjectDeleteExecutor(projectPurgeSnapshot(), executor)
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter) // select → backupID

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("cmd is nil, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

// C9 — NewModelWithAllExecutors wires deleteExecutor.
func TestNewModelWithAllExecutors_WiresDeleteExecutor(t *testing.T) {
	executor := &fakeProjectDeleteExecutor{note: "test"}
	snap := projectPurgeSnapshot()
	m := NewModelWithAllExecutors(snap, nil, nil, nil, nil, nil, executor)

	if m.projectDeleteExecutor == nil {
		t.Fatal("projectDeleteExecutor should be wired, got nil")
	}
}
