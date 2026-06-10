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
	// After physical migration, alpha has no rows. Capture governance state directly.
	beforeGov := readGovernanceRowSnapshot(t, d, "alpha")

	// ArchiveGovernanceProject must detect the merged state via governance record
	// and return ErrGovernanceProjectMergeConflict even though alpha has no rows.
	mutated, err := d.ArchiveGovernanceProject(context.Background(), "alpha", "archive-actor", "archive reason", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))

	if mutated {
		t.Fatal("ArchiveGovernanceProject mutated merged project, want false")
	}
	if !errors.Is(err, hivedb.ErrGovernanceProjectMergeConflict) {
		t.Fatalf("ArchiveGovernanceProject error = %v, want ErrGovernanceProjectMergeConflict", err)
	}
	afterGov := readGovernanceRowSnapshot(t, d, "alpha")
	if afterGov != beforeGov {
		t.Fatalf("archive merged project changed governance row: before=%q after=%q", beforeGov, afterGov)
	}
}

func TestMergeGovernanceProjectIsIdempotentAndPreservesFirstAuditMetadata(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	sourceMemoryID, targetMemoryID := seedGovernanceMergeProjects(t, d)
	firstMergedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	beforeCounters := readGovernanceCounters(t, d)

	if mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "first-actor", "first reason", firstMergedAt); err != nil || !mutated {
		t.Fatalf("MergeGovernanceProject first mutated=%v err=%v, want true nil", mutated, err)
	}
	if mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "second-actor", "second reason", time.Date(2026, 6, 7, 13, 0, 0, 0, time.UTC)); err != nil || mutated {
		t.Fatalf("MergeGovernanceProject second mutated=%v err=%v, want false nil", mutated, err)
	}

	// Verify via governance row directly (alpha has no rows after physical migration).
	requireGovernanceMergedRow(t, d, "alpha", "beta", firstMergedAt)

	target, err := d.GetGovernanceProject(context.Background(), "beta")
	if err != nil {
		t.Fatalf("GetGovernanceProject target: %v", err)
	}
	if target.Merged || target.MergeTarget != "" {
		t.Fatalf("target project merge metadata = %+v, want unmerged", target)
	}
	// Physical migration: source memory is now under the target project.
	if gotProject := requireMemoryProject(t, d, sourceMemoryID); gotProject != "beta" {
		t.Fatalf("source memory project = %q, want beta (physical migration)", gotProject)
	}
	if gotProject := requireMemoryProject(t, d, targetMemoryID); gotProject != "beta" {
		t.Fatalf("target memory project = %q, want beta", gotProject)
	}
	// Memory and mutation counts are preserved (rows moved, not deleted).
	afterCounters := readGovernanceCounters(t, d)
	if afterCounters.MemoryCount != beforeCounters.MemoryCount {
		t.Fatalf("merge changed memory count: before=%d after=%d", beforeCounters.MemoryCount, afterCounters.MemoryCount)
	}
	if afterCounters.MutationCount != beforeCounters.MutationCount {
		t.Fatalf("merge changed mutation count: before=%d after=%d", beforeCounters.MutationCount, afterCounters.MutationCount)
	}
	// sync_state for alpha must be deleted after physical merge.
	var alphaSyncRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = 'alpha'`).Scan(&alphaSyncRows); err != nil {
		t.Fatalf("check alpha sync_state: %v", err)
	}
	if alphaSyncRows != 0 {
		t.Fatal("sync_state for alpha must be deleted after merge")
	}
}

func TestMergeGovernanceProjectRetryIgnoresCurrentTargetLifecycle(t *testing.T) {
	t.Parallel()

	for name, setup := range map[string]func(*testing.T, *hivedb.DB){
		// "beta later merged" scenario: simulate beta→gamma by writing the governance
		// row directly, bypassing addAliasTx. The alias chain guard (ErrAliasSourceIsTarget)
		// now rejects merging a project that is already a target — so the product path
		// is invalid — but we still need to verify the retry idempotency logic when
		// the target's lifecycle changes via a raw governance row (e.g. migrated from
		// a pre-guard state or via a back-door administrative operation).
		"target later merged": func(t *testing.T, d *hivedb.DB) {
			t.Helper()
			_, err := d.RawDB().Exec(`
