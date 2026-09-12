package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress/filelock"
)

func TestSddProgressAdvanceReportsConflictCurrentState(t *testing.T) {
	root := newProgressTestRoot(t)
	first := progressRequest(t, "request-first", "apb-00000000000000000000000000000001", 1, "")
	if _, err := (sddprogress.OpenSpec{Root: root}).Advance(first); err != nil {
		t.Fatal(err)
	}
	stale := progressRequest(t, "request-stale", "apb-00000000000000000000000000000002", 1, "")

	out, err := executeSddProgressAdvance(t, newSddProgressCommand(defaultOpenSpec), root, stale)
	if err != nil {
		t.Fatalf("advance stale: %v", err)
	}
	if out.Outcome != "conflict" || out.Code != "stale" {
		t.Fatalf("output = %#v, want stale conflict", out)
	}
	if out.State.Generation != 1 || out.State.Digest != first.Snapshot.Digest {
		t.Fatalf("current state = %#v, want generation 1 digest %q", out.State, first.Snapshot.Digest)
	}
	if out.Recovery != staleProgressRecovery {
		t.Fatalf("stale recovery = %q, want %q", out.Recovery, staleProgressRecovery)
	}
}

func TestSddProgressAdvanceRecoversInterruptedRequestOnce(t *testing.T) {
	root := newProgressTestRoot(t)
	first := progressRequest(t, "request-first", "apb-00000000000000000000000000000001", 1, "")
	nextEntry := first.Batches[0].Entries[0]
	nextEntry.EntryID = "entry-00000000000000000000000000000002"
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{first.Batches[0].Entries[0], nextEntry})
	if err != nil {
		t.Fatal(err)
	}
	first.Snapshot.StreamSHA256, first.Snapshot.NextEntryIndex, first.Snapshot.NextEntryID = stream, 1, nextEntry.EntryID
	first.Snapshot, _, err = applyprogress.SealSnapshot(first.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (sddprogress.OpenSpec{Root: root}).Advance(first); err != nil {
		t.Fatal(err)
	}
	pending := progressSuccessor(t, first, progressRequest(t, "request-pending", "apb-00000000000000000000000000000002", 2, first.Snapshot.Digest))
	pending.Snapshot.StreamSHA256, pending.Snapshot.NextEntryIndex, pending.Snapshot.NextEntryID = stream, 2, "entry-end"
	pending.Snapshot, _, err = applyprogress.SealSnapshot(pending.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("interrupted before rename")
	open := func(path string) progressAdvancer {
		return sddprogress.OpenSpec{Root: path, BeforeRename: func() error { return interrupted }}
	}

	out, err := executeSddProgressAdvance(t, newSddProgressCommand(open), root, pending)
	if !errors.Is(err, interrupted) || out.Outcome != "recovery" {
		t.Fatalf("interrupted advance = %#v, %v; want recovery and interruption", out, err)
	}
	beforeRetry, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil || !bytes.Contains(beforeRetry, []byte(first.Snapshot.Digest)) {
		t.Fatalf("authoritative snapshot = %q, %v; want old snapshot", beforeRetry, err)
	}

	out, err = executeSddProgressAdvance(t, newSddProgressCommand(defaultOpenSpec), root, pending)
	if err != nil || out.Outcome != "committed" || out.State.Digest != pending.Snapshot.Digest {
		t.Fatalf("retry = %#v, %v; want committed pending snapshot", out, err)
	}
	out, err = executeSddProgressAdvance(t, newSddProgressCommand(defaultOpenSpec), root, pending)
	if err != nil || out.Outcome != "committed" {
		t.Fatalf("identical retry = %#v, %v; want idempotent commit", out, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "apply-evidence"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("evidence entries = %d, %v; want two unique batches", len(entries), err)
	}
}

func TestSddProgressCheckpointRecoversReceiptBeforeHeadAndBlocksChangedRetry(t *testing.T) {
	root := newProgressTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1 first\n- [ ] 2 second\n- [ ] 3 third\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks := []applyprogress.Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}, {ID: "3", Text: "third"}}
	entries := []applyprogress.EvidenceEntry{
		checkpointEntry("third", "3", strings.Repeat("x", 21000)),
		checkpointEntry("first", "1", strings.Repeat("x", 21000)),
		checkpointEntry("second", "2", strings.Repeat("x", 21000)),
	}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Entries: entries, EntryID: "third", BatchID: "apb-000000000000000000000000000000a1"})
	if err != nil || first.Outcome != applyprogress.PlanContinuationRequired {
		t.Fatalf("first plan = %#v, %v", first, err)
	}
	if _, err := (sddprogress.OpenSpec{Root: root}).Advance(sddprogress.AdvanceRequest{RequestID: "checkpoint-interrupted-first", Snapshot: first.Snapshot, Batches: []applyprogress.Batch{first.Batch}}); err != nil {
		t.Fatal(err)
	}
	pending := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Base: &first.Snapshot, ExpectedGeneration: first.Snapshot.Generation, ExpectedRevision: first.Snapshot.Revision, ExpectedDigest: first.Snapshot.Digest, RequestID: "checkpoint-interrupted-second", BatchID: "apb-000000000000000000000000000000a2", Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256}
	interrupted := errors.New("interrupted before rename")
	out, err := executeSddProgressCheckpoint(t, newSddProgressCommand(func(path string) progressAdvancer {
		return sddprogress.OpenSpec{Root: path, BeforeRename: func() error { return interrupted }}
	}), root, pending)
	if !errors.Is(err, interrupted) || out.Outcome != "recovery" || out.Code != "publication_interrupted" {
		t.Fatalf("interrupted checkpoint = %#v, %v", out, err)
	}
	changed := pending
	changed.BatchID = "apb-000000000000000000000000000000a3"
	out, err = executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, changed)
	if err != nil || out.Outcome != "conflict" || out.Code != "request_id_conflict" {
		t.Fatalf("changed interrupted retry = %#v, %v", out, err)
	}
	out, err = executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, pending)
	if err != nil || out.Outcome != "continuation_required" || out.State.Digest == first.Snapshot.Digest {
		t.Fatalf("identical interrupted retry = %#v, %v", out, err)
	}
}

