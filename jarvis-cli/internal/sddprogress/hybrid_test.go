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

func TestHybridSuccessorRejectsDivergentSealBeforePublication(t *testing.T) {
	root := t.TempDir()
	open := OpenSpec{Root: root}
	hive := &checkpointHiveBackend{}
	h := Hybrid{Root: root, OpenSpec: open, Hive: hive}
	if _, err := h.PublishSuccessorGenesis(); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("PublishSuccessorGenesis() = %v, want divergence", err)
	}
	if hive.calls != 0 {
		t.Fatal("mutated Hive before matching seals")
	}
}

type successorHive struct {
	seal, genesis    *applyprogress.Snapshot
	calls            int
	interrupt, forge bool
}

func (b *successorHive) Advance(AdvanceRequest) (AdvanceResult, error) {
	return AdvanceResult{}, ErrConflict
}
func (b *successorHive) CurrentSnapshot(request AdvanceRequest) (*applyprogress.Snapshot, error) {
	if b.seal == nil || request.Snapshot.Project != b.seal.Project || request.Snapshot.Change != b.seal.Change {
		return nil, ErrConflict
	}
	return b.seal, nil
}
func (b *successorHive) PublishSuccessorGenesis(seal applyprogress.Snapshot) (applyprogress.Snapshot, error) {
	b.calls++
	if b.interrupt {
		b.interrupt = false
		return applyprogress.Snapshot{}, errInterrupted
	}
	if b.seal == nil || !sameSnapshot(*b.seal, seal) {
		return applyprogress.Snapshot{}, ErrConflict
	}
	if b.genesis == nil {
		pointer := &applyprogress.SupersedesPointer{Project: seal.Project, Change: seal.Change, SealDigest: seal.Digest, OriginalManifestSHA256: seal.TaskManifestSHA256, Actor: seal.SealIntent.Actor, Reason: seal.SealIntent.Reason, Timestamp: seal.SealIntent.Timestamp, OperationID: seal.SealIntent.OperationID}
		genesis, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: seal.SealIntent.SuccessorProject, Change: seal.SealIntent.SuccessorChange, Generation: 1, Revision: 1, TaskManifestSHA256: seal.SealIntent.SuccessorManifestSHA256, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, Supersedes: pointer})
		if err != nil {
			return applyprogress.Snapshot{}, err
		}
		b.genesis = &genesis
	}
	if b.forge {
		forged := *b.genesis
		pointer := *forged.Supersedes
		pointer.OperationID = "foreign"
		forged.Supersedes = &pointer
		forged, _, _ = applyprogress.SealSnapshot(forged)
		return forged, nil
	}
	return *b.genesis, nil
}

