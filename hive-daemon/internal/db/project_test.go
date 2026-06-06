package db_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

func TestKnownProjects_ReturnsDistinctProjectsFromWriteTables(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if err := d.CreateSession("sess-jarvis", "jarvis-dev", "/repo/jarvis-dev", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := d.SavePrompt(context.Background(), "prompt-only", "captured prompt"); err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}

	projects, err := d.KnownProjects(context.Background())
	if err != nil {
		t.Fatalf("KnownProjects: %v", err)
	}

	got := map[string]string{}
	for _, p := range projects {
		got[p.Name] = p.Directory
	}
	if got["jarvis-dev"] != "/repo/jarvis-dev" {
		t.Fatalf("jarvis-dev directory = %q, want /repo/jarvis-dev; all=%v", got["jarvis-dev"], got)
	}
	if _, ok := got["prompt-only"]; !ok {
		t.Fatalf("KnownProjects missing prompt-only project from user_prompts; all=%v", got)
	}
}

func TestSessionProject_ReturnsProjectForSession(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if err := d.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	projectName, err := d.SessionProject(context.Background(), "sess-alpha")
	if err != nil {
		t.Fatalf("SessionProject: %v", err)
	}
	if projectName != "alpha" {
		t.Fatalf("SessionProject = %q, want alpha", projectName)
	}
}

func TestGovernanceProjectReadModelsSummarizeLocalState(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if err := d.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession alpha: %v", err)
	}
	if err := d.CreateSession("sess-beta", "beta", "/repo/beta", "dev", "test"); err != nil {
		t.Fatalf("CreateSession beta: %v", err)
	}
	if _, err := d.SavePrompt(context.Background(), "alpha", "alpha prompt"); err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}
	activeID := saveGovernanceTestMemory(t, d, "alpha", "Active memory")
	deletedID := saveGovernanceTestMemory(t, d, "alpha", "Deleted memory")
	if err := d.DeleteMemory(deletedID, "tester", "duplicate"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	saveGovernanceTestMemory(t, d, "beta", "Beta memory")

	projects, err := d.ListGovernanceProjects(context.Background())
	if err != nil {
		t.Fatalf("ListGovernanceProjects: %v", err)
	}

	got := map[string]hivedb.GovernanceProject{}
	for _, p := range projects {
		got[p.Name] = p
	}
	alpha := got["alpha"]
	if alpha.Directory != "/repo/alpha" {
		t.Fatalf("alpha directory = %q, want /repo/alpha", alpha.Directory)
	}
	if alpha.ActiveMemoryCount != 1 || alpha.DeletedMemoryCount != 1 || alpha.SessionCount != 2 || alpha.PromptCount != 1 {
		t.Fatalf("alpha counts = active:%d deleted:%d sessions:%d prompts:%d", alpha.ActiveMemoryCount, alpha.DeletedMemoryCount, alpha.SessionCount, alpha.PromptCount)
	}
	if alpha.LastActivityAt.IsZero() {
		t.Fatal("alpha LastActivityAt should be populated")
	}
	if _, ok := got["beta"]; !ok {
		t.Fatalf("missing beta project; all=%v", got)
	}

	detail, err := d.GetGovernanceProject(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("GetGovernanceProject: %v", err)
	}
	if detail.Name != "alpha" || detail.ActiveMemoryCount != 1 || activeID == 0 {
		t.Fatalf("detail = %+v, activeID=%d", detail, activeID)
	}
	if _, err := d.GetGovernanceProject(context.Background(), "missing"); err == nil {
		t.Fatal("GetGovernanceProject should reject an unknown project")
	}
}

func TestGovernanceMemoryReadModelsRespectTombstoneFilter(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	activeID := saveGovernanceTestMemory(t, d, "alpha", "Active memory")
	deletedID := saveGovernanceTestMemory(t, d, "alpha", "Deleted memory")
	if err := d.DeleteMemory(deletedID, "tester", "duplicate"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	saveGovernanceTestMemory(t, d, "beta", "Other project")

	activeOnly, err := d.ListGovernanceMemories(context.Background(), hivedb.GovernanceMemoryFilter{Project: "alpha", Limit: 10})
	if err != nil {
		t.Fatalf("ListGovernanceMemories active: %v", err)
	}
	if len(activeOnly) != 1 || activeOnly[0].ID != activeID || activeOnly[0].Deleted {
		t.Fatalf("activeOnly = %+v, want only active id %d", activeOnly, activeID)
	}

	withDeleted, err := d.ListGovernanceMemories(context.Background(), hivedb.GovernanceMemoryFilter{Project: "alpha", IncludeDeleted: true, Limit: 10})
	if err != nil {
		t.Fatalf("ListGovernanceMemories with deleted: %v", err)
	}
	if len(withDeleted) != 2 {
		t.Fatalf("withDeleted len = %d, want 2; values=%+v", len(withDeleted), withDeleted)
	}
	var deleted hivedb.GovernanceMemory
	for _, memory := range withDeleted {
		if memory.ID == deletedID {
			deleted = memory
		}
	}
	if !deleted.Deleted || deleted.DeletedBy != "tester" || deleted.DeleteReason != "duplicate" || deleted.DeletedAt == nil || deleted.DeletedAt.IsZero() {
		t.Fatalf("deleted memory tombstone = %+v", deleted)
	}
	if activeOnly[0].DeletedAt != nil {
		t.Fatalf("active memory deleted timestamp = %v, want nil", activeOnly[0].DeletedAt)
	}
}

func TestGovernanceReadModelsDoNotMutateRecordsOrSyncState(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	saveGovernanceTestMemory(t, d, "alpha", "Active memory")
	if err := d.RecordSyncFailure("alpha", time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC), 2, time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC), sql.ErrConnDone); err != nil {
		t.Fatalf("RecordSyncFailure: %v", err)
	}
	before := readGovernanceCounters(t, d)

	if _, err := d.ListGovernanceProjects(context.Background()); err != nil {
		t.Fatalf("ListGovernanceProjects: %v", err)
	}
	if _, err := d.GetGovernanceProject(context.Background(), "alpha"); err != nil {
		t.Fatalf("GetGovernanceProject: %v", err)
	}
	if _, err := d.ListGovernanceMemories(context.Background(), hivedb.GovernanceMemoryFilter{Project: "alpha", IncludeDeleted: true, Limit: 10}); err != nil {
		t.Fatalf("ListGovernanceMemories: %v", err)
	}
	after := readGovernanceCounters(t, d)

	if before != after {
		t.Fatalf("read models mutated counters: before=%+v after=%+v", before, after)
	}
}

func saveGovernanceTestMemory(t *testing.T, d *hivedb.DB, projectName, title string) int64 {
	t.Helper()
	if _, err := d.EnsureManualSaveSession(projectName); err != nil {
		t.Fatalf("EnsureManualSaveSession: %v", err)
	}
	id, err := d.SaveMemory(&models.Memory{Project: projectName, Title: title, Content: "content", SessionID: "manual-save-" + projectName})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	return id
}

type governanceCounters struct {
	MemoryCount   int
	MutationCount int
	SyncRows      int
}

func readGovernanceCounters(t *testing.T, d *hivedb.DB) governanceCounters {
	t.Helper()
	var got governanceCounters
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&got.MemoryCount); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memory_mutations`).Scan(&got.MutationCount); err != nil {
		t.Fatalf("count mutations: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state`).Scan(&got.SyncRows); err != nil {
		t.Fatalf("count sync_state: %v", err)
	}
	return got
}