func TestSddProgressCheckpointRejectsAnonymousStageOnlyCrash(t *testing.T) {
	root := newProgressTestRoot(t)
	request := checkpointInput{
		Project: "jarvis-dev", Change: "issue-653", RequestID: "checkpoint-stage-only-retry",
		Tasks:   []applyprogress.Task{{ID: "1.1", Text: "task"}},
		BatchID: "apb-000000000000000000000000000000a4",
		Entries: []applyprogress.EvidenceEntry{checkpointEntry("entry-stage-only", "1.1", "evidence")},
		EntryID: "entry-stage-only",
	}
	stageDir := filepath.Join(root, ".apply-progress-receipts")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(stageDir, ".apply-progress-stage-crash")
	if err := os.WriteFile(stage, []byte("durably staged but unpublished"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, request)
	if !errors.Is(err, sddprogress.ErrPublicationInterrupted) || out.Outcome != "recovery" {
		t.Fatalf("anonymous checkpoint stage = %#v, %v", out, err)
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatalf("checkpoint mutated anonymous stage: %v", err)
	}
}

func TestSddProgressAdvanceRejectsChangedRequestPayload(t *testing.T) {
	root := newProgressTestRoot(t)
	request := progressRequest(t, "request-once", "apb-00000000000000000000000000000001", 1, "")
	if out, err := executeSddProgressAdvance(t, newSddProgressCommand(defaultOpenSpec), root, request); err != nil || out.Outcome != "committed" {
		t.Fatalf("initial advance = %#v, %v", out, err)
	}
	changed := progressRequest(t, request.RequestID, "apb-00000000000000000000000000000002", 2, request.Snapshot.Digest)
	out, err := executeSddProgressAdvance(t, newSddProgressCommand(defaultOpenSpec), root, changed)
	if err != nil {
		t.Fatalf("changed payload error = %v", err)
	}
	if out.Outcome != "conflict" || out.Code != "request_id_conflict" {
		t.Fatalf("changed payload output = %#v, want request conflict", out)
	}
}

func TestSddProgressAdvanceUpgradesLegacyOnlyOnMutation(t *testing.T) {
	root := t.TempDir()
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runSddProgressAdvance(sddprogress.OpenSpec{Root: root}, root, progressLegacyRequest(t))
	if err != nil || out.Outcome != "committed" || out.State.Generation != 1 {
		t.Fatalf("upgrade = %#v, %v", out, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "apply-progress.md")); err != nil || !bytes.Contains(data, []byte(`"schema":"jarvis.sdd-apply-progress/v2"`)) {
		t.Fatalf("v2 source = %q, %v", data, err)
	}
}

func newProgressTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func progressLegacyRequest(t *testing.T) sddprogress.AdvanceRequest {
	t.Helper()
	r := progressRequest(t, "legacy", "apb-00000000000000000000000000000001", 1, "")
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
	r.Snapshot.TaskManifestSHA256, r.Snapshot.Status, r.Snapshot.Coverage = manifest, applyprogress.StatusComplete, []applyprogress.Coverage{{TaskID: legacyID, BatchID: r.Batches[0].BatchID, EntryID: "entry"}}
	r.Snapshot.Batches[0].SHA256 = r.Batches[0].SHA256
	r.Snapshot, _, err = applyprogress.SealSnapshot(r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type commandProgressBackend struct {
	calls        int
	currentCalls int
	err          error
	state        sddprogress.AdvanceResult
	current      sddprogress.AdvanceResult
	authority    *sddprogress.LegacyAuthority
}

func (b *commandProgressBackend) LegacyAuthority(sddprogress.AdvanceRequest) (sddprogress.LegacyAuthority, error) {
	if b.authority == nil {
		return sddprogress.LegacyAuthority{}, nil
	}
	return *b.authority, nil
}

func (b *commandProgressBackend) Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.calls++
	return b.state, b.err
}

func (b *commandProgressBackend) Current(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.currentCalls++
	return b.current, nil
}

func TestConfiguredSddProgressAdvanceUpgradesLegacyInHybridMode(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks, err := os.ReadFile(filepath.Join(root, "tasks.md"))
	if err != nil {
		t.Fatal(err)
	}
	hive := &commandProgressBackend{authority: &sddprogress.LegacyAuthority{Tasks: tasks, Progress: []byte("status: complete\n"), Found: true}}
	oldOpen, oldHive := newProgressOpenSpec, newProgressHive
	t.Cleanup(func() { newProgressOpenSpec, newProgressHive = oldOpen, oldHive })
	newProgressOpenSpec = defaultOpenSpec
	newProgressHive = func() (progressAdvancer, error) { return hive, nil }
	t.Setenv("JARVIS_SDD_STORE_MODE", "hybrid")
	out, err := runSddProgressAdvance(configuredProgressStore(root), root, progressLegacyRequest(t))
	if err != nil || out.Outcome != "committed" || hive.calls != 1 {
		t.Fatalf("upgrade = %#v, %v; hive calls=%d", out, err, hive.calls)
	}
}

func TestConfiguredSddProgressAdvanceUsesHybridReceiptRecovery(t *testing.T) {
	interrupted := errors.New("interrupted hive")
	root, request := t.TempDir(), progressRequest(t, "hybrid", "apb-00000000000000000000000000000001", 1, "")
	open, hive := &commandProgressBackend{}, &commandProgressBackend{err: interrupted}
	oldOpen, oldHive := newProgressOpenSpec, newProgressHive
	t.Cleanup(func() { newProgressOpenSpec, newProgressHive = oldOpen, oldHive })
	newProgressOpenSpec = func(string) progressAdvancer { return open }
	newProgressHive = func() (progressAdvancer, error) { return hive, nil }
	t.Setenv("JARVIS_SDD_STORE_MODE", "hybrid")
	command := newSddProgressCommand(configuredProgressStore)
	out, err := executeSddProgressAdvance(t, command, root, request)
	if !errors.Is(err, interrupted) || out.Outcome != "blocked" || out.Code != "backend_diverged" {
		t.Fatalf("interruption = %#v, %v", out, err)
	}
	hive.err = nil
	out, err = executeSddProgressAdvance(t, command, root, request)
	if err != nil || out.Outcome != "committed" || open.calls != 2 || hive.calls != 2 {
		t.Fatalf("replay = %#v, %v; calls=%d/%d", out, err, open.calls, hive.calls)
	}
	changed := progressRequest(t, request.RequestID, "apb-00000000000000000000000000000002", 1, "")
	out, err = executeSddProgressAdvance(t, command, root, changed)
	if err != nil || out.Code != "request_id_conflict" || open.calls != 2 || hive.calls != 2 {
		t.Fatalf("conflict = %#v, %v; calls=%d/%d", out, err, open.calls, hive.calls)
	}
}

func TestSddProgressAdvancePrioritizesBackendDivergenceOverConflict(t *testing.T) {
	root := t.TempDir()
	state := sddprogress.AdvanceResult{Generation: 1, Revision: 1, Digest: "partially-committed"}
	backend := &commandProgressBackend{state: state, err: errors.Join(sddprogress.ErrBackendDiverged, sddprogress.ErrConflict)}
	out, err := executeSddProgressAdvance(t, newSddProgressCommand(func(string) progressAdvancer { return backend }), root, progressRequest(t, "hybrid-diverged", "apb-00000000000000000000000000000001", 1, ""))
	if !errors.Is(err, sddprogress.ErrBackendDiverged) || out.Outcome != "blocked" || out.Code != "backend_diverged" {
		t.Fatalf("out=%#v err=%v; want backend divergence recovery", out, err)
	}
}

func TestConfiguredSddProgressAdvanceFailsClosedOnStaleMode(t *testing.T) {
	root := t.TempDir()
	oldOpen := newProgressOpenSpec
	t.Cleanup(func() { newProgressOpenSpec = oldOpen })
	newProgressOpenSpec = func(string) progressAdvancer { return &commandProgressBackend{err: sddprogress.ErrConflict} }
	t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")
	out, err := executeSddProgressAdvance(t, newSddProgressCommand(configuredProgressStore), root, progressRequest(t, "stale", "apb-00000000000000000000000000000001", 1, ""))
	if err != nil || out.Outcome != "conflict" || out.Code != "stale" {
		t.Fatalf("stale = %#v, %v", out, err)
	}
}

func TestConfiguredSddProgressAdvanceUsesBackendStateForConflicts(t *testing.T) {
	for mode, hybrid := range map[string]bool{"hive": false, "hybrid": true} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			hive := &commandProgressBackend{err: sddprogress.ErrConflict, state: sddprogress.AdvanceResult{Generation: 7, Revision: 8, Digest: "hive-current"}}
			oldOpen, oldHive := newProgressOpenSpec, newProgressHive
			t.Cleanup(func() { newProgressOpenSpec, newProgressHive = oldOpen, oldHive })
			newProgressOpenSpec = func(string) progressAdvancer { return &commandProgressBackend{} }
			newProgressHive = func() (progressAdvancer, error) { return hive, nil }
			t.Setenv("JARVIS_SDD_STORE_MODE", mode)
			out, err := executeSddProgressAdvance(t, newSddProgressCommand(configuredProgressStore), root, progressRequest(t, mode, "apb-00000000000000000000000000000001", 1, ""))
			if hybrid {
				if !errors.Is(err, sddprogress.ErrBackendDiverged) || out.Outcome != "blocked" || out.Code != "backend_diverged" || out.State != hive.state {
					t.Fatalf("out=%#v err=%v", out, err)
				}
				if hive.calls != 1 {
					t.Fatalf("hive calls=%d", hive.calls)
				}
				return
			}
			if err != nil || out.Outcome != "conflict" || out.State != hive.state {
				t.Fatalf("out=%#v err=%v", out, err)
			}
		})
	}
}

func TestConfiguredSddProgressAdvanceUsesHiveCoordinatesAfterOpenSpecStale(t *testing.T) {
	root := t.TempDir()
	open := &commandProgressBackend{err: sddprogress.ErrConflict}
	hive := &commandProgressBackend{current: sddprogress.AdvanceResult{Generation: 9, Revision: 10, Digest: "hive-current"}}
	oldOpen, oldHive := newProgressOpenSpec, newProgressHive
	t.Cleanup(func() { newProgressOpenSpec, newProgressHive = oldOpen, oldHive })
	newProgressOpenSpec = func(string) progressAdvancer { return open }
	newProgressHive = func() (progressAdvancer, error) { return hive, nil }
	t.Setenv("JARVIS_SDD_STORE_MODE", "hybrid")
	out, err := executeSddProgressAdvance(t, newSddProgressCommand(configuredProgressStore), root, progressRequest(t, "stale-open", "apb-00000000000000000000000000000001", 1, ""))
	if !errors.Is(err, sddprogress.ErrBackendDiverged) || out.Outcome != "blocked" || out.Code != "backend_diverged" || out.State != (sddprogress.AdvanceResult{}) {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	if open.calls != 1 || hive.calls != 0 || hive.currentCalls != 0 {
		t.Fatalf("calls open=%d hive=%d current=%d", open.calls, hive.calls, hive.currentCalls)
	}
}

func executeSddProgressAdvance(t *testing.T, cmd *cobra.Command, root string, request sddprogress.AdvanceRequest) (progressAdvanceOutput, error) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "request.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"advance", "--root", root, "--request", path})
	err = cmd.Execute()
	var output progressAdvanceOutput
	if decodeErr := json.Unmarshal(stdout.Bytes(), &output); decodeErr != nil {
		t.Fatalf("decode command output %q: %v", stdout.String(), decodeErr)
	}
	return output, err
}

func TestSddProgressCheckpointNamesExecutableContinuationUpgradeForDetectedV2(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 3, Revision: 3, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	base = historicalCheckpointSnapshot(t, base)
	store := &configuredCheckpointBackend{current: &base}
	request := checkpointRequest(t, "detected-v2-upgrade", "apb-87878787878787878787878787878787", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := runSddProgressCheckpoint(store, request)
	if !errors.Is(err, sddprogress.ErrLegacyMigration) || output.Outcome != "blocked" || output.Code != "legacy_upgrade_required" {
		t.Fatalf("detected v2 output = %#v, %v", output, err)
	}
	if !strings.Contains(output.Recovery, "jarvis sdd progress upgrade-continuation") || strings.Contains(output.Recovery, "legacy artifact") {
		t.Fatalf("recovery = %q, want executable continuation upgrade only", output.Recovery)
	}
}

func TestSddProgressCheckpointNamesContinuationUpgradeForSuppliedPreContinuationBase(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	base, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 3, Revision: 3, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	base = historicalCheckpointSnapshot(t, base)
	request := checkpointRequest(t, "supplied-v2-upgrade", "apb-88888888888888888888888888888889", &base, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := runSddProgressCheckpoint(&configuredCheckpointBackend{}, request)
	if !errors.Is(err, sddprogress.ErrLegacyMigration) || output.Outcome != "blocked" || output.Code != "legacy_upgrade_required" || output.Recovery != continuationUpgradeRecovery {
		t.Fatalf("supplied v2 output = %#v, %v", output, err)
	}
}

func TestSddProgressUpgradeContinuationMigratesHistoricalPreContinuationEpoch(t *testing.T) {
	root := newProgressTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	batch := func(id, entry string) (applyprogress.Batch, []byte) {
		t.Helper()
		sealed, data, sealErr := applyprogress.SealBatch(applyprogress.Batch{
			Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "issue-653", BatchID: id,
			Entries: []applyprogress.EvidenceEntry{{EntryID: entry, TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "historical", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
		})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return sealed, data
	}
	first, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	secondBatch, secondData := batch("apb-84848484848484848484848484848484", "legacy-2")
	second, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 2, Revision: 2, PreviousDigest: first.Digest, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: secondBatch.BatchID, SHA256: secondBatch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	thirdBatch, thirdData := batch("apb-85858585858585858585858585858585", "legacy-3")
	third, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 3, Revision: 3, PreviousDigest: second.Digest, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: append(append([]applyprogress.BatchRef{}, second.Batches...), applyprogress.BatchRef{BatchID: thirdBatch.BatchID, SHA256: thirdBatch.SHA256})})
	if err != nil {
		t.Fatal(err)
	}
	batches := map[string]applyprogress.Batch{secondBatch.BatchID: secondBatch, thirdBatch.BatchID: thirdBatch}
	if err := applyprogress.ValidateSuccessor(first, second, batches); err != nil {
		t.Fatalf("first historical successor = %v", err)
	}
	if err := applyprogress.ValidateSuccessor(second, third, batches); err != nil {
		t.Fatalf("second historical successor = %v", err)
	}
	third = historicalCheckpointSnapshot(t, third)
	if err := os.MkdirAll(filepath.Join(root, "apply-evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-evidence", secondBatch.BatchID+".json"), secondData, 0o600); err != nil {
		t.Fatal(err)
	}
	thirdPath := filepath.Join(root, "apply-evidence", thirdBatch.BatchID+".json")
	if err := os.WriteFile(thirdPath, thirdData, 0o600); err != nil {
		t.Fatal(err)
	}
	thirdSnapshotData, err := json.Marshal(third)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), thirdSnapshotData, 0o600); err != nil {
		t.Fatal(err)
	}

	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	request := checkpointRequest(t, "historical-upgrade", "apb-86868686868686868686868686868686", &third, 0, "entry-1", entries)
	request.StreamSHA256 = stream
	output, err := runSddProgressUpgradeContinuation(sddprogress.OpenSpec{Root: root}, request)
	if err != nil || output.Outcome != "committed" || output.State.Generation != 3 || output.State.Revision != 4 {
		t.Fatalf("upgrade output = %#v, %v", output, err)
	}
	current, err := (sddprogress.OpenSpec{Root: root}).Current()
	if err != nil || current == nil || current.Generation != 3 || current.Revision != 4 || current.StreamSHA256 != stream {
		t.Fatalf("upgraded current = %#v, %v", current, err)
	}
	if stored, readErr := os.ReadFile(thirdPath); readErr != nil || !bytes.Equal(stored, thirdData) {
		t.Fatalf("historical evidence changed = %q, %v", stored, readErr)
	}
}

