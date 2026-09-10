package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/require"
)

func TestAdvanceApplyProgressGuardsSnapshotAndReceipt(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "request-1", 0, 0, "", "apb-11111111111111111111111111111111")

	committed, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	require.Equal(t, "committed", committed.Outcome)
	require.Equal(t, uint64(1), committed.State.Generation)
	require.Equal(t, uint64(1), committed.State.Revision)

	retry, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	require.Equal(t, committed, retry)

	stale := applyProgressRequest(t, "request-2", 0, 0, "", "apb-22222222222222222222222222222222")
	conflict, err := store.AdvanceApplyProgress(stale)
	require.NoError(t, err)
	require.Equal(t, "conflict", conflict.Outcome)
	require.Equal(t, committed.State, conflict.State)

	reused := first
	reused.Snapshot.Status = applyprogress.StatusComplete
	_, err = store.AdvanceApplyProgress(reused)
	require.ErrorIs(t, err, ErrApplyProgressRequestConflict)

	collision := applyProgressRequest(t, "request-3", 1, 1, committed.State.Digest, "apb-11111111111111111111111111111111")
	collision.Batches[0].Entries[0].Summary = "different bytes"
	collision.Batches[0], _, err = applyprogress.SealBatch(collision.Batches[0])
	require.NoError(t, err)
	collision.Snapshot.Batches[0].SHA256 = collision.Batches[0].SHA256
	collision.Snapshot, _, err = applyprogress.SealSnapshot(collision.Snapshot)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(collision)
	require.ErrorIs(t, err, ErrApplyProgressBatchCollision)

	got, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, committed.State, got)
}

func TestAdvanceApplyProgressConcurrentWritersCommitOneHead(t *testing.T) {
	store := openTestDB(t)
	requests := []ApplyProgressAdvance{
		applyProgressRequest(t, "request-concurrent-a", 0, 0, "", "apb-44444444444444444444444444444444"),
		applyProgressRequest(t, "request-concurrent-b", 0, 0, "", "apb-55555555555555555555555555555555"),
	}
	results := make(chan ApplyProgressAdvanceResult, len(requests))
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Go(func() { result, err := store.AdvanceApplyProgress(request); require.NoError(t, err); results <- result })
	}
	wg.Wait()
	close(results)
	committed, conflict := 0, 0
	for result := range results {
		if result.Outcome == "committed" {
			committed++
		} else if result.Outcome == "conflict" {
			conflict++
		}
	}
	require.Equal(t, 1, committed)
	require.Equal(t, 1, conflict)
}

func TestAdvanceApplyProgressRejectsCoverageContradictingEvidenceWithoutCommit(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*ApplyProgressAdvance)
	}{
		{
			name: "coverage has no matching completion",
			edit: func(request *ApplyProgressAdvance) {
				request.Snapshot.Coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: request.Batches[0].BatchID, EntryID: "entry"}}
			},
		},
		{
			name: "complete snapshot has no coverage or batches",
			edit: func(request *ApplyProgressAdvance) {
				request.Snapshot.Status = applyprogress.StatusComplete
				request.Snapshot.Batches = []applyprogress.BatchRef{}
				request.Batches = []applyprogress.Batch{}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			request := applyProgressRequest(t, "request-invalid-coverage", 0, 0, "", "apb-33333333333333333333333333333333")
			tt.edit(&request)
			var err error
			request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
			require.NoError(t, err)

			_, err = store.AdvanceApplyProgress(request)
			require.ErrorIs(t, err, ErrApplyProgressInvalid)
			_, err = store.GetApplyProgress("project", "change")
			require.ErrorIs(t, err, ErrApplyProgressNotFound)
		})
	}
}

