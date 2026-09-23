package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/require"
)

func TestPublishApplyProgressSuccessorRejectsUnsealedPredecessor(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "publish-first", 0, 0, "", "apb-91919191919191919191919191919191")
	_, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	_, err = store.PublishApplyProgressSuccessor("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	_, err = store.GetApplyProgress("project", "next")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func sealedPublishFixture(t *testing.T) (*DB, applyprogress.Snapshot) {
	return sealedPublishFixtureWithReceipt(t, true)
}

func sealedPublishFixtureWithReceipt(t *testing.T, withReceipt bool) (*DB, applyprogress.Snapshot) {
	t.Helper()
	store := openTestDB(t)
	first := applyProgressRequest(t, "publish-base", 0, 0, "", "apb-91919191919191919191919191919191")
	base, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	tasks := "- [ ] 1.1 successor task\n"
	parsed, err := applyprogress.ParseTasksMarkdown(tasks)
	require.NoError(t, err)
	_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
	require.NoError(t, err)
	topic := "sdd/change/tasks"
	insertSDDMemory(t, store, "project", &topic, "tasks", tasks, "2026-09-10 10:00:00", false)
	seal := base.State.Snapshot
	seal.Schema, seal.Status = applyprogress.SupersessionSnapshotSchema, applyprogress.StatusSuperseded
	seal.Revision++
	seal.PreviousDigest = base.State.Digest
	seal.StreamSHA256, seal.NextEntryIndex, seal.NextEntryID = "", 0, ""
	seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "next", SuccessorManifestSHA256: manifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "publish-seal"}
	seal, _, err = applyprogress.SealSnapshot(seal)
	require.NoError(t, err)
	if withReceipt {
		_, err = store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "publish-seal", ExpectedGeneration: base.State.Generation, ExpectedRevision: base.State.Revision, ExpectedDigest: base.State.Digest, Snapshot: seal, Batches: []applyprogress.Batch{}})
		require.NoError(t, err)
	} else {
		_, data, sealErr := applyprogress.SealSnapshot(seal)
		require.NoError(t, sealErr)
		sealTopic := "sdd/change/apply-progress/v2"
		insertSDDMemory(t, store, "project", &sealTopic, "synced seal", string(data), "2026-09-11 10:00:00", false)
	}
	return store, seal
}

func TestPublishApplyProgressSuccessorCommitsAndReplays(t *testing.T) {
	store, seal := sealedPublishFixture(t)
	result, err := store.PublishApplyProgressSuccessor("project", "change")
	require.NoError(t, err)
	require.Equal(t, "committed", result.Outcome)
	require.Equal(t, seal.SealIntent.SuccessorManifestSHA256, result.State.Snapshot.TaskManifestSHA256)
	require.Equal(t, seal.Digest, result.State.Snapshot.Supersedes.SealDigest)
	require.NotEqual(t, "publish-seal", result.Receipt.RequestID)
	replayed, err := store.PublishApplyProgressSuccessor("project", "change")
	require.NoError(t, err)
	require.Equal(t, result, replayed)
	var count int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = ? AND topic_key = ?`, "project", "sdd/next/tasks").Scan(&count))
	require.Equal(t, 1, count)
}

func TestPublishApplyProgressSuccessorReconcilesPredecessorHead(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			store, seal := sealedPublishFixture(t)
			movePublishFixtureHeadBack(t, store, missing)
			result, err := store.PublishApplyProgressSuccessor("project", "change")
			require.NoError(t, err)
			require.Equal(t, seal.Digest, result.State.Snapshot.Supersedes.SealDigest)
			var head string
			require.NoError(t, store.RawDB().QueryRow(`SELECT digest FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&head))
			require.Equal(t, seal.Digest, head)
		})
	}
}

func movePublishFixtureHeadBack(t *testing.T, store *DB, missing bool) string {
	t.Helper()
	var id int64
	var digest string
	require.NoError(t, store.RawDB().QueryRow(`SELECT id, json_extract(content, '$.digest') FROM memories WHERE project = ? AND topic_key = ? ORDER BY id LIMIT 1`, "project", "sdd/change/apply-progress/v2").Scan(&id, &digest))
	var err error
	if missing {
		_, err = store.RawDB().Exec(`DELETE FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change")
	} else {
		_, err = store.RawDB().Exec(`UPDATE sdd_apply_heads SET snapshot_memory_id = ?, revision = 1, digest = ? WHERE project = ? AND change_name = ?`, id, digest, "project", "change")
	}
	require.NoError(t, err)
	return digest
}

func TestPublishApplyProgressSuccessorAcceptsReceiptlessSignedSeal(t *testing.T) {
	store, seal := sealedPublishFixtureWithReceipt(t, false)
	movePublishFixtureHeadBack(t, store, true)
	published, err := store.PublishApplyProgressSuccessor("project", "change")
	require.NoError(t, err)
	require.Equal(t, seal.Digest, published.State.Snapshot.Supersedes.SealDigest)
	var head string
	require.NoError(t, store.RawDB().QueryRow(`SELECT digest FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&head))
	require.Equal(t, seal.Digest, head)
}

func TestPublishApplyProgressSuccessorBlockedReplayDoesNotReconcileHead(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			store, _ := sealedPublishFixture(t)
			committed, err := store.PublishApplyProgressSuccessor("project", "change")
			require.NoError(t, err)
			oldDigest := movePublishFixtureHeadBack(t, store, missing)
			_, err = store.RawDB().Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, "project", "project", "block-successor-replay")
			require.NoError(t, err)
			replayed, err := store.PublishApplyProgressSuccessor("project", "change")
			require.NoError(t, err)
			require.Equal(t, committed, replayed)
			var digest string
			err = store.RawDB().QueryRow(`SELECT digest FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&digest)
			if missing {
				require.ErrorIs(t, err, sql.ErrNoRows)
			} else {
				require.NoError(t, err)
				require.Equal(t, oldDigest, digest)
			}
		})
	}
}

func TestPublishApplyProgressSuccessorRollsBackHeadReconciliation(t *testing.T) {
	for _, invalidTopology := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid-topology=%t", invalidTopology), func(t *testing.T) {
			store, _ := sealedPublishFixture(t)
			oldDigest := movePublishFixtureHeadBack(t, store, false)
			if invalidTopology {
				topic := "sdd/change/apply-progress/v2"
				insertSDDMemory(t, store, "project", &topic, "invalid snapshot", `{"schema":`, "2026-09-11 10:00:00", false)
			} else {
				topic := "sdd/next/tasks"
				insertSDDMemory(t, store, "project", &topic, "occupied", "old", "2026-09-11 10:00:00", false)
			}
			_, err := store.PublishApplyProgressSuccessor("project", "change")
			require.ErrorIs(t, err, ErrApplyProgressInvalid)
			var head string
			require.NoError(t, store.RawDB().QueryRow(`SELECT digest FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&head))
			require.Equal(t, oldDigest, head)
			var count int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = ? AND topic_key = ?`, "project", "sdd/next/apply-progress/v2").Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestAdvanceApplyProgressRejectsForgedSuccessorGenesis(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "forged-genesis", 0, 0, "", "apb-91919191919191919191919191919191")
	request.Snapshot.Schema = applyprogress.SupersessionSnapshotSchema
	request.Snapshot.Status = applyprogress.StatusPartial
	request.Snapshot.Coverage = []applyprogress.Coverage{}
	request.Snapshot.Batches = []applyprogress.BatchRef{}
	request.Batches = []applyprogress.Batch{}
	request.Snapshot.Supersedes = &applyprogress.SupersedesPointer{Project: "project", Change: "absent", SealDigest: strings.Repeat("a", 64), OriginalManifestSHA256: request.Snapshot.TaskManifestSHA256, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "forged-seal"}
	var err error
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(request)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func TestPublishApplyProgressSuccessorReplayAfterAdvanceReturnsGenesis(t *testing.T) {
	store, _ := sealedPublishFixture(t)
	genesis, err := store.PublishApplyProgressSuccessor("project", "change")
	require.NoError(t, err)
	entry := applyprogress.EvidenceEntry{EntryID: "next-entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "continuation", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{entry})
	require.NoError(t, err)
	continuation := genesis.State.Snapshot
	continuation.Revision++
	continuation.PreviousDigest = genesis.State.Digest
	continuation.StreamSHA256 = stream
	continuation.NextEntryID = entry.EntryID
	continuation, _, err = applyprogress.SealSnapshot(continuation)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "next", RequestID: "next-advance", ExpectedGeneration: 1, ExpectedRevision: 1, ExpectedDigest: genesis.State.Digest, Snapshot: continuation, Batches: []applyprogress.Batch{}})
	require.NoError(t, err)
	replayed, err := store.PublishApplyProgressSuccessor("project", "change")
	require.NoError(t, err)
	require.Equal(t, genesis, replayed)
}

func TestPublishApplyProgressSuccessorRejectsOccupiedAndMismatchedTasks(t *testing.T) {
	for _, tc := range []struct {
		name, topic, title string
		deleted, mismatch  bool
	}{
		{name: "unicode topic tombstone", topic: "\u00a0sdd/next/tasks\u00a0", title: "legacy", deleted: true},
		{name: "unicode title legacy", title: "\u00a0sdd/next/proposal\u00a0", deleted: true},
		{name: "manifest mismatch", mismatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := sealedPublishFixture(t)
			if tc.mismatch {
				topic := "sdd/change/tasks"
				insertSDDMemory(t, store, "project", &topic, "tasks", "- [ ] 1.1 different task\n", "2026-09-11 10:00:00", false)
			} else {
				var topic *string
				if tc.topic != "" {
					topic = &tc.topic
				}
				insertSDDMemory(t, store, "project", topic, tc.title, "old", "2026-09-10 10:00:00", tc.deleted)
			}
			_, err := store.PublishApplyProgressSuccessor("project", "change")
			require.ErrorIs(t, err, ErrApplyProgressInvalid)
			var count int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = ? AND topic_key = ?`, "project", "sdd/next/tasks").Scan(&count))
			require.Zero(t, count)
			_, err = store.GetApplyProgress("project", "next")
			require.ErrorIs(t, err, ErrApplyProgressNotFound)
		})
	}
}

