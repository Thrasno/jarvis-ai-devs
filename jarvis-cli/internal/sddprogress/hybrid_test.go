package sddprogress

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
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

func (b *retryBackend) Advance(AdvanceRequest) (AdvanceResult, error) {
	b.calls++
	return AdvanceResult{}, b.err
}

func (b *retryBackend) Current(AdvanceRequest) (AdvanceResult, error) {
	b.currentCalls++
	return b.current, b.currentErr
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
	hive := &retryBackend{err: errInterrupted}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}
	r := legacyRequest(t, "hybrid-legacy")
	if _, upgraded, err := h.UpgradeLegacy(r); !upgraded || !errors.Is(err, errInterrupted) {
		t.Fatalf("upgrade=%t err=%v", upgraded, err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "apply-progress.md")); !bytes.Equal(got, legacy) {
		t.Fatalf("legacy source changed: %q", got)
	}
	hive.err = nil
	result, upgraded, err := h.UpgradeLegacy(r)
	if err != nil || !upgraded || result.Generation != 1 || hive.calls != 2 {
		t.Fatalf("retry=%#v upgraded=%t err=%v calls=%d", result, upgraded, err, hive.calls)
	}
}

func TestHybridRetriesOnlyRecordedMissingSide(t *testing.T) {
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
			if open.calls != map[bool]int{false: 1, true: 2}[hiveFirst] || hive.calls != map[bool]int{false: 2, true: 1}[hiveFirst] {
				t.Fatalf("calls open=%d hive=%d", open.calls, hive.calls)
			}
			if _, err := h.Advance(r); err != nil || open.calls+hive.calls != 3 {
				t.Fatalf("replay err=%v calls=%d", err, open.calls+hive.calls)
			}
		})
	}
}

func TestHybridRemovesStaleOnlyReceiptAfterTransientHiveCurrentFailure(t *testing.T) {
	r := request(t, "stale-replay", "apb-00000000000000000000000000000001", 1, 1, "")
	open := &retryBackend{err: ErrConflict}
	hive := &retryBackend{currentErr: errInterrupted}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
	if _, err := h.Advance(r); !errors.Is(err, errInterrupted) {
		t.Fatalf("first error = %v, want transient Hive current failure", err)
	}
	if _, found, err := h.receipt(r.RequestID); err != nil || !found {
		t.Fatalf("first receipt found=%t err=%v, want persisted stale-only receipt", found, err)
	}
	hive.currentErr = nil
	hive.current = AdvanceResult{Generation: 9, Revision: 10, Digest: "hive-current"}
	if _, err := h.Advance(r); !errors.Is(err, ErrConflict) {
		t.Fatalf("replay error = %v, want stale conflict", err)
	}
	if _, found, err := h.receipt(r.RequestID); err != nil || found {
		t.Fatalf("replay receipt found=%t err=%v, want stale-only receipt removed", found, err)
	}
}

func TestHybridRetainsPartiallyCommittedReceiptOnStaleCurrentLookup(t *testing.T) {
	r := request(t, "partial-stale", "apb-00000000000000000000000000000001", 1, 1, "")
	open := &retryBackend{}
	hive := &retryBackend{err: ErrConflict, current: AdvanceResult{Generation: 9, Revision: 10, Digest: "hive-current"}}
	h := Hybrid{Root: t.TempDir(), OpenSpec: open, Hive: hive}
	if _, err := h.Advance(r); !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want stale conflict", err)
	}
	receipt, found, err := h.receipt(r.RequestID)
	if err != nil || !found || receipt.OpenSpec != receiptCommitted || receipt.Hive != receiptFailed {
		t.Fatalf("receipt = %#v, found=%t err=%v; want committed OpenSpec receipt retained", receipt, found, err)
	}
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
	if _, err := h.Advance(r); err != nil || open.calls != 1 || hive.calls != 2 {
		t.Fatalf("recovery err=%v calls=%d/%d", err, open.calls, hive.calls)
	}
}