func TestAdvanceApplyProgressRejectsInvalidRequestWithoutCommit(t *testing.T) {
	store := openTestDB(t)
	req := applyProgressRequest(t, "request-invalid", 0, 0, "", "apb-33333333333333333333333333333333")
	req.Snapshot.Generation = 2

	_, err := store.AdvanceApplyProgress(req)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrApplyProgressBatchCollision))
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func TestAdvanceApplyProgressAdvancesExistingHeadAndValidatesReferences(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "request-first", 0, 0, "", "apb-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	firstResult, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)

	second := applyProgressRequest(t, "request-second", 1, 1, firstResult.State.Digest, "apb-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	second.Snapshot.Batches = append([]applyprogress.BatchRef{{BatchID: first.Batches[0].BatchID, SHA256: first.Batches[0].SHA256}}, second.Snapshot.Batches...)
	second.Snapshot, _, err = applyprogress.SealSnapshot(second.Snapshot)
	require.NoError(t, err)
	secondResult, err := store.AdvanceApplyProgress(second)
	require.NoError(t, err)
	require.Equal(t, "committed", secondResult.Outcome)
	require.Equal(t, uint64(2), secondResult.State.Generation)
	require.Equal(t, uint64(2), secondResult.State.Revision)

	stale := applyProgressRequest(t, "request-stale", 1, 1, firstResult.State.Digest, "apb-cccccccccccccccccccccccccccccccc")
	staleResult, err := store.AdvanceApplyProgress(stale)
	require.NoError(t, err)
	require.Equal(t, "conflict", staleResult.Outcome)
	require.Equal(t, secondResult.State, staleResult.State)

	missing := applyProgressRequest(t, "request-missing", 2, 2, secondResult.State.Digest, "apb-dddddddddddddddddddddddddddddddd")
	missing.Snapshot.Batches = append(missing.Snapshot.Batches, applyprogress.BatchRef{BatchID: "apb-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", SHA256: strings.Repeat("e", 64)})
	missing.Snapshot, _, err = applyprogress.SealSnapshot(missing.Snapshot)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(missing)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
}

func TestAdvanceApplyProgressCanonicalizesIdentityAndPreservesCapacityCause(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "request-canonical", 0, 0, "", "apb-ffffffffffffffffffffffffffffffff")
	request.Project = " project "
	committed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, "project", committed.State.Snapshot.Project)

	tooLarge := applyProgressRequest(t, "request-capacity", 1, 1, committed.State.Digest, "apb-99999999999999999999999999999999")
	tooLarge.Batches[0].Entries[0].Summary = strings.Repeat("x", 40_000)
	_, err = store.AdvanceApplyProgress(tooLarge)
	var capacity *applyprogress.CapacityError
	require.ErrorAs(t, err, &capacity)
}

func TestAdvanceApplyProgressReplaysReceiptAfterProjectBlocks(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "request-replay", 0, 0, "", "apb-78787878787878787878787878787878")
	committed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	_, err = store.RawDB().Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, "project", "project", "block-replay")
	require.NoError(t, err)

	replayed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, committed, replayed)
}

func TestApplyProgressDocumentsRejectGeneralDeleteAndRestore(t *testing.T) {
	store := openTestDB(t)
	result, err := store.AdvanceApplyProgress(applyProgressRequest(t, "request-immutable", 0, 0, "", "apb-12121212121212121212121212121212"))
	require.NoError(t, err)

	var snapshotID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT snapshot_memory_id FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&snapshotID))
	require.Error(t, store.DeleteMemory(snapshotID, "tester", "must not mutate evidence"))
	got, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, result.State, got)

	_, err = store.RawDB().Exec(`INSERT INTO memories (sync_id, project, topic_key, category, title, content, tags, files_affected, created_by, session_id, deleted_at) VALUES (?, ?, ?, 'architecture', 'immutable', '{}', '[]', '[]', 'tester', ?, CURRENT_TIMESTAMP)`, "restore-immutable", "project", "sdd/change/apply-evidence/apb-34343434343434343434343434343434", "sdd-apply-project")
	require.NoError(t, err)
	var deletedID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT id FROM memories WHERE sync_id = ?`, "restore-immutable").Scan(&deletedID))
	require.Error(t, store.RestoreMemory(deletedID, "tester"))
}

func TestLegacyMemoriesRebuildRestoresApplyProgressProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec(`CREATE TABLE memories (id INTEGER PRIMARY KEY, sync_id TEXT NOT NULL, project TEXT NOT NULL, topic_key TEXT, category TEXT NOT NULL DEFAULT '', title TEXT NOT NULL, content TEXT NOT NULL, tags TEXT NOT NULL DEFAULT '[]', files_affected TEXT NOT NULL DEFAULT '[]', created_by TEXT NOT NULL DEFAULT 'unknown', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, synced_at DATETIME, confidence TEXT NOT NULL DEFAULT '', impact_score INTEGER NOT NULL DEFAULT 0)`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	var trigger string
	require.NoError(t, store.RawDB().QueryRow(`SELECT name FROM sqlite_master WHERE type = 'trigger' AND name = 'protect_sdd_apply_progress_documents'`).Scan(&trigger))
	_, err = store.AdvanceApplyProgress(applyProgressRequest(t, "rebuilt", 0, 0, "", "apb-13131313131313131313131313131313"))
	require.NoError(t, err)
	var snapshotID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT snapshot_memory_id FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&snapshotID))
	require.Error(t, store.DeleteMemory(snapshotID, "tester", "must stay immutable"))
}

func applyProgressRequest(t *testing.T, requestID string, generation, revision uint64, digest, batchID string) ApplyProgressAdvance {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema: applyprogress.EvidenceSchema, Project: "project", Change: "change", BatchID: batchID,
		Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
	})
	require.NoError(t, err)
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", Generation: generation + 1, Revision: revision + 1, PreviousDigest: digest,
		TaskManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	require.NoError(t, err)
	return ApplyProgressAdvance{Project: "project", Change: "change", RequestID: requestID, ExpectedGeneration: generation, ExpectedRevision: revision, ExpectedDigest: digest, Snapshot: snapshot, Batches: []applyprogress.Batch{batch}}
}
