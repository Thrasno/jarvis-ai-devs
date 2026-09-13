package sddprogress

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var errInterrupted = errors.New("interrupted before rename")

func TestOpenSpecAdvanceRejectsTamperedBatchBeforeWriting(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 one\n- [ ] 1.2 two\n- [ ] 1.3 three\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks := []applyprogress.Task{{ID: "1.1", Text: "one"}, {ID: "1.2", Text: "two"}, {ID: "1.3", Text: "three"}}
	entries := []applyprogress.EvidenceEntry{
		{EntryID: "e1", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "e2", TaskIDs: []string{"1.2"}, CompletesTaskIDs: []string{"1.2"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "e3", TaskIDs: []string{"1.3"}, CompletesTaskIDs: []string{"1.3"}, Kind: applyprogress.EvidenceGreen, Summary: "last", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
	}
	planned, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "project", Change: "change", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: "apb-00000000000000000000000000000002"})
	if err != nil {
		t.Fatal(err)
	}
	planned.Batch.Entries[0].Summary = "tampered"

	store := OpenSpec{Root: root}
	_, err = store.Advance(AdvanceRequest{RequestID: "tampered-batch", Snapshot: planned.Snapshot, Batches: []applyprogress.Batch{planned.Batch}})
	if err == nil {
		t.Fatal("Advance() accepted a tampered batch")
	}
	if _, statErr := os.Stat(filepath.Join(root, "apply-progress.md")); !os.IsNotExist(statErr) {
		t.Fatalf("tampered batch wrote a snapshot: %v", statErr)
	}
}

func TestOpenSpecCurrentAndArchiveRejectUnresolvedReceiptSuccessor(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "receipt-head", applyprogress.StatusComplete)
	store := OpenSpec{Root: root}
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	pending := request.Snapshot
	pending.Revision++
	pending.PreviousDigest = request.Snapshot.Digest
	pending, _, err := applyprogress.SealSnapshot(pending)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(immutableReceipt{Payload: strings.Repeat("a", 64), Snapshot: pending})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".apply-progress-receipts", "pending.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Current(); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Current() error = %v, want publication interruption", err)
	}
	if err := store.Archive(filepath.Join(t.TempDir(), "archive")); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Archive() error = %v, want publication interruption", err)
	}
}

func TestOpenSpecRejectsAnonymousCrashStageFiles(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "crash-stage-retry", applyprogress.StatusComplete)
	store := OpenSpec{Root: root}
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	stages := []string{
		filepath.Join(root, ".apply-progress-receipts", ".apply-progress-stage-receipt-crash"),
		filepath.Join(root, "apply-evidence", ".apply-progress-stage-batch-crash"),
	}
	for _, path := range stages {
		if err := os.WriteFile(path, []byte("durably staged but unpublished"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := store.Advance(request); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Advance() error = %v, want anonymous stage residue to fail closed", err)
	}
	for _, path := range stages {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Advance() mutated anonymous stage %q: %v", path, err)
		}
	}
	if err := store.Archive(filepath.Join(t.TempDir(), "archive")); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Archive() error = %v, want anonymous stage residue to fail closed", err)
	}
}

func TestOpenSpecArchiveRejectsAnonymousCrashStageFiles(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "crash-stage-archive", applyprogress.StatusComplete)
	store := OpenSpec{Root: root}
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, ".apply-progress-receipts", ".apply-progress-stage-archive-crash")
	if err := os.WriteFile(stage, []byte("durably staged but unpublished"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "archive")
	if err := store.Archive(archive); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Archive() error = %v, want anonymous stage residue to fail closed", err)
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatalf("Archive() mutated anonymous stage: %v", err)
	}
}

func TestOpenSpecAdvanceBlocksLaterDifferentRequestAfterInterruptedPublication(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	first := request(t, "interrupted-head", "apb-00000000000000000000000000000001", 1, 1, "")
	prepareContinuationSuccessor(t, &first, "apb-00000000000000000000000000000002")
	if _, err := store.Advance(first); err != nil {
		t.Fatal(err)
	}
	pending := successorRequest(t, first, request(t, "interrupted-request", "apb-00000000000000000000000000000002", 2, 2, first.Snapshot.Digest))
	store.BeforeRename = func() error { return errInterrupted }
	if _, err := store.Advance(pending); !errors.Is(err, errInterrupted) {
		t.Fatalf("interrupted Advance() error = %v, want interruption", err)
	}
	if _, err := store.Current(); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Current() error = %v, want publication interruption", err)
	}

	store.BeforeRename = nil
	later := pending
	later.RequestID = "later-request"
	if _, err := store.Advance(later); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("later Advance() error = %v, want publication interruption", err)
	}
	if _, err := store.Advance(pending); err != nil {
		t.Fatalf("exact recovery Advance() error = %v", err)
	}
}

