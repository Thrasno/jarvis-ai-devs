package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func TestArchiveGovernanceProjectIsIdempotentAndPreservesFirstAuditMetadata(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	saveGovernanceTestMemory(t, d, "alpha", "Archived memory")
	if err := d.RecordSyncFailure("alpha", time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC), 2, time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC), sql.ErrConnDone); err != nil {
		t.Fatalf("RecordSyncFailure: %v", err)
	}
	firstArchivedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	secondArchivedAt := time.Date(2026, 6, 7, 13, 0, 0, 0, time.UTC)
	beforeCounters := readGovernanceCounters(t, d)
	beforeSyncState := readGovernanceSyncState(t, d, "alpha")

	firstMutated, err := d.ArchiveGovernanceProject(context.Background(), "alpha", "first-actor", "first reason", firstArchivedAt)
	if err != nil {
		t.Fatalf("ArchiveGovernanceProject first: %v", err)
	}
	if !firstMutated {
		t.Fatal("ArchiveGovernanceProject first mutated = false, want true")
	}
	secondMutated, err := d.ArchiveGovernanceProject(context.Background(), "alpha", "second-actor", "second reason", secondArchivedAt)
	if err != nil {
		t.Fatalf("ArchiveGovernanceProject second: %v", err)
	}
	if secondMutated {
		t.Fatal("ArchiveGovernanceProject second mutated = true, want false for idempotent retry")
	}

	detail, err := d.GetGovernanceProject(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("GetGovernanceProject: %v", err)
	}
	if !detail.Archived || detail.ArchivedAt == nil {
		t.Fatalf("project should be archived with timestamp: %+v", detail)
	}
	if !detail.ArchivedAt.Equal(firstArchivedAt) {
		t.Fatalf("ArchivedAt = %v, want first archive time %v", detail.ArchivedAt, firstArchivedAt)
	}
	if detail.ArchivedBy != "first-actor" || detail.ArchiveReason != "first reason" {
		t.Fatalf("archive audit metadata = by:%q reason:%q, want first actor/reason", detail.ArchivedBy, detail.ArchiveReason)
	}
	afterCounters := readGovernanceCounters(t, d)
	if afterCounters.MemoryCount != beforeCounters.MemoryCount || afterCounters.MutationCount != beforeCounters.MutationCount || afterCounters.SyncRows != beforeCounters.SyncRows {
		t.Fatalf("archive mutated memory/sync counters: before=%+v after=%+v", beforeCounters, afterCounters)
	}
	afterSyncState := readGovernanceSyncState(t, d, "alpha")
	if afterSyncState != beforeSyncState {
		t.Fatalf("archive mutated sync_state: before=%+v after=%+v", beforeSyncState, afterSyncState)
	}
}

func TestArchiveGovernanceProjectRejectsUnknownProjectWithoutMetadataMutation(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	saveGovernanceTestMemory(t, d, "alpha", "Known memory")
	beforeCounters := readGovernanceCounters(t, d)
	beforeGovernanceRows := readProjectGovernanceRows(t, d)

	mutated, err := d.ArchiveGovernanceProject(context.Background(), "missing", "actor", "reason", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))
	if mutated {
		t.Fatal("ArchiveGovernanceProject mutated unknown project, want false")
	}
	if !errors.Is(err, hivedb.ErrGovernanceProjectNotFound) {
		t.Fatalf("ArchiveGovernanceProject error = %v, want ErrGovernanceProjectNotFound", err)
	}
	afterCounters := readGovernanceCounters(t, d)
	if afterCounters != beforeCounters {
		t.Fatalf("unknown project archive mutated counters: before=%+v after=%+v", beforeCounters, afterCounters)
	}
	afterGovernanceRows := readProjectGovernanceRows(t, d)
	if afterGovernanceRows != beforeGovernanceRows {
		t.Fatalf("unknown project archive mutated governance metadata rows: before=%d after=%d", beforeGovernanceRows, afterGovernanceRows)
	}
}