func TestSddProgressCheckpointRejectsAmbiguousLegacyArtifact(t *testing.T) {
	root := newProgressTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := checkpointRequest(t, "legacy-checkpoint", "apb-0000000000000000000000000000000f", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, request)
	if err == nil || output.Outcome != "invalid" || output.Code != "legacy_migration" {
		t.Fatalf("legacy checkpoint = %#v, %v", output, err)
	}
}

func TestSddProgressCheckpointRejectsAllDoneLegacyWithSuppliedEvidenceBeforeWrite(t *testing.T) {
	root := newProgressTestRoot(t)
	legacy := []byte("status: complete\n")
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	request := checkpointRequest(t, "automatic-import", "apb-0000000000000000000000000000000e", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1.1", "evidence")})
	request.Tasks = []applyprogress.Task{{ID: "1.1", Text: "task"}}
	output, err := runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, request)
	if !errors.Is(err, sddprogress.ErrLegacyMigration) || output.Outcome != "invalid" || output.Code != "legacy_migration" {
		t.Fatalf("all-done legacy with evidence = %#v, %v", output, err)
	}
	persisted, readErr := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if readErr != nil || !bytes.Equal(persisted, legacy) {
		t.Fatalf("legacy source = %q, %v; want unchanged %q", persisted, readErr, legacy)
	}
}

func TestSddProgressCheckpointImportsPartialLegacyThenContinuesOriginalStream(t *testing.T) {
	root := newProgressTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 imported\n- [ ] 1.2 pending\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: partial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-2", "1.2", "new evidence")}
	first := checkpointInput{
		Project: "jarvis-dev", Change: "issue-653",
		Tasks:     []applyprogress.Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "pending"}},
		RequestID: "partial-legacy-import", BatchID: "apb-00000000000000000000000000000051",
		Entries: entries, EntryID: "entry-2",
	}
	output, err := runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, first)
	if err != nil || output.Outcome != "continuation_required" || output.Code != "legacy_imported" || output.Snapshot == nil || output.NextEntryIndex != 0 || output.NextEntryID != "entry-2" || len(output.StreamSHA256) != 64 {
		t.Fatalf("partial legacy import = %#v, %v", output, err)
	}
	if output.Snapshot.StreamSHA256 != output.StreamSHA256 || output.Snapshot.NextEntryID != output.NextEntryID {
		t.Fatalf("imported continuation = %#v", output.Snapshot)
	}

	changedRetry := first
	changedRetry.Entries = append([]applyprogress.EvidenceEntry(nil), first.Entries...)
	changedRetry.Entries[0].Summary = "changed retry payload"
	retry, retryErr := runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, changedRetry)
	if retryErr != nil || retry.Outcome != "conflict" || retry.Code != "request_id_conflict" {
		t.Fatalf("changed legacy-import retry = %#v, %v", retry, retryErr)
	}

	second := first
	second.Base = output.Snapshot
	second.ExpectedGeneration, second.ExpectedRevision, second.ExpectedDigest = output.State.Generation, output.State.Revision, output.State.Digest
	second.RequestID, second.BatchID = "partial-legacy-continue", "apb-00000000000000000000000000000052"
	second.EntryIndex, second.EntryID, second.StreamSHA256 = output.NextEntryIndex, output.NextEntryID, output.StreamSHA256
	output, err = runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, second)
	if err != nil || output.Outcome != "committed" || output.Snapshot == nil || output.Snapshot.Status != applyprogress.StatusComplete {
		t.Fatalf("continued imported stream = %#v, %v", output, err)
	}
}

func TestSddProgressCheckpointRejectsIncompleteAutomaticLegacyFutureStreamBeforeWrite(t *testing.T) {
	root := newProgressTestRoot(t)
	legacy := []byte("status: partial\n")
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 imported\n- [ ] 1.2 pending\n- [ ] 1.3 pending\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	request := checkpointInput{
		Project: "jarvis-dev", Change: "issue-653", RequestID: "partial-legacy-incomplete", BatchID: "apb-00000000000000000000000000000053",
		Tasks:   []applyprogress.Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "pending"}, {ID: "1.3", Text: "pending"}},
		Entries: []applyprogress.EvidenceEntry{checkpointEntry("entry-2", "1.2", "only one remaining task")}, EntryID: "entry-2",
	}
	output, err := runSddProgressCheckpoint(sddprogress.OpenSpec{Root: root}, request)
	if err == nil || output.Outcome != "invalid" || output.Code != string(applyprogress.CodeInvalidPlan) {
		t.Fatalf("incomplete legacy future stream = %#v, %v", output, err)
	}
	persisted, readErr := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if readErr != nil || !bytes.Equal(persisted, legacy) {
		t.Fatalf("legacy source = %q, %v; want unchanged %q", persisted, readErr, legacy)
	}
}

func TestAutomaticLegacyCheckpointPreflightsFutureOversizedEntry(t *testing.T) {
	entries := []applyprogress.EvidenceEntry{
		checkpointEntry("second", "1.2", "fits"),
		checkpointEntry("third", "1.3", strings.Repeat("x", applyprogress.MaxDocumentRunes)),
	}
	input := checkpointInput{
		Project: "jarvis-dev", Change: "issue-653", RequestID: "legacy-preflight", BatchID: "apb-00000000000000000000000000000054",
		Tasks:   []applyprogress.Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "second"}, {ID: "1.3", Text: "third"}},
		Entries: entries, EntryID: "second",
	}
	_, err := automaticLegacyCheckpointFromArtifacts([]byte("status: partial\n"), "- [x] 1.1 imported\n- [ ] 1.2 second\n- [ ] 1.3 third\n", input)
	if !isProgressCapacityError(err) {
		t.Fatalf("automatic legacy checkpoint error = %v, want future batch capacity preflight", err)
	}
}

func TestSddProgressCheckpointValidatesRequestIDBeforeReceiptLookup(t *testing.T) {
	request := checkpointRequest(t, "../unsafe", "apb-00000000000000000000000000000055", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", strings.Repeat("x", applyprogress.MaxDocumentRunes))})
	backend := &checkpointBackend{requestIDUsed: true}
	output, err := runSddProgressCheckpoint(backend, request)
	if err == nil || output.Code != "validation" || backend.requestIDCalls != 0 {
		t.Fatalf("checkpoint = %#v, %v; receipt lookups=%d, want validation before lookup", output, err, backend.requestIDCalls)
	}
}

func TestSddProgressProgressVerbsRejectTrailingJSONValues(t *testing.T) {
	for _, verb := range []string{"advance", "checkpoint", "upgrade-continuation"} {
		t.Run(verb, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "request.json")
			if err := os.WriteFile(path, []byte(`{} {}`), 0o600); err != nil {
				t.Fatal(err)
			}
			command := newSddProgressCommand(defaultOpenSpec)
			command.SetArgs([]string{verb, "--root", root, "--request", path})
			if err := command.Execute(); err == nil {
				t.Fatal("command accepted trailing JSON value")
			}
		})
	}
}

func TestSddProgressCurrentReadPreservesStructuredErrors(t *testing.T) {
	base := progressRequest(t, "typed-read-base", "apb-00000000000000000000000000000054", 1, "").Snapshot
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1.1", "evidence")}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		readErr  error
		outcome  string
		code     string
		detail   string
		recovery string
	}{
		{name: "project not found", readErr: &hiveclient.ApplyProgressError{StatusCode: http.StatusNotFound, Result: hiveclient.ApplyProgressResult{Outcome: "not_found", Code: "project_not_found", Recovery: "select or create the Hive project before retrying"}}, outcome: "not_found", code: "project_not_found", recovery: "select or create the Hive project before retrying"},
		{name: "validation detail", readErr: &hiveclient.ApplyProgressError{StatusCode: http.StatusUnprocessableEntity, Result: hiveclient.ApplyProgressResult{Outcome: "invalid", Code: "validation", Detail: "project", Recovery: "repair the project identifier and retry"}}, outcome: "invalid", code: "validation", detail: "project", recovery: "repair the project identifier and retry"},
		{name: "backend divergence", readErr: sddprogress.ErrBackendDiverged, outcome: "blocked", code: "backend_diverged", recovery: "retry the identical request only after the missing backend is available"},
		{name: "unavailable", readErr: &hiveclient.ApplyProgressError{StatusCode: http.StatusServiceUnavailable, Result: hiveclient.ApplyProgressResult{Outcome: "unavailable", Code: "unavailable", Recovery: "start the Hive daemon before retrying"}}, outcome: "unavailable", code: "unavailable", recovery: "start the Hive daemon before retrying"},
		{name: "transport failure", readErr: errors.New("dial Hive daemon"), outcome: "recovery", code: "read_current_failed", recovery: "read the authoritative snapshot before retrying"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &checkpointBackend{current: &base, currentErr: tt.readErr}
			request := checkpointInput{Project: base.Project, Change: base.Change, Tasks: []applyprogress.Task{{ID: "1.1", Text: "task"}}, Base: &base, ExpectedGeneration: base.Generation, ExpectedRevision: base.Revision, ExpectedDigest: base.Digest, RequestID: "current-read-" + strings.ReplaceAll(tt.name, " ", "-"), Entries: entries, EntryID: "entry-1", StreamSHA256: stream}
			for _, run := range []struct {
				name string
				run  func(progressAdvancer, checkpointInput) (checkpointOutput, error)
			}{
				{name: "upgrade continuation", run: runSddProgressUpgradeContinuation},
				{name: "checkpoint", run: runSddProgressCheckpoint},
			} {
				t.Run(run.name, func(t *testing.T) {
					output, gotErr := run.run(store, request)
					if gotErr == nil || output.Outcome != tt.outcome || output.Code != tt.code || output.Detail != tt.detail || output.Recovery != tt.recovery {
						t.Fatalf("current read = %#v, %v; want %s/%s detail=%q recovery=%q", output, gotErr, tt.outcome, tt.code, tt.detail, tt.recovery)
					}
				})
			}
		})
	}
}

