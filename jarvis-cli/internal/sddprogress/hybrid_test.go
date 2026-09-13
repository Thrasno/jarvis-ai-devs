package sddprogress

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress/filelock"
)

func TestResolveHybridFailsClosedOnDifferentMissingOrInvalidSides(t *testing.T) {
	first := request(t, "first", "apb-00000000000000000000000000000001", 1, 1, "")
	other := request(t, "other", "apb-00000000000000000000000000000002", 1, 1, "")
	for name, hive := range map[string]func() (ResolvedProgress, error){
		"different valid snapshot": func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: other.Snapshot}, nil },
		"same digest different state": func() (ResolvedProgress, error) {
			divergent := first.Snapshot
			divergent.Generation++
			return ResolvedProgress{Snapshot: divergent}, nil
		},
		"missing side": func() (ResolvedProgress, error) { return ResolvedProgress{}, ErrMissingBackend },
		"invalid side": func() (ResolvedProgress, error) { return ResolvedProgress{}, errors.New("invalid snapshot") },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveHybrid(func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: first.Snapshot}, nil }, hive)
			if !errors.Is(err, ErrBackendDiverged) {
				t.Fatalf("ResolveHybrid() error = %v, want backend divergence", err)
			}
		})
	}
}

func TestResolveHybridAcceptsOnlyEqualValidatedSnapshots(t *testing.T) {
	request := request(t, "same", "apb-00000000000000000000000000000001", 1, 1, "")
	got, err := ResolveHybrid(
		func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: request.Snapshot}, nil },
		func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: request.Snapshot}, nil },
	)
	if err != nil || got.Snapshot.Digest != request.Snapshot.Digest {
		t.Fatalf("ResolveHybrid() = %#v, %v; want matching digest %q", got, err, request.Snapshot.Digest)
	}
}

type retryBackend struct {
	calls        int
	currentCalls int
	err          error
	currentErr   error
	current      AdvanceResult
}

type checkpointHiveBackend struct {
	retryBackend
	snapshot *applyprogress.Snapshot
}

type legacyRetryBackend struct {
	retryBackend
	authority LegacyAuthority
	request   AdvanceRequest
}

func (b *legacyRetryBackend) LegacyAuthority(AdvanceRequest) (LegacyAuthority, error) {
	return b.authority, nil
}

func (b *legacyRetryBackend) Advance(request AdvanceRequest) (AdvanceResult, error) {
	b.request = request
	b.calls++
	if b.err != nil {
		return AdvanceResult{}, b.err
	}
	b.authority.Found = false
	payload, err := payloadDigestFor(request)
	if err != nil {
		return AdvanceResult{}, err
	}
	return AdvanceResult{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, PayloadSHA256: payload}, nil
}

func (b *legacyRetryBackend) AdvanceLegacy(request AdvanceRequest, _ []byte) (AdvanceResult, error) {
	return b.Advance(request)
}

func (b *checkpointHiveBackend) CurrentSnapshot(AdvanceRequest) (*applyprogress.Snapshot, error) {
	return b.snapshot, nil
}

func (b *retryBackend) Advance(request AdvanceRequest) (AdvanceResult, error) {
	b.calls++
	if b.err != nil {
		return AdvanceResult{}, b.err
	}
	payload, err := payloadDigestFor(request)
	if err != nil {
		return AdvanceResult{}, err
	}
	return AdvanceResult{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, PayloadSHA256: payload}, nil
}

func (b *retryBackend) Current(AdvanceRequest) (AdvanceResult, error) {
	b.currentCalls++
	return b.current, b.currentErr
}

func TestHybridDoesNotBindReceiptBeforeBackendRequestValidation(t *testing.T) {
	for name, conflict := range map[string]int{"first backend": 0, "second backend": 1} {
		t.Run(name, func(t *testing.T) {
			original := request(t, "hybrid-existing", "apb-00000000000000000000000000000001", 1, 1, "")
			changed := request(t, original.RequestID, "apb-00000000000000000000000000000002", 1, 1, "")
			open, hive := &retryBackend{}, &retryBackend{}
			[]*retryBackend{open, hive}[conflict].err = ErrRequestConflict
			h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
			if _, err := h.Advance(changed); !errors.Is(err, ErrRequestConflict) {
				t.Fatalf("changed error = %v, want request conflict", err)
			}
			if _, found, err := h.receipt(original.RequestID); err != nil || found {
				t.Fatalf("changed receipt found=%t err=%v, want no receipt", found, err)
			}
			open.err, hive.err = nil, nil
			if _, err := h.Advance(original); err != nil {
				t.Fatalf("original replay error = %v", err)
			}
		})
	}
}

