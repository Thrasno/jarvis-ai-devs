package sddprogress

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var errInterrupted = errors.New("interrupted before rename")

func TestOpenSpecAdvanceRejectsBatchCollisionAndStaleState(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	first := request(t, "request-1", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := store.Advance(first); err != nil {
		t.Fatal(err)
	}
	second := request(t, "request-2", "apb-00000000000000000000000000000001", 2, 2, first.Snapshot.Digest)
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
	collision := request(t, "request-3", "apb-00000000000000000000000000000001", 3, 3, second.Snapshot.Digest)
	collision.Batches[0].Entries[0].Summary = "different"
	collision.Batches[0], _, _ = applyprogress.SealBatch(collision.Batches[0])
	if _, err := store.Advance(collision); !errors.Is(err, ErrBatchCollision) {
		t.Fatalf("Advance() error = %v, want batch collision", err)
	}
	missing := request(t, "request-missing", "apb-00000000000000000000000000000003", 3, 3, second.Snapshot.Digest)
	missing.Batches = nil
	if _, err := store.Advance(missing); !errors.Is(err, ErrMissingBatch) {
		t.Fatalf("Advance() error = %v, want missing referenced batch", err)
	}
	stale := request(t, "request-4", "apb-00000000000000000000000000000003", 2, 2, first.Snapshot.Digest)
	if _, err := store.Advance(stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want stale conflict", err)
	}
}

func TestOpenSpecAdvanceDurablyStagesBeforeInterruptedRename(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	store := OpenSpec{Root: root}
	old := request(t, "request-old", "apb-00000000000000000000000000000001", 1, 1, "")
	if _, err := store.Advance(old); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil {
		t.Fatal(err)
	}
	pending := request(t, "request-pending", "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest)
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
		name      string
		batches   bool
		finalPath func(string, AdvanceRequest) string
	}{
		{"evidence", true, func(root string, request AdvanceRequest) string {
			return filepath.Join(root, "apply-evidence", request.Batches[0].BatchID+".json")
		}},
		{"receipt", false, func(root string, request AdvanceRequest) string {
			return filepath.Join(root, ".apply-progress-receipts", request.RequestID+".json")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newOpenSpecTestRoot(t)
			store := OpenSpec{Root: root}
			old := request(t, "request-old", "apb-00000000000000000000000000000001", 1, 1, "")
			if _, err := store.Advance(old); err != nil {
				t.Fatal(err)
			}
			pending := request(t, "request-pending", "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest)
			if !test.batches {
				pending.Batches = nil
				pending.Snapshot.Batches = old.Snapshot.Batches
				pending.Snapshot, _, _ = applyprogress.SealSnapshot(pending.Snapshot)
			}
			store.BeforeImmutablePublish = func() error { return errInterrupted }
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
	if _, err := store.Advance(newRequest); !errors.Is(err, ErrConflict) {
		t.Fatalf("Advance() error = %v, want stale coordinate conflict", err)
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
	if _, err := store.Advance(old); err != nil {
		t.Fatal(err)
	}
	pending := request(t, "request-pending", "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest)
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
			r := request(t, name, "apb-00000000000000000000000000000002", 2, 2, old.Snapshot.Digest)
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
		"tasks.md":          []byte("- [x] 1.1 task\n"),
		"proposal.md":       []byte("# Proposal\n"),
		"design.md":         []byte("# Design\n"),
		"verify-report.md":  []byte("## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"),
		"archive-report.md": []byte("# Archive Report\n"),
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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: generation, Revision: revision, PreviousDigest: previous,
		TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return AdvanceRequest{RequestID: requestID, ExpectedGeneration: generation - 1, ExpectedRevision: revision - 1, ExpectedDigest: previous, Batches: []applyprogress.Batch{batch}, Snapshot: snapshot}
}