func TestAdvanceApplyProgressSupersessionAfterHiveTasksChange(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "seal-first", 0, 0, "", "apb-91919191919191919191919191919191")
	base, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	topic := "sdd/change/tasks"
	insertSDDMemory(t, store, "project", &topic, "tasks", "- [ ] 1.1 new task\n", "2026-09-10 10:00:00", false)
	seal := base.State.Snapshot
	seal.Schema = applyprogress.SupersessionSnapshotSchema
	seal.Status = applyprogress.StatusSuperseded
	seal.Revision++
	seal.PreviousDigest = base.State.Digest
	seal.StreamSHA256, seal.NextEntryIndex, seal.NextEntryID = "", 0, ""
	seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "next", SuccessorManifestSHA256: strings.Repeat("c", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal-request"}
	seal, _, err = applyprogress.SealSnapshot(seal)
	require.NoError(t, err)
	require.True(t, applyprogress.IsSupersessionSeal(base.State.Snapshot, seal))
	request := ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "seal-request", ExpectedGeneration: base.State.Generation, ExpectedRevision: base.State.Revision, ExpectedDigest: base.State.Digest, Snapshot: seal, Batches: []applyprogress.Batch{}}
	committed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, "committed", committed.Outcome)
	replayed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, committed, replayed)
	current, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, seal.Digest, current.Digest)
	changed := request
	changed.Snapshot.SealIntent = &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "different", SuccessorManifestSHA256: strings.Repeat("c", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal-request"}
	_, err = store.AdvanceApplyProgress(changed)
	require.ErrorIs(t, err, ErrApplyProgressRequestConflict)
	stale := request
	stale.RequestID = "stale-seal"
	conflict, err := store.AdvanceApplyProgress(stale)
	require.NoError(t, err)
	require.Equal(t, "conflict", conflict.Outcome)
	require.Equal(t, committed.State, conflict.State)
}

func TestAdvanceApplyProgressSealRejectsCorruptOldEvidence(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "corrupt-seal-first", 0, 0, "", "apb-92929292929292929292929292929292")
	base, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	allowApplyProgressCorruption(t, store)
	_, err = store.RawDB().Exec(`UPDATE memories SET content = ? WHERE project = ? AND topic_key = ?`, `{"schema":`, "project", "sdd/change/apply-evidence/"+first.Batches[0].BatchID)
	require.NoError(t, err)
	candidate := base.State.Snapshot
	candidate.Schema, candidate.Status = applyprogress.SupersessionSnapshotSchema, applyprogress.StatusSuperseded
	candidate.Revision++
	candidate.PreviousDigest = base.State.Digest
	candidate.SealIntent = &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "next", SuccessorManifestSHA256: strings.Repeat("c", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "corrupt-seal"}
	candidate, _, err = applyprogress.SealSnapshot(candidate)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "corrupt-seal", ExpectedGeneration: base.State.Generation, ExpectedRevision: base.State.Revision, ExpectedDigest: base.State.Digest, Snapshot: candidate, Batches: []applyprogress.Batch{}})
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
}

func TestAdvanceApplyProgressOrdinaryAdvanceChecksChangedTasks(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "ordinary-first", 0, 0, "", "apb-93939393939393939393939393939393")
	base, err := store.AdvanceApplyProgress(first)
	require.NoError(t, err)
	topic := "sdd/change/tasks"
	insertSDDMemory(t, store, "project", &topic, "tasks", "- [ ] 1.1 new task\n", "2026-09-10 10:00:00", false)
	ordinary := applyProgressContinuationUpgrade(t, base.State, "ordinary-after-edit", "next-entry")
	_, err = store.AdvanceApplyProgress(ordinary)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	current, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, base.State.Digest, current.Digest)
}

func TestAdvanceApplyProgressRefusesOverCapacitySuccessorBeforeWriting(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "snapshot-capacity", 0, 0, "", "apb-00000000000000000000000000000001")
	for i := 0; i < 500; i++ {
		request.Snapshot.Batches = append(request.Snapshot.Batches, applyprogress.BatchRef{BatchID: fmt.Sprintf("apb-%032x", i+2), SHA256: strings.Repeat("a", 64)})
	}

	_, err := store.AdvanceApplyProgress(request)
	var exhausted *applyprogress.SnapshotCapacityExhaustedError
	require.ErrorAs(t, err, &exhausted)
	require.NotNil(t, exhausted.Capacity)
	require.Zero(t, exhausted.Capacity.CurrentRunes)
	require.Greater(t, exhausted.Capacity.ProjectedRunes, applyprogress.MaxDocumentRunes)
	require.Equal(t, applyprogress.MaxDocumentRunes, exhausted.Capacity.CeilingRunes)

	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
	var documents, receipts int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = ?`, "project").Scan(&documents))
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sdd_apply_receipts WHERE request_id = ?`, request.RequestID).Scan(&receipts))
	require.Zero(t, documents)
	require.Zero(t, receipts)
}

func TestAdvanceApplyProgressRejectsTamperedBatchBeforeWriting(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "tampered-batch", 0, 0, "", "apb-00000000000000000000000000000001")
	request.Batches[0].Entries[0].Summary = "tampered"

	_, err := store.AdvanceApplyProgress(request)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func TestAdvanceApplyProgressRejectsImportedEvidenceWithoutLegacyProvenance(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "imported-without-provenance", 0, 0, "", "apb-01010101010101010101010101010101")
	request.Batches[0].Entries[0].Kind = applyprogress.EvidenceImported
	var err error
	request.Batches[0], _, err = applyprogress.SealBatch(request.Batches[0])
	require.NoError(t, err)
	request.Snapshot.Batches[0].SHA256 = request.Batches[0].SHA256
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	require.NoError(t, err)

	_, err = store.AdvanceApplyProgress(request)
	var validation *applyprogress.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, applyprogress.ValidationCode("legacy_migration"), validation.Code)
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func TestAdvanceApplyProgressBindsImportedEvidenceToAuthoritativeHiveLegacySource(t *testing.T) {
	legacy := "status: complete\n"
	topic, tasksTopic := "sdd/change/apply-progress", "sdd/change/tasks"
	tasks := "- [x] 1.1 authoritative legacy task\n"

	t.Run("initial import and exact replay commit", func(t *testing.T) {
		store := openTestDB(t)
		insertSDDMemory(t, store, "project", &topic, "ignored", legacy, "2026-09-10 10:00:00", false)
		insertSDDMemory(t, store, "project", &tasksTopic, "ignored", tasks, "2026-09-10 10:00:00", false)
		request := deterministicImportedApplyProgressRequest(t, "imported-authorized", legacy, tasks)

		committed, err := store.AdvanceApplyProgress(request)
		require.NoError(t, err)
		replayed, err := store.AdvanceApplyProgress(request)
		require.NoError(t, err)
		require.Equal(t, committed, replayed)

		changed := request
		changed.LegacySourceSHA256 = applyprogress.LegacySourceSHA256([]byte("changed source\n"))
		_, err = store.AdvanceApplyProgress(changed)
		require.ErrorIs(t, err, ErrApplyProgressRequestConflict)
	})

	t.Run("changed source is rejected before writes", func(t *testing.T) {
		store := openTestDB(t)
		insertSDDMemory(t, store, "project", &topic, "ignored", legacy, "2026-09-10 10:00:00", false)
		insertSDDMemory(t, store, "project", &tasksTopic, "ignored", tasks, "2026-09-10 10:00:00", false)
		request := deterministicImportedApplyProgressRequest(t, "imported-mismatched", "changed", tasks)

		_, err := store.AdvanceApplyProgress(request)
		var validation *applyprogress.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, applyprogress.CodeLegacyMigration, validation.Code)
		_, err = store.GetApplyProgress("project", "change")
		require.ErrorIs(t, err, ErrApplyProgressNotFound)
	})

	t.Run("v2 head cannot append imported evidence", func(t *testing.T) {
		store := openTestDB(t)
		insertSDDMemory(t, store, "project", &topic, "ignored", legacy, "2026-09-10 10:00:00", false)
		insertSDDMemory(t, store, "project", &tasksTopic, "ignored", tasks, "2026-09-10 10:00:00", false)
		first := deterministicImportedApplyProgressRequest(t, "imported-first", legacy, tasks)
		committed, err := store.AdvanceApplyProgress(first)
		require.NoError(t, err)

		successor := deterministicImportedApplyProgressRequest(t, "imported-successor", legacy, tasks)
		successor.ExpectedGeneration, successor.ExpectedRevision, successor.ExpectedDigest = committed.State.Generation, committed.State.Revision, committed.State.Digest
		successor.Snapshot = committed.State.Snapshot
		successor.Snapshot.Revision++
		successor.Snapshot.PreviousDigest = committed.State.Digest
		successor.Snapshot.Batches = append(append([]applyprogress.BatchRef{}, committed.State.Snapshot.Batches...), applyprogress.BatchRef{BatchID: successor.Batches[0].BatchID, SHA256: successor.Batches[0].SHA256})
		successor.Snapshot, _, err = applyprogress.SealSnapshot(successor.Snapshot)
		require.NoError(t, err)

		_, err = store.AdvanceApplyProgress(successor)
		require.ErrorIs(t, err, ErrApplyProgressInvalid)
	})
}

func TestAdvanceApplyProgressRejectsImportedEvidenceThatDoesNotMatchAuthoritativeLegacyTasks(t *testing.T) {
	store := openTestDB(t)
	legacyProgress := "status: complete\n"
	progressTopic, tasksTopic := "sdd/change/apply-progress", "sdd/change/tasks"
	insertSDDMemory(t, store, "project", &progressTopic, "ignored", legacyProgress, "2026-09-10 10:00:00", false)
	insertSDDMemory(t, store, "project", &tasksTopic, "ignored", "- [x] 1.1 authoritative legacy task\n", "2026-09-10 10:00:00", false)

	request := importedApplyProgressRequest(t, "imported-task-mismatch", "apb-21212121212121212121212121212121", legacyProgress)
	_, err := store.AdvanceApplyProgress(request)
	var validation *applyprogress.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, applyprogress.CodeLegacyMigration, validation.Code)
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func TestGetApplyProgressReceiptReturnsOnlyBoundIdentity(t *testing.T) {
	store := openTestDB(t)
	committed, err := store.AdvanceApplyProgress(applyProgressRequest(t, "receipt-identity", 0, 0, "", "apb-10101010101010101010101010101010"))
	require.NoError(t, err)

	receipt, err := store.GetApplyProgressReceipt("project", "change", "receipt-identity")
	require.NoError(t, err)
	require.Equal(t, committed.Receipt, receipt)

	_, err = store.GetApplyProgressReceipt("project", "other-change", "receipt-identity")
	require.ErrorIs(t, err, ErrApplyProgressReceiptNotFound)
}