INSERT INTO hive_project_governance (project, merge_target, merged_at, merged_by, merge_reason)
VALUES ('beta', 'gamma', '2026-06-07 11:00:00', 'actor', 'old')
ON CONFLICT(project) DO UPDATE SET
    merge_target = excluded.merge_target,
    merged_at    = excluded.merged_at,
    merged_by    = excluded.merged_by,
    merge_reason = excluded.merge_reason`)
			if err != nil {
				t.Fatalf("raw insert beta->gamma governance: %v", err)
			}
		},
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
			// Capture governance state before retry (alpha rows are already in beta,
			// so use a direct governance-row snapshot instead of GetGovernanceProject).
			beforeGov := readGovernanceRowSnapshot(t, d, "alpha")

			mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "retry-actor", "retry reason", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))

			if err != nil {
				t.Fatalf("MergeGovernanceProject retry: %v", err)
			}
			if mutated {
				t.Fatal("MergeGovernanceProject retry mutated = true, want false")
			}
			requireGovernanceMergedRow(t, d, "alpha", "beta", firstMergedAt)
			afterGov := readGovernanceRowSnapshot(t, d, "alpha")
			if afterGov != beforeGov {
				t.Fatalf("merge retry changed governance row: before=%q after=%q", beforeGov, afterGov)
			}
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
		// "missing target" removed: target existence is no longer required (physical migration
		// creates the target implicitly). A merge to a new project name now succeeds.
		{name: "same source and target", source: "alpha", target: "alpha", wantErr: hivedb.ErrGovernanceProjectMergeInvalid},
		{name: "archived source", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectArchived, setup: func(t *testing.T, d *hivedb.DB) { archiveGovernanceProjectForTest(t, d, "alpha") }},
		{name: "archived target", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectArchived, setup: func(t *testing.T, d *hivedb.DB) { archiveGovernanceProjectForTest(t, d, "beta") }},
		{name: "target already merged", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectMergeConflict, setup: func(t *testing.T, d *hivedb.DB) { mergeGovernanceProjectForTest(t, d, "beta", "gamma") }},
		{name: "source already merged into different target", source: "alpha", target: "beta", wantErr: hivedb.ErrGovernanceProjectMergeConflict, setup: func(t *testing.T, d *hivedb.DB) { mergeGovernanceProjectForTest(t, d, "alpha", "gamma") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openGovernanceTestDB(t)

			seedGovernanceMergeProjects(t, d)

			// Capture snapshot before setup for cases where setup physically moves rows.
			// For "source already merged into different target", setup migrates alpha's rows
			// to gamma, so we must read the governance counters before setup runs.
			beforeCounters := readGovernanceCounters(t, d)

			if tt.setup != nil {
				tt.setup(t, d)
			}

			mutated, err := d.MergeGovernanceProject(context.Background(), tt.source, tt.target, "tester", "new", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC))
			if mutated {
				t.Fatal("MergeGovernanceProject mutated invalid merge, want false")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MergeGovernanceProject error = %v, want %v", err, tt.wantErr)
			}
			// Verify no additional DML happened (counters unchanged from pre-test state).
			afterCounters := readGovernanceCounters(t, d)
			if afterCounters.MemoryCount != beforeCounters.MemoryCount {
				t.Fatalf("invalid merge changed memory count: before=%d after=%d", beforeCounters.MemoryCount, afterCounters.MemoryCount)
			}
			if afterCounters.MutationCount != beforeCounters.MutationCount {
				t.Fatalf("invalid merge changed mutation count: before=%d after=%d", beforeCounters.MutationCount, afterCounters.MutationCount)
			}
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

// readGovernanceRowSnapshot returns a string snapshot of the governance record for
// a project. Unlike readGovernanceSnapshot, it reads directly from
// hive_project_governance and does not require rows in memories/sessions/user_prompts.
func readGovernanceRowSnapshot(t *testing.T, d *hivedb.DB, project string) string {
	t.Helper()
	var mergeTarget, mergedAt, mergedBy, mergeReason string
	err := d.RawDB().QueryRow(`
