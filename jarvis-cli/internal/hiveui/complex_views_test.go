package hiveui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestComplexViewportMakesBatchMergeResultsReachable(t *testing.T) {
	results := make([]hiveclient.MergeResult, 12)
	for i := range results {
		results[i] = hiveclient.MergeResult{Source: fmt.Sprintf("source-%02d", i), Target: "target", Mutated: true}
	}
	m := Model{
		snapshot:                  Snapshot{DashboardState: DashboardHealthy},
		screen:                    ScreenProjectMerge,
		projectMergeBatchExecutor: &fakeProjectMergeBatchExecutor{},
		mergeStep:                 mergeStepResult,
		mergeBatchResult:          &hiveclient.ProjectMergeBatchResult{Target: "target", BackupID: "backup-1", Results: results},
	}
	m = resizeComplexViewport(t, m, 80, 7)

	assertFrameFitsHeight(t, m.View(), 7)
	assertContains(t, m.View(), "RESULT", "more")
	assertNotContains(t, m.View(), "source-11")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(Model)
	if m.viewport.offset == 0 {
		t.Fatal("end did not advance the bounded batch-result viewport")
	}
	assertFrameFitsHeight(t, m.View(), 7)
	assertContains(t, m.View(), "source-11", "more")

	m = resizeComplexViewport(t, m, 80, 5)
	if m.viewport.offset > max(0, m.viewportItemCount()-m.viewport.height) {
		t.Fatalf("resize left an unclamped viewport offset: %#v", m.viewport)
	}
	assertFrameFitsHeight(t, m.View(), 5)

	m = resizeComplexViewport(t, m, 80, 1)
	if got := m.View(); got != "Terminal too small" {
		t.Fatalf("tiny complex frame = %q, want deterministic terminal warning", got)
	}
}

func TestComplexViewportPreservesFormNavigationAndBoundsMultilineErrors(t *testing.T) {
	m := newConfigModelWithService(&fakeConfigService{})
	m.configLoading = false
	m.configLoadErr = assertErr(strings.Repeat("configuration failure ", 12))
	m = resizeComplexViewport(t, m, 80, 6)

	assertFrameFitsHeight(t, m.View(), 6)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(Model)
	if m.viewport.offset == 0 {
		t.Fatal("page down did not advance the bounded configuration-error viewport")
	}
	assertFrameFitsHeight(t, m.View(), 6)
	assertContains(t, m.View(), "more")
}

func TestComplexViewportKeepsConfirmationTextOwnership(t *testing.T) {
	m := projectMergeAtConfirmation()
	m = resizeComplexViewport(t, m, 80, 6)
	phrase := "MERGE project alpha INTO beta"

	m = sendText(m, phrase)
	if m.projectMergeConfirmation != phrase {
		t.Fatalf("confirmation = %q, want %q after typing through a bounded frame", m.projectMergeConfirmation, phrase)
	}
	assertFrameFitsHeight(t, m.View(), 6)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(Model)
	if m.viewport.offset == 0 {
		t.Fatal("page down did not advance the bounded confirmation viewport")
	}
	if m.projectMergeConfirmation != phrase {
		t.Fatalf("confirmation changed after viewport navigation: %q", m.projectMergeConfirmation)
	}
}

func TestComplexViewportMakesNormalizationOverviewReachable(t *testing.T) {
	m := openNormalization(t, &fakeNormalizationService{plans: []hiveclient.MigrationPlan{executablePlan()}})
	m = resizeComplexViewport(t, m, 80, 6)

	assertFrameFitsHeight(t, m.View(), 6)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(Model)
	if m.viewport.offset == 0 {
		t.Fatal("end did not advance the bounded normalization viewport")
	}
	assertFrameFitsHeight(t, m.View(), 6)
	assertContains(t, m.View(), "more")
}

func TestComplexViewportFollowsPurgeSelectionWithoutTakingItsKeys(t *testing.T) {
	projects := make([]hiveclient.Project, 10)
	for i := range projects {
		projects[i] = hiveclient.Project{Name: fmt.Sprintf("project-%02d", i)}
	}
	m := NewModelWithSnapshotAndProjectDeleteExecutor(Snapshot{DashboardState: DashboardHealthy, Projects: projects}, &fakeProjectDeleteExecutor{}).startProjectPurge()
	m = resizeComplexViewport(t, m, 80, 7)

	for range projects {
		m = sendRune(m, 'j')
	}
	if m.projectIndex != len(projects)-1 {
		t.Fatalf("projectIndex = %d, want the final purge project", m.projectIndex)
	}
	assertFrameFitsHeight(t, m.View(), 7)
	assertContains(t, m.View(), "project-09")
}

func resizeComplexViewport(t *testing.T, m Model, width, height int) Model {
	t.Helper()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model)
}

func assertFrameFitsHeight(t *testing.T, view string, height int) {
	t.Helper()
	if lines := len(strings.Split(view, "\n")); lines > height {
		t.Fatalf("frame has %d lines, want at most %d:\n%s", lines, height, view)
	}
}