func TestAdvanceApplyProgressRejectsInvalidRequestIDBeforeWriting(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "invalid request id", 0, 0, "", "apb-10101010101010101010101010101010")
	_, err := store.AdvanceApplyProgress(request)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

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
	require.ErrorIs(t, err, ErrApplyProgressInvalid)

	got, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, boundedApplyProgressState(committed.State), got)
}

func TestGetApplyProgressReconstructsUniqueValidatedHead(t *testing.T) {
	store := openTestDB(t)
	committed, err := store.AdvanceApplyProgress(applyProgressRequest(t, "request-head-missing", 0, 0, "", "apb-15151515151515151515151515151515"))
	require.NoError(t, err)
	_, err = store.RawDB().Exec(`DELETE FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change")
	require.NoError(t, err)
	recovered, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, boundedApplyProgressState(committed.State), recovered)
}

func TestGetApplyProgressRejectsForkedOrInvalidReconstructedHeads(t *testing.T) {
	for _, tt := range []struct {
		name string
		add  func(t *testing.T, store *DB, base ApplyProgressState)
	}{
		{
			name: "forked terminal snapshots",
			add: func(t *testing.T, store *DB, base ApplyProgressState) {
				for _, status := range []applyprogress.Status{applyprogress.StatusPartial, applyprogress.StatusComplete} {
					snapshot := base.Snapshot
					snapshot.Revision++
					snapshot.PreviousDigest = base.Digest
					snapshot.Status = status
					var err error
					snapshot, _, err = applyprogress.SealSnapshot(snapshot)
					require.NoError(t, err)
					insertRawApplyProgressSnapshot(t, store, snapshot)
				}
			},
		},
		{
			name: "missing predecessor",
			add: func(t *testing.T, store *DB, base ApplyProgressState) {
				snapshot := base.Snapshot
				snapshot.Revision++
				snapshot.PreviousDigest = strings.Repeat("c", 64)
				var err error
				snapshot, _, err = applyprogress.SealSnapshot(snapshot)
				require.NoError(t, err)
				insertRawApplyProgressSnapshot(t, store, snapshot)
			},
		},
		{
			name: "corrupt referenced batch",
			add: func(t *testing.T, store *DB, base ApplyProgressState) {
				allowApplyProgressCorruption(t, store)
				_, err := store.RawDB().Exec(`UPDATE memories SET content = ? WHERE project = ? AND topic_key = ?`, `{"schema":`, "project", "sdd/change/apply-evidence/"+base.Snapshot.Batches[0].BatchID)
				require.NoError(t, err)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			committed, err := store.AdvanceApplyProgress(applyProgressRequest(t, "request-reconstruct-"+strings.ReplaceAll(tt.name, " ", "-"), 0, 0, "", "apb-16161616161616161616161616161616"))
			require.NoError(t, err)
			tt.add(t, store, committed.State)
			_, err = store.RawDB().Exec(`DELETE FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change")
			require.NoError(t, err)
			_, err = store.GetApplyProgress("project", "change")
			require.ErrorIs(t, err, ErrApplyProgressInvalid)
		})
	}
}