SELECT COALESCE(merge_target,''), COALESCE(merged_at,''), COALESCE(merged_by,''), COALESCE(merge_reason,'')
FROM hive_project_governance WHERE project = ?`, project).Scan(&mergeTarget, &mergedAt, &mergedBy, &mergeReason)
	if err != nil {
		t.Fatalf("readGovernanceRowSnapshot %s: %v", project, err)
	}
	return fmt.Sprintf("target=%s|at=%s|by=%s|reason=%s", mergeTarget, mergedAt, mergedBy, mergeReason)
}

// requireGovernanceMergedRow verifies the governance record for source directly
// without requiring rows in write tables (useful after physical migration).
func requireGovernanceMergedRow(t *testing.T, d *hivedb.DB, source, target string, mergedAt time.Time) {
	t.Helper()
	var gotTarget, gotMergedAt, gotMergedBy string
	err := d.RawDB().QueryRow(`
SELECT COALESCE(merge_target,''), COALESCE(merged_at,''), COALESCE(merged_by,'')
FROM hive_project_governance WHERE project = ?`, source).Scan(&gotTarget, &gotMergedAt, &gotMergedBy)
	if err != nil {
		t.Fatalf("requireGovernanceMergedRow %s: %v", source, err)
	}
	if gotTarget != target {
		t.Fatalf("governance merge_target = %q, want %q", gotTarget, target)
	}
	wantAt := mergedAt.UTC().Format("2006-01-02 15:04:05")
	if gotMergedAt != wantAt {
		t.Fatalf("governance merged_at = %q, want %q", gotMergedAt, wantAt)
	}
	if gotMergedBy != "first-actor" {
		t.Fatalf("governance merged_by = %q, want first-actor", gotMergedBy)
	}
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

// TestMergeGovernanceProject_CreatesAlias verifies that MergeGovernanceProject
// creates a persistent local alias atomically with the governance record.
func TestMergeGovernanceProject_CreatesAlias(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	seedGovernanceMergeProjects(t, d)

	if mutated, err := d.MergeGovernanceProject(context.Background(), "alpha", "beta", "actor", "merge test", time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)); err != nil || !mutated {
		t.Fatalf("MergeGovernanceProject mutated=%v err=%v, want true nil", mutated, err)
	}

	target, found, err := d.ResolveAlias(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("ResolveAlias after merge: %v", err)
	}
	if !found {
		t.Fatal("expected alias found=true after merge")
	}
	if target != "beta" {
		t.Fatalf("alias target = %q, want beta", target)
	}
}

// TestMergeGovernanceProject_RollsBackWhenAddAliasTxFails verifies that when
// addAliasTx rejects the alias (because the source is already a target in an
// existing alias), the deferred rollback reverts the governance record too —
// no partial state is left behind.
func TestMergeGovernanceProject_RollsBackWhenAddAliasTxFails(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	// Seed projects A, B, and X so the governance lookup succeeds.
	saveGovernanceTestMemory(t, d, "A", "A memory")
	saveGovernanceTestMemory(t, d, "B", "B memory")
	saveGovernanceTestMemory(t, d, "X", "X memory")

	// Seed alias X→A so that A is already a target.
	// When MergeGovernanceProject("A", "B", ...) runs, addAliasTx("A", "B", ...)
	// will hit the bidirectional chain guard and return ErrAliasSourceIsTarget.
	seedAlias(t, d, "X", "A")

	_, err := d.MergeGovernanceProject(context.Background(), "A", "B", "actor", "test rollback", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("MergeGovernanceProject: expected error due to alias chain guard, got nil")
	}
	if !errors.Is(err, hivedb.ErrAliasSourceIsTarget) {
		t.Fatalf("expected ErrAliasSourceIsTarget wrapped in merge error, got: %v", err)
	}

	// Governance record for A must not exist (rollback must have reverted it).
	var governanceRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance WHERE project = 'A'`).Scan(&governanceRows); err != nil {
		t.Fatalf("count governance rows for A: %v", err)
	}
	if governanceRows != 0 {
		t.Fatalf("hive_project_governance has %d row(s) for A after rollback, want 0", governanceRows)
	}

	// Alias A→B must not exist (guard fired before insert, and tx rolled back).
	_, found, resolveErr := d.ResolveAlias(context.Background(), "A")
	if resolveErr != nil {
		t.Fatalf("ResolveAlias A: %v", resolveErr)
	}
	if found {
		t.Fatal("alias A→B must not exist after rollback")
	}
}