func historicalCheckpointSnapshot(t *testing.T, snapshot applyprogress.Snapshot) applyprogress.Snapshot {
	t.Helper()
	type payload struct {
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
	}
	wirePayload := payload{Schema: snapshot.Schema, Project: snapshot.Project, Change: snapshot.Change, Generation: snapshot.Generation, Revision: snapshot.Revision, PreviousDigest: snapshot.PreviousDigest, TaskManifestSHA256: snapshot.TaskManifestSHA256, Status: snapshot.Status, Coverage: snapshot.Coverage, Batches: snapshot.Batches}
	payloadJSON, err := json.Marshal(wirePayload)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payloadJSON)
	data, err := json.Marshal(struct {
		payload
		Digest string `json:"digest"`
	}{payload: wirePayload, Digest: fmt.Sprintf("%x", sum)})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := applyprogress.DecodeCanonicalSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestSddProgressCheckpointStartsIndependentStreamFromExhaustedUnboundSnapshot(t *testing.T) {
	root := newProgressTestRoot(t)
	tasks := []applyprogress.Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	first := checkpointInput{
		Project: "jarvis-dev", Change: "issue-653", Tasks: tasks,
		RequestID: "independent-first", BatchID: "apb-000000000000000000000000000000d3",
		Entries: []applyprogress.EvidenceEntry{checkpointEntry("first", "1", "first stream")}, EntryID: "first",
	}
	firstOutput, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, first)
	if err != nil || firstOutput.Outcome != "committed" || firstOutput.Snapshot == nil || firstOutput.Snapshot.Status != applyprogress.StatusPartial || firstOutput.Snapshot.StreamSHA256 != "" {
		t.Fatalf("exhausted first stream = %#v, %v; want committed ordinary unbound partial", firstOutput, err)
	}

	second := checkpointInput{
		Project: first.Project, Change: first.Change, Tasks: tasks, Base: firstOutput.Snapshot,
		ExpectedGeneration: firstOutput.State.Generation, ExpectedRevision: firstOutput.State.Revision, ExpectedDigest: firstOutput.State.Digest,
		RequestID: "independent-second", BatchID: "apb-000000000000000000000000000000d4",
		Entries: []applyprogress.EvidenceEntry{checkpointEntry("second", "2", "second stream")}, EntryID: "second",
	}
	secondOutput, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, second)
	if err != nil || secondOutput.Outcome != "committed" || secondOutput.Snapshot == nil || secondOutput.Snapshot.Status != applyprogress.StatusComplete {
		t.Fatalf("independent successor = %#v, %v; want normal completion without migration CAS", secondOutput, err)
	}
}

func TestSddProgressCheckpointRequiresUpgradeForPreContinuationV2Resume(t *testing.T) {
	base := historicalCheckpointSnapshot(t, progressRequest(t, "old-v2", "apb-00000000000000000000000000000041", 1, "").Snapshot)
	request := checkpointRequest(t, "old-v2-resume", "apb-00000000000000000000000000000042", &base, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := runSddProgressCheckpoint(&checkpointBackend{current: &base}, request)
	if err == nil || output.Outcome != "blocked" || output.Code != "legacy_upgrade_required" {
		t.Fatalf("old v2 resume = %#v, %v", output, err)
	}
	if want := "run `jarvis sdd progress upgrade-continuation` with the same request ID and request inputs"; output.Recovery != want {
		t.Fatalf("legacy continuation recovery = %q, want %q", output.Recovery, want)
	}
}

func TestSddProgressUpgradeContinuationPreservesPreContinuationEvidence(t *testing.T) {
	root := newProgressTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 imported\n- [ ] 1.2 pending\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tasks := []applyprogress.Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "pending"}}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	prior, priorData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "issue-653", BatchID: "apb-00000000000000000000000000000040", Entries: []applyprogress.EvidenceEntry{checkpointEntry("imported-1", "1.1", "imported")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "apply-evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-evidence", prior.BatchID+".json"), priorData, 0o600); err != nil {
		t.Fatal(err)
	}
	base, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: prior.BatchID, EntryID: "imported-1"}}, Batches: []applyprogress.BatchRef{{BatchID: prior.BatchID, SHA256: prior.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	base = historicalCheckpointSnapshot(t, base)
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1.2", "evidence")}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	request := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Base: &base, ExpectedGeneration: 1, ExpectedRevision: 1, ExpectedDigest: base.Digest, RequestID: "upgrade-continuation", Entries: entries, StreamSHA256: stream}
	output, err := runSddProgressUpgradeContinuation(sddprogress.OpenSpec{Root: root}, request)
	if err != nil || output.Outcome != "committed" || output.Snapshot == nil || output.Snapshot.NextEntryID != "entry-1" {
		t.Fatalf("upgrade = %#v, %v", output, err)
	}
	if !reflect.DeepEqual(output.Snapshot.Batches, base.Batches) || !reflect.DeepEqual(output.Snapshot.Coverage, base.Coverage) || output.Snapshot.Status != base.Status || output.Snapshot.TaskManifestSHA256 != base.TaskManifestSHA256 {
		t.Fatalf("upgrade rewrote immutable prefix: %#v", output.Snapshot)
	}
	if output.Snapshot.Generation != base.Generation || output.Snapshot.Revision != base.Revision+1 || output.Snapshot.PreviousDigest != base.Digest || output.Snapshot.StreamSHA256 != stream || output.Snapshot.NextEntryIndex != 0 || output.Snapshot.NextEntryID != entries[0].EntryID {
		t.Fatalf("upgrade did not make the CAS-only continuation mutation: %#v", output.Snapshot)
	}
}

func TestCheckpointContinuationJSONUsesPresenceAwareNextCursorKeys(t *testing.T) {
	base := historicalCheckpointSnapshot(t, progressRequest(t, "legacy-output-base", "apb-00000000000000000000000000000044", 1, "").Snapshot)
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1.1", "evidence")}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := runSddProgressUpgradeContinuation(&checkpointBackend{current: &base}, checkpointInput{
		Project: base.Project, Change: base.Change, Tasks: []applyprogress.Task{{ID: "1.1", Text: "task"}}, Base: &base,
		ExpectedGeneration: base.Generation, ExpectedRevision: base.Revision, ExpectedDigest: base.Digest,
		RequestID: "legacy-output-upgrade", Entries: entries, StreamSHA256: stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		output checkpointOutput
	}{
		{name: "initial partial legacy", output: checkpointOutput{Outcome: "continuation_required", Code: "legacy_imported", NextEntryIndex: 0, NextEntryID: "entry-1", StreamSHA256: stream}},
		{name: "upgrade continuation", output: upgraded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, marshalErr := json.Marshal(tt.output)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields["next_entry_index"]; !ok {
				t.Fatalf("continuation JSON omitted zero index: %s", data)
			}
			if _, ok := fields["next_entry_id"]; !ok {
				t.Fatalf("continuation JSON omitted next entry ID: %s", data)
			}
			if _, legacy := fields["entry_id"]; legacy {
				t.Fatalf("continuation JSON emitted legacy entry_id: %s", data)
			}
		})
	}
}

func TestSddProgressUpgradeContinuationMapsStoreErrorsLikeCheckpoint(t *testing.T) {
	base := historicalCheckpointSnapshot(t, progressRequest(t, "legacy-upgrade-base", "apb-00000000000000000000000000000043", 1, "").Snapshot)
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1.1", "evidence")}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name          string
		failure       error
		outcome, code string
		handled       bool
	}{
		{name: "stale", failure: sddprogress.ErrConflict, outcome: "conflict", code: "stale", handled: true},
		{name: "capacity", failure: &applyprogress.CapacityError{Document: "snapshot", Runes: applyprogress.MaxDocumentRunes + 1}, outcome: "invalid", code: "capacity", handled: true},
		{name: "validation", failure: applyprogress.ErrInvalidID, outcome: "invalid", code: "validation"},
		{name: "lock", failure: filelock.ErrBusy, outcome: "blocked", code: "lock_busy"},
		{name: "divergence", failure: sddprogress.ErrBackendDiverged, outcome: "blocked", code: "backend_diverged"},
		{name: "daemon", failure: &hiveclient.ApplyProgressError{StatusCode: http.StatusServiceUnavailable, Result: hiveclient.ApplyProgressResult{Outcome: "unavailable", Code: "unavailable", Recovery: "start daemon"}}, outcome: "unavailable", code: "unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &checkpointBackend{current: &base, advanceErr: tt.failure}
			request := checkpointInput{Project: base.Project, Change: base.Change, Tasks: []applyprogress.Task{{ID: "1.1", Text: "task"}}, Base: &base, ExpectedGeneration: base.Generation, ExpectedRevision: base.Revision, ExpectedDigest: base.Digest, RequestID: "upgrade-" + tt.name, Entries: entries, StreamSHA256: stream}
			output, err := runSddProgressUpgradeContinuation(store, request)
			if output.Outcome != tt.outcome || output.Code != tt.code || (tt.handled && err != nil) || (!tt.handled && err == nil) {
				t.Fatalf("upgrade error = %#v, %v", output, err)
			}
		})
	}
}

func TestSddProgressCheckpointCommitsInitialStream(t *testing.T) {
	root := newProgressTestRoot(t)
	request := checkpointRequest(t, "checkpoint-1", "apb-00000000000000000000000000000001", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, request)
	if err != nil || output.Outcome != "committed" || output.State.Generation != 1 {
		t.Fatalf("checkpoint = %#v, %v; want initial committed state", output, err)
	}
}

type checkpointBackend struct {
	current        *applyprogress.Snapshot
	currentErr     error
	calls          int
	currentCalls   int
	requestIDUsed  bool
	requestIDCalls int
	advanceErr     error
	advanceResult  sddprogress.AdvanceResult
}

func (b *checkpointBackend) Current() (*applyprogress.Snapshot, error) {
	b.currentCalls++
	return b.current, b.currentErr
}
func (b *checkpointBackend) Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.calls++
	return b.advanceResult, b.advanceErr
}
func (b *checkpointBackend) RequestIDUsed(string) bool {
	b.requestIDCalls++
	return b.requestIDUsed
}

type pureHiveCheckpointBackend struct {
	current      *applyprogress.Snapshot
	tasks        string
	calls        int
	taskCalls    int
	currentCalls int
	receiptFound bool
	receiptCalls int
}

func (b *pureHiveCheckpointBackend) CurrentCheckpoint(string, string) (*applyprogress.Snapshot, error) {
	b.currentCalls++
	return b.current, nil
}

func (b *pureHiveCheckpointBackend) FetchAuthoritativeTasks(string, string) (string, error) {
	b.taskCalls++
	return b.tasks, nil
}

func (b *pureHiveCheckpointBackend) Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.calls++
	return sddprogress.AdvanceResult{}, nil
}

func (b *pureHiveCheckpointBackend) ReceiptIdentity(_, _, _ string) (bool, error) {
	b.receiptCalls++
	return b.receiptFound, nil
}

