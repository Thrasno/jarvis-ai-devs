package sddprogress

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

type receiptCheckingOpenSpec struct {
	OpenSpec
	receiptPath string
	receiptSeen bool
}

func (s *receiptCheckingOpenSpec) AdvanceLegacy(request AdvanceRequest, legacy []byte) (AdvanceResult, error) {
	_, err := os.Stat(s.receiptPath)
	s.receiptSeen = err == nil
	if err != nil {
		return AdvanceResult{}, err
	}
	return s.OpenSpec.AdvanceLegacy(request, legacy)
}

func TestHybridLegacyMigrationWritesRecoveryReceiptBeforeOpenSpecAndRecoversExactRetry(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	tasks := []byte("- [x] 1.1 task\n")
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), tasks, 0o600); err != nil {
		t.Fatal(err)
	}

	request := legacyRequest(t, "legacy-recovery")
	receiptPath := filepath.Join(root, ".apply-progress-hybrid-receipts", request.RequestID+".json")
	open := &receiptCheckingOpenSpec{OpenSpec: OpenSpec{Root: root}, receiptPath: receiptPath}
	hive := &legacyRetryBackend{retryBackend: retryBackend{err: errInterrupted}, authority: LegacyAuthority{Tasks: tasks, Progress: legacy, Found: true}}
	hybrid := Hybrid{Root: root, OpenSpec: open, Hive: hive}

	if _, upgraded, err := hybrid.UpgradeLegacy(request); !upgraded || !errors.Is(err, errInterrupted) {
		t.Fatalf("first migration = upgraded %t, error %v; want interrupted partial migration", upgraded, err)
	}
	if !open.receiptSeen {
		t.Fatal("hybrid migration mutated OpenSpec before persisting its recovery receipt")
	}
	if current, err := (OpenSpec{Root: root}).Current(); err != nil || current == nil || current.Digest != request.Snapshot.Digest {
		t.Fatalf("partial OpenSpec migration = %#v, %v; want migrated exact snapshot", current, err)
	}
	if receipt, found, err := hybrid.receipt(request.RequestID); err != nil || !found || receipt.OpenSpec != receiptCommitted || receipt.Hive != receiptFailed {
		t.Fatalf("partial receipt = %#v, found=%t, err=%v", receipt, found, err)
	}

	changed := request
	changed.Batches = append([]applyprogress.Batch(nil), request.Batches...)
	changed.Snapshot.Batches = append([]applyprogress.BatchRef(nil), request.Snapshot.Batches...)
	changed.Batches[0].Entries = append([]applyprogress.EvidenceEntry(nil), request.Batches[0].Entries...)
	changed.Batches[0].Entries[0].Summary = "changed payload"
	var err error
	changed.Batches[0], _, err = applyprogress.SealBatch(changed.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	changed.Snapshot.Batches[0].SHA256 = changed.Batches[0].SHA256
	changed.Snapshot, _, err = applyprogress.SealSnapshot(changed.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := hybrid.UpgradeLegacy(changed); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("changed recovery = %v, want request conflict", err)
	}

	hive.err = nil
	if result, upgraded, err := hybrid.UpgradeLegacy(request); err != nil || !upgraded || result.Digest != request.Snapshot.Digest {
		t.Fatalf("exact recovery = %#v, upgraded=%t, err=%v", result, upgraded, err)
	}
}

func TestOpenSpecInspectPublicationPrioritizesInterruptedLegacyPublication(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".apply-progress-receipts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".apply-progress-receipts", "interrupted.json"), []byte(`{"payload":`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := (OpenSpec{Root: root}).InspectPublication(); !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("InspectPublication() error = %v, want publication interruption before legacy migration", err)
	}
}

func TestOpenSpecArchiveRequiresCanonicalVerifyReadiness(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "strict-verify", applyprogress.StatusComplete)
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "verify-report.md"), []byte("PASS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (OpenSpec{Root: root}).Archive(filepath.Join(t.TempDir(), "archive")); !errors.Is(err, ErrConflict) {
		t.Fatalf("Archive() error = %v, want strict verify readiness rejection", err)
	}
}