// ─── Phase 1: Physical Row Migration Tests ───────────────────────────────────

// TestMergeGovernanceProject_PhysicalRowMigration verifies that MergeGovernanceProject
// migrates all rows from source to target in memories, user_prompts, and sessions.
func TestMergeGovernanceProject_PhysicalRowMigration(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	// Seed source rows in all three tables.
	srcMemID := saveGovernanceTestMemory(t, d, "src", "Source memory 1")
	saveGovernanceTestMemory(t, d, "src", "Source memory 2")
	if err := d.CreateSession("sess-src-1", "src", "/repo/src", "dev", "test"); err != nil {
		t.Fatalf("CreateSession src: %v", err)
	}
	if _, err := d.SavePrompt(context.Background(), "src", "source prompt"); err != nil {
		t.Fatalf("SavePrompt src: %v", err)
	}
	// Seed a target row too (existing target scenario).
	saveGovernanceTestMemory(t, d, "dst", "Target existing memory")

	mutated, err := d.MergeGovernanceProject(context.Background(), "src", "dst", "actor", "consolidate", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MergeGovernanceProject: %v", err)
	}
	if !mutated {
		t.Fatal("MergeGovernanceProject mutated = false, want true")
	}

	// All source memories must now belong to dst.
	if got := requireMemoryProject(t, d, srcMemID); got != "dst" {
		t.Fatalf("source memory project = %q, want dst", got)
	}
	// No row should retain project = src in memories.
	var srcMemCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'src'`).Scan(&srcMemCount); err != nil {
		t.Fatalf("count src memories: %v", err)
	}
	if srcMemCount != 0 {
		t.Fatalf("memories with project=src after merge: %d, want 0", srcMemCount)
	}
	// No session should retain project = src.
	var srcSessCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE project = 'src'`).Scan(&srcSessCount); err != nil {
		t.Fatalf("count src sessions: %v", err)
	}
	if srcSessCount != 0 {
		t.Fatalf("sessions with project=src after merge: %d, want 0", srcSessCount)
	}
	// No user_prompt should retain project = src.
	var srcPromptCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM user_prompts WHERE project = 'src'`).Scan(&srcPromptCount); err != nil {
		t.Fatalf("count src user_prompts: %v", err)
	}
	if srcPromptCount != 0 {
		t.Fatalf("user_prompts with project=src after merge: %d, want 0", srcPromptCount)
	}
}

// TestMergeGovernanceProject_NewTarget verifies that a merge succeeds when the
// target project does not yet have any rows (target created implicitly by UPDATE).
func TestMergeGovernanceProject_NewTarget(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	// Only seed source — no target rows at all.
	saveGovernanceTestMemory(t, d, "src-new", "Source memory for new target")
	if err := d.CreateSession("sess-src-new", "src-new", "/repo/src-new", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	mutated, err := d.MergeGovernanceProject(context.Background(), "src-new", "brand-new-target", "actor", "new target", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MergeGovernanceProject to new target: %v", err)
	}
	if !mutated {
		t.Fatal("MergeGovernanceProject mutated = false, want true")
	}

	// Rows should now be under the new target.
	var dstCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'brand-new-target'`).Scan(&dstCount); err != nil {
		t.Fatalf("count dst memories: %v", err)
	}
	if dstCount == 0 {
		t.Fatal("no memories found under brand-new-target after merge")
	}
	var srcCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'src-new'`).Scan(&srcCount); err != nil {
		t.Fatalf("count src memories: %v", err)
	}
	if srcCount != 0 {
		t.Fatalf("memories still under src-new after merge to new target: %d", srcCount)
	}
}

// TestMergeGovernanceProject_PendingMutationsMigrated verifies that only
// memory_mutations with synced_at IS NULL are migrated; synced ones are unchanged.
func TestMergeGovernanceProject_PendingMutationsMigrated(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	saveGovernanceTestMemory(t, d, "src-mut", "src memory for mutations")
	saveGovernanceTestMemory(t, d, "dst-mut", "dst memory")

	// Insert a pending mutation (synced_at IS NULL) for src-mut.
	_, err := d.RawDB().Exec(`