func TestSddProgressCheckpointPureHiveRejectsInvalidAuthoritativeTasksBeforeWrites(t *testing.T) {
	request := checkpointRequest(t, "pure-hive-tasks", "apb-00000000000000000000000000000071", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	for _, tt := range []struct {
		name  string
		tasks string
	}{
		{name: "missing", tasks: ""},
		{name: "malformed", tasks: "- [z] 1 task\n"},
		{name: "caller mismatch", tasks: "- [ ] 1 different task\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &pureHiveCheckpointBackend{tasks: tt.tasks}
			output, err := runSddProgressCheckpoint(store, request)
			if err == nil || output.Outcome != "invalid" || store.taskCalls != 1 || store.currentCalls != 0 || store.calls != 0 {
				t.Fatalf("checkpoint = %#v, %v; task/current/write calls=%d/%d/%d", output, err, store.taskCalls, store.currentCalls, store.calls)
			}
		})
	}
}

func TestSddProgressCheckpointCapacityDoesNotAdvance(t *testing.T) {
	for name, test := range map[string]struct {
		request  checkpointInput
		outcome  string
		document string
	}{
		"evidence item": {checkpointRequest(t, "capacity-entry", "apb-00000000000000000000000000000002", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", strings.Repeat("x", applyprogress.MaxDocumentRunes))}), "evidence_item_too_large", "batch"},
		"snapshot":      {checkpointSnapshotCapacityRequest(t), "snapshot_capacity_exhausted", "snapshot"},
	} {
		t.Run(name, func(t *testing.T) {
			request := test.request
			backend := &checkpointBackend{current: request.Base}
			output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(func(string) progressAdvancer { return backend }), t.TempDir(), request)
			if err != nil || backend.calls != 0 || output.Outcome != test.outcome || output.Code != test.outcome {
				t.Fatalf("capacity = %#v, %v; calls=%d", output, err, backend.calls)
			}
			if output.Capacity == nil || output.Capacity.Document != test.document || output.Capacity.Runes <= applyprogress.MaxDocumentRunes || output.Capacity.Limit != applyprogress.MaxDocumentRunes {
				t.Fatalf("capacity bounds = %#v", output.Capacity)
			}
			data, err := json.Marshal(output)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			capacity, ok := body["capacity"].(map[string]any)
			if !ok || capacity["document"] != test.document || capacity["runes"] != float64(output.Capacity.Runes) || capacity["limit"] != float64(applyprogress.MaxDocumentRunes) {
				t.Fatalf("capacity JSON = %s", data)
			}
		})
	}
}

func TestSddProgressCheckpointPureHivePrioritizesCommittedRequestIDBeforeCapacity(t *testing.T) {
	for name, request := range map[string]checkpointInput{
		"evidence item": checkpointRequest(t, "pure-hive-capacity-entry", "apb-000000000000000000000000000000d1", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", strings.Repeat("x", applyprogress.MaxDocumentRunes))}),
		"snapshot":      checkpointSnapshotCapacityRequest(t),
	} {
		t.Run(name, func(t *testing.T) {
			current := request.Base
			if name == "snapshot" {
				stale := *request.Base
				stale.Revision++
				current = &stale
			}
			backend := &pureHiveCheckpointBackend{tasks: "- [ ] 1 task\n", current: current, receiptFound: true}
			output, err := runSddProgressCheckpoint(backend, request)
			if err != nil || output.Outcome != "conflict" || output.Code != "request_id_conflict" || backend.receiptCalls != 1 || backend.calls != 0 {
				t.Fatalf("checkpoint = %#v, %v; receipt/write calls=%d/%d", output, err, backend.receiptCalls, backend.calls)
			}
		})
	}
}

func TestSddProgressCheckpointPureHiveNoWriteReceiptProbeUsesRequestCoordinates(t *testing.T) {
	request := checkpointRequest(t, "pure-hive-coordinate-probe", "apb-000000000000000000000000000000d2", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", strings.Repeat("x", applyprogress.MaxDocumentRunes))})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/apply-progress"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1 task\n"}]}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/receipts/"):
			if got, want := r.URL.Path, "/sdd/changes/issue-653/apply-progress/receipts/"+request.RequestID; got != want {
				t.Fatalf("receipt probe path = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("project"), request.Project; got != want {
				t.Fatalf("receipt probe project = %q, want %q", got, want)
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	output, err := runSddProgressCheckpoint(hiveProgressAdvancer{client: client}, request)
	if err != nil || output.Code != string(applyprogress.PlanEvidenceItemTooLarge) {
		t.Fatalf("checkpoint = %#v, %v", output, err)
	}
}

func TestSddProgressCheckpointGuardsOnlyUnboundStreamsAndPreservesConflictPrecedence(t *testing.T) {
	tasks := []applyprogress.Task{{ID: "1", Text: "first"}, {ID: "2", Text: "second"}}
	entries := []applyprogress.EvidenceEntry{
		checkpointEntry("first", "1", "tiny"),
		checkpointEntry("second", "2", strings.Repeat("x", applyprogress.MaxDocumentRunes)),
	}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	base := applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: make([]applyprogress.BatchRef, applyprogress.SnapshotCheckpointReferenceGuard), StreamSHA256: stream, NextEntryID: "first"}
	for i := range base.Batches {
		base.Batches[i] = applyprogress.BatchRef{BatchID: fmt.Sprintf("apb-%032x", i+30000), SHA256: strings.Repeat("a", 64)}
	}
	base, _, err = applyprogress.SealSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	activeRequest := checkpointInput{Project: base.Project, Change: base.Change, Tasks: tasks, Base: &base, ExpectedGeneration: base.Generation, ExpectedRevision: base.Revision, ExpectedDigest: base.Digest, RequestID: "frequency-active", BatchID: "apb-000000000000000000000000000000b1", Entries: entries, EntryID: "first", StreamSHA256: stream}
	activeBackend := &checkpointBackend{current: &base}
	active, err := runSddProgressCheckpoint(activeBackend, activeRequest)
	if err != nil || active.Outcome != "continuation_required" || active.Frequency != nil || activeBackend.calls != 1 {
		t.Fatalf("active continuation = %#v, %v; writes=%d", active, err, activeBackend.calls)
	}

	unbound := base
	unbound.StreamSHA256, unbound.NextEntryIndex, unbound.NextEntryID = "", 0, ""
	unbound, _, err = applyprogress.SealSnapshot(unbound)
	if err != nil {
		t.Fatal(err)
	}
	request := checkpointInput{Project: unbound.Project, Change: unbound.Change, Tasks: tasks, Base: &unbound, ExpectedGeneration: unbound.Generation, ExpectedRevision: unbound.Revision, ExpectedDigest: unbound.Digest, RequestID: "frequency-unbound", BatchID: "apb-000000000000000000000000000000b2", Entries: entries, EntryID: "first"}
	backend := &checkpointBackend{current: &unbound}
	guarded, err := runSddProgressCheckpoint(backend, request)
	if err != nil || guarded.Outcome != "checkpoint_consolidation_required" || guarded.Code != "checkpoint_consolidation_required" || guarded.Frequency == nil || guarded.Frequency.References != applyprogress.SnapshotCheckpointReferenceGuard || !strings.Contains(guarded.Recovery, "accumulate or combine") || backend.calls != 0 {
		t.Fatalf("unbound frequency guard = %#v, %v; writes=%d", guarded, err, backend.calls)
	}

	pureHive := &pureHiveCheckpointBackend{current: &unbound, tasks: "- [ ] 1 first\n- [ ] 2 second\n", receiptFound: true}
	conflict, err := runSddProgressCheckpoint(pureHive, request)
	if err != nil || conflict.Outcome != "conflict" || conflict.Code != "request_id_conflict" || pureHive.receiptCalls != 1 || pureHive.calls != 0 {
		t.Fatalf("pure Hive consolidation conflict = %#v, %v; receipt/write calls=%d/%d", conflict, err, pureHive.receiptCalls, pureHive.calls)
	}

	staleCurrent := unbound
	staleCurrent.Revision++
	staleCurrent, _, err = applyprogress.SealSnapshot(staleCurrent)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := runSddProgressCheckpoint(&checkpointBackend{current: &staleCurrent}, request)
	if err != nil || stale.Outcome != "conflict" || stale.Code != "stale" {
		t.Fatalf("stale consolidation = %#v, %v", stale, err)
	}
}

func TestSddProgressCheckpointPreflightsNewStreamBeforeWrite(t *testing.T) {
	entries := make([]applyprogress.EvidenceEntry, 400)
	for i := range entries {
		entries[i] = applyprogress.EvidenceEntry{EntryID: fmt.Sprintf("e%d", i), TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceVerification, Summary: strings.Repeat("x", 20000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}
	}
	request := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: []applyprogress.Task{{ID: "1", Text: "one"}}, RequestID: "stream-preflight", BatchID: "apb-000000000000000000000000000000c1", Entries: entries, EntryID: entries[0].EntryID}
	backend := &checkpointBackend{}

	output, err := runSddProgressCheckpoint(backend, request)
	if err != nil || output.Outcome != "stream_preflight_required" || output.Code != "stream_preflight_required" || output.Capacity == nil || output.Capacity.Document != "snapshot" || backend.calls != 0 {
		t.Fatalf("checkpoint = %#v, %v; writes=%d, want typed no-write stream preflight", output, err, backend.calls)
	}
}

func TestSddProgressCheckpointPreservesRequestIDConflictBeforeCapacity(t *testing.T) {
	request := checkpointSnapshotCapacityRequest(t)
	backend := &checkpointBackend{current: request.Base, requestIDUsed: true}

	output, err := runSddProgressCheckpoint(backend, request)
	if err != nil || output.Outcome != "conflict" || output.Code != "request_id_conflict" {
		t.Fatalf("checkpoint = %#v, %v; want request ID conflict before capacity", output, err)
	}
}

func TestSddProgressCheckpointReturnsStaleBeforeCapacityPlanning(t *testing.T) {
	request := checkpointSnapshotCapacityRequest(t)
	advanced := *request.Base
	advanced.Revision++
	backend := &checkpointBackend{current: &advanced}

	output, err := runSddProgressCheckpoint(backend, request)
	if err != nil || output.Outcome != "conflict" || output.Code != "stale" {
		t.Fatalf("checkpoint = %#v, %v; want stale conflict before capacity outcome", output, err)
	}
	if backend.calls != 0 || backend.currentCalls != 1 {
		t.Fatalf("backend calls = advance:%d current:%d; want no advance and one current read", backend.calls, backend.currentCalls)
	}
}

func TestSddProgressCheckpointRejectsChangedContinuationBeforeBackend(t *testing.T) {
	tasks := []applyprogress.Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}, {ID: "3", Text: "three"}}
	entries := []applyprogress.EvidenceEntry{
		checkpointEntry("e1", "1", strings.Repeat("é", 21000)),
		checkpointEntry("e2", "2", strings.Repeat("é", 21000)),
		checkpointEntry("e3", "3", "last"),
	}
	first, err := applyprogress.PlanCheckpoint(applyprogress.PlanInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Entries: entries, EntryID: "e1", BatchID: "apb-00000000000000000000000000000030"})
	if err != nil || first.Outcome != applyprogress.PlanContinuationRequired {
		t.Fatalf("initial plan = %#v, %v", first, err)
	}
	request := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Base: &first.Snapshot, ExpectedGeneration: first.Snapshot.Generation, ExpectedRevision: first.Snapshot.Revision, ExpectedDigest: first.Snapshot.Digest, RequestID: "checkpoint-resume", BatchID: "apb-00000000000000000000000000000031", Entries: entries, EntryIndex: first.NextEntryIndex, EntryID: first.NextEntryID, StreamSHA256: first.StreamSHA256}
	for _, name := range []string{"reordered", "modified", "truncated", "extended"} {
		t.Run(name, func(t *testing.T) {
			changed := append([]applyprogress.EvidenceEntry(nil), entries...)
			switch name {
			case "reordered":
				changed[0], changed[2] = changed[2], changed[0]
			case "modified":
				changed[0].Summary = "changed"
			case "truncated":
				changed = changed[:2]
			case "extended":
				changed = append(changed, applyprogress.EvidenceEntry{EntryID: "e4", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "extra", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}})
			}
			request.Entries = changed
			backend := &checkpointBackend{current: &first.Snapshot}
			output, err := runSddProgressCheckpoint(backend, request)
			if !errors.Is(err, applyprogress.ErrInvalidValue) || output.Outcome != "invalid" || backend.currentCalls != 1 || backend.calls != 0 {
				t.Fatalf("changed continuation = %#v, %v; backend resolve/advance calls=%d/%d", output, err, backend.currentCalls, backend.calls)
			}
		})
	}
}