func TestOpenSpecArchiveReturnsTypedRegenerationCodeForNoncanonicalVerifyReport(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "noncanonical-verify", applyprogress.StatusComplete)
	store := OpenSpec{Root: root}
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "verify-report.md"), []byte("## Verification Report\n\nStatus: PASS\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := store.Archive(filepath.Join(t.TempDir(), "archive"))
	var verifyErr *applyprogress.VerifyReportError
	if !errors.As(err, &verifyErr) {
		t.Fatalf("Archive() error = %v, want typed verify report error", err)
	}
	if verifyErr.Code != applyprogress.VerifyReportReasonRegenerateWithSDDVerify {
		t.Fatalf("code = %q, want %q", verifyErr.Code, applyprogress.VerifyReportReasonRegenerateWithSDDVerify)
	}
}

func TestOpenSpecRejectsAnonymousStagingResidue(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "anonymous-stage", applyprogress.StatusComplete)
	stage := filepath.Join(root, ".apply-progress-receipts", ".apply-progress-stage-anonymous")
	if err := os.MkdirAll(filepath.Dir(stage), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("unbound residue"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := (OpenSpec{Root: root}).Advance(request)
	if !errors.Is(err, ErrPublicationInterrupted) {
		t.Fatalf("Advance() error = %v, want anonymous staging residue to fail closed", err)
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatalf("Advance() mutated anonymous staging residue: %v", err)
	}
}

func TestOpenSpecInspectPublicationRejectsStagingWithoutHead(t *testing.T) {
	for _, directory := range []string{".", "apply-evidence", ".apply-progress-receipts"} {
		t.Run(directory, func(t *testing.T) {
			root := newOpenSpecTestRoot(t)
			stage := filepath.Join(root, directory, ".apply-progress-stage-anonymous")
			if err := os.MkdirAll(filepath.Dir(stage), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stage, []byte("unbound residue"), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := (OpenSpec{Root: root}).InspectPublication()
			if !errors.Is(err, ErrPublicationInterrupted) {
				t.Fatalf("InspectPublication() error = %v, want publication interruption", err)
			}
			if _, err := os.Stat(stage); err != nil {
				t.Fatalf("InspectPublication() mutated staging residue: %v", err)
			}
		})
	}
}

func TestOpenSpecMigratesLegacyPhaseTasksThroughCurrentAndArchive(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	legacyTasks := "## Compatibility\n- [x] RED: reproduce\n- [x] GREEN: fix\n"
	legacyProgress := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(legacyTasks), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacyProgress, 0o600); err != nil {
		t.Fatal(err)
	}

	parsed, err := applyprogress.ParseLegacyTasksMarkdown(legacyTasks)
	if err != nil {
		t.Fatal(err)
	}
	converted, manifest, err := applyprogress.LegacyTaskManifest(parsed.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	request := validArchiveRequest(t, "legacy-phase-archive", applyprogress.StatusComplete)
	entry := &request.Batches[0].Entries[0]
	entry.Kind = applyprogress.EvidenceImported
	entry.TaskIDs = []string{converted[0].ID, converted[1].ID}
	entry.CompletesTaskIDs = []string{converted[0].ID, converted[1].ID}
	request.Batches[0], _, err = applyprogress.SealBatch(request.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	request.Snapshot.TaskManifestSHA256 = manifest
	request.Snapshot.Coverage = []applyprogress.Coverage{
		{TaskID: converted[0].ID, BatchID: request.Batches[0].BatchID, EntryID: entry.EntryID},
		{TaskID: converted[1].ID, BatchID: request.Batches[0].BatchID, EntryID: entry.EntryID},
	}
	request.Snapshot.Batches[0].SHA256 = request.Batches[0].SHA256
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}

	store := OpenSpec{Root: root}
	if _, upgraded, err := store.UpgradeLegacy(request); err != nil || !upgraded {
		t.Fatalf("UpgradeLegacy() = upgraded %t, error %v; want completed phase-style migration", upgraded, err)
	}
	if current, err := store.Current(); err != nil || current == nil || current.Digest != request.Snapshot.Digest {
		t.Fatalf("Current() = %#v, %v; want migrated authoritative snapshot", current, err)
	}
	if err := store.Archive(filepath.Join(t.TempDir(), "archive")); err != nil {
		t.Fatalf("Archive() = %v; want migrated legacy topology archived", err)
	}
}

func TestOpenSpecLoadsMigratedLegacyTaskManifest(t *testing.T) {
	root := t.TempDir()
	content := "## Implementation\n\n- [x] RED establish failing behavior\n- [ ] GREEN make it pass\n"
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := applyprogress.ParseLegacyTasksMarkdown(content)
	if err != nil {
		t.Fatal(err)
	}
	want, manifest, err := applyprogress.LegacyTaskManifest(parsed.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (OpenSpec{Root: root}).authoritativeTasksForManifest(manifest)
	if err != nil {
		t.Fatalf("authoritativeTasksForManifest() error = %v", err)
	}
	if len(got) != len(want) || got[0].ID != want[0].ID || got[1].ID != want[1].ID {
		t.Fatalf("authoritative tasks = %#v, want %#v", got, want)
	}
}

func TestOpenSpecFinalizesOnlyByteIdenticalDeterministicStage(t *testing.T) {
	request := validArchiveRequest(t, "exact-stage", applyprogress.StatusComplete)
	for _, tt := range []struct {
		name     string
		contents func([]byte) []byte
		wantErr  bool
	}{
		{name: "exact retry", contents: func(data []byte) []byte { return data }},
		{name: "changed residue", contents: func([]byte) []byte { return []byte("changed") }, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newOpenSpecTestRoot(t)
			store := OpenSpec{Root: root}
			snapshot, snapshotData, err := applyprogress.SealSnapshot(request.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := payloadDigest(snapshotData, request)
			if err != nil {
				t.Fatal(err)
			}
			receiptData, err := json.Marshal(immutableReceipt{Payload: payload, Snapshot: snapshot})
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, ".apply-progress-receipts", request.RequestID+".json")
			stage := stagingPath(target, receiptData)
			if err := os.MkdirAll(filepath.Dir(stage), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stage, tt.contents(receiptData), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = store.Advance(request)
			if tt.wantErr {
				if !errors.Is(err, ErrPublicationInterrupted) {
					t.Fatalf("Advance() error = %v, want changed stage to fail closed", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Advance() exact retry = %v", err)
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("exact retry did not finalize its deterministic stage: %v", err)
			}
		})
	}
}

func TestOpenSpecRejectsSymlinkedPathComponentsBeforePublication(t *testing.T) {
	base := t.TempDir()
	actualParent := filepath.Join(base, "actual")
	root := filepath.Join(actualParent, "change")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(base, "linked")
	if err := os.Symlink(actualParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	if _, err := (OpenSpec{Root: filepath.Join(linkedParent, "change")}).Advance(validArchiveRequest(t, "symlink-ancestor", applyprogress.StatusComplete)); err == nil {
		t.Fatal("Advance() accepted a symlinked ancestor of the change root")
	}

	root = newOpenSpecTestRoot(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "apply-evidence")); err != nil {
		t.Fatal(err)
	}
	if _, err := (OpenSpec{Root: root}).Advance(validArchiveRequest(t, "symlink-evidence", applyprogress.StatusComplete)); err == nil {
		t.Fatal("Advance() accepted a symlinked authoritative evidence directory")
	}
}

func TestOpenSpecArchiveRevalidatesDestinationAndLifecycleUnderLock(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := validArchiveRequest(t, "archive-lock", applyprogress.StatusComplete)
	if _, err := (OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	linkedParent := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatal(err)
	}
	if err := (OpenSpec{Root: root}).Archive(filepath.Join(linkedParent, "archive")); err == nil {
		t.Fatal("Archive() accepted a symlinked destination parent")
	}

	store := OpenSpec{Root: root, BeforeArchiveValidate: func() error {
		return os.WriteFile(filepath.Join(root, "verify-report.md"), []byte("FAIL\n"), 0o600)
	}}
	if err := store.Archive(filepath.Join(t.TempDir(), "archive")); err == nil {
		t.Fatal("Archive() renamed after lifecycle validation became stale under the lock")
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); err != nil {
		t.Fatalf("Archive() moved stale topology: %v", err)
	}
}

func TestOpenSpecRejectsSymlinkedRootAndSelfArchive(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	link := filepath.Join(t.TempDir(), "change-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	request := validArchiveRequest(t, "symlink-root", applyprogress.StatusComplete)
	if _, err := (OpenSpec{Root: link}).Advance(request); err == nil {
		t.Fatal("Advance() accepted a symlinked change root")
	}

	store := OpenSpec{Root: root}
	if _, err := store.Advance(validArchiveRequest(t, "self-archive", applyprogress.StatusComplete)); err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(root); err == nil {
		t.Fatal("Archive() accepted its source directory as destination")
	}
}