func TestHybridRejectsMismatchedTaskManifestBeforeHiveCommit(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	hive := &retryBackend{}
	request := request(t, "hybrid-mismatched-manifest", "apb-00000000000000000000000000000009", 1, 1, "")
	request.Snapshot.TaskManifestSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	var err error
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive, HiveFirst: true}

	if _, err := h.Advance(request); err == nil {
		t.Fatal("Advance() succeeded with mismatched task manifest")
	} else {
		var validation *applyprogress.ValidationError
		if !errors.As(err, &validation) || validation.Code != applyprogress.CodeTaskManifestMismatch {
			t.Fatalf("Advance() error = %v, want typed task manifest mismatch", err)
		}
	}
	if hive.calls != 0 {
		t.Fatalf("Hive Advance calls = %d, want no write before manifest validation", hive.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); !os.IsNotExist(err) {
		t.Fatalf("mismatched manifest wrote OpenSpec snapshot: %v", err)
	}
}

func TestHybridLockLivesOutsideMovableChangeRoot(t *testing.T) {
	root := t.TempDir()
	h := Hybrid{Root: root}
	unlock, err := h.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlock() }()
	name := "." + filepath.Base(root) + ".apply-progress-hybrid.lock"
	if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
		t.Fatalf("lock unexpectedly inside movable root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), name)); err != nil {
		t.Fatalf("external hybrid lock: %v", err)
	}
}

func TestHybridCurrentAcceptsOnlyEqualResolvedSnapshots(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	r := request(t, "current", "apb-00000000000000000000000000000001", 1, 1, "")
	hive := &checkpointHiveBackend{}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}
	if current, err := h.Current(r); err != nil || current != nil {
		t.Fatalf("initial current=%#v err=%v", current, err)
	}
	if _, err := (OpenSpec{Root: root}).Advance(r); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Current(r); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("missing Hive current error=%v", err)
	}
	hive.snapshot = &r.Snapshot
	current, err := h.Current(r)
	if err != nil || current == nil || current.Digest != r.Snapshot.Digest {
		t.Fatalf("matched current=%#v err=%v", current, err)
	}
}

func TestHybridReturnsStaleCoordinatesOnlyAfterMatchingCurrentSnapshots(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	current := request(t, "current", "apb-00000000000000000000000000000009", 1, 1, "")
	if _, err := (OpenSpec{Root: root}).Advance(current); err != nil {
		t.Fatal(err)
	}
	hive := &checkpointHiveBackend{snapshot: &current.Snapshot}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}
	stale := request(t, "stale", "apb-00000000000000000000000000000010", 1, 1, "")
	result, err := h.Advance(stale)
	if !errors.Is(err, ErrConflict) || errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("error = %v, want validated stale conflict", err)
	}
	if result.Generation != current.Snapshot.Generation || result.Revision != current.Snapshot.Revision || result.Digest != current.Snapshot.Digest {
		t.Fatalf("result = %#v, want current snapshot coordinates", result)
	}
}