func successorHybridFixture(t *testing.T) (Hybrid, *successorHive, OpenSpec) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "source")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte("- [ ] 1.1 task\n"), 0600); err != nil {
		t.Fatal(err)
	}
	open := OpenSpec{Root: root}
	initial := request(t, "old-request", "apb-00000000000000000000000000000001", 1, 1, "")
	prepareContinuationSuccessor(t, &initial, "apb-00000000000000000000000000000002")
	if _, err := open.Advance(initial); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasks, []byte("- [ ] 1.1 changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "changed"}})
	if err != nil {
		t.Fatal(err)
	}
	seal := initial
	seal.RequestID = "seal-request"
	seal.ExpectedGeneration, seal.ExpectedRevision, seal.ExpectedDigest = initial.Snapshot.Generation, initial.Snapshot.Revision, initial.Snapshot.Digest
	seal.Batches = nil
	seal.Snapshot.Schema = applyprogress.SupersessionSnapshotSchema
	seal.Snapshot.Revision++
	seal.Snapshot.PreviousDigest = initial.Snapshot.Digest
	seal.Snapshot.Status = applyprogress.StatusSuperseded
	seal.Snapshot.StreamSHA256, seal.Snapshot.NextEntryIndex, seal.Snapshot.NextEntryID = "", 0, ""
	seal.Snapshot.SealIntent = &applyprogress.SealIntent{SuccessorProject: "jarvis-dev", SuccessorChange: "successor", SuccessorManifestSHA256: manifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal-request"}
	seal.Snapshot, _, err = applyprogress.SealSnapshot(seal.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := open.Advance(seal); err != nil {
		t.Fatal(err)
	}
	hive := &successorHive{seal: &seal.Snapshot}
	return Hybrid{Root: root, OpenSpec: open, Hive: hive}, hive, OpenSpec{Root: filepath.Join(parent, "successor")}
}

func TestHybridSuccessorRejectsForgedOpenSpecResultBeforeHive(t *testing.T) {
	h, hive, target := successorHybridFixture(t)
	h.publishOpenSpecSuccessor = func(source, destination OpenSpec) (AdvanceResult, error) {
		result, err := source.PublishSuccessorGenesis(destination)
		result.Digest = "0000000000000000000000000000000000000000000000000000000000000000"
		return result, err
	}
	if _, err := h.PublishSuccessorGenesis(); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("forged OpenSpec result: %v", err)
	}
	if hive.calls != 0 {
		t.Fatal("published Hive after forged OpenSpec result")
	}
	if published, err := target.InspectPublication(); err != nil || published == nil {
		t.Fatalf("fixture did not publish OpenSpec: %v, %v", published, err)
	}
}

func TestHybridSuccessorDivergenceBeforeMutation(t *testing.T) {
	h, hive, target := successorHybridFixture(t)
	other := *hive.seal
	intent := *other.SealIntent
	intent.SuccessorChange = "foreign"
	other.SealIntent = &intent
	var err error
	other, _, err = applyprogress.SealSnapshot(other)
	if err != nil {
		t.Fatal(err)
	}
	hive.seal = &other
	if _, err := h.PublishSuccessorGenesis(); !errors.Is(err, ErrBackendDiverged) {
		t.Fatalf("divergent seal: %v", err)
	}
	if hive.calls != 0 {
		t.Fatal("Hive publication before seal agreement")
	}
	if _, err := os.Lstat(target.Root); !os.IsNotExist(err) {
		t.Fatalf("OpenSpec publication before seal agreement: %v", err)
	}
}

func TestHybridSuccessorInterruptedRemoteRequiresExactRetry(t *testing.T) {
	h, hive, target := successorHybridFixture(t)
	hive.interrupt = true
	if _, err := h.PublishSuccessorGenesis(); !errors.Is(err, errInterrupted) {
		t.Fatalf("interruption: %v", err)
	}
	published, err := target.InspectPublication()
	if err != nil || published == nil {
		t.Fatalf("OpenSpec first: %v, %v", published, err)
	}
	if hive.genesis != nil {
		t.Fatal("credited interrupted Hive publication")
	}
	got, err := h.PublishSuccessorGenesis()
	if err != nil || !sameSnapshot(*published, got) || hive.calls != 2 {
		t.Fatalf("exact retry: %v, %v, calls %d", got, err, hive.calls)
	}
	if got.Generation != 1 || got.Revision != 1 || got.PreviousDigest != "" || got.StreamSHA256 != "" || len(got.Batches) != 0 || len(got.Coverage) != 0 || got.Supersedes == nil || got.Supersedes.SealDigest != hive.seal.Digest {
		t.Fatalf("successor inherited predecessor credit or lost authority: %+v", got)
	}
	if _, err := h.PublishSuccessorGenesis(); err != nil || hive.calls != 3 {
		t.Fatalf("idempotent replay: %v, calls %d", err, hive.calls)
	}
}

func TestHybridSuccessorRejectsForeignTargetAndForgedRemoteGenesis(t *testing.T) {
	for _, tc := range []struct {
		name           string
		foreign, forge bool
	}{{"occupied target", true, false}, {"forged returned genesis", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			h, hive, target := successorHybridFixture(t)
			if tc.foreign {
				if err := os.Mkdir(target.Root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target.Root, "foreign"), []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			hive.forge = tc.forge
			if _, err := h.PublishSuccessorGenesis(); err == nil {
				t.Fatal("accepted foreign successor")
			}
			if tc.foreign && hive.calls != 0 {
				t.Fatal("published Hive despite foreign OpenSpec target")
			}
			if tc.forge && (hive.genesis == nil || hive.calls != 1) {
				t.Fatal("forged return did not reach validation")
			}
		})
	}
}

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
	authority      LegacyAuthority
	authorityCalls int
	authorityErr   error
	request        AdvanceRequest
}

