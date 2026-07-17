package hiveui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

type guardedMemoryWorkflowFake struct {
	capabilities hiveclient.Capabilities
	backup       hiveclient.Backup
	current      hiveclient.Memory
	deleted      hiveclient.Memory
	receipt      hiveclient.MutationReceipt
	requests     []hiveclient.GuardRequest
}

func (f *guardedMemoryWorkflowFake) Capabilities(context.Context) (hiveclient.Capabilities, error) {
	return f.capabilities, nil
}

func (f *guardedMemoryWorkflowFake) CreateBackup(context.Context) (hiveclient.Backup, error) {
	return f.backup, nil
}

func (f *guardedMemoryWorkflowFake) MemoryByID(context.Context, int64) (hiveclient.Memory, error) {
	return f.current, nil
}

func (f *guardedMemoryWorkflowFake) DeletedMemoryByID(context.Context, int64, string) (hiveclient.Memory, error) {
	return f.deleted, nil
}

func (f *guardedMemoryWorkflowFake) ExecuteGuard(_ context.Context, request hiveclient.GuardRequest) (hiveclient.GuardResult, error) {
	f.requests = append(f.requests, request)
	return hiveclient.GuardResult{Mutated: true, Receipt: &f.receipt}, nil
}

func (f *guardedMemoryWorkflowFake) MutationReceipt(_ context.Context, requestID string, targetID int64, project, syncID string) (hiveclient.MutationReceipt, error) {
	return f.receipt, nil
}

func guardedWorkflowSnapshot(deleted bool) Snapshot {
	capabilities := hiveclient.Capabilities{DeleteRestore: true, ExpectedIdentity: true, RequestReceipts: true, MutationSyncV2: true}
	return Snapshot{
		DashboardState: DashboardHealthy,
		Projects:       []hiveclient.Project{{Name: "alpha", ActiveMemoryCount: 1}},
		Memories:       []hiveclient.Memory{{ID: 7, SyncID: "sync-7", Project: "alpha", Title: "Target", Deleted: deleted}},
		Capabilities:   &capabilities,
	}
}

func TestGuardedMemoryWorkflowDisablesActionsWhenCapabilityContractIsIncomplete(t *testing.T) {
	workflow := &guardedMemoryWorkflowFake{capabilities: hiveclient.Capabilities{DeleteRestore: true}}
	snapshot := guardedWorkflowSnapshot(false)
	snapshot.Capabilities = &workflow.capabilities
	m := NewModelWithGuardedMemoryWorkflow(snapshot, workflow)
	m = openMemoryDetail(m)

	assertContains(t, m.View(), "Delete and restore are unavailable: hive-daemon does not advertise the complete safety contract")
	assertNotContains(t, m.View(), "d delete guarded")
	if got := sendRune(m, 'd'); got.Screen() != ScreenMemoryDetail {
		t.Fatalf("screen = %v, want detail while guarded operations are unavailable", got.Screen())
	}
}

func TestGuardedMemoryWorkflowSeparatesActiveAndRecentlyDeleted(t *testing.T) {
	snapshot := guardedWorkflowSnapshot(false)
	snapshot.DeletedMemories = []hiveclient.Memory{{ID: 8, SyncID: "sync-8", Project: "alpha", Title: "Deleted", Deleted: true}}
	m := NewModelWithSnapshot(snapshot)
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Target")
	assertNotContains(t, m.View(), "Deleted")

	m = sendRune(m, 'x')
	assertContains(t, m.View(), "recently deleted", "Deleted")
	assertNotContains(t, m.View(), "Target")
}

func TestLoadSnapshotLoadsDeletedMemoriesPerProject(t *testing.T) {
	deletedProjects := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/governance/health":
			writeJSON(w, map[string]any{"projects": []any{}})
		case "/governance/projects":
			writeJSON(w, map[string]any{"projects": []map[string]any{{"name": "proj-a"}, {"name": "proj-b"}}})
		case "/governance/memories":
			project := r.URL.Query().Get("project")
			if project == "" {
				http.Error(w, `{"error":"project required"}`, http.StatusBadRequest)
				return
			}
			if r.URL.Query().Get("deleted_only") == "true" {
				deletedProjects[project] = true
				writeJSON(w, map[string]any{"memories": []map[string]any{{"sync_id": "deleted-" + project, "project": project}}})
				return
			}
			writeJSON(w, map[string]any{"memories": []any{}})
		case "/governance/warnings":
			writeJSON(w, map[string]any{"warnings": []any{}})
		case "/governance/backups":
			writeJSON(w, map[string]any{"backups": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := hiveclient.New(srv.URL)
	if err != nil {
		t.Fatalf("hiveclient.New: %v", err)
	}
	snap := LoadSnapshot(context.Background(), client, srv.URL, "")
	if len(snap.DeletedMemories) != 2 {
		t.Fatalf("DeletedMemories len = %d, want 2", len(snap.DeletedMemories))
	}
	if !deletedProjects["proj-a"] || !deletedProjects["proj-b"] {
		t.Fatalf("deleted per-project requests = %#v, want proj-a and proj-b", deletedProjects)
	}
}

func TestGuardedMemoryWorkflowCreatesBackupRereadsIdentityAndReconcilesReceipt(t *testing.T) {
	workflow := &guardedMemoryWorkflowFake{
		capabilities: hiveclient.Capabilities{DeleteRestore: true, ExpectedIdentity: true, RequestReceipts: true, MutationSyncV2: true},
		backup:       hiveclient.Backup{ID: "fresh-backup"},
		current:      hiveclient.Memory{ID: 7, SyncID: "sync-7", Project: "alpha"},
		receipt:      hiveclient.MutationReceipt{RequestID: "request-7", TargetID: 7, Project: "alpha", EntitySyncID: "sync-7", LocalStatus: "committed", SharedStatus: "pending"},
	}
	m := NewModelWithGuardedMemoryWorkflow(guardedWorkflowSnapshot(false), workflow)
	m = openMemoryDetail(m)
	m = sendRune(m, 'd')
	m = sendText(m, "cleanup")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "DELETE memory 7")
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("submit command is nil")
	}
	updated, _ = updated.Update(command())
	m = updated.(Model)

	if len(workflow.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(workflow.requests))
	}
	request := workflow.requests[0]
	if request.BackupID != "fresh-backup" || request.ExpectedProject != "alpha" || request.ExpectedSyncID != "sync-7" || request.RequestID == "" || request.Reason != "cleanup" {
		t.Fatalf("request = %#v, want fresh backup, current identity, request id, and reason", request)
	}
	assertContains(t, m.View(), "Local status: committed", "Shared status: pending")
	if len(m.snapshot.Memories) != 0 || len(m.snapshot.DeletedMemories) != 1 || !m.snapshot.DeletedMemories[0].Deleted {
		t.Fatalf("active/deleted slices = %#v / %#v, want target moved to recently deleted", m.snapshot.Memories, m.snapshot.DeletedMemories)
	}
}