func TestHybridLegacyUpgradeRestoresSourceAfterHiveFailure(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := os.ReadFile(filepath.Join(root, "tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	hive := &legacyRetryBackend{retryBackend: retryBackend{err: errInterrupted}, authority: LegacyAuthority{Tasks: tasks, Progress: legacy, Found: true}}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}
	r := legacyRequest(t, "hybrid-legacy")
	r.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(legacy)
	if _, upgraded, err := h.UpgradeLegacy(r); !upgraded || !errors.Is(err, errInterrupted) {
		t.Fatalf("upgrade=%t err=%v", upgraded, err)
	}
	if current, currentErr := (OpenSpec{Root: root}).Current(); currentErr != nil || current == nil || current.Digest != r.Snapshot.Digest {
		t.Fatalf("migrated OpenSpec current = %#v, %v; want durable partial migration", current, currentErr)
	}
	hive.err = nil
	result, upgraded, err := h.UpgradeLegacy(r)
	if err != nil || !upgraded || result.Generation != 1 || hive.calls != 2 {
		t.Fatalf("retry=%#v upgraded=%t err=%v calls=%d", result, upgraded, err, hive.calls)
	}
}

func TestHybridLegacyUpgradeRequiresMatchingAuthoritiesBeforePublishing(t *testing.T) {
	legacy := []byte("status: complete\n")
	for _, tt := range []struct {
		name      string
		authority func([]byte) LegacyAuthority
		wantError bool
	}{
		{name: "missing Hive authority", authority: func(tasks []byte) LegacyAuthority { return LegacyAuthority{Tasks: tasks} }, wantError: true},
		{name: "different Hive legacy bytes", authority: func(tasks []byte) LegacyAuthority {
			return LegacyAuthority{Tasks: tasks, Progress: []byte("status: partial\n"), Found: true}
		}, wantError: true},
		{name: "different Hive tasks bytes", authority: func([]byte) LegacyAuthority {
			return LegacyAuthority{Tasks: []byte("- [x] 1.1 other task\n"), Progress: legacy, Found: true}
		}, wantError: true},
		{name: "identical authorities migrate once", authority: func(tasks []byte) LegacyAuthority {
			return LegacyAuthority{Tasks: tasks, Progress: legacy, Found: true}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
				t.Fatal(err)
			}
			tasks := []byte("- [x] 1.1 task\n")
			if err := os.WriteFile(filepath.Join(root, "tasks.md"), tasks, 0o600); err != nil {
				t.Fatal(err)
			}
			hive := &legacyRetryBackend{authority: tt.authority(tasks)}
			h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}

			result, upgraded, err := h.UpgradeLegacy(legacyRequest(t, "hybrid-authority"))
			if tt.wantError {
				if upgraded || !errors.Is(err, ErrBackendDiverged) {
					t.Fatalf("upgrade=%t result=%#v err=%v, want backend divergence without migration", upgraded, result, err)
				}
				if hive.calls != 0 {
					t.Fatalf("Hive calls = %d, want no publication", hive.calls)
				}
				if got, readErr := os.ReadFile(filepath.Join(root, "apply-progress.md")); readErr != nil || !bytes.Equal(got, legacy) {
					t.Fatalf("OpenSpec authority mutated before comparison: %q, %v", got, readErr)
				}
				return
			}
			if err != nil || !upgraded || result.Generation != 1 || hive.calls != 1 {
				t.Fatalf("upgrade=%t result=%#v err=%v hive calls=%d", upgraded, result, err, hive.calls)
			}
			if got, want := hive.request.LegacySourceSHA256, applyprogress.LegacySourceSHA256(legacy); got != want {
				t.Fatalf("Hive legacy source binding = %q, want %q", got, want)
			}
			for _, path := range []string{
				filepath.Join(root, ".apply-progress-receipts", "hybrid-authority.json"),
				filepath.Join(root, ".apply-progress-hybrid-receipts", "hybrid-authority.json"),
			} {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("migration receipt %s: %v", path, err)
				}
			}
			replay := legacyRequest(t, "hybrid-authority")
			replay.LegacySourceSHA256 = applyprogress.LegacySourceSHA256(legacy)
			if _, upgraded, err := h.UpgradeLegacy(replay); err != nil || upgraded || hive.calls != 1 {
				t.Fatalf("second migration upgraded=%t err=%v hive calls=%d", upgraded, err, hive.calls)
			}
		})
	}
}

func TestHybridReconcilesIncompleteReceiptBeforeDurableSuccess(t *testing.T) {
	r := request(t, "hybrid", "apb-00000000000000000000000000000001", 1, 1, "")
	for name, hiveFirst := range map[string]bool{"hive missing": false, "openspec missing": true} {
		t.Run(name, func(t *testing.T) {
			open, hive := &retryBackend{}, &retryBackend{}
			if hiveFirst {
				open.err = errInterrupted
			} else {
				hive.err = errInterrupted
			}
			h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive, HiveFirst: hiveFirst}
			if _, err := h.Advance(r); !errors.Is(err, errInterrupted) {
				t.Fatalf("first err=%v", err)
			}
			open.err, hive.err = nil, nil
			if _, err := h.Advance(r); err != nil {
				t.Fatal(err)
			}
			if open.calls != 2 || hive.calls != 2 {
				t.Fatalf("calls open=%d hive=%d, want both sides reconciled", open.calls, hive.calls)
			}
			if _, err := h.Advance(r); err != nil || open.calls+hive.calls != 4 {
				t.Fatalf("durable replay err=%v calls=%d", err, open.calls+hive.calls)
			}
		})
	}
}