func insertRawApplyProgressSnapshot(t *testing.T, store *DB, snapshot applyprogress.Snapshot) {
	t.Helper()
	_, data, err := applyprogress.SealSnapshot(snapshot)
	require.NoError(t, err)
	tx, err := store.RawDB().Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = store.insertApplyProgressMemory(tx, "project", "change", "apply-progress/v2", "Apply progress snapshot", data)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestGetAndAdvanceApplyProgressReconcileRemoteImmutableSnapshots(t *testing.T) {
	for _, tt := range []struct {
		name    string
		remotes int
		getWant string
		advance bool
	}{
		{name: "unique remote successor advances local head", remotes: 1, getWant: "advance"},
		{name: "remote successor fork fails closed", remotes: 2, getWant: "invalid"},
		{name: "local advance cannot write through unseen remote fork", remotes: 2, advance: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			first, err := store.AdvanceApplyProgress(applyProgressRequest(t, fmt.Sprintf("reconcile-first-%d", tt.remotes), 0, 0, "", "apb-72727272727272727272727272727272"))
			require.NoError(t, err)

			for i := range tt.remotes {
				remote := applyProgressSuccessor(t, first.State, "reconcile-remote-"+tt.name+string(rune('a'+i)), fmt.Sprintf("apb-%032x", 0x730+i), fmt.Sprintf("remote-entry-%d", i))
				for _, document := range []struct {
					eventID string
					syncID  string
					topic   string
					content string
				}{
					{"reconcile-snapshot-" + string(rune('a'+i)), "reconcile-snapshot-sync-" + string(rune('a'+i)), "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, remote.Snapshot)},
				} {
					applied, err := store.ApplyRemoteMutation(immutableCreateEvent(document.eventID, document.syncID, document.topic, document.content))
					require.NoError(t, err)
					require.True(t, applied)
				}
			}

			if tt.advance {
				_, err := store.AdvanceApplyProgress(applyProgressSuccessor(t, first.State, "reconcile-local-write", "apb-74747474747474747474747474747474", "local-entry"))
				require.ErrorIs(t, err, ErrApplyProgressInvalid)
				return
			}
			got, err := store.GetApplyProgress("project", "change")
			switch tt.getWant {
			case "advance":
				require.NoError(t, err)
				require.Equal(t, uint64(2), got.Revision)
			case "invalid":
				require.ErrorIs(t, err, ErrApplyProgressInvalid)
			default:
				t.Fatalf("unknown get expectation %q", tt.getWant)
			}
		})
	}
}

func TestLoadApplyProgressCandidatesCachesCumulativeBatchLoads(t *testing.T) {
	store := openTestDB(t)
	first, err := store.AdvanceApplyProgress(applyProgressRequest(t, "cached-batch-first", 0, 0, "", "apb-75757575757575757575757575757575"))
	require.NoError(t, err)
	successor := applyProgressContinuationUpgrade(t, first.State, "cached-batch-successor", "cached-entry")
	insertRawApplyProgressSnapshot(t, store, successor.Snapshot)

	tx, err := store.RawDB().Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	query := &countingApplyProgressQuery{applyProgressQuery: tx}

	lineage, err := newApplyProgressLineageResolver(store, query).resolve("project", "change")
	require.NoError(t, err)
	require.Equal(t, successor.Snapshot.Digest, lineage.terminal.snapshot.Digest)
	require.Equal(t, 1, query.batchLoads, "each immutable batch must load once per lineage operation")
}

type countingApplyProgressQuery struct {
	applyProgressQuery
	batchLoads int
}

func (q *countingApplyProgressQuery) Query(query string, args ...any) (*sql.Rows, error) {
	if strings.Contains(query, "SELECT content FROM memories WHERE project") {
		q.batchLoads++
	}
	return q.applyProgressQuery.Query(query, args...)
}

func TestGetApplyProgressDerivesRemoteSuccessorWithoutMutatingHead(t *testing.T) {
	store := openTestDB(t)
	first, err := store.AdvanceApplyProgress(applyProgressRequest(t, "read-only-first", 0, 0, "", "apb-76767676767676767676767676767676"))
	require.NoError(t, err)
	remote := applyProgressSuccessor(t, first.State, "read-only-remote", "apb-77777777777777777777777777777777", "remote-entry")
	applied, err := store.ApplyRemoteMutation(immutableCreateEvent("read-only-remote-snapshot", "read-only-remote-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, remote.Snapshot)))
	require.NoError(t, err)
	require.True(t, applied)

	var beforeChanges int
	require.NoError(t, store.RawDB().QueryRow(`SELECT total_changes()`).Scan(&beforeChanges))
	var beforeHead string
	require.NoError(t, store.RawDB().QueryRow(`SELECT digest FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&beforeHead))

	resolved, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, remote.Snapshot.Digest, resolved.Digest)

	evidence, err := store.GetApplyProgressEvidence("project", "change", first.State.Snapshot.Batches[0].BatchID, remote.Snapshot.Digest)
	require.NoError(t, err)
	require.Equal(t, first.State.Snapshot.Batches[0].BatchID, evidence.BatchID)

	var afterChanges int
	require.NoError(t, store.RawDB().QueryRow(`SELECT total_changes()`).Scan(&afterChanges))
	var afterHead string
	require.NoError(t, store.RawDB().QueryRow(`SELECT digest FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&afterHead))
	require.Equal(t, beforeChanges, afterChanges)
	require.Equal(t, beforeHead, afterHead)
}

func TestGetApplyProgressReconcilesHistoricalPreContinuationV2Lineage(t *testing.T) {
	store := openTestDB(t)
	first, err := store.AdvanceApplyProgress(applyProgressRequest(t, "legacy-epoch-first", 0, 0, "", "apb-81818181818181818181818181818181"))
	require.NoError(t, err)

	legacySuccessor := func(base ApplyProgressState, batchID, entryID string) (ApplyProgressState, applyprogress.Batch) {
		t.Helper()
		batch, _, sealErr := applyprogress.SealBatch(applyprogress.Batch{
			Schema: applyprogress.EvidenceSchema, Project: "project", Change: "change", BatchID: batchID,
			Entries: []applyprogress.EvidenceEntry{{EntryID: entryID, TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "historical", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
		})
		require.NoError(t, sealErr)
		snapshot := base.Snapshot
		snapshot.Generation, snapshot.Revision, snapshot.PreviousDigest = base.Generation+1, base.Revision+1, base.Digest
		snapshot.Batches = append(append([]applyprogress.BatchRef{}, base.Snapshot.Batches...), applyprogress.BatchRef{BatchID: batch.BatchID, SHA256: batch.SHA256})
		var snapshotErr error
		snapshot, _, snapshotErr = applyprogress.SealSnapshot(snapshot)
		require.NoError(t, snapshotErr)
		return ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot}, batch
	}
	second, secondBatch := legacySuccessor(first.State, "apb-82828282828282828282828282828282", "legacy-2")
	third, thirdBatch := legacySuccessor(second, "apb-83838383838383838383838383838383", "legacy-3")
	for _, document := range []struct {
		eventID string
		syncID  string
		topic   string
		content string
	}{
		{"legacy-epoch-second-evidence", "legacy-epoch-second-evidence-sync", "sdd/change/apply-evidence/" + secondBatch.BatchID, canonicalBatchContent(t, secondBatch)},
		{"legacy-epoch-second-snapshot", "legacy-epoch-second-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, second.Snapshot)},
		{"legacy-epoch-third-evidence", "legacy-epoch-third-evidence-sync", "sdd/change/apply-evidence/" + thirdBatch.BatchID, canonicalBatchContent(t, thirdBatch)},
		{"legacy-epoch-third-snapshot", "legacy-epoch-third-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, third.Snapshot)},
	} {
		applied, applyErr := store.ApplyRemoteMutation(immutableCreateEvent(document.eventID, document.syncID, document.topic, document.content))
		require.NoError(t, applyErr)
		require.True(t, applied)
	}

	resolved, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, third.Digest, resolved.Digest)
	require.Equal(t, uint64(3), resolved.Generation)
	require.Equal(t, uint64(3), resolved.Revision)
}

func TestAdvanceApplyProgressUpgradesHistoricalExplicitZeroRoot(t *testing.T) {
	tasks := []applyprogress.Task{{ID: "1", Text: "one"}}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	require.NoError(t, err)
	root, rootBytes := historicalExplicitZeroRoot(t, manifest)
	entry := applyprogress.EvidenceEntry{EntryID: "upgrade-entry", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{"1"}, Kind: applyprogress.EvidenceGreen, Summary: "upgrade", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{entry})
	require.NoError(t, err)
	upgraded, err := applyprogress.UpgradeLegacyContinuation(root, tasks, []applyprogress.EvidenceEntry{entry}, stream)
	require.NoError(t, err)
	request := ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "historical-zero-upgrade", ExpectedGeneration: root.Generation, ExpectedRevision: root.Revision, ExpectedDigest: root.Digest, Snapshot: upgraded, Batches: []applyprogress.Batch{}}

	store := openTestDB(t)
	topic := "sdd/change/apply-progress/v2"
	rootID := insertSDDMemory(t, store, "project", &topic, "historical explicit-zero root", string(rootBytes), "2026-09-10 10:00:00", false)
	_, err = store.RawDB().Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES (?, ?, ?, ?, ?, ?)`, "project", "change", rootID, root.Generation, root.Revision, root.Digest)
	require.NoError(t, err)

	committed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, "committed", committed.Outcome)
	require.Equal(t, upgraded.Digest, committed.State.Digest)

	var preservedRootBytes string
	require.NoError(t, store.RawDB().QueryRow(`SELECT content FROM memories WHERE id = ?`, rootID).Scan(&preservedRootBytes))
	require.Equal(t, string(rootBytes), preservedRootBytes, "the historical root bytes remain immutable")
	var headID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT snapshot_memory_id FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&headID))
	require.NotEqual(t, rootID, headID, "the existing explicit-zero head must advance through CAS")

	replayed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)
	require.Equal(t, committed, replayed)
	receipt, err := store.GetApplyProgressReceipt("project", "change", request.RequestID)
	require.NoError(t, err)
	require.Equal(t, committed.Receipt, receipt)
	head, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, boundedApplyProgressState(committed.State), head)

	stale := request
	stale.RequestID = "historical-zero-stale"
	conflict, err := store.AdvanceApplyProgress(stale)
	require.NoError(t, err)
	require.Equal(t, "conflict", conflict.Outcome)
	require.Equal(t, committed.State, conflict.State)
}

func TestAdvanceApplyProgressInsertsWhenHeadIsActuallyAbsent(t *testing.T) {
	store := openTestDB(t)
	committed, err := store.AdvanceApplyProgress(applyProgressRequest(t, "actually-absent-head", 0, 0, "", "apb-84848484848484848484848484848484"))
	require.NoError(t, err)
	require.Equal(t, "committed", committed.Outcome)
	var heads int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&heads))
	require.Equal(t, 1, heads)
}

func TestAdvanceApplyProgressReconcilesMissingHeadBeforeSuccessor(t *testing.T) {
	store := openTestDB(t)
	first, err := store.AdvanceApplyProgress(applyProgressRequest(t, "reconcile-missing-head", 0, 0, "", "apb-85858585858585858585858585858585"))
	require.NoError(t, err)
	_, err = store.RawDB().Exec(`DELETE FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change")
	require.NoError(t, err)

	successor := applyProgressSuccessor(t, first.State, "reconcile-missing-head-successor", "apb-86868686868686868686868686868686", "successor")
	committed, err := store.AdvanceApplyProgress(successor)
	require.NoError(t, err)
	require.Equal(t, "committed", committed.Outcome)
	require.Equal(t, first.State.Revision+1, committed.State.Revision)
}

func historicalExplicitZeroRoot(t *testing.T, manifest string) (applyprogress.Snapshot, []byte) {
	t.Helper()
	payload := struct {
		Schema             string                   `json:"schema"`
		Project            string                   `json:"project"`
		Change             string                   `json:"change"`
		Generation         uint64                   `json:"generation"`
		Revision           uint64                   `json:"revision"`
		PreviousDigest     string                   `json:"previous_digest"`
		TaskManifestSHA256 string                   `json:"task_manifest_sha256"`
		Status             applyprogress.Status     `json:"status"`
		Coverage           []applyprogress.Coverage `json:"coverage"`
		Batches            []applyprogress.BatchRef `json:"batches"`
		StreamSHA256       string                   `json:"stream_sha256"`
		NextEntryIndex     int                      `json:"next_entry_index"`
		NextEntryID        string                   `json:"next_entry_id"`
	}{
		Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", TaskManifestSHA256: manifest,
		Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{},
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	sum := sha256.Sum256(payloadBytes)
	document := struct {
		Schema             string                   `json:"schema"`
		Project            string                   `json:"project"`
		Change             string                   `json:"change"`
		Generation         uint64                   `json:"generation"`
		Revision           uint64                   `json:"revision"`
		PreviousDigest     string                   `json:"previous_digest"`
		TaskManifestSHA256 string                   `json:"task_manifest_sha256"`
		Status             applyprogress.Status     `json:"status"`
		Coverage           []applyprogress.Coverage `json:"coverage"`
		Batches            []applyprogress.BatchRef `json:"batches"`
		StreamSHA256       string                   `json:"stream_sha256"`
		NextEntryIndex     int                      `json:"next_entry_index"`
		NextEntryID        string                   `json:"next_entry_id"`
		Digest             string                   `json:"digest"`
	}{
		Schema: payload.Schema, Project: payload.Project, Change: payload.Change, Generation: payload.Generation, Revision: payload.Revision,
		PreviousDigest: payload.PreviousDigest, TaskManifestSHA256: payload.TaskManifestSHA256, Status: payload.Status,
		Coverage: payload.Coverage, Batches: payload.Batches, StreamSHA256: payload.StreamSHA256, NextEntryIndex: payload.NextEntryIndex, NextEntryID: payload.NextEntryID,
		Digest: hex.EncodeToString(sum[:]),
	}
	data, err := json.Marshal(document)
	require.NoError(t, err)
	decoded, err := applyprogress.DecodeCanonicalSnapshot(data)
	require.NoError(t, err)
	require.True(t, decoded.RequiresContinuationUpgrade())
	return decoded, data
}

func applyProgressSuccessor(t *testing.T, base ApplyProgressState, requestID, _ string, entryID string) ApplyProgressAdvance {
	t.Helper()
	return applyProgressContinuationUpgrade(t, base, requestID, entryID)
}

func applyProgressContinuationUpgrade(t *testing.T, base ApplyProgressState, requestID, entryID string) ApplyProgressAdvance {
	t.Helper()
	entry := applyprogress.EvidenceEntry{EntryID: entryID, TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "continuation", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{entry})
	require.NoError(t, err)
	snapshot := base.Snapshot
	snapshot.Revision++
	snapshot.PreviousDigest = base.Digest
	snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = stream, 0, entryID
	snapshot, _, err = applyprogress.SealSnapshot(snapshot)
	require.NoError(t, err)
	return ApplyProgressAdvance{Project: "project", Change: "change", RequestID: requestID, ExpectedGeneration: base.Generation, ExpectedRevision: base.Revision, ExpectedDigest: base.Digest, Snapshot: snapshot, Batches: []applyprogress.Batch{}}
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

	second := applyProgressContinuationUpgrade(t, firstResult.State, "request-second", "entry-2")
	secondResult, err := store.AdvanceApplyProgress(second)
	require.NoError(t, err)
	require.Equal(t, "committed", secondResult.Outcome)
	require.Equal(t, uint64(1), secondResult.State.Generation)
	require.Equal(t, uint64(2), secondResult.State.Revision)

	stale := applyProgressRequest(t, "request-stale", 1, 1, firstResult.State.Digest, "apb-cccccccccccccccccccccccccccccccc")
	staleResult, err := store.AdvanceApplyProgress(stale)
	require.NoError(t, err)
	require.Equal(t, "conflict", staleResult.Outcome)
	require.Equal(t, secondResult.State, staleResult.State)

	missing := applyProgressRequest(t, "request-missing", 1, 2, secondResult.State.Digest, "apb-dddddddddddddddddddddddddddddddd")
	missing.Snapshot.Batches = append(missing.Snapshot.Batches, applyprogress.BatchRef{BatchID: "apb-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", SHA256: strings.Repeat("e", 64)})
	missing.Snapshot, _, err = applyprogress.SealSnapshot(missing.Snapshot)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(missing)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
}

func TestAdvanceApplyProgressPersistsOutOfTaskOrderSplitStream(t *testing.T) {
	store := openTestDB(t)
	tasks := []applyprogress.Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}, {ID: "3", Text: "third"}}
	entries := []applyprogress.EvidenceEntry{
		{EntryID: "third", TaskIDs: []string{"3"}, CompletesTaskIDs: []string{"3"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "first", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{"1"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "second", TaskIDs: []string{"2"}, CompletesTaskIDs: []string{"2"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
	}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Tasks: tasks, Entries: entries, EntryID: "third", BatchID: "apb-00000000000000000000000000000091"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanContinuationRequired, first.Outcome)
	firstResult, err := store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "out-of-order-first", Snapshot: first.Snapshot, Batches: []applyprogress.Batch{first.Batch}})
	require.NoError(t, err)

	second, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: "apb-00000000000000000000000000000092"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanContinuationRequired, second.Outcome)
	_, err = store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "out-of-order-second", ExpectedGeneration: firstResult.State.Generation, ExpectedRevision: firstResult.State.Revision, ExpectedDigest: firstResult.State.Digest, Snapshot: second.Snapshot, Batches: []applyprogress.Batch{second.Batch}})
	require.NoError(t, err)
	current, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Len(t, current.Snapshot.Coverage, 2)
	require.Equal(t, []string{"1", "3"}, []string{current.Snapshot.Coverage[0].TaskID, current.Snapshot.Coverage[1].TaskID})
}

func TestAdvanceApplyProgressPermitsFutureStreamAfterExhaustedIncompleteCheckpoint(t *testing.T) {
	store := openTestDB(t)
	tasks := []applyprogress.Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	firstEntries := []applyprogress.EvidenceEntry{applyprogress.EvidenceEntry{EntryID: "first", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{"1"}, Kind: applyprogress.EvidenceGreen, Summary: "first task", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Tasks: tasks, Entries: firstEntries, EntryID: "first", BatchID: "apb-23232323232323232323232323232323"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanCommitted, first.Outcome)
	require.Equal(t, applyprogress.StatusPartial, first.Snapshot.Status)
	firstResult, err := store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "future-stream-first", Snapshot: first.Snapshot, Batches: []applyprogress.Batch{first.Batch}})
	require.NoError(t, err)

	secondEntries := []applyprogress.EvidenceEntry{applyprogress.EvidenceEntry{EntryID: "second", TaskIDs: []string{"2"}, CompletesTaskIDs: []string{"2"}, Kind: applyprogress.EvidenceGreen, Summary: "second task", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}
	second, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: secondEntries, EntryID: "second", BatchID: "apb-24242424242424242424242424242424"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanCommitted, second.Outcome)
	require.Equal(t, applyprogress.StatusComplete, second.Snapshot.Status)
	_, err = store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "future-stream-second", ExpectedGeneration: firstResult.State.Generation, ExpectedRevision: firstResult.State.Revision, ExpectedDigest: firstResult.State.Digest, Snapshot: second.Snapshot, Batches: []applyprogress.Batch{second.Batch}})
	require.NoError(t, err)
}