func TestArchiveGovernanceProjectRejectsMergedProjectWithoutSilentNoop(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	seedGovernanceMergeProjects(t, d)
	mergeGovernanceProjectForTest(t, d, "alpha", "beta")
	before := readGovernanceSnapshot(t, d, "alpha")

	mutated, err := d.ArchiveGovernanceProject(context.Background(), "alpha", "archive-actor", "archive reason", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))

	if mutated {
		t.Fatal("ArchiveGovernanceProject mutated merged project, want false")
	}
	if !errors.Is(err, hivedb.ErrGovernanceProjectMergeConflict) {
		t.Fatalf("ArchiveGovernanceProject error = %v, want ErrGovernanceProjectMergeConflict", err)
	}
	requireGovernanceSnapshot(t, d, "alpha", before, "archive merged project")
}

func TestMergeGovernanceProjectIsIdempotentAndPreservesFirstAuditMetadata(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	sourceMemoryID, targetMemoryID := seedGovernanceMergeProjects(t, d)
	firstMergedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	beforeCounters := readGovernanceCounters(t, d)
	beforeSyncState := readGovernanceSyncState(t, d, "alpha")

	if mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "first-actor", "first reason", firstMergedAt); err != nil || !mutated {
		t.Fatalf("MergeGovernanceProject first mutated=%v err=%v, want true nil", mutated, err)
	}
	if mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "second-actor", "second reason", time.Date(2026, 6, 7, 13, 0, 0, 0, time.UTC)); err != nil || mutated {
		t.Fatalf("MergeGovernanceProject second mutated=%v err=%v, want false nil", mutated, err)
	}

	requireProjectMerged(t, d, "alpha", "beta", firstMergedAt)

	target, err := d.GetGovernanceProject(context.Background(), "beta")
	if err != nil {
		t.Fatalf("GetGovernanceProject target: %v", err)
	}
	if target.Merged || target.MergeTarget != "" {
		t.Fatalf("target project merge metadata = %+v, want unmerged", target)
	}
	if gotProject := requireMemoryProject(t, d, sourceMemoryID); gotProject != "alpha" {
		t.Fatalf("source memory project = %q, want alpha", gotProject)
	}
	if gotProject := requireMemoryProject(t, d, targetMemoryID); gotProject != "beta" {
		t.Fatalf("target memory project = %q, want beta", gotProject)
	}
	if afterCounters := readGovernanceCounters(t, d); afterCounters != beforeCounters {
		t.Fatalf("merge mutated memory/sync counters: before=%+v after=%+v", beforeCounters, afterCounters)
	}
	if afterSyncState := readGovernanceSyncState(t, d, "alpha"); afterSyncState != beforeSyncState {
		t.Fatalf("merge mutated sync_state: before=%+v after=%+v", beforeSyncState, afterSyncState)
	}
}

func TestMergeGovernanceProjectRetryIgnoresCurrentTargetLifecycle(t *testing.T) {
	t.Parallel()

	for name, setup := range map[string]func(*testing.T, *hivedb.DB){
		"target later merged":   func(t *testing.T, d *hivedb.DB) { mergeGovernanceProjectForTest(t, d, "beta", "gamma") },
		"target later archived": func(t *testing.T, d *hivedb.DB) { archiveGovernanceProjectForTest(t, d, "beta") },
	} {
		t.Run(name, func(t *testing.T) {
			d := openGovernanceTestDB(t)
			seedGovernanceMergeProjects(t, d)
			firstMergedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
			if mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "first-actor", "first reason", firstMergedAt); err != nil || !mutated {
				t.Fatalf("MergeGovernanceProject alpha->beta setup mutated=%v err=%v, want true nil", mutated, err)
			}
			setup(t, d)
			before := readGovernanceSnapshot(t, d, "alpha")
			mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "retry-actor", "retry reason", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))

			if err != nil {
				t.Fatalf("MergeGovernanceProject retry: %v", err)
			}
			if mutated {
				t.Fatal("MergeGovernanceProject retry mutated = true, want false")
			}
			requireProjectMerged(t, d, "alpha", "beta", firstMergedAt)
			requireGovernanceSnapshot(t, d, "alpha", before, "merge retry")
		})
	}
}