func TestHybridRemovesStaleOnlyReceiptAfterTransientHiveCurrentFailure(t *testing.T) {
	r := request(t, "stale-replay", "apb-00000000000000000000000000000001", 1, 1, "")
	open := &retryBackend{err: ErrConflict}
	hive := &retryBackend{currentErr: errInterrupted}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
	if _, err := h.Advance(r); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("first error = %v, want unvalidated backend divergence", err)
	}
	if _, found, err := h.receipt(r.RequestID); err != nil || !found {
		t.Fatalf("first receipt found=%t err=%v, want persisted stale-only receipt", found, err)
	}
	hive.currentErr = nil
	hive.current = AdvanceResult{Generation: 9, Revision: 10, Digest: "hive-current"}
	if _, err := h.Advance(r); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("replay error = %v, want unvalidated backend divergence", err)
	}
	if _, found, err := h.receipt(r.RequestID); err != nil || !found {
		t.Fatalf("replay receipt found=%t err=%v, want stale-only receipt retained", found, err)
	}
}

func TestHybridRetainsPartiallyCommittedReceiptOnStaleCurrentLookup(t *testing.T) {
	r := request(t, "partial-stale", "apb-00000000000000000000000000000001", 1, 1, "")
	open := &retryBackend{}
	hive := &retryBackend{err: ErrConflict, current: AdvanceResult{Generation: 9, Revision: 10, Digest: "hive-current"}}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
	if _, err := h.Advance(r); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("error = %v, want unvalidated backend divergence", err)
	}
	receipt, found, err := h.receipt(r.RequestID)
	if err != nil || !found || receipt.OpenSpec != receiptCommitted || receipt.Hive != receiptFailed {
		t.Fatalf("receipt = %#v, found=%t err=%v; want committed OpenSpec receipt retained", receipt, found, err)
	}
}

type acknowledgementBackend struct {
	calls       int
	readerCalls int
	ack         func(AdvanceRequest) AdvanceResult
	current     *applyprogress.Snapshot
	currentErr  error
}

func (b *acknowledgementBackend) Advance(request AdvanceRequest) (AdvanceResult, error) {
	b.calls++
	if b.ack != nil {
		return b.ack(request), nil
	}
	payload, err := payloadDigestFor(request)
	if err != nil {
		return AdvanceResult{}, err
	}
	return AdvanceResult{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, PayloadSHA256: payload}, nil
}

func (b *acknowledgementBackend) Current() (*applyprogress.Snapshot, error) {
	b.readerCalls++
	return b.current, b.currentErr
}

func (b *acknowledgementBackend) CurrentSnapshot(AdvanceRequest) (*applyprogress.Snapshot, error) {
	b.readerCalls++
	return b.current, b.currentErr
}

func TestHybridSuccessUsesDurableAcknowledgementsWithoutPostCommitReaders(t *testing.T) {
	r := request(t, "acknowledged", "apb-00000000000000000000000000000011", 1, 1, "")
	open, hive := &acknowledgementBackend{}, &acknowledgementBackend{}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}

	result, err := h.Advance(r)
	if err != nil || result.Digest != r.Snapshot.Digest {
		t.Fatalf("Advance() = %#v, %v; want durable success", result, err)
	}
	if open.readerCalls != 0 || hive.readerCalls != 0 {
		t.Fatalf("post-commit Current calls = %d/%d, want none", open.readerCalls, hive.readerCalls)
	}
	receipt, found, err := h.receipt(r.RequestID)
	if err != nil || !found || !hybridReceiptAcknowledges(h, r, receipt.Payload, "openspec") || !hybridReceiptAcknowledges(h, r, receipt.Payload, "hive") {
		t.Fatalf("receipt = %#v, found=%t err=%v; want exact durable acknowledgements", receipt, found, err)
	}
}

func TestHybridIgnoresPeerSuccessorAndPostCommitLockBusyAfterExactAcknowledgements(t *testing.T) {
	r := request(t, "acknowledged-peer", "apb-00000000000000000000000000000012", 1, 1, "")
	peer := r.Snapshot
	peer.Revision++
	for _, currentErr := range []error{nil, filelock.ErrBusy} {
		t.Run("post-commit reader unavailable", func(t *testing.T) {
			open := &acknowledgementBackend{current: &peer, currentErr: currentErr}
			hive := &acknowledgementBackend{current: &peer, currentErr: currentErr}
			h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}

			if _, err := h.Advance(r); err != nil {
				t.Fatalf("Advance() error = %v, want success from durable exact acknowledgements", err)
			}
			if open.readerCalls != 0 || hive.readerCalls != 0 {
				t.Fatalf("post-commit reader calls = %d/%d, want none", open.readerCalls, hive.readerCalls)
			}
		})
	}
}