INSERT INTO memory_mutations (event_id, entity_sync_id, project, op, occurred_at)
VALUES ('evt-pending-mut', 'sync-pending-mut', 'src-mut', 'save', '2026-06-10 10:00:00')`)
	if err != nil {
		t.Fatalf("insert pending mutation: %v", err)
	}
	// Insert a synced mutation (synced_at IS NOT NULL) for src-mut.
	_, err = d.RawDB().Exec(`
INSERT INTO memory_mutations (event_id, entity_sync_id, project, op, occurred_at, synced_at)
VALUES ('evt-synced-mut', 'sync-synced-mut', 'src-mut', 'save', '2026-06-10 09:00:00', '2026-06-10 09:05:00')`)
	if err != nil {
		t.Fatalf("insert synced mutation: %v", err)
	}

	if _, err := d.MergeGovernanceProject(context.Background(), "src-mut", "dst-mut", "actor", "reason", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MergeGovernanceProject: %v", err)
	}

	// Pending mutation must now belong to dst-mut.
	var pendingProject string
	if err := d.RawDB().QueryRow(`SELECT project FROM memory_mutations WHERE event_id = 'evt-pending-mut'`).Scan(&pendingProject); err != nil {
		t.Fatalf("read pending mutation project: %v", err)
	}
	if pendingProject != "dst-mut" {
		t.Fatalf("pending mutation project = %q, want dst-mut", pendingProject)
	}
	// Synced mutation must remain on src-mut.
	var syncedProject string
	if err := d.RawDB().QueryRow(`SELECT project FROM memory_mutations WHERE event_id = 'evt-synced-mut'`).Scan(&syncedProject); err != nil {
		t.Fatalf("read synced mutation project: %v", err)
	}
	if syncedProject != "src-mut" {
		t.Fatalf("synced mutation project = %q, want src-mut (must not migrate)", syncedProject)
	}
}

// TestMergeGovernanceProject_WarningsAndSyncStateDeleted verifies that hive_warnings
// and sync_state rows for the source are deleted, but the __auth__ row is preserved.
func TestMergeGovernanceProject_WarningsAndSyncStateDeleted(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	saveGovernanceTestMemory(t, d, "src-ws", "src memory for cleanup test")
	saveGovernanceTestMemory(t, d, "dst-ws", "dst memory")

	// Insert a warning for src-ws.
	if _, err := d.RawDB().Exec(`
INSERT INTO hive_warnings (severity, source, message) VALUES ('warn', 'src-ws', 'test warning')`); err != nil {
		t.Fatalf("insert hive_warning: %v", err)
	}
	// Insert sync_state for src-ws.
	if err := d.RecordSyncFailure("src-ws", time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC), 1, time.Date(2026, 6, 10, 10, 1, 0, 0, time.UTC), fmt.Errorf("test failure")); err != nil {
		t.Fatalf("RecordSyncFailure src-ws: %v", err)
	}
	// Insert __auth__ sync_state row (must survive).
	if _, err := d.RawDB().Exec(`
INSERT OR IGNORE INTO sync_state (project, last_error) VALUES ('__auth__', '')`); err != nil {
		t.Fatalf("insert __auth__ sync_state: %v", err)
	}

	if _, err := d.MergeGovernanceProject(context.Background(), "src-ws", "dst-ws", "actor", "reason", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("MergeGovernanceProject: %v", err)
	}

	// hive_warnings for src-ws must be deleted.
	var warnCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_warnings WHERE source = 'src-ws'`).Scan(&warnCount); err != nil {
		t.Fatalf("count hive_warnings: %v", err)
	}
	if warnCount != 0 {
		t.Fatalf("hive_warnings for src-ws after merge: %d, want 0", warnCount)
	}
	// sync_state for src-ws must be deleted.
	var srcSyncCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = 'src-ws'`).Scan(&srcSyncCount); err != nil {
		t.Fatalf("count sync_state src-ws: %v", err)
	}
	if srcSyncCount != 0 {
		t.Fatalf("sync_state for src-ws after merge: %d, want 0", srcSyncCount)
	}
	// __auth__ row must NOT be deleted.
	var authCount int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = '__auth__'`).Scan(&authCount); err != nil {
		t.Fatalf("count __auth__ sync_state: %v", err)
	}
	if authCount != 1 {
		t.Fatalf("__auth__ sync_state count after merge: %d, want 1", authCount)
	}
}