func TestMergeGovernanceProjectRejectsInvalidProjectsWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		setup   func(*testing.T, *hivedb.DB)
		source  string
		target  string
		wantErr error
	}{
		{name: "missing source", source: "missing", target: "beta", wantErr: hivedb.ErrGovernanceProjectNotFound},
		{name: "missing target", source: "alpha", target: "missing", wantErr: hivedb.ErrGovernanceProjectNotFound},
		{name: "same source and target", source: "alpha", target: "alpha", wantErr: hivedb.ErrGovernanceProjectMergeInvalid},
		{name: "archived source", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectArchived, setup: func(t *testing.T, d *hivedb.DB) { archiveGovernanceProjectForTest(t, d, "alpha") }},
		{name: "archived target", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectArchived, setup: func(t *testing.T, d *hivedb.DB) { archiveGovernanceProjectForTest(t, d, "beta") }},
		{name: "target already merged", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectMergeConflict, setup: func(t *testing.T, d *hivedb.DB) { mergeGovernanceProjectForTest(t, d, "beta", "gamma") }},
		{name: "source already merged into different target", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectMergeConflict, setup: func(t *testing.T, d *hivedb.DB) { mergeGovernanceProjectForTest(t, d, "alpha", "gamma") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openGovernanceTestDB(t)

			seedGovernanceMergeProjects(t, d)
			if tt.setup != nil {
				tt.setup(t, d)
			}

			before := readGovernanceSnapshot(t, d, "alpha")

			mutated, err := d.MergeGovernanceProject(context.Background(), tt.source, tt.target, "tester", "new", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))
			if mutated {
				t.Fatal("MergeGovernanceProject mutated invalid merge, want false")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MergeGovernanceProject error = %v, want %v", err, tt.wantErr)
			}
			requireGovernanceSnapshot(t, d, "alpha", before, "invalid merge")
		})
	}
}

func openGovernanceTestDB(t *testing.T) *hivedb.DB {
	t.Helper()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func seedGovernanceMergeProjects(t *testing.T, d *hivedb.DB) (int64, int64) {
	t.Helper()
	sourceID := saveGovernanceTestMemory(t, d, "alpha", "Alpha memory")
	targetID := saveGovernanceTestMemory(t, d, "beta", "Beta memory")
	saveGovernanceTestMemory(t, d, "gamma", "Gamma memory")
	if err := d.RecordSyncFailure("alpha", time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC), 2, time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC), sql.ErrConnDone); err != nil {
		t.Fatalf("RecordSyncFailure: %v", err)
	}
	return sourceID, targetID
}