func TestSddProgressCheckpointDurablyContinues162725Runes(t *testing.T) {
	root := newProgressTestRoot(t)
	tasks, entries := make([]applyprogress.Task, 5), make([]applyprogress.EvidenceEntry, 5)
	for i := range entries {
		id := fmt.Sprintf("%d", i+1)
		tasks[i], entries[i] = applyprogress.Task{ID: id, Text: id}, checkpointEntry("entry-"+id, id, strings.Repeat("é", 32545))
	}
	var base *applyprogress.Snapshot
	var streamSHA256 string
	for cursor := 0; cursor < len(entries); {
		request := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, Base: base, ExpectedGeneration: checkpointState(base).Generation, ExpectedRevision: checkpointState(base).Revision, ExpectedDigest: checkpointState(base).Digest, RequestID: fmt.Sprintf("checkpoint-%d", cursor), BatchID: fmt.Sprintf("apb-%032x", cursor+10), Entries: entries, EntryIndex: cursor, EntryID: entries[cursor].EntryID, StreamSHA256: streamSHA256}
		output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, request)
		if err != nil || output.Snapshot == nil || output.Receipt == nil || output.Receipt.RequestID != request.RequestID || !sameAdvanceCoordinates(output.State, checkpointState(output.Snapshot)) {
			t.Fatalf("checkpoint %d = %#v, %v", cursor, output, err)
		}
		if cursor == 1 {
			before, err := os.ReadDir(filepath.Join(root, "apply-evidence"))
			if err != nil {
				t.Fatal(err)
			}
			replay, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, request)
			after, readErr := os.ReadDir(filepath.Join(root, "apply-evidence"))
			if err != nil || readErr != nil || replay.State != output.State || len(after) != len(before) {
				t.Fatalf("replay = %#v, %v; batches=%d/%d", replay, err, len(before), len(after))
			}
		}
		if output.Outcome == "continuation_required" {
			if output.NextEntryIndex <= cursor || output.NextEntryID != entries[output.NextEntryIndex].EntryID || len(output.StreamSHA256) != 64 {
				t.Fatalf("continuation = %#v", output)
			}
			base, cursor, streamSHA256 = output.Snapshot, output.NextEntryIndex, output.StreamSHA256
			continue
		}
		if output.Outcome != "committed" || cursor != len(entries)-1 {
			t.Fatalf("final output = %#v", output)
		}
		base, cursor = output.Snapshot, len(entries)
	}
	resolved, err := (sddprogress.OpenSpec{Root: root}).Current()
	if err != nil || resolved == nil || len(resolved.Coverage) != len(tasks) {
		t.Fatalf("resolved = %#v, %v", resolved, err)
	}
	var recovered []applyprogress.EvidenceEntry
	for _, ref := range resolved.Batches {
		data, err := os.ReadFile(filepath.Join(root, "apply-evidence", ref.BatchID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if utf8.RuneCount(data) > applyprogress.MaxDocumentRunes {
			t.Fatalf("batch %q exceeds capacity", ref.BatchID)
		}
		batch, err := applyprogress.DecodeCanonicalBatch(data)
		if err != nil {
			t.Fatal(err)
		}
		recovered = append(recovered, batch.Entries...)
	}
	if len(recovered) != len(entries) || utf8.RuneCountInString(strings.Join(checkpointSummaries(recovered), "")) != 162725 {
		t.Fatalf("recovered = %d entries", len(recovered))
	}
	for i := range entries {
		if recovered[i].EntryID != entries[i].EntryID || recovered[i].Summary != entries[i].Summary {
			t.Fatalf("entry %d = %#v", i, recovered[i])
		}
	}
}

func checkpointSummaries(entries []applyprogress.EvidenceEntry) []string {
	summaries := make([]string, len(entries))
	for i := range entries {
		summaries[i] = entries[i].Summary
	}
	return summaries
}

func sameAdvanceCoordinates(left, right sddprogress.AdvanceResult) bool {
	return left.Generation == right.Generation && left.Revision == right.Revision && left.Digest == right.Digest
}

func checkpointSnapshotCapacityRequest(t *testing.T) checkpointInput {
	t.Helper()
	tasks := []applyprogress.Task{{ID: "1", Text: "task"}}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "fits")}
	stream, err := applyprogress.StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	base := applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, StreamSHA256: stream, NextEntryID: "entry-1"}
	for i := 0; ; i++ {
		base.Batches = append(base.Batches, applyprogress.BatchRef{BatchID: fmt.Sprintf("apb-%032x", i+100), SHA256: strings.Repeat("a", 64)})
		sealed, _, err := applyprogress.SealSnapshot(base)
		if err != nil {
			base.Batches = base.Batches[:len(base.Batches)-1]
			base, _, err = applyprogress.SealSnapshot(base)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		base = sealed
	}
	request := checkpointRequest(t, "capacity-snapshot", "apb-00000000000000000000000000000003", &base, 0, "entry-1", entries)
	request.StreamSHA256 = stream
	return request
}

func TestSddProgressCheckpointRejectsStaleAndChangedRequestID(t *testing.T) {
	root := newProgressTestRoot(t)
	first := checkpointRequest(t, "checkpoint-once", "apb-00000000000000000000000000000004", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, first)
	if err != nil || output.Outcome != "committed" {
		t.Fatalf("first = %#v, %v", output, err)
	}
	base := output.Snapshot
	stale := first
	stale.RequestID, stale.BatchID = "checkpoint-stale", "apb-00000000000000000000000000000005"
	output, err = executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, stale)
	if err != nil || output.Code != "stale" {
		t.Fatalf("stale = %#v, %v", output, err)
	}
	changed := checkpointRequest(t, first.RequestID, "apb-00000000000000000000000000000006", base, 0, "entry-2", []applyprogress.EvidenceEntry{{EntryID: "entry-2", TaskIDs: []string{"1"}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "changed", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}})
	output, err = executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, changed)
	if !errors.Is(err, applyprogress.ErrInvalidValue) || output.Code != string(applyprogress.CodeInvalidPlan) || output.Detail != "stream_sha256" {
		t.Fatalf("changed stream = %#v, %v", output, err)
	}
}

func TestSddProgressCheckpointRejectsDifferentIDCandidateReplay(t *testing.T) {
	root := newProgressTestRoot(t)
	request := checkpointRequest(t, "checkpoint-once", "apb-00000000000000000000000000000007", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	if output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, request); err != nil || output.Outcome != "committed" {
		t.Fatalf("first = %#v, %v", output, err)
	}
	request.RequestID = "checkpoint-different-id"
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, request)
	if err != nil || output.Code != "request_id_conflict" || output.Receipt != nil {
		t.Fatalf("replay = %#v, %v", output, err)
	}
}

func checkpointRequest(t *testing.T, requestID, batchID string, base *applyprogress.Snapshot, index int, entryID string, entries []applyprogress.EvidenceEntry) checkpointInput {
	t.Helper()
	return checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: []applyprogress.Task{{ID: "1", Text: "task"}}, Base: base, ExpectedGeneration: checkpointState(base).Generation, ExpectedRevision: checkpointState(base).Revision, ExpectedDigest: checkpointState(base).Digest, RequestID: requestID, BatchID: batchID, Entries: entries, EntryIndex: index, EntryID: entryID}
}

func checkpointEntry(id, task, summary string) applyprogress.EvidenceEntry {
	return applyprogress.EvidenceEntry{EntryID: id, TaskIDs: []string{task}, CompletesTaskIDs: []string{task}, Kind: applyprogress.EvidenceGreen, Summary: summary, Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}
}