func TestHybridRejectsWrongAcknowledgementIdentity(t *testing.T) {
	r := request(t, "wrong-ack", "apb-00000000000000000000000000000013", 1, 1, "")
	for name, mutate := range map[string]func(*AdvanceResult){
		"generation": func(result *AdvanceResult) { result.Generation++ },
		"revision":   func(result *AdvanceResult) { result.Revision++ },
		"digest":     func(result *AdvanceResult) { result.Digest = "wrong-digest" },
		"payload":    func(result *AdvanceResult) { result.PayloadSHA256 = "wrong-payload" },
	} {
		t.Run(name, func(t *testing.T) {
			open := &acknowledgementBackend{ack: func(request AdvanceRequest) AdvanceResult {
				payload, err := payloadDigestFor(request)
				if err != nil {
					t.Fatal(err)
				}
				result := AdvanceResult{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, PayloadSHA256: payload}
				mutate(&result)
				return result
			}}
			h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: &acknowledgementBackend{}}

			if _, err := h.Advance(r); !errors.Is(err, ErrBackendDiverged) {
				t.Fatalf("Advance() error = %v, want backend divergence", err)
			}
			receipt, found, err := h.receipt(r.RequestID)
			if err != nil || !found || receipt.OpenSpec == receiptCommitted || hybridReceiptAcknowledges(h, r, receipt.Payload, "openspec") {
				t.Fatalf("receipt = %#v, found=%t err=%v; wrong acknowledgement must not commit", receipt, found, err)
			}
		})
	}
}

func TestHybridReplaysLegacyCompleteReceiptWithoutAcknowledgements(t *testing.T) {
	r := request(t, "legacy-receipt", "apb-00000000000000000000000000000014", 1, 1, "")
	open, hive := &acknowledgementBackend{}, &acknowledgementBackend{}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
	payload, err := payloadDigestFor(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.writeReceipt(r.RequestID, hybridReceipt{Payload: payload, OpenSpec: receiptCommitted, Hive: receiptCommitted}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Advance(r); err != nil {
		t.Fatalf("Advance() error = %v, want safe legacy receipt replay", err)
	}
	if open.calls != 1 || hive.calls != 1 {
		t.Fatalf("legacy receipt replay calls = %d/%d, want both sides reconciled", open.calls, hive.calls)
	}
	receipt, found, err := h.receipt(r.RequestID)
	if err != nil || !found || !hybridReceiptAcknowledges(h, r, payload, "openspec") || !hybridReceiptAcknowledges(h, r, payload, "hive") {
		t.Fatalf("replayed receipt = %#v, found=%t err=%v; want exact acknowledgements", receipt, found, err)
	}
}

type recordedAcknowledgement struct {
	Generation uint64 `json:"generation"`
	Revision   uint64 `json:"revision"`
	Digest     string `json:"digest"`
	Payload    string `json:"payload"`
}

func hybridReceiptAcknowledges(h Hybrid, request AdvanceRequest, payload, side string) bool {
	data, err := readRegularFile(filepath.Join(h.Root, ".apply-progress-hybrid-receipts", request.RequestID+".json"))
	if err != nil {
		return false
	}
	var receipt struct {
		OpenSpec *recordedAcknowledgement `json:"openspec_ack"`
		Hive     *recordedAcknowledgement `json:"hive_ack"`
	}
	if json.Unmarshal(data, &receipt) != nil {
		return false
	}
	ack := receipt.OpenSpec
	if side == "hive" {
		ack = receipt.Hive
	}
	return ack != nil && ack.Generation == request.Snapshot.Generation && ack.Revision == request.Snapshot.Revision && ack.Digest == request.Snapshot.Digest && ack.Payload == payload
}

func TestHybridRecordsFailureAndRejectsChangedRequest(t *testing.T) {
	r := request(t, "hybrid", "apb-00000000000000000000000000000001", 1, 1, "")
	open, hive := &retryBackend{}, &retryBackend{err: errInterrupted}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
	if _, err := h.Advance(r); !errors.Is(err, errInterrupted) {
		t.Fatalf("first err=%v", err)
	}
	receipt, found, err := h.receipt(r.RequestID)
	if err != nil || !found || receipt.OpenSpec != receiptCommitted || receipt.Hive != receiptFailed {
		t.Fatalf("receipt = %#v, %t, %v", receipt, found, err)
	}
	changed := request(t, r.RequestID, "apb-00000000000000000000000000000002", 1, 1, "")
	if _, err := h.Advance(changed); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("changed err=%v", err)
	}
	hive.err = nil
	if _, err := h.Advance(r); err != nil || open.calls != 2 || hive.calls != 2 {
		t.Fatalf("recovery err=%v calls=%d/%d", err, open.calls, hive.calls)
	}
}