func TestOpenSpecCurrentAndArchiveRejectForkOrphanTornAndStagingReceipts(t *testing.T) {
	for _, tt := range []struct {
		name  string
		write func(t *testing.T, root string, head AdvanceRequest)
	}{
		{
			name: "fork",
			write: func(t *testing.T, root string, head AdvanceRequest) {
				t.Helper()
				fork := head.Snapshot
				fork.Revision++
				fork.PreviousDigest = head.Snapshot.Digest
				fork.TaskManifestSHA256 = strings.Repeat("1", 64)
				fork, _, err := applyprogress.SealSnapshot(fork)
				if err != nil {
					t.Fatal(err)
				}
				writeCanonicalReceipt(t, root, "fork", fork)
			},
		},
		{
			name: "orphan",
			write: func(t *testing.T, root string, head AdvanceRequest) {
				t.Helper()
				orphan := head.Snapshot
				orphan.Generation++
				orphan.Revision = 1
				orphan.PreviousDigest = ""
				orphan.TaskManifestSHA256 = strings.Repeat("2", 64)
				orphan, _, err := applyprogress.SealSnapshot(orphan)
				if err != nil {
					t.Fatal(err)
				}
				writeCanonicalReceipt(t, root, "orphan", orphan)
			},
		},
		{
			name: "torn JSON",
			write: func(t *testing.T, root string, _ AdvanceRequest) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, ".apply-progress-receipts", "torn.json"), []byte(`{"payload":`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "staging residue",
			write: func(t *testing.T, root string, _ AdvanceRequest) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, ".apply-progress-receipts", ".apply-progress-stage-crash"), []byte("staged"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newOpenSpecTestRoot(t)
			request := validArchiveRequest(t, "receipt-"+strings.ReplaceAll(tt.name, " ", "-"), applyprogress.StatusComplete)
			store := OpenSpec{Root: root}
			if _, err := store.Advance(request); err != nil {
				t.Fatal(err)
			}
			tt.write(t, root, request)
			_, err := store.Current()
			if !errors.Is(err, ErrPublicationInterrupted) {
				t.Fatalf("Current() error = %v, want publication interruption", err)
			}
			if tt.name == "staging residue" {
				if _, statErr := os.Stat(filepath.Join(root, ".apply-progress-receipts", ".apply-progress-stage-crash")); statErr != nil {
					t.Fatalf("Current() mutated staging residue: %v", statErr)
				}
			}
			err = store.Archive(filepath.Join(t.TempDir(), "archive"))
			if !errors.Is(err, ErrPublicationInterrupted) {
				t.Fatalf("Archive() error = %v, want publication interruption", err)
			}
		})
	}
}

