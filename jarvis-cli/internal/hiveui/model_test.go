package hiveui

import (
	"strings"
	"testing"

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
	assertContains(t, m.View(), "Project viewer", "Memory warnings", "Backup / Restore (disabled)", "Merge projects (disabled)", "Delete memories (disabled)")

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
	assertContains(t, view, "j/k move", "enter read-only notice", "q quit")
	assertNotContains(t, view, "r retry", "enter open", "w warnings", "g health", "c config")
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

func TestEnterOnReadOnlyPlaceholdersSetsExplicitStatusWithoutMutation(t *testing.T) {
	for index, action := range dashboardActions() {
		if action.disabled {
			continue
		}
		t.Run(action.label, func(t *testing.T) {
			m := NewModelWithSnapshot(Snapshot{DashboardState: DashboardHealthy})
			m.cursor = index
			m = sendKey(m, tea.KeyEnter)

			if m.Screen() != ScreenDashboard {
				t.Fatalf("screen = %v, want dashboard", m.Screen())
			}
			if m.cursor != index {
				t.Fatalf("cursor = %d, want %d", m.cursor, index)
			}
			assertContains(t, m.View(), "Navigation is not available in this read-only TUI slice", "No local Hive state was changed")
		})
	}
}

func TestDestructiveEntriesAreDisabledAndDoNotMutate(t *testing.T) {
	wantDisabled := map[string]bool{
		"Merge projects":   true,
		"Delete projects":  true,
		"Delete memories":  true,
		"Backup / Restore": true,
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