func TestAdvanceApplyProgressRejectsInitialCursorWithoutMatchingEvidence(t *testing.T) {
	store := openTestDB(t)
	tasks := []applyprogress.Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []applyprogress.EvidenceEntry{
		{EntryID: "e1", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{"1"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "e2", TaskIDs: []string{"2"}, CompletesTaskIDs: []string{"2"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "e3", TaskIDs: []string{"3"}, CompletesTaskIDs: []string{"3"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
	}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: "apb-20202020202020202020202020202020"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanContinuationRequired, first.Outcome)

	invalid := first.Snapshot
	invalid.NextEntryIndex = 0
	invalid, _, err = applyprogress.SealSnapshot(invalid)
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "initial-cursor", Snapshot: invalid, Batches: []applyprogress.Batch{first.Batch}})
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	_, err = store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressNotFound)
}

func TestAdvanceApplyProgressRejectsCursorWithoutMatchingAppendedEvidence(t *testing.T) {
	store := openTestDB(t)
	tasks := []applyprogress.Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []applyprogress.EvidenceEntry{
		{EntryID: "e1", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{"1"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "e2", TaskIDs: []string{"2"}, CompletesTaskIDs: []string{"2"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "e3", TaskIDs: []string{"3"}, CompletesTaskIDs: []string{"3"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
	}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: "apb-21212121212121212121212121212121"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanContinuationRequired, first.Outcome)
	firstRequest := ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "cursor-first", Snapshot: first.Snapshot, Batches: []applyprogress.Batch{first.Batch}}
	firstResult, err := store.AdvanceApplyProgress(firstRequest)
	require.NoError(t, err)

	second, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: "apb-22222222222222222222222222222222"})
	require.NoError(t, err)
	require.Equal(t, applyprogress.PlanContinuationRequired, second.Outcome)
	invalid := second.Snapshot
	invalid.NextEntryIndex = first.NextEntryIndex
	invalid, _, err = applyprogress.SealSnapshot(invalid)
	require.NoError(t, err)
	request := ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "cursor-under-advance", ExpectedGeneration: firstResult.State.Generation, ExpectedRevision: firstResult.State.Revision, ExpectedDigest: firstResult.State.Digest, Snapshot: invalid, Batches: []applyprogress.Batch{second.Batch}}

	_, err = store.AdvanceApplyProgress(request)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
	current, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, boundedApplyProgressState(firstResult.State), current)
}

func TestAdvanceApplyProgressRejectsRevisionOverflow(t *testing.T) {
	store := openTestDB(t)
	req := applyProgressRequest(t, "request-overflow", 1, math.MaxUint64, strings.Repeat("a", 64), "apb-88888888888888888888888888888888")
	_, err := store.AdvanceApplyProgress(req)
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
	require.Equal(t, "evidence batch", capacity.Document)
	require.Greater(t, capacity.Runes, applyprogress.MaxDocumentRunes)
	require.Equal(t, applyprogress.MaxDocumentRunes, capacity.Limit)
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

func TestAdvanceApplyProgressReceiptsRemainBoundedAndReplayCanonicalBatches(t *testing.T) {
	const batchCount = 64

	store := openTestDB(t)
	entries := make([]applyprogress.EvidenceEntry, batchCount+1)
	for i := range entries {
		entries[i] = applyprogress.EvidenceEntry{
			EntryID:          fmt.Sprintf("receipt-size-entry-%d", i),
			TaskIDs:          []string{},
			CompletesTaskIDs: []string{},
			Kind:             applyprogress.EvidenceGreen,
			Summary:          strings.Repeat("receipt-size-evidence", 512),
			Command:          "go test",
			Outcome:          applyprogress.OutcomePass,
			Files:            []string{},
		}
	}
	stream, err := applyprogress.StreamSHA256(entries)
	require.NoError(t, err)

	var current ApplyProgressState
	var latest ApplyProgressAdvance
	for i := 0; i < batchCount; i++ {
		batch, _, err := applyprogress.SealBatch(applyprogress.Batch{
			Schema:  applyprogress.EvidenceSchema,
			Project: "project",
			Change:  "change",
			BatchID: fmt.Sprintf("apb-%032x", i+1),
			Entries: []applyprogress.EvidenceEntry{entries[i]},
		})
		require.NoError(t, err)

		refs := append([]applyprogress.BatchRef{}, current.Snapshot.Batches...)
		refs = append(refs, applyprogress.BatchRef{BatchID: batch.BatchID, SHA256: batch.SHA256})
		snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
			Schema:             applyprogress.SnapshotSchema,
			Project:            "project",
			Change:             "change",
			Generation:         1,
			Revision:           uint64(i + 1),
			PreviousDigest:     current.Digest,
			TaskManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:             applyprogress.StatusPartial,
			Coverage:           []applyprogress.Coverage{},
			Batches:            refs,
			StreamSHA256:       stream,
			NextEntryIndex:     i + 1,
			NextEntryID:        entries[i+1].EntryID,
		})
		require.NoError(t, err)

		latest = ApplyProgressAdvance{
			Project:            "project",
			Change:             "change",
			RequestID:          fmt.Sprintf("receipt-size-request-%d", i),
			ExpectedGeneration: current.Generation,
			ExpectedRevision:   current.Revision,
			ExpectedDigest:     current.Digest,
			Snapshot:           snapshot,
			Batches:            []applyprogress.Batch{batch},
		}
		committed, err := store.AdvanceApplyProgress(latest)
		require.NoError(t, err)
		require.Equal(t, "committed", committed.Outcome)
		current = committed.State
	}

	rows, err := store.RawDB().Query(`SELECT response_json FROM sdd_apply_receipts WHERE project = ? AND change_name = ?`, "project", "change")
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	count := 0
	for rows.Next() {
		var response string
		require.NoError(t, rows.Scan(&response))
		require.LessOrEqual(t, len(response), 200)
		require.NotContains(t, response, "receipt-size-evidence")
		require.NotContains(t, response, `"batches"`)
		var stored applyProgressReceiptResult
		require.NoError(t, json.Unmarshal([]byte(response), &stored))
		require.Equal(t, "committed", stored.Outcome)
		count++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, batchCount, count)

	replayed, err := store.AdvanceApplyProgress(latest)
	require.NoError(t, err)
	require.Equal(t, current, replayed.State)
	require.Len(t, replayed.State.Batches, batchCount)

	allowApplyProgressCorruption(t, store)
	_, err = store.RawDB().Exec(`UPDATE memories SET content = ? WHERE project = ? AND topic_key = ?`, `{"schema":`, "project", "sdd/change/apply-evidence/apb-00000000000000000000000000000001")
	require.NoError(t, err)
	_, err = store.AdvanceApplyProgress(latest)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
}

func TestSaveFromRemoteReservedApplyProgressDocumentsAcceptValidatedCreateOrExactReplay(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "generic-remote-replay", 0, 0, "", "apb-45454545454545454545454545454545")
	_, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)

	for _, topic := range []string{
		"sdd/change/apply-progress/v2",
		"sdd/change/apply-evidence/" + request.Batches[0].BatchID,
	} {
		t.Run(topic, func(t *testing.T) {
			var syncID, project, content string
			require.NoError(t, store.RawDB().QueryRow(`SELECT sync_id, project, content FROM memories WHERE topic_key = ?`, topic).Scan(&syncID, &project, &content))

			exactReplay := &models.Memory{SyncID: syncID, Project: " " + project + " ", TopicKey: &topic, Content: content}
			require.NoError(t, store.SaveFromRemote(exactReplay), "an exact replay must be a no-op")

			otherTopic := "sdd/change/apply-progress/v2"
			if topic == otherTopic {
				otherTopic = "sdd/change/apply-evidence/" + request.Batches[0].BatchID
			}
			for _, tt := range []struct {
				name string
				mem  *models.Memory
			}{
				{name: "same sync ID with changed bytes", mem: &models.Memory{SyncID: syncID, Project: project, TopicKey: &topic, Content: content + " changed"}},
				{name: "same sync ID with changed topic", mem: &models.Memory{SyncID: syncID, Project: project, TopicKey: &otherTopic, Content: content}},
				{name: "new sync ID", mem: &models.Memory{SyncID: syncID + "-new", Project: project, TopicKey: &topic, Content: content}},
				{name: "cross project", mem: &models.Memory{SyncID: syncID, Project: "other-project", TopicKey: &topic, Content: content}},
			} {
				t.Run(tt.name, func(t *testing.T) {
					err := store.SaveFromRemote(tt.mem)
					if tt.name == "new sync ID" {
						require.NoError(t, err)
						return
					}
					var rejected *RemoteImmutableCreateRejectedError
					require.ErrorAs(t, err, &rejected)
					require.Equal(t, ImmutableRemoteCreateRejectionSyncIDConflict, rejected.RejectionCode)
				})
			}

			var persistedProject, persistedTopic, persistedContent string
			require.NoError(t, store.RawDB().QueryRow(`SELECT project, topic_key, content FROM memories WHERE sync_id = ?`, syncID).Scan(&persistedProject, &persistedTopic, &persistedContent))
			require.Equal(t, project, persistedProject)
			require.Equal(t, topic, persistedTopic)
			require.Equal(t, content, persistedContent, "reserved content must remain immutable")
			var documents int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE topic_key = ?`, topic).Scan(&documents))
			require.Equal(t, 2, documents, "only the validated fresh CREATE may add a reserved document")
		})
	}
}

func TestSaveFromRemoteAdmitsFreshValidatedApplyProgressDocumentWithoutJournal(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "fresh-legacy-receive", 0, 0, "", "apb-46464646464646464646464646464646")
	topic := "sdd/change/apply-evidence/" + request.Batches[0].BatchID
	remote := &models.Memory{SyncID: "fresh-legacy-evidence", Project: "project", TopicKey: &topic, Category: "architecture", Title: "remote evidence", Content: canonicalBatchContent(t, request.Batches[0]), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}

	require.NoError(t, store.SaveFromRemote(remote))
	require.NoError(t, store.SaveFromRemote(remote), "exact replay must be a no-op")

	var documents, mutations int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, remote.SyncID).Scan(&documents))
	require.Equal(t, 1, documents)
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE entity_sync_id = ?`, remote.SyncID).Scan(&mutations))
	require.Zero(t, mutations, "legacy receive must not journal a local mutation")
}