func archiveGovernanceProjectForTest(t *testing.T, d *hivedb.DB, project string) {
	t.Helper()
	if _, err := d.ArchiveGovernanceProject(context.Background(), project, "actor", "old", time.Date(2026, 6, 7, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ArchiveGovernanceProject setup: %v", err)
	}
}

func mergeGovernanceProjectForTest(t *testing.T, d *hivedb.DB, source, target string) {
	t.Helper()
	if _, err := d.MergeGovernanceProject(context.Background(), source, target, "actor", "old", time.Date(2026, 6, 7, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MergeGovernanceProject setup: %v", err)
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

func requireProjectMerged(t *testing.T, d *hivedb.DB, project, target string, mergedAt time.Time) {
	t.Helper()
	detail, err := d.GetGovernanceProject(context.Background(), project)
	if err != nil {
		t.Fatalf("GetGovernanceProject %s: %v", project, err)
	}
	if !detail.Merged || detail.MergeTarget != target || detail.MergedAt == nil || !detail.MergedAt.Equal(mergedAt) || detail.MergedBy != "first-actor" || detail.MergeReason != "first reason" {
		t.Fatalf("project merge metadata = %+v, want %s->%s first audit", detail, project, target)
	}
}

func requireMemoryProject(t *testing.T, d *hivedb.DB, id int64) string {
	t.Helper()
	var project string
	if err := d.RawDB().QueryRow(`SELECT project FROM memories WHERE id = ?`, id).Scan(&project); err != nil {
		t.Fatalf("read memory project: %v", err)
	}
	return project
}

type governanceCounters struct {
	MemoryCount   int
	MutationCount int
	SyncRows      int
}

type governanceSyncState struct {
	LastAttemptAt       string
	LastSuccessAt       string
	LastFailureAt       string
	ConsecutiveFailures int
	BackoffUntil        string
	LastError           string
	LastSyncAt          string
}

func readGovernanceSnapshot(t *testing.T, d *hivedb.DB, project string) string {
	t.Helper()
	detail, err := d.GetGovernanceProject(context.Background(), project)
	if err != nil {
		t.Fatalf("GetGovernanceProject snapshot: %v", err)
	}
	return fmt.Sprintf("%+v|%t|%v|%s|%s|%t|%s|%v|%s|%s|%+v", readGovernanceCounters(t, d), detail.Archived, detail.ArchivedAt, detail.ArchivedBy, detail.ArchiveReason, detail.Merged, detail.MergeTarget, detail.MergedAt, detail.MergedBy, detail.MergeReason, readGovernanceSyncState(t, d, project))
}

func requireGovernanceSnapshot(t *testing.T, d *hivedb.DB, project string, before, label string) {
	t.Helper()
	if after := readGovernanceSnapshot(t, d, project); after != before {
		t.Fatalf("%s mutated governance state: before=%+v after=%+v", label, before, after)
	}
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

func readGovernanceSyncState(t *testing.T, d *hivedb.DB, project string) governanceSyncState {
	t.Helper()
	var got governanceSyncState
	if err := d.RawDB().QueryRow(`
SELECT COALESCE(last_attempt_at, ''), COALESCE(last_success_at, ''), COALESCE(last_failure_at, ''),
       consecutive_failures, COALESCE(backoff_until, ''), last_error, COALESCE(last_sync_at, '')
FROM sync_state
WHERE project = ?`, project).Scan(&got.LastAttemptAt, &got.LastSuccessAt, &got.LastFailureAt, &got.ConsecutiveFailures, &got.BackoffUntil, &got.LastError, &got.LastSyncAt); err != nil {
		t.Fatalf("read sync_state: %v", err)
	}
	return got
}

func readProjectGovernanceRows(t *testing.T, d *hivedb.DB) int {
	t.Helper()
	var rows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance`).Scan(&rows); err != nil {
		t.Fatalf("count project governance rows: %v", err)
	}
	return rows
}

// Task 1.3 — GetGovernanceMemoryByID + ErrGovernanceMemoryNotFound

func TestGetGovernanceMemoryByID_Found(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	if _, err := d.EnsureManualSaveSession("proj-a"); err != nil {
		t.Fatalf("EnsureManualSaveSession: %v", err)
	}
	id, err := d.SaveMemory(&models.Memory{
		Project:   "proj-a",
		Title:     "test memory",
		Content:   "rich content here",
		SessionID: "manual-save-proj-a",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	got, err := d.GetGovernanceMemoryByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetGovernanceMemoryByID: %v", err)
	}
	if got.ID != id {
		t.Fatalf("ID = %d, want %d", got.ID, id)
	}
	if got.Content != "rich content here" {
		t.Fatalf("Content = %q, want %q", got.Content, "rich content here")
	}
	if got.Title != "test memory" {
		t.Fatalf("Title = %q, want %q", got.Title, "test memory")
	}
}

func TestGetGovernanceMemoryByID_NotFound(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)

	_, err := d.GetGovernanceMemoryByID(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for missing memory, got nil")
	}
	if !errors.Is(err, hivedb.ErrGovernanceMemoryNotFound) {
		t.Fatalf("error = %v, want ErrGovernanceMemoryNotFound", err)
	}
}

func TestGetGovernanceMemoryByID_EmptyContent(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	if err := d.CreateSession("sess-proj-b", "proj-b", "/repo/proj-b", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Insert directly to bypass domain validation (empty content is allowed at DB level)
	var id int64
	err := d.RawDB().QueryRow(`
INSERT INTO memories (sync_id, project, topic_key, category, title, content, created_by, session_id, created_at, updated_at)
VALUES ('sync-empty', 'proj-b', NULL, 'manual', 'empty content memory', '', 'tester', 'sess-proj-b', '2026-06-08 10:00:00', '2026-06-08 10:00:00')
RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert empty content memory: %v", err)
	}

	got, err := d.GetGovernanceMemoryByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetGovernanceMemoryByID: %v", err)
	}
	if got.Content != "" {
		t.Fatalf("Content = %q, want empty string", got.Content)
	}
}

func TestGetGovernanceMemoryByID_ExcludesDeletedMemories(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	id := saveGovernanceTestMemory(t, d, "proj-c", "deleted mem")
	if err := d.DeleteMemory(id, "tester", "stale"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}

	_, err := d.GetGovernanceMemoryByID(context.Background(), id)
	if err == nil {
		t.Fatal("expected error for deleted memory, got nil")
	}
	if !errors.Is(err, hivedb.ErrGovernanceMemoryNotFound) {
		t.Fatalf("error = %v, want ErrGovernanceMemoryNotFound", err)
	}
}

// Task 1.2 — Content in GovernanceMemory list query

func TestListGovernanceMemories_ContentPopulated(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)

	if _, err := d.EnsureManualSaveSession("proj-a"); err != nil {
		t.Fatalf("EnsureManualSaveSession: %v", err)
	}
	id, err := d.SaveMemory(&models.Memory{
		Project:   "proj-a",
		Title:     "mem with content",
		Content:   "this is the content body",
		SessionID: "manual-save-proj-a",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if id == 0 {
		t.Fatal("SaveMemory returned 0 id")
	}

	memories, err := d.ListGovernanceMemories(context.Background(), hivedb.GovernanceMemoryFilter{Project: "proj-a", Limit: 10})
	if err != nil {
		t.Fatalf("ListGovernanceMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}
	if memories[0].Content != "this is the content body" {
		t.Fatalf("Content = %q, want %q", memories[0].Content, "this is the content body")
	}
}

// Task 1.1 — UnsyncedCount CTE

func TestListGovernanceProjects_UnsyncedCount(t *testing.T) {
	t.Parallel()

	t.Run("project with unsynced memories", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		// Save 3 memories — all unsynced (synced_at IS NULL)
		saveGovernanceTestMemory(t, d, "proj-a", "mem1")
		saveGovernanceTestMemory(t, d, "proj-a", "mem2")
		saveGovernanceTestMemory(t, d, "proj-a", "mem3")

		projects, err := d.ListGovernanceProjects(context.Background())
		if err != nil {
			t.Fatalf("ListGovernanceProjects: %v", err)
		}
		got := map[string]hivedb.GovernanceProject{}
		for _, p := range projects {
			got[p.Name] = p
		}
		if got["proj-a"].UnsyncedCount != 3 {
			t.Fatalf("proj-a UnsyncedCount = %d, want 3", got["proj-a"].UnsyncedCount)
		}
	})

	t.Run("project with no unsynced memories", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		id := saveGovernanceTestMemory(t, d, "proj-b", "synced mem")
		// Mark as synced by setting synced_at
		if _, err := d.RawDB().Exec(`UPDATE memories SET synced_at = '2026-06-08 10:00:00' WHERE id = ?`, id); err != nil {
			t.Fatalf("mark synced: %v", err)
		}

		projects, err := d.ListGovernanceProjects(context.Background())
		if err != nil {
			t.Fatalf("ListGovernanceProjects: %v", err)
		}
		got := map[string]hivedb.GovernanceProject{}
		for _, p := range projects {
			got[p.Name] = p
		}
		if got["proj-b"].UnsyncedCount != 0 {
			t.Fatalf("proj-b UnsyncedCount = %d, want 0", got["proj-b"].UnsyncedCount)
		}
	})

	t.Run("project with no memories at all", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		// Create a project via session only (no memories)
		if err := d.CreateSession("sess-empty", "proj-empty", "/repo/empty", "dev", "test"); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		projects, err := d.ListGovernanceProjects(context.Background())
		if err != nil {
			t.Fatalf("ListGovernanceProjects: %v", err)
		}
		got := map[string]hivedb.GovernanceProject{}
		for _, p := range projects {
			got[p.Name] = p
		}
		if got["proj-empty"].UnsyncedCount != 0 {
			t.Fatalf("proj-empty UnsyncedCount = %d, want 0", got["proj-empty"].UnsyncedCount)
		}
	})

	t.Run("soft-deleted memories excluded from unsynced count", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		id := saveGovernanceTestMemory(t, d, "proj-c", "deleted mem")
		if err := d.DeleteMemory(id, "tester", "stale"); err != nil {
			t.Fatalf("DeleteMemory: %v", err)
		}

		projects, err := d.ListGovernanceProjects(context.Background())
		if err != nil {
			t.Fatalf("ListGovernanceProjects: %v", err)
		}
		got := map[string]hivedb.GovernanceProject{}
		for _, p := range projects {
			got[p.Name] = p
		}
		if got["proj-c"].UnsyncedCount != 0 {
			t.Fatalf("proj-c UnsyncedCount = %d, want 0 (soft-deleted excluded)", got["proj-c"].UnsyncedCount)
		}
	})
}