func executeSddProgressCheckpoint(t *testing.T, cmd *cobra.Command, root string, request checkpointInput) (checkpointOutput, error) {
	t.Helper()
	var tasks strings.Builder
	for _, task := range request.Tasks {
		fmt.Fprintf(&tasks, "- [ ] %s %s\n", task.ID, task.Text)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(tasks.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	_, wantManifest, err := applyprogress.TaskManifest(request.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := applyprogress.ParseTasksMarkdown(tasks.String())
	if err != nil {
		t.Fatal(err)
	}
	_, gotManifest, err := applyprogress.TaskManifest(parsed.Tasks)
	if err != nil || wantManifest != gotManifest {
		t.Fatalf("test root manifest=%q request manifest=%q error=%v", gotManifest, wantManifest, err)
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "checkpoint.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"checkpoint", "--root", root, "--request", path})
	err = cmd.Execute()
	var output checkpointOutput
	if decodeErr := json.Unmarshal(stdout.Bytes(), &output); decodeErr != nil {
		t.Fatalf("decode command output %q: %v", stdout.String(), decodeErr)
	}
	return output, err
}

type configuredCheckpointBackend struct {
	current    *applyprogress.Snapshot
	calls      []sddprogress.AdvanceRequest
	seen       map[string]sddprogress.AdvanceRequest
	advanceErr error
}

func (b *configuredCheckpointBackend) Current() (*applyprogress.Snapshot, error) {
	return b.current, nil
}
func (b *configuredCheckpointBackend) CurrentSnapshot(sddprogress.AdvanceRequest) (*applyprogress.Snapshot, error) {
	return b.current, nil
}

func (b *configuredCheckpointBackend) Advance(request sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.calls = append(b.calls, request)
	if prior, found := b.seen[request.RequestID]; found && !reflect.DeepEqual(prior, request) {
		return sddprogress.AdvanceResult{}, sddprogress.ErrRequestConflict
	}
	if b.advanceErr != nil {
		return sddprogress.AdvanceResult{}, b.advanceErr
	}
	if b.seen == nil {
		b.seen = map[string]sddprogress.AdvanceRequest{}
	}
	b.seen[request.RequestID] = request
	b.current = &request.Snapshot
	return checkpointState(&request.Snapshot), nil
}

func TestConfiguredSddProgressCheckpointUsesHiveAndHybridStores(t *testing.T) {
	for _, mode := range []string{"hive", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			open, hive := &configuredCheckpointBackend{}, &configuredCheckpointBackend{}
			oldOpen, oldHive := newProgressOpenSpec, newProgressHive
			t.Cleanup(func() { newProgressOpenSpec, newProgressHive = oldOpen, oldHive })
			newProgressOpenSpec = func(string) progressAdvancer { return open }
			newProgressHive = func() (progressAdvancer, error) { return hive, nil }
			t.Setenv("JARVIS_SDD_STORE_MODE", mode)
			request := checkpointRequest(t, "checkpoint-"+mode, "apb-00000000000000000000000000000008", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})

			output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), root, request)
			if err != nil || output.Outcome != "committed" || !sameAdvanceCoordinates(output.State, checkpointState(output.Snapshot)) {
				t.Fatalf("checkpoint = %#v, %v", output, err)
			}
			if len(hive.calls) != 1 || hive.calls[0].RequestID != request.RequestID || hive.calls[0].Batches[0].BatchID != request.BatchID {
				t.Fatalf("hive request = %#v", hive.calls)
			}
			replay, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), root, request)
			if err != nil || replay.State != output.State {
				t.Fatalf("replay = %#v, %v", replay, err)
			}
			changed := request
			changed.Entries = append([]applyprogress.EvidenceEntry{}, request.Entries...)
			changed.Entries[0].Summary = "changed evidence"
			conflict, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), root, changed)
			if err != nil || conflict.Code != "request_id_conflict" {
				t.Fatalf("changed replay = %#v, %v", conflict, err)
			}
			if mode == "hybrid" && (len(open.calls) != 1 || len(hive.calls) != 1) {
				t.Fatalf("hybrid replay mutated durable acknowledged sides: calls=%d/%d", len(open.calls), len(hive.calls))
			}
		})
	}
}

func TestSddProgressCheckpointContinuationParityAcrossStores(t *testing.T) {
	for _, mode := range []string{"openspec", "hive", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			open, hive := &configuredCheckpointBackend{}, &configuredCheckpointBackend{}
			oldOpen, oldHive := newProgressOpenSpec, newProgressHive
			t.Cleanup(func() { newProgressOpenSpec, newProgressHive = oldOpen, oldHive })
			newProgressOpenSpec = func(string) progressAdvancer { return open }
			newProgressHive = func() (progressAdvancer, error) { return hive, nil }
			t.Setenv("JARVIS_SDD_STORE_MODE", mode)
			tasks := []applyprogress.Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}}
			entries := []applyprogress.EvidenceEntry{
				checkpointEntry("entry-1", "1", strings.Repeat("é", 32545)),
				checkpointEntry("entry-2", "2", strings.Repeat("é", 32545)),
			}
			first := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: tasks, RequestID: "parity-first-" + mode, BatchID: "apb-00000000000000000000000000000012", Entries: entries, EntryID: "entry-1"}
			output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), root, first)
			if err != nil || output.Outcome != "continuation_required" || output.NextEntryIndex != 1 || output.NextEntryID != "entry-2" || len(output.StreamSHA256) != 64 {
				t.Fatalf("first checkpoint = %#v, %v", output, err)
			}
			second := first
			second.Base, second.ExpectedGeneration, second.ExpectedRevision, second.ExpectedDigest, second.StreamSHA256 = output.Snapshot, output.State.Generation, output.State.Revision, output.State.Digest, output.StreamSHA256
			second.RequestID, second.BatchID, second.EntryIndex, second.EntryID = "parity-second-"+mode, "apb-00000000000000000000000000000013", output.NextEntryIndex, output.NextEntryID
			output, err = executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), root, second)
			if err != nil || output.Outcome != "committed" || output.Snapshot == nil || output.Snapshot.Generation != 1 {
				t.Fatalf("continuation = %#v, %v", output, err)
			}
			if mode == "openspec" && len(open.calls) != 2 || mode == "hive" && len(hive.calls) != 2 || mode == "hybrid" && (len(open.calls) != 2 || len(hive.calls) != 2) {
				t.Fatalf("store calls open=%d hive=%d", len(open.calls), len(hive.calls))
			}
		})
	}
}

func TestSddProgressCheckpointRecoversHybridReceiptBeforeDivergence(t *testing.T) {
	for name, hiveFirst := range map[string]bool{"openspec then hive": false, "hive then openspec": true} {
		t.Run(name, func(t *testing.T) {
			root, interrupted := t.TempDir(), errors.New("interrupted")
			open, hive := &configuredCheckpointBackend{}, &configuredCheckpointBackend{}
			if hiveFirst {
				open.advanceErr = interrupted
			} else {
				hive.advanceErr = interrupted
			}
			command := newSddProgressCommand(func(root string) progressAdvancer {
				return sddprogress.Hybrid{Root: root, OpenSpec: open, Hive: hive, HiveFirst: hiveFirst}
			})
			request := checkpointRequest(t, "recovery-"+strings.ReplaceAll(name, " ", "-"), "apb-00000000000000000000000000000009", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
			if _, err := executeSddProgressCheckpoint(t, command, root, request); !errors.Is(err, sddprogress.ErrBackendDiverged) {
				t.Fatalf("initial error = %v", err)
			}
			open.advanceErr, hive.advanceErr = nil, nil
			output, err := executeSddProgressCheckpoint(t, command, root, request)
			if err != nil || output.Outcome != "committed" {
				t.Fatalf("replay = %#v, %v", output, err)
			}
			if len(open.calls) != 2 || len(hive.calls) != 2 {
				t.Fatalf("recovery calls = %d/%d, want both sides reconciled", len(open.calls), len(hive.calls))
			}
		})
	}
}

func TestSddProgressCheckpointBlocksDivergentCompletedHybridReceipt(t *testing.T) {
	root := newProgressTestRoot(t)
	open, hive := &configuredCheckpointBackend{}, &configuredCheckpointBackend{}
	command := newSddProgressCommand(func(root string) progressAdvancer {
		return sddprogress.Hybrid{Root: root, OpenSpec: open, Hive: hive}
	})
	request := checkpointRequest(t, "completed-divergence", "apb-00000000000000000000000000000011", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	if output, err := executeSddProgressCheckpoint(t, command, root, request); err != nil || output.Outcome != "committed" {
		t.Fatalf("commit = %#v, %v", output, err)
	}
	hive.current = nil
	output, err := executeSddProgressCheckpoint(t, command, root, request)
	if !errors.Is(err, sddprogress.ErrBackendDiverged) || output.Code != "backend_diverged" || len(open.calls) != 1 || len(hive.calls) != 1 {
		t.Fatalf("divergence = %#v, %v; calls=%d/%d", output, err, len(open.calls), len(hive.calls))
	}
}

func TestConfiguredSddProgressCheckpointImportsLegacyHiveArtifacts(t *testing.T) {
	posts := 0
	legacyID, err := applyprogress.LegacyTaskID("1.1", "imported")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/artifacts") {
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 imported\n- [ ] 1.2 pending\n"},{"artifact":"apply-progress","content":"status: partial\n"}]}`))
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
			return
		}
		posts++
		var request hiveclient.ApplyProgressAdvanceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Snapshot.Coverage) != 1 || request.Snapshot.Coverage[0].TaskID != legacyID || request.Snapshot.StreamSHA256 == "" || len(request.Batches) != 1 || len(request.Batches[0].Entries) != 1 || request.Batches[0].Entries[0].Kind != applyprogress.EvidenceImported {
			t.Fatalf("legacy Hive advance = %#v", request)
		}
		if got, want := request.LegacySourceSHA256, applyprogress.LegacySourceSHA256([]byte("status: partial\n")); got != want {
			t.Fatalf("legacy Hive source binding = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(hiveclient.ApplyProgressResult{Outcome: "committed", State: hiveclient.ApplyProgressState{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, Snapshot: request.Snapshot}})
	}))
	defer server.Close()
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	oldHive := newProgressHive
	t.Cleanup(func() { newProgressHive = oldHive })
	newProgressHive = func() (progressAdvancer, error) { return hiveProgressAdvancer{client: client}, nil }
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")
	request := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: []applyprogress.Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "pending"}}, RequestID: "hive-legacy-import", BatchID: "apb-00000000000000000000000000000072", Entries: []applyprogress.EvidenceEntry{checkpointEntry("entry-2", "1.2", "new evidence")}, EntryID: "entry-2"}
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), t.TempDir(), request)
	if err != nil || output.Outcome != "continuation_required" || output.Code != "legacy_imported" || output.Snapshot == nil || posts != 1 {
		t.Fatalf("legacy Hive checkpoint = %#v, %v; posts=%d", output, err, posts)
	}
}