func TestGuardedMemoryWorkflowRejectsIdentityDriftAndLocksDuplicateSubmit(t *testing.T) {
	workflow := &guardedMemoryWorkflowFake{
		capabilities: hiveclient.Capabilities{DeleteRestore: true, ExpectedIdentity: true, RequestReceipts: true, MutationSyncV2: true},
		backup:       hiveclient.Backup{ID: "fresh-backup"},
		current:      hiveclient.Memory{ID: 7, SyncID: "changed", Project: "alpha"},
	}
	m := NewModelWithGuardedMemoryWorkflow(guardedWorkflowSnapshot(false), workflow)
	m = openMemoryDetail(m)
	m = sendRune(m, 'd')
	m = sendText(m, "cleanup")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "DELETE memory 7")
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("submit command is nil")
	}
	updated, _ = updated.Update(command())
	m = updated.(Model)
	if len(workflow.requests) != 0 {
		t.Fatalf("requests = %d, want 0 after identity drift", len(workflow.requests))
	}
	assertContains(t, m.View(), "Memory identity changed during revalidation")
}

func TestGuardedMemoryWorkflowRestoreRequiresReasonAndRefreshesActiveSlice(t *testing.T) {
	workflow := &guardedMemoryWorkflowFake{
		capabilities: hiveclient.Capabilities{DeleteRestore: true, ExpectedIdentity: true, RequestReceipts: true, MutationSyncV2: true},
		backup:       hiveclient.Backup{ID: "fresh-backup"},
		deleted:      hiveclient.Memory{ID: 7, SyncID: "sync-7", Project: "alpha", Deleted: true},
		receipt:      hiveclient.MutationReceipt{RequestID: "request-restore", TargetID: 7, Project: "alpha", EntitySyncID: "sync-7", LocalStatus: "committed", SharedStatus: "completed"},
	}
	snapshot := guardedWorkflowSnapshot(false)
	snapshot.Memories = nil
	snapshot.DeletedMemories = []hiveclient.Memory{{ID: 7, SyncID: "sync-7", Project: "alpha", Title: "Target", Deleted: true}}
	snapshot.TimelineMemories = append([]hiveclient.Memory(nil), snapshot.DeletedMemories...)
	snapshot.Projects[0] = hiveclient.Project{Name: "alpha", DeletedMemoryCount: 1}
	m := NewModelWithGuardedMemoryWorkflow(snapshot, workflow)
	m = sendKey(m, tea.KeyEnter)
	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'x')
	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'r')
	m = sendKey(m, tea.KeyEnter)
	assertContains(t, m.View(), "Restore reason is required")
	m = sendText(m, "undo cleanup")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "RESTORE memory 7")
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("restore submit command is nil")
	}
	updated, _ = updated.Update(command())
	m = updated.(Model)
	if len(workflow.requests) != 1 || workflow.requests[0].Reason != "undo cleanup" {
		t.Fatalf("restore requests = %#v, want one request with mandatory reason", workflow.requests)
	}
	if len(m.snapshot.Memories) != 1 || m.snapshot.Memories[0].Deleted || len(m.snapshot.DeletedMemories) != 0 {
		t.Fatalf("active/deleted slices = %#v / %#v, want restored target active only", m.snapshot.Memories, m.snapshot.DeletedMemories)
	}
	if len(m.snapshot.TimelineMemories) != 1 || m.snapshot.TimelineMemories[0].Deleted || m.snapshot.Projects[0].ActiveMemoryCount != 1 || m.snapshot.Projects[0].DeletedMemoryCount != 0 {
		t.Fatalf("timeline/project counts = %#v / %#v, want restored target and active 1 deleted 0", m.snapshot.TimelineMemories, m.snapshot.Projects[0])
	}
}