func TestSQLiteApplyProgressImmutableProtectionBlocksGenericMutation(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "sqlite-immutable-protection", 0, 0, "", "apb-13131313131313131313131313131313")
	committed, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)

	var snapshotID, evidenceID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT snapshot_memory_id FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&snapshotID))
	require.NoError(t, store.RawDB().QueryRow(`SELECT id FROM memories WHERE project = ? AND topic_key = ?`, "project", "sdd/change/apply-evidence/"+committed.State.Snapshot.Batches[0].BatchID).Scan(&evidenceID))

	t.Run("blocks arbitrary updates to protected snapshot and evidence", func(t *testing.T) {
		for _, id := range []int64{snapshotID, evidenceID} {
			_, err := store.RawDB().Exec(`UPDATE memories SET title = 'tampered' WHERE id = ?`, id)
			require.Error(t, err)
			require.Contains(t, err.Error(), "immutable apply progress document")
		}
	})

	t.Run("blocks protected soft delete", func(t *testing.T) {
		for _, id := range []int64{snapshotID, evidenceID} {
			require.Error(t, store.DeleteMemory(id, "tester", "generic delete"))
		}
	})

	t.Run("blocks restore of a protected document", func(t *testing.T) {
		var topic, content string
		require.NoError(t, store.RawDB().QueryRow(`SELECT topic_key, content FROM memories WHERE id = ?`, snapshotID).Scan(&topic, &content))
		result, err := store.RawDB().Exec(`INSERT INTO memories (sync_id, project, topic_key, title, content, session_id, deleted_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, "protected-deleted", "project", topic, "deleted protected snapshot", content, "sdd-apply-project")
		require.NoError(t, err)
		deletedID, err := result.LastInsertId()
		require.NoError(t, err)
		require.Error(t, store.RestoreMemory(deletedID, "tester"))
	})

	t.Run("allows ordinary memory update soft delete and restore", func(t *testing.T) {
		id, err := saveTestMemory(t, store, newMemory("ordinary-protection", "ordinary", "content"))
		require.NoError(t, err)
		_, err = store.RawDB().Exec(`UPDATE memories SET title = 'updated ordinary' WHERE id = ?`, id)
		require.NoError(t, err)
		require.NoError(t, store.DeleteMemory(id, "tester", "ordinary delete"))
		require.NoError(t, store.RestoreMemory(id, "tester"))
		memory, err := store.GetMemory(id)
		require.NoError(t, err)
		require.Equal(t, "updated ordinary", memory.Title)
	})

	t.Run("blocks arbitrary receipt update", func(t *testing.T) {
		var original string
		require.NoError(t, store.RawDB().QueryRow(`SELECT response_json FROM sdd_apply_receipts WHERE request_id = ?`, request.RequestID).Scan(&original))
		_, err := store.RawDB().Exec(`UPDATE sdd_apply_receipts SET response_json = 'tampered' WHERE request_id = ?`, request.RequestID)
		require.Error(t, err)
		var persisted string
		require.NoError(t, store.RawDB().QueryRow(`SELECT response_json FROM sdd_apply_receipts WHERE request_id = ?`, request.RequestID).Scan(&persisted))
		require.Equal(t, original, persisted)
	})

	t.Run("blocks receipt delete", func(t *testing.T) {
		_, err := store.RawDB().Exec(`DELETE FROM sdd_apply_receipts WHERE request_id = ?`, request.RequestID)
		require.Error(t, err)
	})

	t.Run("blocks receipt replacement restore", func(t *testing.T) {
		_, err := store.RawDB().Exec(`INSERT OR REPLACE INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) SELECT request_id, project, change_name, payload_sha256, response_json FROM sdd_apply_receipts WHERE request_id = ?`, request.RequestID)
		require.Error(t, err)
	})
}

func TestApplyProgressDocumentsRejectGeneralDeleteAndRestore(t *testing.T) {
	store := openTestDB(t)
	topic := "sdd/change/apply-progress/v2"
	_, err := store.SaveMemory(&models.Memory{Project: "project", TopicKey: &topic, Title: "poison", Content: "{}"})
	require.ErrorIs(t, err, ErrApplyProgressTopicReserved)
	err = store.SaveFromRemote(&models.Memory{SyncID: "reserved-remote", Project: "project", TopicKey: &topic, Title: "poison", Content: "{}"})
	var rejected *RemoteImmutableCreateRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, ImmutableRemoteCreateRejectionInvalidDocument, rejected.RejectionCode)
	var genericCount, mutations, sessions int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE topic_key = ?`, topic).Scan(&genericCount))
	require.Zero(t, genericCount, "generic writes must not create immutable-topic poison")
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memory_mutations`).Scan(&mutations))
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions))
	require.Zero(t, mutations)
	require.Zero(t, sessions)

	result, err := store.AdvanceApplyProgress(applyProgressRequest(t, "request-immutable", 0, 0, "", "apb-12121212121212121212121212121212"))
	require.NoError(t, err)

	var snapshotID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT snapshot_memory_id FROM sdd_apply_heads WHERE project = ? AND change_name = ?`, "project", "change").Scan(&snapshotID))
	require.Error(t, store.DeleteMemory(snapshotID, "tester", "must not mutate evidence"))
	got, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, boundedApplyProgressState(result.State), got)

	_, err = store.RawDB().Exec(`INSERT INTO memories (sync_id, project, topic_key, category, title, content, tags, files_affected, created_by, session_id, deleted_at) VALUES (?, ?, ?, 'architecture', 'immutable', '{}', '[]', '[]', 'tester', ?, CURRENT_TIMESTAMP)`, "restore-immutable", "project", "sdd/change/apply-evidence/apb-34343434343434343434343434343434", "sdd-apply-project")
	require.NoError(t, err)
	var deletedID int64
	require.NoError(t, store.RawDB().QueryRow(`SELECT id FROM memories WHERE sync_id = ?`, "restore-immutable").Scan(&deletedID))
	require.NoError(t, store.RestoreMemory(deletedID, "tester"), "invalid historic topic records must remain recoverable")
	require.NoError(t, store.DeleteMemory(deletedID, "tester", "remove invalid historic topic record"))
}

func TestGetApplyProgressReturnsBoundedSnapshotAndEvidenceLookupReturnsOnlyReferencedBatch(t *testing.T) {
	store := openTestDB(t)
	committed, err := store.AdvanceApplyProgress(applyProgressRequest(t, "get-batches", 0, 0, "", "apb-90909090909090909090909090909090"))
	require.NoError(t, err)

	state, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Empty(t, state.Batches, "guarded snapshot reads must not aggregate historical evidence bodies")
	require.LessOrEqual(t, len(canonicalSnapshotContent(t, state.Snapshot)), applyprogress.MaxDocumentRunes)

	batch, err := store.GetApplyProgressEvidence("project", "change", committed.State.Snapshot.Batches[0].BatchID, committed.State.Digest)
	require.NoError(t, err)
	require.Equal(t, committed.State.Snapshot.Batches[0].BatchID, batch.BatchID)
	require.LessOrEqual(t, len(canonicalBatchContent(t, batch)), applyprogress.MaxDocumentRunes)

	orphan := applyProgressRequest(t, "orphan-evidence", 0, 0, "", "apb-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee").Batches[0]
	_, orphanData, err := applyprogress.SealBatch(orphan)
	require.NoError(t, err)
	tx, err := store.RawDB().Begin()
	require.NoError(t, err)
	require.NoError(t, store.storeApplyProgressBatch(tx, "project", "change", orphan, orphanData))
	require.NoError(t, tx.Commit())
	_, err = store.GetApplyProgressEvidence("project", "change", orphan.BatchID, committed.State.Digest)
	require.ErrorIs(t, err, ErrApplyProgressEvidenceNotFound, "unreferenced immutable history must remain unreadable")

	_, err = store.GetApplyProgressEvidence("project", "change", "apb-ffffffffffffffffffffffffffffffff", committed.State.Digest)
	require.ErrorIs(t, err, ErrApplyProgressEvidenceNotFound)

	_, err = store.GetApplyProgressEvidence("project", "change", committed.State.Snapshot.Batches[0].BatchID, strings.Repeat("f", 64))
	require.ErrorIs(t, err, ErrApplyProgressEvidenceNotFound)
}