func TestConfiguredSddProgressCheckpointContinuesFromGuardedV2HeadAfterPartialLegacyImport(t *testing.T) {
	var imported applyprogress.Snapshot
	var events []string
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/apply-progress"):
			events = append(events, "current")
			if posts == 0 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(hiveclient.ApplyProgressResult{Outcome: "current", Code: "ok", State: hiveclient.ApplyProgressState{Generation: imported.Generation, Revision: imported.Revision, Digest: imported.Digest, Snapshot: imported}})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/artifacts"):
			events = append(events, "artifacts")
			// The exact-topic projection remains after migration; it must not be
			// treated as a fresh legacy authority once guarded v2 state exists.
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 imported\n- [ ] 1.2 pending\n"},{"artifact":"apply-progress","content":"status: partial\n"}]}`))
		default:
			posts++
			var request hiveclient.ApplyProgressAdvanceRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			imported = request.Snapshot
			_ = json.NewEncoder(w).Encode(hiveclient.ApplyProgressResult{Outcome: "committed", State: hiveclient.ApplyProgressState{Generation: imported.Generation, Revision: imported.Revision, Digest: imported.Digest, Snapshot: imported}})
		}
	}))
	t.Cleanup(server.Close)
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := hiveProgressAdvancer{client: client}
	entries := []applyprogress.EvidenceEntry{checkpointEntry("entry-2", "1.2", "new evidence")}
	first := checkpointInput{Project: "jarvis-dev", Change: "issue-653", Tasks: []applyprogress.Task{{ID: "1.1", Text: "imported"}, {ID: "1.2", Text: "pending"}}, RequestID: "hive-legacy-first", BatchID: "apb-00000000000000000000000000000074", Entries: entries, EntryID: "entry-2"}
	firstOutput, err := runSddProgressCheckpoint(store, first)
	if err != nil || firstOutput.Outcome != "continuation_required" || posts != 1 {
		t.Fatalf("first checkpoint = %#v, %v; posts=%d", firstOutput, err, posts)
	}
	second := first
	second.Base = &imported
	second.ExpectedGeneration, second.ExpectedRevision, second.ExpectedDigest = imported.Generation, imported.Revision, imported.Digest
	second.RequestID, second.BatchID = "hive-legacy-second", "apb-00000000000000000000000000000075"
	second.StreamSHA256, second.EntryIndex, second.EntryID = imported.StreamSHA256, imported.NextEntryIndex, imported.NextEntryID
	secondOutput, err := runSddProgressCheckpoint(store, second)
	if err != nil || secondOutput.Outcome != "committed" || posts != 2 {
		t.Fatalf("second checkpoint = %#v, %v; posts=%d", secondOutput, err, posts)
	}
	if len(events) != 4 || events[0] != "current" || events[1] != "artifacts" || events[2] != "current" || events[3] != "artifacts" {
		t.Fatalf("guarded state must be queried before each legacy artifact projection: events=%v", events)
	}
}

func TestConfiguredSddProgressCheckpointReportsAmbiguousLegacyHiveArtifacts(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/apply-progress") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/artifacts") {
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1 task\n"},{"artifact":"apply-progress","content":"status: complete\n"},{"artifact":"apply-progress","content":"status: complete\n"}]}`))
			return
		}
		posts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	oldHive := newProgressHive
	t.Cleanup(func() { newProgressHive = oldHive })
	newProgressHive = func() (progressAdvancer, error) { return hiveProgressAdvancer{client: client}, nil }
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")
	request := checkpointRequest(t, "hive-legacy-ambiguous", "apb-00000000000000000000000000000073", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), t.TempDir(), request)
	if !errors.Is(err, sddprogress.ErrLegacyMigration) || output.Outcome != "invalid" || output.Code != "legacy_migration" || posts != 0 {
		t.Fatalf("ambiguous legacy Hive checkpoint = %#v, %v; posts=%d", output, err, posts)
	}
}

func TestConfiguredSddProgressCheckpointTreatsHiveNotFoundAsInitial(t *testing.T) {
	artifactFetches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/artifacts") {
			artifactFetches++
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1 task\n"}]}`))
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
			return
		}
		var request hiveclient.ApplyProgressAdvanceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(hiveclient.ApplyProgressResult{Outcome: "committed", State: hiveclient.ApplyProgressState{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, Snapshot: request.Snapshot}})
	}))
	defer server.Close()
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	oldHive := newProgressHive
	t.Cleanup(func() { newProgressHive = oldHive })
	newProgressHive = func() (progressAdvancer, error) { return hiveProgressAdvancer{client: client}, nil }
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")
	request := checkpointRequest(t, "hive-initial", "apb-00000000000000000000000000000010", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(configuredProgressStore), t.TempDir(), request)
	if err != nil || output.Outcome != "committed" || output.State.Generation != 1 || artifactFetches != 1 {
		t.Fatalf("initial hive checkpoint = %#v, %v; task artifact fetches=%d", output, err, artifactFetches)
	}
}

func TestSddProgressCapacityOutputsIncludeStructuredDocumentBounds(t *testing.T) {
	capacity := &applyprogress.CapacityError{Document: "snapshot", Runes: applyprogress.MaxDocumentRunes + 7}
	cases := []struct {
		name  string
		value any
	}{
		{name: "advance", value: func() any {
			output, _ := runSddProgressAdvance(failedProgressAdvancer{capacity}, "", sddprogress.AdvanceRequest{})
			return output
		}()},
		{name: "checkpoint", value: func() any {
			output, _ := checkpointStoreErrorOutput(capacity, sddprogress.AdvanceResult{}, sddprogress.AdvanceResult{})
			return output
		}()},
		{name: "continuation", value: func() any {
			output, _ := checkpointStoreErrorOutput(capacity, sddprogress.AdvanceResult{}, sddprogress.AdvanceResult{})
			return output
		}()},
		{name: "hive", value: func() any {
			output, _ := runSddProgressAdvance(failedProgressAdvancer{&hiveclient.ApplyProgressError{StatusCode: http.StatusRequestEntityTooLarge, Result: hiveclient.ApplyProgressResult{Outcome: "invalid", Code: "capacity", Capacity: &hiveclient.ApplyProgressCapacity{Document: capacity.Document, Runes: capacity.Runes, Limit: applyprogress.MaxDocumentRunes}}}}, "", sddprogress.AdvanceRequest{})
			return output
		}()},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			capacityBody, ok := body["capacity"].(map[string]any)
			if !ok || capacityBody["document"] != "snapshot" || capacityBody["runes"] != float64(applyprogress.MaxDocumentRunes+7) || capacityBody["limit"] != float64(applyprogress.MaxDocumentRunes) {
				t.Fatalf("capacity JSON = %s", data)
			}
		})
	}
}

func TestCheckpointStoreErrorOutputPreservesDaemonValidationDetailAndCapacity(t *testing.T) {
	daemon := &hiveclient.ApplyProgressError{StatusCode: http.StatusUnprocessableEntity, Result: hiveclient.ApplyProgressResult{
		Outcome: "invalid", Code: "validation", Detail: "snapshot.next_entry_id", Recovery: "repair the cursor",
		Capacity: &hiveclient.ApplyProgressCapacity{Document: "snapshot", Runes: applyprogress.MaxDocumentRunes + 3, Limit: 39000},
	}}
	output, handled := checkpointStoreErrorOutput(daemon, sddprogress.AdvanceResult{}, sddprogress.AdvanceResult{})
	if handled || output.Detail != daemon.Result.Detail || output.Capacity == nil || output.Capacity.Document != daemon.Result.Capacity.Document || output.Capacity.Runes != daemon.Result.Capacity.Runes || output.Capacity.Limit != daemon.Result.Capacity.Limit {
		t.Fatalf("daemon output = %#v, handled=%t; want preserved detail and capacity", output, handled)
	}
}

func TestHiveProgressAdvancerPreservesRemoteCapacityLimit(t *testing.T) {
	const limit = 40000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/apply-progress/advance") {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte(`{"outcome":"invalid","code":"capacity","capacity":{"document":"evidence batch","runes":40017,"limit":40000}}`))
	}))
	t.Cleanup(server.Close)
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = (hiveProgressAdvancer{client: client}).Advance(sddprogress.AdvanceRequest{RequestID: "capacity"})
	var capacity *applyprogress.CapacityError
	if !errors.As(err, &capacity) || capacity.Document != "evidence batch" || capacity.Runes != 40017 || capacity.Limit != limit {
		t.Fatalf("Advance() error = %#v, want structured remote capacity", err)
	}
}

func TestSddProgressAdvanceReturnsHandledRemoteOutcomesAtExitZero(t *testing.T) {
	request := progressRequest(t, "handled-remote", "apb-00000000000000000000000000000039", 1, "")
	for _, code := range []string{"request_id_conflict", "batch_collision", "capacity"} {
		t.Run(code, func(t *testing.T) {
			output, err := runSddProgressAdvance(failedProgressAdvancer{&hiveclient.ApplyProgressError{StatusCode: http.StatusConflict, Result: hiveclient.ApplyProgressResult{Outcome: "conflict", Code: code}}}, "", request)
			if err != nil || output.Code != code {
				t.Fatalf("output = %#v, err = %v", output, err)
			}
		})
	}
}

func TestSddProgressAdvancePrioritizesBackendDivergenceAndCapacity(t *testing.T) {
	request := progressRequest(t, "mapped-local-error", "apb-00000000000000000000000000000040", 1, "")
	for name, failure := range map[string]error{
		"partially committed stale": fmt.Errorf("%w: %w", sddprogress.ErrBackendDiverged, sddprogress.ErrConflict),
		"capacity":                  &applyprogress.CapacityError{Document: "snapshot", Runes: applyprogress.MaxDocumentRunes + 1},
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runSddProgressAdvance(failedProgressAdvancer{failure}, "", request)
			if name == "partially committed stale" && (err == nil || output.Outcome != "blocked" || output.Code != "backend_diverged") {
				t.Fatalf("output = %#v, want backend divergence", output)
			}
			if name == "capacity" && (err != nil || output.Outcome != "invalid" || output.Code != "capacity") {
				t.Fatalf("output = %#v, err = %v; want handled capacity", output, err)
			}
		})
	}
}

func TestCapacityWarningOutputIncludesThresholdAndPreflightGuidance(t *testing.T) {
	warning := capacityWarningOutputFor(&applyprogress.CapacityWarning{
		Document: "snapshot", Runes: 32000, Threshold: 32000, Limit: applyprogress.MaxDocumentRunes, Remaining: 8000,
	})
	if warning == nil || warning.Threshold != 32000 || warning.Remaining != 8000 || !strings.Contains(warning.Guidance, "consolidate evidence") {
		t.Fatalf("capacity warning output = %#v, want threshold, remaining runes, and consolidation guidance", warning)
	}
	data, err := json.Marshal(warning)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["threshold"] != float64(32000) || wire["remaining"] != float64(8000) {
		t.Fatalf("capacity warning JSON = %s, want threshold and remaining fields", data)
	}
}

func progressSuccessor(t *testing.T, prior, next sddprogress.AdvanceRequest) sddprogress.AdvanceRequest {
	t.Helper()
	next.Batches[0].Entries[0].EntryID = "entry-" + next.Batches[0].BatchID[4:]
	var err error
	next.Batches[0], _, err = applyprogress.SealBatch(next.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	next.Snapshot.Batches[0].SHA256 = next.Batches[0].SHA256
	next.Snapshot.Batches = append(append([]applyprogress.BatchRef{}, prior.Snapshot.Batches...), next.Snapshot.Batches...)
	next.Snapshot, _, err = applyprogress.SealSnapshot(next.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func progressRequest(t *testing.T, requestID, batchID string, generation uint64, previous string) sddprogress.AdvanceRequest {
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
	if generation > 1 {
		expectedGeneration = epoch
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: epoch, Revision: generation, PreviousDigest: previous,
		TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sddprogress.AdvanceRequest{RequestID: requestID, ExpectedGeneration: expectedGeneration, ExpectedRevision: generation - 1, ExpectedDigest: previous, Batches: []applyprogress.Batch{batch}, Snapshot: snapshot}
}