func writeCanonicalReceipt(t *testing.T, root, id string, snapshot applyprogress.Snapshot) {
	t.Helper()
	data, err := json.Marshal(immutableReceipt{Payload: strings.Repeat("a", 64), Snapshot: snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".apply-progress-receipts", id+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOpenSpecAdvanceRequiresExistingValidTasksRoot(t *testing.T) {
	root := t.TempDir()
	store := OpenSpec{Root: root}
	request := request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := store.Advance(request); !errors.Is(err, ErrInvalidChangeRoot) {
		t.Fatalf("Advance() without tasks.md error = %v, want invalid change root", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [z] 1.1 malformed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(request); !errors.Is(err, ErrInvalidChangeRoot) {
		t.Fatalf("Advance() with malformed tasks.md error = %v, want invalid change root", err)
	}
}

func TestOpenSpecAdvanceRejectsMismatchedTaskManifestBeforeAnyWrite(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := request(t, "mismatched-manifest", "apb-00000000000000000000000000000009", 1, 1, "")
	request.Snapshot.TaskManifestSHA256 = strings.Repeat("0", 64)
	var err error
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := (OpenSpec{Root: root}).Advance(request); err == nil {
		t.Fatal("Advance() succeeded with mismatched task manifest")
	} else {
		var validation *applyprogress.ValidationError
		if !errors.As(err, &validation) || validation.Code != applyprogress.CodeTaskManifestMismatch {
			t.Fatalf("Advance() error = %v, want typed task manifest mismatch", err)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "apply-progress.md"),
		filepath.Join(root, "apply-evidence", request.Batches[0].BatchID+".json"),
		filepath.Join(root, ".apply-progress-receipts", request.RequestID+".json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("mismatched manifest wrote %s: %v", path, err)
		}
	}
}

func TestOpenSpecAdvanceRejectsBatchCollisionAndStaleState(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	first := request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")
	prepareContinuationSuccessor(t, &first, "apb-00000000000000000000000000000002")
	if _, err := store.Advance(first); err != nil {
		t.Fatal(err)
	}
	second := successorRequest(t, first, request(t, "request-2", "apb-00000000000000000000000000000002", 2, 2, first.Snapshot.Digest))
	if _, err := store.Advance(second); err != nil {
		t.Fatal(err)
	}
	wantBatch, wantData, err := applyprogress.SealBatch(first.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	gotBatch, err := os.ReadFile(filepath.Join(root, "apply-evidence", wantBatch.BatchID+".json"))
	if err != nil || !bytes.Equal(gotBatch, wantData) {
		t.Fatalf("reused batch = %q, %v; want byte-identical sealed batch", gotBatch, err)
	}
	collision := successorRequest(t, second, request(t, "request-3", "apb-00000000000000000000000000000001", 3, 3, second.Snapshot.Digest))
	collision.Batches[0].Entries[0].Summary = "different"
	collision.Batches[0], _, _ = applyprogress.SealBatch(collision.Batches[0])
	if _, err := store.Advance(collision); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want invalid continuation conflict", err)
	}
	missing := successorRequest(t, second, request(t, "request-missing", "apb-00000000000000000000000000000003", 3, 3, second.Snapshot.Digest))
	missing.Batches = nil
	if _, err := store.Advance(missing); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want invalid continuation conflict", err)
	}
	stale := request(t, "request-4", "apb-00000000000000000000000000000003", 2, 2, first.Snapshot.Digest)
	if _, err := store.Advance(stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want stale conflict", err)
	}
}

func TestOpenSpecAdvanceReplacesOnlyTornUnreferencedBatchAndReceipt(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := request(t, "torn-request", "apb-00000000000000000000000000000001", 1, 1, "")
	batchPath := filepath.Join(root, "apply-evidence", request.Batches[0].BatchID+".json")
	receiptPath := filepath.Join(root, ".apply-progress-receipts", request.RequestID+".json")
	if err := os.MkdirAll(filepath.Dir(batchPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(batchPath, []byte(`{"schema":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, []byte(`{"payload":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatalf("Advance() with torn files: %v", err)
	}
	batchData, err := os.ReadFile(batchPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyprogress.DecodeCanonicalBatch(batchData); err != nil {
		t.Fatalf("recovered batch = %q: %v", batchData, err)
	}
	receiptData, err := os.ReadFile(receiptPath)
	if err != nil || !bytes.Contains(receiptData, []byte(`"payload"`)) {
		t.Fatalf("recovered receipt = %q, %v", receiptData, err)
	}
}

func TestOpenSpecAdvanceDurablyStagesBeforeInterruptedRename(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	old := request(t, "request-old", "apb-00000000000000000000000000000001", 1, 1, "")
	prepareContinuationSuccessor(t, &old, "apb-00000000000000000000000000000002")
	if _, err := store.Advance(old); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil {
		t.Fatal(err)
	}
	pending := successorRequest(t, old, request(t, "request-pending", "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest))
	store.BeforeRename = func() error { return errInterrupted }
	if _, err := store.Advance(pending); !errors.Is(err, errInterrupted) {
		t.Fatalf("Advance() error = %v, want interruption", err)
	}
	for _, path := range []string{
		filepath.Join(root, "apply-evidence", pending.Batches[0].BatchID+".json"),
		filepath.Join(root, ".apply-progress-receipts", pending.RequestID+".json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("durable staged file %q: %v", path, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("snapshot after interruption = %q, %v; want old snapshot %q", after, err, before)
	}
}

func TestOpenSpecAdvanceDoesNotLeavePartialImmutableFilesAfterInterruptedPublish(t *testing.T) {
	for _, test := range []struct {
		name        string
		interruptAt int
		finalPath   func(string, AdvanceRequest) string
	}{
		{"evidence", 1, func(root string, request AdvanceRequest) string {
			return filepath.Join(root, "apply-evidence", request.Batches[0].BatchID+".json")
		}},
		{"receipt", 2, func(root string, request AdvanceRequest) string {
			return filepath.Join(root, ".apply-progress-receipts", request.RequestID+".json")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newOpenSpecTestRoot(t)
			store := OpenSpec{Root: root}
			old := request(t, "request-old", "apb-00000000000000000000000000000001", 1, 1, "")
			prepareContinuationSuccessor(t, &old, "apb-00000000000000000000000000000002")
			if _, err := store.Advance(old); err != nil {
				t.Fatal(err)
			}
			pending := successorRequest(t, old, request(t, "request-pending", "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest))
			published := 0
			store.BeforeImmutablePublish = func() error {
				published++
				if published == test.interruptAt {
					return errInterrupted
				}
				return nil
			}
			if _, err := store.Advance(pending); !errors.Is(err, errInterrupted) {
				t.Fatalf("Advance() error = %v, want interruption", err)
			}
			if _, err := os.Stat(test.finalPath(root, pending)); !os.IsNotExist(err) {
				t.Fatalf("partial immutable file stat error = %v, want no final file", err)
			}
		})
	}
}

func TestOpenSpecAdvanceFsyncsCreatedEvidenceAndReceipts(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	var dirs []string
	store := OpenSpec{Root: root, SyncDir: func(path string) error { dirs = append(dirs, path); return nil }}
	if _, err := store.Advance(request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{filepath.Join(root, "apply-evidence"), filepath.Join(root, ".apply-progress-receipts")} {
		if !contains(dirs, want) {
			t.Fatalf("synced directories = %q, missing %q", dirs, want)
		}
	}
}

func TestOpenSpecAdvanceRejectsNewRequestWithCurrentSnapshotAndStaleCoordinates(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	first := request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := store.Advance(first); err != nil {
		t.Fatal(err)
	}

	newRequest := first
	newRequest.RequestID = "request-2"
	if _, err := store.Advance(newRequest); !errors.Is(err, ErrConflict) && !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("Advance() error = %v, want guarded request conflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".apply-progress-receipts", newRequest.RequestID+".json")); !os.IsNotExist(err) {
		t.Fatalf("new request receipt stat error = %v, want no receipt for conflict", err)
	}
}

func TestOpenSpecAdvanceRetriesDirectorySyncForExistingImmutableBytes(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	var evidenceSyncs int
	store := OpenSpec{Root: root, SyncDir: func(path string) error {
		if path == filepath.Join(root, "apply-evidence") {
			evidenceSyncs++
			if evidenceSyncs == 1 {
				return errInterrupted
			}
		}
		return nil
	}}
	advance := request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := store.Advance(advance); !errors.Is(err, errInterrupted) {
		t.Fatalf("first Advance() error = %v, want directory-sync interruption", err)
	}
	if _, err := store.Advance(advance); err != nil {
		t.Fatalf("retry Advance() error = %v", err)
	}
	if evidenceSyncs != 2 {
		t.Fatalf("evidence directory syncs = %d, want retry after existing immutable bytes", evidenceSyncs)
	}
}

func TestOpenSpecAdvanceRecoversFromDeadLockFile(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	if err := os.WriteFile(store.lockPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")); err != nil {
		t.Fatalf("Advance() with crash-left lock file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.lock")); !os.IsNotExist(err) {
		t.Fatalf("in-root lock stat error = %v, want no lock in archive topology", err)
	}
}

func TestOpenSpecAdvanceRetriesInterruptedRequestOnce(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	old := request(t, "request-old", "apb-00000000000000000000000000000001", 1, 1, "")
	prepareContinuationSuccessor(t, &old, "apb-00000000000000000000000000000002")
	if _, err := store.Advance(old); err != nil {
		t.Fatal(err)
	}
	pending := successorRequest(t, old, request(t, "request-pending", "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest))
	store.BeforeRename = func() error { return errInterrupted }
	if _, err := store.Advance(pending); !errors.Is(err, errInterrupted) {
		t.Fatalf("first Advance() error = %v, want interruption", err)
	}
	store.BeforeRename = nil
	if _, err := store.Advance(pending); err != nil {
		t.Fatalf("retry Advance() error = %v", err)
	}
	published, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil || string(published) == old.Snapshot.Digest || !bytes.Contains(published, []byte(pending.Snapshot.Digest)) {
		t.Fatalf("retry snapshot = %q, %v; want published pending digest", published, err)
	}
	if _, err := store.Advance(pending); err != nil {
		t.Fatalf("idempotent Advance() error = %v", err)
	}
	changed := request(t, pending.RequestID, "apb-00000000000000000000000000000003", 3, 3, pending.Snapshot.Digest)
	if _, err := store.Advance(changed); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("changed request Advance() error = %v, want request conflict", err)
	}
	for name, change := range map[string]func(*AdvanceRequest){
		"generation": func(r *AdvanceRequest) { r.ExpectedGeneration++ },
		"revision":   func(r *AdvanceRequest) { r.ExpectedRevision++ },
		"digest": func(r *AdvanceRequest) {
			r.ExpectedDigest = "0000000000000000000000000000000000000000000000000000000000000000"
		},
	} {
		t.Run("changed CAS "+name, func(t *testing.T) {
			changed := pending
			change(&changed)
			if _, err := store.Advance(changed); !errors.Is(err, ErrRequestConflict) {
				t.Fatalf("Advance() error = %v, want request conflict", err)
			}
		})
	}
}

func TestOpenSpecAdvanceReplaysCommittedReceiptAfterSuccessorAndRejectsTornEvidence(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	first := request(t, "delayed-replay-a", "apb-00000000000000000000000000000031", 1, 1, "")
	secondBatchID := "apb-00000000000000000000000000000032"
	thirdBatchID := "apb-00000000000000000000000000000033"
	secondEntry := first.Batches[0].Entries[0]
	secondEntry.EntryID = "entry-" + secondBatchID[4:]
	thirdEntry := secondEntry
	thirdEntry.EntryID = "entry-" + thirdBatchID[4:]
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{first.Batches[0].Entries[0], secondEntry, thirdEntry})
	if err != nil {
		t.Fatal(err)
	}
	first.Snapshot.StreamSHA256, first.Snapshot.NextEntryIndex, first.Snapshot.NextEntryID = stream, 1, secondEntry.EntryID
	first.Snapshot, _, err = applyprogress.SealSnapshot(first.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.Advance(first)
	if err != nil {
		t.Fatal(err)
	}
	second := successorRequest(t, first, request(t, "delayed-replay-b", secondBatchID, 2, 2, first.Snapshot.Digest))
	second.Snapshot.NextEntryID = thirdEntry.EntryID
	second.Snapshot, _, err = applyprogress.SealSnapshot(second.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(second); err != nil {
		t.Fatal(err)
	}
	third := successorRequest(t, second, request(t, "delayed-replay-c", thirdBatchID, 3, 3, second.Snapshot.Digest))
	if _, err := store.Advance(third); err != nil {
		t.Fatal(err)
	}

	replayed, err := store.Advance(first)
	if err != nil || replayed != committed {
		t.Fatalf("delayed replay = %#v, %v; want %#v, nil", replayed, err, committed)
	}

	changed := first
	changed.Batches = append([]applyprogress.Batch(nil), first.Batches...)
	changed.Batches[0].Entries = append([]applyprogress.EvidenceEntry(nil), first.Batches[0].Entries...)
	changed.Snapshot.Batches = append([]applyprogress.BatchRef(nil), first.Snapshot.Batches...)
	changed.Batches[0].Entries[0].Summary = "changed payload"
	changed.Batches[0], _, err = applyprogress.SealBatch(changed.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	changed.Snapshot.Batches[0].SHA256 = changed.Batches[0].SHA256
	changed.Snapshot, _, err = applyprogress.SealSnapshot(changed.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(changed); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("changed delayed replay error = %v, want request conflict", err)
	}

	batchPath := filepath.Join(root, "apply-evidence", first.Batches[0].BatchID+".json")
	if err := os.WriteFile(batchPath, []byte(`{"schema":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(first); !errors.Is(err, ErrMissingBatch) {
		t.Fatalf("delayed replay with torn evidence error = %v, want missing batch", err)
	}
}

func TestOpenSpecAdvancePersistsOutOfTaskOrderSplitStream(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1 first\n- [ ] 2 second\n- [ ] 3 third\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks := []applyprogress.Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}, {ID: "3", Text: "third"}}
	entries := []applyprogress.EvidenceEntry{
		{EntryID: "third", TaskIDs: []string{"3"}, CompletesTaskIDs: []string{"3"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "first", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{"1"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
		{EntryID: "second", TaskIDs: []string{"2"}, CompletesTaskIDs: []string{"2"}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("x", 21000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}},
	}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Entries: entries, EntryID: "third", BatchID: "apb-00000000000000000000000000000081"})
	if err != nil || first.Outcome != applyprogress.PlanContinuationRequired {
		t.Fatalf("first plan = %#v, %v", first, err)
	}
	store := OpenSpec{Root: root}
	if _, err := store.Advance(AdvanceRequest{RequestID: "out-of-order-first", Snapshot: first.Snapshot, Batches: []applyprogress.Batch{first.Batch}}); err != nil {
		t.Fatal(err)
	}
	second, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "jarvis-dev", Change: "issue-653", Base: &first.Snapshot, Tasks: tasks, Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256, BatchID: "apb-00000000000000000000000000000082"})
	if err != nil || second.Outcome != applyprogress.PlanContinuationRequired {
		t.Fatalf("second plan = %#v, %v", second, err)
	}
	if _, err := store.Advance(AdvanceRequest{RequestID: "out-of-order-second", ExpectedGeneration: first.Snapshot.Generation, ExpectedRevision: first.Snapshot.Revision, ExpectedDigest: first.Snapshot.Digest, Snapshot: second.Snapshot, Batches: []applyprogress.Batch{second.Batch}}); err != nil {
		t.Fatalf("second advance = %v", err)
	}
	current, err := store.Current()
	if err != nil || current == nil || len(current.Coverage) != 2 || current.Coverage[0].TaskID != "1" || current.Coverage[1].TaskID != "3" {
		t.Fatalf("current manifest-ordered coverage = %#v, %v", current, err)
	}
}

func TestOpenSpecAdvanceRejectsCompletedEvidenceOmittedFromCoverage(t *testing.T) {
	store := OpenSpec{Root: newOpenSpecTestRoot(t)}
	r := request(t, "request-omitted-coverage", "apb-00000000000000000000000000000001", 1, 1, "")
	r.Batches[0].Entries[0].TaskIDs = []string{"task"}
	r.Batches[0].Entries[0].CompletesTaskIDs = []string{"task"}
	var err error
	r.Batches[0], _, err = applyprogress.SealBatch(r.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	r.Snapshot.Batches[0].SHA256 = r.Batches[0].SHA256
	r.Snapshot, _, err = applyprogress.SealSnapshot(r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(r); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want rejected omitted coverage", err)
	}
}

func TestOpenSpecAdvanceRejectsInvalidInitialEvidenceTopologyBeforePublication(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := request(t, "initial-duplicate-entry", "apb-00000000000000000000000000000091", 1, 1, "")
	duplicate := request.Batches[0]
	duplicate.BatchID = "apb-00000000000000000000000000000092"
	var err error
	duplicate, _, err = applyprogress.SealBatch(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	request.Batches = append(request.Batches, duplicate)
	request.Snapshot.Batches = append(request.Snapshot.Batches, applyprogress.BatchRef{BatchID: duplicate.BatchID, SHA256: duplicate.SHA256})
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := (OpenSpec{Root: root}).Advance(request); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want invalid initial topology conflict", err)
	}
	for _, path := range []string{
		filepath.Join(root, "apply-progress.md"),
		filepath.Join(root, "apply-evidence", request.Batches[0].BatchID+".json"),
		filepath.Join(root, ".apply-progress-receipts", request.RequestID+".json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid initial topology published %s: %v", path, err)
		}
	}
}

func TestOpenSpecAdvanceRejectsInvalidPublicationTopology(t *testing.T) {
	for name, mutate := range map[string]func(*AdvanceRequest){
		"successor": func(r *AdvanceRequest) { r.Snapshot.Generation++ },
		"previous digest": func(r *AdvanceRequest) {
			r.Snapshot.PreviousDigest = "0000000000000000000000000000000000000000000000000000000000000000"
		},
		"batch identity": func(r *AdvanceRequest) {
			r.Batches[0].Project = "other"
			r.Batches[0], _, _ = applyprogress.SealBatch(r.Batches[0])
			r.Snapshot.Batches[0].SHA256 = r.Batches[0].SHA256
		},
		"coverage": func(r *AdvanceRequest) {
			r.Snapshot.Coverage = []applyprogress.Coverage{{TaskID: "task", BatchID: r.Batches[0].BatchID, EntryID: "entry"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := OpenSpec{Root: newOpenSpecTestRoot(t)}
			old := request(t, "request-old", "apb-00000000000000000000000000000001", 1, 1, "")
			if _, err := store.Advance(old); err != nil {
				t.Fatal(err)
			}
			r := request(t, "topology-"+strings.ReplaceAll(name, " ", "-"), "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest)
			mutate(&r)
			r.Snapshot, _, _ = applyprogress.SealSnapshot(r.Snapshot)
			if _, err := store.Advance(r); !errors.Is(err, ErrConflict) && !errors.Is(err, ErrMissingBatch) {
				t.Fatalf("Advance() error = %v, want rejected topology", err)
			}
		})
	}
}

func newOpenSpecTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string][]byte{
		"tasks.md":         []byte("- [x] 1.1 task\n"),
		"proposal.md":      []byte("# Proposal\n"),
		"design.md":        []byte("# Design\n"),
		"verify-report.md": []byte("## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"),
		filepath.Join("specs", "base", "spec.md"): []byte("# Delta\n"),
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestOpenSpecArchiveDoesNotRequireOwnArchiveReport(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	request := validArchiveRequest(t, "archive-without-report", applyprogress.StatusComplete)
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "archive-report.md")); !os.IsNotExist(err) {
		t.Fatalf("archive report stat error = %v, want no pre-created archive report", err)
	}

	archive := filepath.Join(t.TempDir(), "archive")
	if err := store.Archive(archive); err != nil {
		t.Fatalf("Archive() without archive-report.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archive, "archive-report.md")); !os.IsNotExist(err) {
		t.Fatalf("archived archive report stat error = %v, want no archive report", err)
	}
}

func TestOpenSpecArchiveMovesSnapshotEvidenceAndReceiptsTogether(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	request := validArchiveRequest(t, "archive-request", applyprogress.StatusComplete)
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		filepath.Join(root, "specs", "delivery", "spec.md"):           []byte("# Delivery Delta\n"),
		filepath.Join(root, "specs", "delivery", "nested", "spec.md"): []byte("# Nested Delivery Delta\n"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archiveRoot := filepath.Join(t.TempDir(), "issue-653")
	if err := store.Archive(archiveRoot); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(archiveRoot, "apply-progress.md"),
		filepath.Join(archiveRoot, "apply-evidence", request.Batches[0].BatchID+".json"),
		filepath.Join(archiveRoot, ".apply-progress-receipts", request.RequestID+".json"),
		filepath.Join(archiveRoot, "specs", "delivery", "spec.md"),
		filepath.Join(archiveRoot, "specs", "delivery", "nested", "spec.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("archived topology %q: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); !os.IsNotExist(err) {
		t.Fatalf("active snapshot stat error = %v, want moved", err)
	}
}

func TestOpenSpecArchiveAcceptsReceiptAncestorChain(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	first := request(t, "ancestor-first", "apb-00000000000000000000000000000041", 1, 1, "")
	secondBatchID := "apb-00000000000000000000000000000042"
	secondBase := request(t, "ancestor-second", secondBatchID, 2, 2, first.Snapshot.Digest)
	secondBase.Batches[0].Entries[0].EntryID = "entry-" + secondBatchID[4:]
	secondBase.Batches[0].Entries[0].TaskIDs, secondBase.Batches[0].Entries[0].CompletesTaskIDs = []string{"1.1"}, []string{"1.1"}
	var err error
	secondBase.Batches[0], _, err = applyprogress.SealBatch(secondBase.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	firstStream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{first.Batches[0].Entries[0], secondBase.Batches[0].Entries[0]})
	if err != nil {
		t.Fatal(err)
	}
	first.Snapshot.StreamSHA256, first.Snapshot.NextEntryIndex, first.Snapshot.NextEntryID = firstStream, 1, secondBase.Batches[0].Entries[0].EntryID
	first.Snapshot, _, err = applyprogress.SealSnapshot(first.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(first); err != nil {
		t.Fatal(err)
	}
	secondBase.ExpectedDigest, secondBase.Snapshot.PreviousDigest = first.Snapshot.Digest, first.Snapshot.Digest
	second := successorRequest(t, first, secondBase)
	second.Snapshot.Status = applyprogress.StatusComplete
	second.Snapshot.Coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: second.Batches[0].BatchID, EntryID: second.Batches[0].Entries[0].EntryID}}
	second.Snapshot.NextEntryIndex, second.Snapshot.NextEntryID, second.Snapshot.StreamSHA256 = 0, "", ""
	second.Snapshot, _, err = applyprogress.SealSnapshot(second.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(second); err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(filepath.Join(t.TempDir(), "archive")); err != nil {
		t.Fatalf("Archive() with receipt ancestor chain: %v", err)
	}
}

func TestOpenSpecArchiveRejectsTornReceiptsAndStagingDebris(t *testing.T) {
	for _, tt := range []struct {
		name string
		path string
		data []byte
	}{
		{name: "torn JSON receipt", path: filepath.Join(".apply-progress-receipts", "torn.json"), data: []byte(`{"payload":`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newOpenSpecTestRoot(t)
			request := validArchiveRequest(t, "archive-"+strings.ReplaceAll(tt.name, " ", "-"), applyprogress.StatusComplete)
			store := OpenSpec{Root: root}
			if _, err := store.Advance(request); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, tt.path)
			if err := os.WriteFile(path, tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := store.Archive(filepath.Join(t.TempDir(), "issue-653")); err == nil {
				t.Fatal("Archive() accepted unvalidated staging or receipt topology")
			}
			if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); err != nil {
				t.Fatalf("archive moved invalid topology: %v", err)
			}
		})
	}
}

func TestOpenSpecArchiveCleansTrailingSeparatorsAndLeavesLockOutsideTopology(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "archive-trailing", applyprogress.StatusComplete)
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	archiveRoot := filepath.Join(t.TempDir(), "archive")
	if err := (OpenSpec{Root: root + string(os.PathSeparator)}).Archive(archiveRoot + string(os.PathSeparator)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(archiveRoot, "apply-progress.md")); err != nil {
		t.Fatalf("archived progress: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archiveRoot, "."+filepath.Base(root)+".apply-progress.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock moved into archive topology: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "."+filepath.Base(root)+".apply-progress.lock")); err != nil {
		t.Fatalf("external lock missing: %v", err)
	}
}

func TestOpenSpecArchiveAcceptsCurrentTaskManifest(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "current-manifest", applyprogress.StatusComplete)
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (OpenSpec{Root: root}).Archive(filepath.Join(t.TempDir(), "issue-653")); err != nil {
		t.Fatalf("Archive() with current manifest: %v", err)
	}
}

func TestOpenSpecArchiveRejectsPartialReplacementAfterLock(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "replacement", applyprogress.StatusComplete)
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	partial := request.Snapshot
	partial.Status = applyprogress.StatusPartial
	_, data, err := applyprogress.SealSnapshot(partial)
	if err != nil {
		t.Fatal(err)
	}
	store := OpenSpec{Root: root, BeforeArchiveValidate: func() error {
		return os.WriteFile(filepath.Join(root, "apply-progress.md"), data, 0o600)
	}}
	if err := store.Archive(filepath.Join(t.TempDir(), "issue-653")); err == nil {
		t.Fatal("Archive() accepted partial replacement after lock")
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); err != nil {
		t.Fatalf("replacement moved despite rejection: %v", err)
	}
}

func TestOpenSpecArchiveRejectsPartialSnapshotUnderLock(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "partial", applyprogress.StatusPartial)
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (OpenSpec{Root: root}).Archive(filepath.Join(t.TempDir(), "issue-653")); err == nil {
		t.Fatal("Archive() succeeded for partial snapshot")
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); err != nil {
		t.Fatalf("partial snapshot moved: %v", err)
	}
}

func TestOpenSpecArchiveRejectsInvalidTasks(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := request(t, "missing-tasks", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [z] 1.1 malformed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (OpenSpec{Root: root}).Archive(filepath.Join(t.TempDir(), "issue-653")); err == nil {
		t.Fatal("Archive() succeeded with invalid current tasks")
	}
}

func TestOpenSpecArchiveRejectsStaleTaskManifest(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	request := request(t, "stale-manifest", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 changed task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(filepath.Join(t.TempDir(), "issue-653")); err == nil {
		t.Fatal("Archive() succeeded with a stale task manifest")
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); err != nil {
		t.Fatalf("archive moved stale progress: %v", err)
	}
}

func validArchiveRequest(t *testing.T, requestID string, status applyprogress.Status) AdvanceRequest {
	t.Helper()
	r := request(t, requestID, "apb-00000000000000000000000000000001", 1, 1, "")
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	if status == applyprogress.StatusComplete {
		r.Batches[0].Entries[0].TaskIDs = []string{"1.1"}
		r.Batches[0].Entries[0].CompletesTaskIDs = []string{"1.1"}
		r.Batches[0], _, err = applyprogress.SealBatch(r.Batches[0])
		if err != nil {
			t.Fatal(err)
		}
		r.Snapshot.Coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: r.Batches[0].BatchID, EntryID: "entry"}}
	}
	r.Snapshot.Status, r.Snapshot.TaskManifestSHA256 = status, manifest
	r.Snapshot.Batches[0].SHA256 = r.Batches[0].SHA256
	r.Snapshot, _, err = applyprogress.SealSnapshot(r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLegacyConversionAcceptsHistoricalPhaseCheckboxes(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte(`# Historical Tasks
## Compatibility
- [x] RED: reproduce the defect
- [x] GREEN: accept the historical snapshot
- [x] TRIANGULATE: exercise alternate input
- [x] REFACTOR: clarify the decoder

## Parent Actions
- [x] RED: workflow prose
`), 0o600); err != nil {
		t.Fatal(err)
	}
	converted, status, err := legacyConversion([]byte("status: complete\n"), tasks)
	if err != nil || status != applyprogress.StatusComplete || len(converted.Tasks) != 4 || len(converted.Completed) != 4 {
		t.Fatalf("legacyConversion() = %#v, %q, %v", converted, status, err)
	}
	for _, task := range converted.Tasks {
		if !strings.HasPrefix(task.ID, "legacy-") || task.Path == "" {
			t.Fatalf("legacy task identity = %#v, want path-derived legacy ID", task)
		}
	}
}

func TestLegacyConversionRejectsOneFieldTaskRow(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte("- [x] 1.1 task\n- [x] garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := legacyConversion([]byte("status: complete\n"), tasks); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("legacyConversion error = %v", err)
	}
}

func TestLegacyConversionRejectsNondigitMultiwordTaskID(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte("- [x] 1.1 task\n- [x] garbage words\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := legacyConversion([]byte("status: complete\n"), tasks); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("legacyConversion error = %v", err)
	}
}

func TestLegacyConversionRejectsMalformedTaskMarker(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte("- [x] 1.1 task\n- [z] 1.2 malformed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := legacyConversion([]byte("status: complete\n"), tasks); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("legacyConversion error = %v, want migration failure", err)
	}
}

func TestLegacyConversionRejectsCompleteMarkerWithIncompleteTasks(t *testing.T) {
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte("- [x] 1.1 done\n- [ ] 1.2 pending\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := legacyConversion([]byte("status: complete\n"), tasks); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("legacyConversion error = %v, want migration failure", err)
	}
}

func TestOpenSpecLegacyUpgradeRejectsFabricatedGreenEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := legacyRequest(t, "legacy-green")
	request.Batches[0].Entries[0].Kind = applyprogress.EvidenceGreen
	var err error
	request.Batches[0], _, err = applyprogress.SealBatch(request.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	request.Snapshot.Batches[0].SHA256 = request.Batches[0].SHA256
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := (OpenSpec{Root: root}).UpgradeLegacy(request); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("UpgradeLegacy() error = %v, want legacy migration failure", err)
	}
}

func TestOpenSpecLegacyUpgradeOwnsProvenanceAndExactRecoveryPayload(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := legacyRequest(t, "legacy-provenance")
	request.LegacySourceSHA256 = applyprogress.LegacySourceSHA256([]byte("caller-controlled"))
	expected := request
	expected.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(legacy)
	_, snapshotData, err := applyprogress.SealSnapshot(expected.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	wantPayload, err := payloadDigest(snapshotData, expected)
	if err != nil {
		t.Fatal(err)
	}

	store := OpenSpec{Root: root}
	result, upgraded, err := store.UpgradeLegacy(request)
	if err != nil || !upgraded {
		t.Fatalf("UpgradeLegacy() = %#v, %t, %v", result, upgraded, err)
	}
	if result.PayloadSHA256 != wantPayload {
		t.Fatalf("payload = %q, want source-bound %q", result.PayloadSHA256, wantPayload)
	}
	// The exact source-bound payload remains the only low-level recovery replay.
	replayed, err := store.Advance(expected)
	if err != nil || replayed != result {
		t.Fatalf("Advance(exact recovery) = %#v, %v; want %#v, nil", replayed, err, result)
	}
	changed := expected
	changed.LegacySourceSHA256 = applyprogress.LegacySourceSHA256([]byte("changed"))
	if _, err := store.Advance(changed); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("Advance(changed provenance) error = %v, want request conflict", err)
	}
}

func TestOpenSpecAdvanceRejectsCallerSuppliedImportedProvenance(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := legacyRequest(t, "caller-imported")
	request.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(legacy)
	if _, err := (OpenSpec{Root: root}).Advance(request); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("Advance() error = %v, want legacy migration failure", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("legacy source after rejected advance = %q, %v", got, err)
	}
}

func TestOpenSpecLegacyUpgradeNormalizesFullCoverageDespitePartialMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: partial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := legacyRequest(t, "legacy-full-coverage")
	result, upgraded, err := (OpenSpec{Root: root}).UpgradeLegacy(request)
	if err != nil || !upgraded || result.Generation != 1 {
		t.Fatalf("upgrade = %#v, %t, %v; want authoritative complete conversion", result, upgraded, err)
	}
	current, err := (OpenSpec{Root: root}).Current()
	if err != nil || current == nil || current.Status != applyprogress.StatusComplete {
		t.Fatalf("current = %#v, %v; want complete snapshot", current, err)
	}
}

func TestOpenSpecLegacyUpgradeCommitsOnlyAfterConversion(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := legacyRequest(t, "legacy")
	result, upgraded, err := (OpenSpec{Root: root}).UpgradeLegacy(r)
	if err != nil || !upgraded || result.Generation != 1 {
		t.Fatalf("upgrade = %#v, %t, %v", result, upgraded, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil || !bytes.Contains(data, []byte(r.Snapshot.Digest)) {
		t.Fatalf("published = %q, %v", data, err)
	}
}

func TestOpenSpecLegacyUpgradeFailurePreservesSourceAndReadOnlyArchive(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n- [x] 1.1 duplicate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := legacyRequest(t, "legacy")
	if _, _, err := (OpenSpec{Root: root}).UpgradeLegacy(r); !errors.Is(err, ErrLegacyMigration) {
		t.Fatalf("upgrade error = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "apply-progress.md")); !bytes.Equal(got, legacy) {
		t.Fatalf("legacy source changed: %q", got)
	}
	if err := (OpenSpec{Root: root}).Archive(filepath.Join(t.TempDir(), "archive")); err == nil {
		t.Fatal("legacy archive succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.lock")); !os.IsNotExist(err) {
		t.Fatalf("read-only legacy archive wrote lock: %v", err)
	}
}

func TestOpenSpecLegacyUpgradeRestoresSourceAfterPostRenameSyncFailure(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := legacyRequest(t, "legacy-sync")
	store := OpenSpec{Root: root, SyncDir: func(path string) error {
		if path == root {
			return errInterrupted
		}
		return nil
	}}
	if _, _, err := store.UpgradeLegacy(r); !errors.Is(err, errInterrupted) {
		t.Fatalf("upgrade error = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "apply-progress.md")); !bytes.Equal(got, legacy) {
		t.Fatalf("legacy source changed: %q", got)
	}
	result, upgraded, err := (OpenSpec{Root: root}).UpgradeLegacy(r)
	if err != nil || !upgraded || result.Generation != 1 {
		t.Fatalf("retry upgrade = %#v, %t, %v", result, upgraded, err)
	}
}

func TestOpenSpecLegacyUpgradeInterruptedCommitPreservesSource(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := legacyRequest(t, "legacy")
	_, _, err := (OpenSpec{Root: root, BeforeRename: func() error { return errInterrupted }}).UpgradeLegacy(r)
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("upgrade error = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "apply-progress.md")); !bytes.Equal(got, legacy) {
		t.Fatalf("legacy source changed: %q", got)
	}
	result, upgraded, err := (OpenSpec{Root: root}).UpgradeLegacy(r)
	if err != nil || !upgraded || result.Generation != 1 {
		t.Fatalf("retry upgrade = %#v, %t, %v", result, upgraded, err)
	}
}

func legacyRequest(t *testing.T, requestID string) AdvanceRequest {
	t.Helper()
	r := validArchiveRequest(t, requestID, applyprogress.StatusComplete)
	// A legacy source retains its path-and-normalized-text hash; native task IDs
	// are not authorized to replace the historical migration identity.
	legacyID, err := applyprogress.LegacyTaskID("1.1", "task")
	if err != nil {
		t.Fatal(err)
	}
	r.Batches[0].Entries[0].TaskIDs, r.Batches[0].Entries[0].CompletesTaskIDs = []string{legacyID}, []string{legacyID}
	r.Batches[0].Entries[0].Kind = applyprogress.EvidenceImported
	r.Batches[0], _, err = applyprogress.SealBatch(r.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.LegacyTaskManifest([]applyprogress.Task{{Path: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	r.Snapshot.TaskManifestSHA256, r.Snapshot.Coverage = manifest, []applyprogress.Coverage{{TaskID: legacyID, BatchID: r.Batches[0].BatchID, EntryID: "entry"}}
	r.Snapshot.Batches[0].SHA256 = r.Batches[0].SHA256
	r.Snapshot, _, err = applyprogress.SealSnapshot(r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func prepareContinuationSuccessor(t *testing.T, request *AdvanceRequest, successorBatchID string) {
	t.Helper()
	next := request.Batches[0].Entries[0]
	next.EntryID = "entry-" + successorBatchID[4:]
	entries := append(append([]applyprogress.EvidenceEntry{}, request.Batches[0].Entries...), next)
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	request.Snapshot.StreamSHA256, request.Snapshot.NextEntryIndex, request.Snapshot.NextEntryID = stream, len(request.Batches[0].Entries), next.EntryID
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
}

func successorRequest(t *testing.T, prior, next AdvanceRequest) AdvanceRequest {
	t.Helper()
	for index := range next.Batches {
		for entry := range next.Batches[index].Entries {
			next.Batches[index].Entries[entry].EntryID = "entry-" + next.Batches[index].BatchID[4:]
		}
		var err error
		next.Batches[index], _, err = applyprogress.SealBatch(next.Batches[index])
		if err != nil {
			t.Fatal(err)
		}
		for ref := range next.Snapshot.Batches {
			if next.Snapshot.Batches[ref].BatchID == next.Batches[index].BatchID {
				next.Snapshot.Batches[ref].SHA256 = next.Batches[index].SHA256
			}
		}
	}
	next.Snapshot.Batches = append(append([]applyprogress.BatchRef{}, prior.Snapshot.Batches...), next.Snapshot.Batches...)
	next.Snapshot.StreamSHA256 = prior.Snapshot.StreamSHA256
	next.Snapshot.NextEntryIndex = prior.Snapshot.NextEntryIndex + len(next.Batches[0].Entries)
	next.Snapshot.NextEntryID = "entry-end"
	var err error
	next.Snapshot, _, err = applyprogress.SealSnapshot(next.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func request(t *testing.T, requestID, batchID string, generation, revision uint64, previous string) AdvanceRequest {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "issue-653", BatchID: batchID,
		Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	epoch := uint64(1)
	expectedGeneration := uint64(0)
	if revision > 1 {
		expectedGeneration = epoch
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: epoch, Revision: revision, PreviousDigest: previous,
		TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return AdvanceRequest{RequestID: requestID, ExpectedGeneration: expectedGeneration, ExpectedRevision: revision - 1, ExpectedDigest: previous, Batches: []applyprogress.Batch{batch}, Snapshot: snapshot}
}