func TestApplyRemoteMutationSkipsImmutableApplyProgressDeleteAndRestore(t *testing.T) {
	store := openTestDB(t)
	_, err := store.AdvanceApplyProgress(applyProgressRequest(t, "request-remote-immutable", 0, 0, "", "apb-14141414141414141414141414141414"))
	require.NoError(t, err)
	var syncID string
	require.NoError(t, store.RawDB().QueryRow(`SELECT sync_id FROM memories WHERE project = ? AND topic_key = ?`, "project", "sdd/change/apply-progress/v2").Scan(&syncID))
	for _, op := range []MutationOp{MutationOpDelete, MutationOpRestore} {
		applied, err := store.ApplyRemoteMutation(MutationEnvelope{EventID: "remote-immutable-" + string(op), EntitySyncID: syncID, Project: "project", Op: op})
		require.NoError(t, err)
		require.False(t, applied)
	}
	topic := "sdd/change/apply-progress/v2"
	for _, event := range []MutationEnvelope{
		{EventID: "remote-immutable-update", EntitySyncID: syncID, Project: "project", Op: MutationOpUpdate, Memory: &MutationMemoryPayload{TopicKey: &topic}},
		{EventID: "remote-immutable-delete", EntitySyncID: syncID, Project: "project", Op: MutationOpDelete},
		{EventID: "remote-immutable-restore", EntitySyncID: syncID, Project: "project", Op: MutationOpRestore},
	} {
		applied, err := store.ApplyRemoteMutation(event)
		require.NoError(t, err)
		require.False(t, applied)
	}
	applied, err := store.ApplyRemoteMutation(MutationEnvelope{EventID: "remote-immutable-create", EntitySyncID: "new-immutable", Project: "project", Op: MutationOpCreate, Memory: &MutationMemoryPayload{TopicKey: &topic}})
	require.Error(t, err, "malformed immutable CREATE must stop remote pull rather than be skipped")
	require.False(t, applied)
	var deletedAt sql.NullString
	require.NoError(t, store.RawDB().QueryRow(`SELECT deleted_at FROM memories WHERE sync_id = ?`, syncID).Scan(&deletedAt))
	require.False(t, deletedAt.Valid)
}

func TestApplyRemoteMutationWarnsWhenSkippingImmutableMutations(t *testing.T) {
	store := openTestDB(t)
	_, err := store.AdvanceApplyProgress(applyProgressRequest(t, "request-remote-warning", 0, 0, "", "apb-15151515151515151515151515151515"))
	require.NoError(t, err)
	var syncID string
	require.NoError(t, store.RawDB().QueryRow(`SELECT sync_id FROM memories WHERE project = ? AND topic_key = ?`, "project", "sdd/change/apply-progress/v2").Scan(&syncID))
	var logs bytes.Buffer
	logger.Log.SetOutput(&logs)
	t.Cleanup(func() { logger.Log.SetOutput(os.Stderr) })
	topic := "sdd/change/apply-progress/v2"
	for _, event := range []MutationEnvelope{
		{EventID: "immutable-warning-update", EntitySyncID: syncID, Project: "project", Op: MutationOpUpdate, Memory: &MutationMemoryPayload{TopicKey: &topic, Content: "secret-content"}},
		{EventID: "immutable-warning-delete", EntitySyncID: syncID, Project: "project", Op: MutationOpDelete},
		{EventID: "immutable-warning-restore", EntitySyncID: syncID, Project: "project", Op: MutationOpRestore},
	} {
		applied, err := store.ApplyRemoteMutation(event)
		require.NoError(t, err)
		require.False(t, applied)
	}
	logged := logs.String()
	require.Equal(t, 3, strings.Count(logged, "skipped immutable apply-progress remote mutation"), logged)
	require.NotContains(t, logged, "secret-content")
}

func TestApplyRemoteMutationReconstructsRemoteImmutableSnapshotChain(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "remote-chain-first", 0, 0, "", "apb-17171717171717171717171717171717")
	firstState := ApplyProgressState{Generation: first.Snapshot.Generation, Revision: first.Snapshot.Revision, Digest: first.Snapshot.Digest, Snapshot: first.Snapshot}
	second := applyProgressContinuationUpgrade(t, firstState, "remote-chain-second", "entry-2")
	var err error

	for _, document := range []struct {
		eventID string
		syncID  string
		topic   string
		content string
	}{
		{"remote-chain-first-evidence", "remote-chain-first-evidence-sync", "sdd/change/apply-evidence/" + first.Batches[0].BatchID, canonicalBatchContent(t, first.Batches[0])},
		{"remote-chain-first-snapshot", "remote-chain-first-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, first.Snapshot)},
		{"remote-chain-second-snapshot", "remote-chain-second-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, second.Snapshot)},
	} {
		applied, err := store.ApplyRemoteMutation(immutableCreateEvent(document.eventID, document.syncID, document.topic, document.content))
		require.NoError(t, err)
		require.True(t, applied)
	}

	tx, err := store.RawDB().Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	secondBatches, err := resolveApplyProgressBatches(tx, second.Snapshot)
	require.NoError(t, err)
	require.NoError(t, applyprogress.ValidateSuccessor(first.Snapshot, second.Snapshot, secondBatches))
	require.NoError(t, tx.Rollback())

	reconstructed, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, second.Snapshot.Digest, reconstructed.Digest)
}

func TestApplyRemoteMutationKeepsImmutableSnapshotForkDetectable(t *testing.T) {
	store := openTestDB(t)
	base := applyProgressRequest(t, "remote-fork-base", 0, 0, "", "apb-19191919191919191919191919191919")
	left := applyProgressRequest(t, "remote-fork-left", 1, 1, base.Snapshot.Digest, "apb-20202020202020202020202020202020")
	right := applyProgressRequest(t, "remote-fork-right", 1, 1, base.Snapshot.Digest, "apb-21212121212121212121212121212121")
	for name, successor := range map[string]*ApplyProgressAdvance{"left": &left, "right": &right} {
		successor.Batches[0].Entries[0].EntryID = "entry-" + name
		var err error
		successor.Batches[0], _, err = applyprogress.SealBatch(successor.Batches[0])
		require.NoError(t, err)
		successor.Snapshot.Batches[0].SHA256 = successor.Batches[0].SHA256
		successor.Snapshot.Batches = append([]applyprogress.BatchRef{{BatchID: base.Batches[0].BatchID, SHA256: base.Batches[0].SHA256}}, successor.Snapshot.Batches...)
		successor.Snapshot, _, err = applyprogress.SealSnapshot(successor.Snapshot)
		require.NoError(t, err)
	}

	for _, document := range []struct {
		eventID string
		syncID  string
		topic   string
		content string
	}{
		{"remote-fork-base-evidence", "remote-fork-base-evidence-sync", "sdd/change/apply-evidence/" + base.Batches[0].BatchID, canonicalBatchContent(t, base.Batches[0])},
		{"remote-fork-base-snapshot", "remote-fork-base-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, base.Snapshot)},
		{"remote-fork-left-evidence", "remote-fork-left-evidence-sync", "sdd/change/apply-evidence/" + left.Batches[0].BatchID, canonicalBatchContent(t, left.Batches[0])},
		{"remote-fork-left-snapshot", "remote-fork-left-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, left.Snapshot)},
		{"remote-fork-right-evidence", "remote-fork-right-evidence-sync", "sdd/change/apply-evidence/" + right.Batches[0].BatchID, canonicalBatchContent(t, right.Batches[0])},
		{"remote-fork-right-snapshot", "remote-fork-right-snapshot-sync", "sdd/change/apply-progress/v2", canonicalSnapshotContent(t, right.Snapshot)},
	} {
		applied, err := store.ApplyRemoteMutation(immutableCreateEvent(document.eventID, document.syncID, document.topic, document.content))
		require.NoError(t, err)
		require.True(t, applied)
	}

	_, err := store.GetApplyProgress("project", "change")
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
}

func allowApplyProgressCorruption(t *testing.T, store *DB) {
	t.Helper()
	_, err := store.RawDB().Exec(`DROP TRIGGER protect_sdd_apply_progress_documents`)
	require.NoError(t, err)
}

func boundedApplyProgressState(state ApplyProgressState) ApplyProgressState {
	state.Batches = nil
	return state
}

func canonicalBatchContent(t *testing.T, batch applyprogress.Batch) string {
	t.Helper()
	_, data, err := applyprogress.SealBatch(batch)
	require.NoError(t, err)
	return string(data)
}

func canonicalSnapshotContent(t *testing.T, snapshot applyprogress.Snapshot) string {
	t.Helper()
	_, data, err := applyprogress.SealSnapshot(snapshot)
	require.NoError(t, err)
	return string(data)
}

func TestApplyRemoteMutationRejectsMalformedImmutableApplyProgressCreates(t *testing.T) {
	store := openTestDB(t)
	topic := "sdd/change/apply-progress/v2"

	for _, tt := range []struct {
		name  string
		event MutationEnvelope
	}{
		{
			name:  "missing memory payload",
			event: MutationEnvelope{EventID: "malformed-immutable-missing-payload", EntityType: "memory", EntitySyncID: "malformed-immutable-missing-payload-sync", Project: "project", Op: MutationOpCreate},
		},
		{
			name:  "invalid canonical snapshot",
			event: immutableCreateEvent("malformed-immutable-invalid-content", "malformed-immutable-invalid-content-sync", topic, `{"schema":`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			applied, err := store.ApplyRemoteMutation(tt.event)
			require.Error(t, err)
			require.False(t, applied)
			var count int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, tt.event.EntitySyncID).Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestApplyRemoteMutationImportsImmutableApplyProgressCreatesWithoutOverwrite(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "remote-create-source", 0, 0, "", "apb-17171717171717171717171717171717")
	batch, batchData, err := applyprogress.SealBatch(request.Batches[0])
	require.NoError(t, err)
	snapshot, snapshotData, err := applyprogress.SealSnapshot(request.Snapshot)
	require.NoError(t, err)

	batchTopic := "sdd/change/apply-evidence/" + batch.BatchID
	for _, event := range []MutationEnvelope{
		immutableCreateEvent("remote-evidence-create", "remote-evidence-sync", batchTopic, string(batchData)),
		immutableCreateEvent("remote-snapshot-create", "remote-snapshot-sync", "sdd/change/apply-progress/v2", string(snapshotData)),
	} {
		applied, err := store.ApplyRemoteMutation(event)
		require.NoError(t, err)
		require.True(t, applied)
	}

	reconstructed, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, snapshot.Digest, reconstructed.Digest)

	conflicting := applyProgressRequest(t, "remote-create-conflict", 0, 0, "", "apb-18181818181818181818181818181818")
	_, conflictingData, err := applyprogress.SealSnapshot(conflicting.Snapshot)
	require.NoError(t, err)
	applied, err := store.ApplyRemoteMutation(immutableCreateEvent("remote-snapshot-conflict", "remote-snapshot-sync", "sdd/change/apply-progress/v2", string(conflictingData)))
	require.Error(t, err, "conflicting immutable sync ID must block rather than overwrite")
	require.False(t, applied)
	var stored string
	require.NoError(t, store.RawDB().QueryRow(`SELECT content FROM memories WHERE sync_id = ?`, "remote-snapshot-sync").Scan(&stored))
	require.Equal(t, string(snapshotData), stored, "conflicting CREATE must not overwrite immutable bytes")

	applied, err = store.ApplyRemoteMutation(MutationEnvelope{EventID: "remote-snapshot-delete", EntitySyncID: "remote-snapshot-sync", Project: "project", Op: MutationOpDelete})
	require.NoError(t, err)
	require.False(t, applied)
	recovered, err := store.GetApplyProgress("project", "change")
	require.NoError(t, err)
	require.Equal(t, snapshot.Digest, recovered.Digest)
}

func TestApplyRemoteMutationImmutableClassificationHonorsWritability(t *testing.T) {
	valid := applyProgressRequest(t, "immutable-classification", 0, 0, "", "apb-26262626262626262626262626262626")
	validTopic := "sdd/change/apply-evidence/" + valid.Batches[0].BatchID
	invalidTopic := "sdd/change/apply-progress/v2"

	for _, tt := range []struct {
		name        string
		event       MutationEnvelope
		wantBlocked bool
		wantInvalid bool
	}{
		{
			name:        "ordinary create is blocked before persistence",
			event:       immutableCreateEvent("ordinary-blocked", "ordinary-blocked-sync", "ordinary/topic", "ordinary content"),
			wantBlocked: true,
		},
		{
			name:        "valid immutable create is blocked before session or persistence",
			event:       immutableCreateEvent("immutable-valid-blocked", "immutable-valid-blocked-sync", validTopic, canonicalBatchContent(t, valid.Batches[0])),
			wantBlocked: true,
		},
		{
			name:        "invalid immutable create remains invalid instead of becoming a no-op",
			event:       immutableCreateEvent("immutable-invalid", "immutable-invalid-sync", invalidTopic, `{"schema":`),
			wantInvalid: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestDB(t)
			const project = "project"
			_, err := store.RecordProjectBlock(context.Background(), ProjectBlockCommand{
				CommandID: "block-" + tt.event.EventID, AckToken: "ack-" + tt.event.EventID,
				Project: project, CanonicalProjectKey: canonicalProjectKey(project), Action: "block", Generation: 1,
			})
			require.NoError(t, err)

			applied, err := store.ApplyRemoteMutation(tt.event)
			require.False(t, applied)
			if tt.wantBlocked {
				require.ErrorIs(t, err, ErrProjectBlocked)
			} else if tt.wantInvalid {
				require.Error(t, err)
			}

			var memories, sessions, mutations int
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, tt.event.EntitySyncID).Scan(&memories))
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, tt.event.Memory.SessionID).Scan(&sessions))
			require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, tt.event.EventID).Scan(&mutations))
			require.Zero(t, memories)
			require.Zero(t, sessions)
			require.Zero(t, mutations)
		})
	}
}