func (b *legacyRetryBackend) LegacyAuthority(AdvanceRequest) (LegacyAuthority, error) {
	b.authorityCalls++
	return b.authority, b.authorityErr
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
	recovery := legacyRequest(t, "hybrid-legacy")
	result, upgraded, err := h.UpgradeLegacy(recovery)
	if err != nil || !upgraded || result.Generation != 1 || hive.calls != 2 {
		t.Fatalf("retry=%#v upgraded=%t err=%v calls=%d", result, upgraded, err, hive.calls)
	}
	if got, want := hive.request.LegacySourceSHA256, applyprogress.LegacySourceSHA256(legacy); got != want {
		t.Fatalf("derived recovery source binding = %q, want %q", got, want)
	}
}

func TestHybridUpgradeLegacyClassifiesExactPartialV2ReceiptAsOrdinary(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	request := request(t, "hybrid-v2-recovery", "apb-00000000000000000000000000000001", 1, 1, "")
	hive := &legacyRetryBackend{retryBackend: retryBackend{err: errInterrupted}}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}

	if _, err := h.Advance(request); !errors.Is(err, errInterrupted) {
		t.Fatalf("initial advance error = %v, want interruption", err)
	}
	hive.err = nil
	if _, upgraded, err := h.UpgradeLegacy(request); err != nil || upgraded {
		t.Fatalf("UpgradeLegacy() upgraded=%t err=%v, want ordinary v2 classification", upgraded, err)
	}
	if hive.authorityCalls != 0 {
		t.Fatalf("LegacyAuthority calls = %d, want no legacy authority lookup", hive.authorityCalls)
	}
	if _, err := h.Advance(request); err != nil {
		t.Fatalf("ordinary receipt recovery error = %v", err)
	}
	payload, err := payloadDigestFor(request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, found, err := h.receipt(request.RequestID)
	if err != nil || !found || !receiptComplete(receipt, request, payload) {
		t.Fatalf("recovered receipt found=%t receipt=%#v err=%v, want both acknowledgements", found, receipt, err)
	}
}