// TestMergeGovernanceProject_RollbackOnFailure verifies that a forced mid-tx failure
// rolls back all changes, leaving the source rows intact.
// We use the alias chain guard (ErrAliasSourceIsTarget) to trigger a failure
// inside the transaction, then verify source state is untouched.
func TestMergeGovernanceProject_RollbackOnFailure(t *testing.T) {
	t.Parallel()

	d := openGovernanceTestDB(t)
	saveGovernanceTestMemory(t, d, "src-rb", "rollback memory")
	// Pre-seed: X→src-rb alias so src-rb is already a TARGET.
	// When we try to merge src-rb→beta-rb, addAliasTx fires ErrAliasSourceIsTarget.
	saveGovernanceTestMemory(t, d, "X-rb", "X memory")
	seedAlias(t, d, "X-rb", "src-rb")
	saveGovernanceTestMemory(t, d, "beta-rb", "beta memory")

	var srcCountBefore int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'src-rb'`).Scan(&srcCountBefore); err != nil {
		t.Fatalf("count before: %v", err)
	}

	_, err := d.MergeGovernanceProject(context.Background(), "src-rb", "beta-rb", "actor", "reason", time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("MergeGovernanceProject expected error (chain guard), got nil")
	}

	// Source rows must be intact (rollback verified).
	var srcCountAfter int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'src-rb'`).Scan(&srcCountAfter); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if srcCountAfter != srcCountBefore {
		t.Fatalf("rollback: memories count changed from %d to %d", srcCountBefore, srcCountAfter)
	}
	// Governance record must not exist after rollback.
	var govRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance WHERE project = 'src-rb'`).Scan(&govRows); err != nil {
		t.Fatalf("count governance rows: %v", err)
	}
	if govRows != 0 {
		t.Fatalf("governance rows after failed merge: %d, want 0", govRows)
	}
}

// TestKnownProjects_ExcludesAliasedSources verifies that source_project values
// that have an active alias are hidden from KnownProjects.
func TestKnownProjects_ExcludesAliasedSources(t *testing.T) {
	t.Parallel()

	t.Run("source hidden, target visible after alias creation", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		saveGovernanceTestMemory(t, d, "Foo", "Foo memory")
		saveGovernanceTestMemory(t, d, "Bar", "Bar memory")

		seedAlias(t, d, "Foo", "Bar")

		projects, err := d.KnownProjects(context.Background())
		if err != nil {
			t.Fatalf("KnownProjects: %v", err)
		}
		got := map[string]bool{}
		for _, p := range projects {
			got[p.Name] = true
		}
		if got["Foo"] {
			t.Fatal("KnownProjects contains Foo (aliased source), expected hidden")
		}
		if !got["Bar"] {
			t.Fatal("KnownProjects missing Bar (alias target), expected visible")
		}
	})

	t.Run("both projects visible when no alias exists", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		saveGovernanceTestMemory(t, d, "Foo", "Foo memory")
		saveGovernanceTestMemory(t, d, "Bar", "Bar memory")

		projects, err := d.KnownProjects(context.Background())
		if err != nil {
			t.Fatalf("KnownProjects: %v", err)
		}
		got := map[string]bool{}
		for _, p := range projects {
			got[p.Name] = true
		}
		if !got["Foo"] {
			t.Fatal("KnownProjects missing Foo (no alias)")
		}
		if !got["Bar"] {
			t.Fatal("KnownProjects missing Bar (no alias)")
		}
	})

	t.Run("source reappears after alias removal", func(t *testing.T) {
		t.Parallel()
		d := openGovernanceTestDB(t)

		saveGovernanceTestMemory(t, d, "Foo", "Foo memory")
		saveGovernanceTestMemory(t, d, "Bar", "Bar memory")
		seedAlias(t, d, "Foo", "Bar")

		if err := d.RemoveAlias(context.Background(), "Foo"); err != nil {
			t.Fatalf("RemoveAlias: %v", err)
		}
		projects, err := d.KnownProjects(context.Background())
		if err != nil {
			t.Fatalf("KnownProjects after removal: %v", err)
		}
		got := map[string]bool{}
		for _, p := range projects {
			got[p.Name] = true
		}
		if !got["Foo"] {
			t.Fatal("KnownProjects missing Foo after alias removal")
		}
	})
}