func immutableCreateEvent(eventID, syncID, topic, content string) MutationEnvelope {
	return MutationEnvelope{
		EventID: eventID, EntityType: "memory", EntitySyncID: syncID, Project: "project", Op: MutationOpCreate,
		Memory: &MutationMemoryPayload{Project: "project", TopicKey: &topic, Category: "architecture", Title: "immutable apply progress", Content: content, CreatedBy: "peer", SessionID: "manual-save-project"},
	}
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

func TestApplyRemoteMutationReturnsSanitizedRejectionForMalformedImmutableCreate(t *testing.T) {
	store := openTestDB(t)
	const content = `{"schema":"secret remote content"}`
	topic := " sdd/change/apply-progress/v2 "
	event := immutableCreateEvent("malformed-immutable-event", "malformed-immutable-sync", topic, content)
	event.Project = " Project "

	applied, err := store.ApplyRemoteMutation(event)

	require.False(t, applied)
	var rejected *RemoteImmutableCreateRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, event.EventID, rejected.EventID)
	require.Equal(t, "project", rejected.CanonicalProject)
	require.Equal(t, "sdd/change/apply-progress/v2", rejected.Topic)
	require.Equal(t, MutationOpCreate, rejected.Operation)
	require.Equal(t, ImmutableRemoteCreateRejectionInvalidDocument, rejected.RejectionCode)
	digest := sha256.Sum256([]byte(content))
	require.Equal(t, hex.EncodeToString(digest[:]), rejected.PayloadDigest)
	require.NotContains(t, rejected.Error(), content)
	require.NotContains(t, fmt.Sprintf("%+v", rejected), content)

	var memories, mutations, sessions int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE sync_id = ?`, event.EntitySyncID).Scan(&memories))
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memory_mutations WHERE event_id = ?`, event.EventID).Scan(&mutations))
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, event.Memory.SessionID).Scan(&sessions))
	require.Zero(t, memories)
	require.Zero(t, mutations)
	require.Zero(t, sessions)
}

func TestApplyRemoteMutationReturnsSanitizedRejectionForImmutableSyncIDCollision(t *testing.T) {
	store := openTestDB(t)
	first := applyProgressRequest(t, "immutable-collision-first", 0, 0, "", "apb-51515151515151515151515151515151")
	second := applyProgressRequest(t, "immutable-collision-second", 0, 0, "", "apb-52525252525252525252525252525252")
	_, firstContent, err := applyprogress.SealSnapshot(first.Snapshot)
	require.NoError(t, err)
	_, secondContent, err := applyprogress.SealSnapshot(second.Snapshot)
	require.NoError(t, err)

	applied, err := store.ApplyRemoteMutation(immutableCreateEvent("immutable-collision-first-event", "immutable-collision-sync", "sdd/change/apply-progress/v2", string(firstContent)))
	require.NoError(t, err)
	require.True(t, applied)

	applied, err = store.ApplyRemoteMutation(immutableCreateEvent("immutable-collision-second-event", "immutable-collision-sync", "sdd/change/apply-progress/v2", string(secondContent)))

	require.False(t, applied)
	var rejected *RemoteImmutableCreateRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, "immutable-collision-second-event", rejected.EventID)
	require.Equal(t, "project", rejected.CanonicalProject)
	require.Equal(t, "sdd/change/apply-progress/v2", rejected.Topic)
	require.Equal(t, MutationOpCreate, rejected.Operation)
	require.Equal(t, ImmutableRemoteCreateRejectionSyncIDConflict, rejected.RejectionCode)
	digest := sha256.Sum256(secondContent)
	require.Equal(t, hex.EncodeToString(digest[:]), rejected.PayloadDigest)
	require.NotContains(t, rejected.Error(), string(secondContent))

	var stored string
	require.NoError(t, store.RawDB().QueryRow(`SELECT content FROM memories WHERE sync_id = ?`, "immutable-collision-sync").Scan(&stored))
	require.Equal(t, string(firstContent), stored)
}

func deterministicImportedApplyProgressRequest(t *testing.T, requestID, source, tasksContent string) ApplyProgressAdvance {
	t.Helper()
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(tasksContent)
	require.NoError(t, err)
	converted, err := applyprogress.ConvertLegacy(applyprogress.LegacyProgress{Tasks: parsed.Tasks, Completed: parsed.CompletedIDs})
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(source))
	batchID := "apb-" + hex.EncodeToString(sum[:16])
	entryID := "imported-" + hex.EncodeToString(sum[:8])
	entry := applyprogress.EvidenceEntry{EntryID: entryID, TaskIDs: converted.Completed, CompletesTaskIDs: append([]string(nil), converted.Completed...), Kind: applyprogress.EvidenceImported, Summary: "imported legacy progress sha256=" + hex.EncodeToString(sum[:]), Command: "legacy import", Outcome: applyprogress.OutcomePass, Files: []string{"apply-progress.md"}}
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "project", Change: "change", BatchID: batchID, Entries: []applyprogress.EvidenceEntry{entry}})
	require.NoError(t, err)
	coverage := make([]applyprogress.Coverage, 0, len(converted.Completed))
	for _, task := range converted.Tasks {
		for _, completed := range converted.Completed {
			if task.ID == completed {
				coverage = append(coverage, applyprogress.Coverage{TaskID: task.ID, BatchID: batchID, EntryID: entryID})
			}
		}
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: converted.TaskManifestSHA256, Status: applyprogress.StatusComplete, Coverage: coverage, Batches: []applyprogress.BatchRef{{BatchID: batchID, SHA256: batch.SHA256}}})
	require.NoError(t, err)
	return ApplyProgressAdvance{Project: "project", Change: "change", RequestID: requestID, LegacySourceSHA256: applyprogress.LegacySourceSHA256([]byte(source)), Snapshot: snapshot, Batches: []applyprogress.Batch{batch}}
}

func importedApplyProgressRequest(t *testing.T, requestID, batchID, source string) ApplyProgressAdvance {
	t.Helper()
	request := applyProgressRequest(t, requestID, 0, 0, "", batchID)
	request.Batches[0].Entries[0].Kind = applyprogress.EvidenceImported
	var err error
	request.Batches[0], _, err = applyprogress.SealBatch(request.Batches[0])
	require.NoError(t, err)
	request.Snapshot.Batches[0].SHA256 = request.Batches[0].SHA256
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	require.NoError(t, err)
	request.LegacySourceSHA256 = applyprogress.LegacySourceSHA256([]byte(source))
	return request
}

func applyProgressRequest(t *testing.T, requestID string, generation, revision uint64, digest, batchID string) ApplyProgressAdvance {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema: applyprogress.EvidenceSchema, Project: "project", Change: "change", BatchID: batchID,
		Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
	})
	require.NoError(t, err)
	snapshotGeneration := generation
	if snapshotGeneration == 0 {
		snapshotGeneration = 1
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", Generation: snapshotGeneration, Revision: revision + 1, PreviousDigest: digest,
		TaskManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	require.NoError(t, err)
	return ApplyProgressAdvance{Project: "project", Change: "change", RequestID: requestID, ExpectedGeneration: generation, ExpectedRevision: revision, ExpectedDigest: digest, Snapshot: snapshot, Batches: []applyprogress.Batch{batch}}
}

func TestAdvanceApplyProgressRejectsReplayWithTornReferencedEvidence(t *testing.T) {
	store := openTestDB(t)
	request := applyProgressRequest(t, "request-torn-replay", 0, 0, "", "apb-19191919191919191919191919191919")
	_, err := store.AdvanceApplyProgress(request)
	require.NoError(t, err)

	allowApplyProgressCorruption(t, store)
	_, err = store.RawDB().Exec(`UPDATE memories SET content = ? WHERE project = ? AND topic_key = ?`, `{"schema":`, "project", "sdd/change/apply-evidence/"+request.Batches[0].BatchID)
	require.NoError(t, err)

	_, err = store.AdvanceApplyProgress(request)
	require.ErrorIs(t, err, ErrApplyProgressInvalid)
}