func TestHybridUpgradeLegacyRejectsDistinctV2ReceiptPayload(t *testing.T) {
	root := newOpenSpecTestRoot(t)
	original := request(t, "hybrid-v2-conflict", "apb-00000000000000000000000000000001", 1, 1, "")
	tasks, err := os.ReadFile(filepath.Join(root, "tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte("status: complete\n")
	hive := &legacyRetryBackend{
		retryBackend: retryBackend{err: errInterrupted},
		authority:    LegacyAuthority{Tasks: tasks, Progress: legacy, Found: true},
	}
	h := Hybrid{Root: root, OpenSpec: OpenSpec{Root: root}, Hive: hive}
	if _, err := h.Advance(original); !errors.Is(err, errInterrupted) {
		t.Fatalf("initial advance error = %v, want interruption", err)
	}

	changed := request(t, original.RequestID, "apb-00000000000000000000000000000002", 1, 1, "")
	if _, upgraded, err := h.UpgradeLegacy(changed); upgraded || !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("UpgradeLegacy() upgraded=%t err=%v, want request conflict", upgraded, err)
	}
	if hive.authorityCalls != 1 {
		t.Fatalf("LegacyAuthority calls = %d, want one protected legacy recovery lookup", hive.authorityCalls)
	}
	if hive.calls != 1 {
		t.Fatalf("Hive Advance calls = %d, want no reclassified ordinary recovery", hive.calls)
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

// sealHiveFixture uses an independent durable backend behind the Hive seam.
// It also provides a one-shot transport interruption before Hive publication.
type sealHiveFixture struct {
	store     OpenSpec
	interrupt bool
}

func (b *sealHiveFixture) Advance(r AdvanceRequest) (AdvanceResult, error) {
	if b.interrupt {
		b.interrupt = false
		return AdvanceResult{}, errInterrupted
	}
	return b.store.Advance(r)
}

func (b *sealHiveFixture) CurrentSnapshot(AdvanceRequest) (*applyprogress.Snapshot, error) {
	return b.store.Current()
}

func TestHybridSupersessionAfterBothTaskAuthoritiesChange(t *testing.T) {
	for _, hiveFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "hive interrupted", true: "openspec interrupted"}[hiveFirst], func(t *testing.T) {
			root, hiveRoot := t.TempDir(), t.TempDir()
			for _, dir := range []string{root, hiveRoot} {
				if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			open := OpenSpec{Root: root}
			hive := &sealHiveFixture{store: OpenSpec{Root: hiveRoot}}
			initial := request(t, "old-request", "apb-00000000000000000000000000000001", 1, 1, "")
			prepareContinuationSuccessor(t, &initial, "apb-00000000000000000000000000000002")
			for _, store := range []OpenSpec{open, hive.store} {
				if _, err := store.Advance(initial); err != nil {
					t.Fatal(err)
				}
			}
			for _, dir := range []string{root, hiveRoot} {
				if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("- [ ] 1.1 changed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, newManifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "changed"}})
			if err != nil {
				t.Fatal(err)
			}
			seal := initial
			seal.RequestID = "seal-request"
			seal.ExpectedGeneration, seal.ExpectedRevision, seal.ExpectedDigest = initial.Snapshot.Generation, initial.Snapshot.Revision, initial.Snapshot.Digest
			seal.Batches = nil
			seal.Snapshot.Schema = applyprogress.SupersessionSnapshotSchema
			seal.Snapshot.Revision++
			seal.Snapshot.PreviousDigest = initial.Snapshot.Digest
			seal.Snapshot.Status = applyprogress.StatusSuperseded
			seal.Snapshot.StreamSHA256, seal.Snapshot.NextEntryIndex, seal.Snapshot.NextEntryID = "", 0, ""
			seal.Snapshot.SealIntent = &applyprogress.SealIntent{SuccessorProject: "jarvis-dev", SuccessorChange: "successor", SuccessorManifestSHA256: newManifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal-request"}
			seal.Snapshot, _, err = applyprogress.SealSnapshot(seal.Snapshot)
			if err != nil || !applyprogress.IsSupersessionSeal(initial.Snapshot, seal.Snapshot) {
				t.Fatalf("seal fixture: %v", err)
			}
			if hiveFirst {
				open.BeforeRename = func() error { return errInterrupted }
			} else {
				hive.interrupt = true
			}
			h := Hybrid{Root: root, OpenSpec: open, Hive: hive, HiveFirst: hiveFirst}
			if _, err := h.Advance(seal); !errors.Is(err, errInterrupted) {
				t.Fatalf("interrupted seal: %v", err)
			}
			if _, err := h.Current(seal); !errors.Is(err, ErrBackendDiverged) {
				t.Fatalf("partial heads: %v", err)
			}
			open.BeforeRename = nil
			h.OpenSpec = open
			if _, err := h.Advance(seal); err != nil {
				t.Fatalf("recovery: %v", err)
			}
			current, err := h.Current(seal)
			if err != nil || current == nil || current.Digest != seal.Snapshot.Digest {
				t.Fatalf("equal sealed heads: %v, %v", current, err)
			}
			if _, err := h.Advance(seal); err != nil {
				t.Fatalf("exact retry: %v", err)
			}
			evidencePath := filepath.Join(hiveRoot, "apply-evidence", initial.Batches[0].BatchID+".json")
			evidence, err := os.ReadFile(evidencePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(evidencePath, []byte("corrupt"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := h.Current(seal); !errors.Is(err, ErrBackendDiverged) {
				t.Fatalf("corrupt Hive evidence: %v", err)
			}
			if err := os.WriteFile(evidencePath, evidence, 0o600); err != nil {
				t.Fatal(err)
			}
			conflict := seal
			conflict.ExpectedRevision++
			if _, err := h.Advance(conflict); !errors.Is(err, ErrRequestConflict) {
				t.Fatalf("different payload: %v", err)
			}
		})
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
	if _, err := h.Advance(r); err != nil || open.calls != 2 || hive.calls != 2 {
		t.Fatalf("recovery err=%v calls=%d/%d", err, open.calls, hive.calls)
	}
}
