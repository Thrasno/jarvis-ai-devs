package hiveui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestNewModelWithAllExecutors_WiresAllThreeExecutors(t *testing.T) {
	guard := &fakeGuardExecutor{}
	archive := &fakeProjectArchiveExecutor{note: "test"}
	merge := &fakeProjectMergeExecutor{note: "test"}

	// Build a snapshot that has non-zero memory IDs (required for guard hotkey),
	// at least two projects (required for merge target), and a backup (required for merge guard).
	snapshot := projectMergeSnapshot() // has alpha+beta projects, backup-merge, memories with ID 7
	snapshot.Memories[0].ID = 7

	m := NewModelWithAllExecutors(snapshot, guard, archive, merge)

	// Assert guard executor is wired: guard hotkey appears in memory detail.
	detail := openMemoryDetail(m)
	assertContains(t, detail.View(), "d delete guarded by backup ID and exact confirmation")

	// Assert archive executor is wired: 'a' hotkey appears in projects view.
	projects := sendKey(m, tea.KeyEnter)
	assertContains(t, projects.View(), "a archive guarded by backup ID and exact confirmation")

	// Assert merge executor is wired: 'm' hotkey appears in projects view.
	assertContains(t, projects.View(), "m merge guarded by backup ID and exact confirmation")
}

func TestNewModelWithAllExecutors_EmptyDaemonURLDefaults(t *testing.T) {
	snapshot := Snapshot{DaemonURL: ""}
	m := NewModelWithAllExecutors(snapshot, nil, nil, nil)
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
	assertContains(t, m.View(), "dashboard / projects", "core-api", "web-client", "3481", "n/a", "2m ago")
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
			wantText:   "d delete guarded by backup ID and exact confirmation",
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

func TestGuardedMemoryDeleteRequiresBackupAndExactConfirmationBeforeDispatch(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), executor)
	m = openGuardedMemoryDelete(m)

	assertContains(t, m.View(), "guarded memory delete", "target mem_8f3a91c0", "Backup ID is required", "Confirmation must match exactly", "No delete will run until both fields pass guards")

	m = sendKey(m, tea.KeyEnter)
	if len(executor.requests) != 0 {
		t.Fatalf("dispatch count = %d, want 0 without backup", len(executor.requests))
	}
	assertContains(t, m.View(), "Backup ID is required before guarded delete")

	m = sendText(m, "backup-1")
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
	if request.Operation != "delete" || request.TargetType != "memory" || request.TargetID != 7 || request.BackupID != "backup-1" || request.Confirmation != "DELETE memory 7" {
		t.Fatalf("request = %#v, want guarded memory delete with exact confirmation", request)
	}
	assertContains(t, m.View(), "Guarded memory delete dispatched through hive-daemon", "No direct SQLite or cloud mutation was performed by the TUI")
}

func TestGuardedMemoryDeletePendingBlocksDuplicateEscAndReopen(t *testing.T) {
	executor := &fakeGuardExecutor{}
	m := readyGuardedMemoryDelete(executor)

	updated, firstCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if firstCmd == nil {
		t.Fatal("first cmd is nil, want guarded delete dispatch")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'d'}}} {
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
	assertNotContains(t, m.View(), "esc back")

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
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'r'}}} {
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
	assertNotContains(t, m.View(), "esc back")

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
