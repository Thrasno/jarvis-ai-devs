package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
)

func TestSddProgressAdvanceReportsConflictCurrentState(t *testing.T) {
	root := t.TempDir()
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
}

func TestSddProgressAdvanceRecoversInterruptedRequestOnce(t *testing.T) {
	root := t.TempDir()
	first := progressRequest(t, "request-first", "apb-00000000000000000000000000000001", 1, "")
	if _, err := (sddprogress.OpenSpec{Root: root}).Advance(first); err != nil {
		t.Fatal(err)
	}
	pending := progressRequest(t, "request-pending", "apb-00000000000000000000000000000002", 2, first.Snapshot.Digest)
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

func TestSddProgressAdvanceRejectsChangedRequestPayload(t *testing.T) {
	root := t.TempDir()
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

func progressLegacyRequest(t *testing.T) sddprogress.AdvanceRequest {
	t.Helper()
	converted, err := applyprogress.ConvertLegacy(applyprogress.LegacyProgress{Tasks: []applyprogress.Task{{Path: "1.1", Text: "task"}}, Completed: []string{"1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	r := progressRequest(t, "legacy", "apb-00000000000000000000000000000001", 1, "")
	r.Batches[0].Entries[0].TaskIDs, r.Batches[0].Entries[0].CompletesTaskIDs = []string{converted.Tasks[0].ID}, []string{converted.Tasks[0].ID}
	r.Batches[0], _, err = applyprogress.SealBatch(r.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	r.Snapshot.TaskManifestSHA256, r.Snapshot.Status, r.Snapshot.Coverage = converted.TaskManifestSHA256, applyprogress.StatusComplete, []applyprogress.Coverage{{TaskID: converted.Tasks[0].ID, BatchID: r.Batches[0].BatchID, EntryID: "entry"}}
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
}

func (b *commandProgressBackend) Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.calls++
	return b.state, b.err
}

func (b *commandProgressBackend) Current(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.currentCalls++
	return b.current, nil
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
	if err != nil || out.Outcome != "committed" || open.calls != 1 || hive.calls != 2 {
		t.Fatalf("replay = %#v, %v; calls=%d/%d", out, err, open.calls, hive.calls)
	}
	changed := progressRequest(t, request.RequestID, "apb-00000000000000000000000000000002", 1, "")
	out, err = executeSddProgressAdvance(t, command, root, changed)
	if err != nil || out.Code != "request_id_conflict" || open.calls != 1 || hive.calls != 2 {
		t.Fatalf("conflict = %#v, %v; calls=%d/%d", out, err, open.calls, hive.calls)
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
			if err != nil || out.Outcome != "conflict" || out.State != hive.state {
				t.Fatalf("out=%#v err=%v", out, err)
			}
			if hybrid && hive.calls != 1 {
				t.Fatalf("hive calls=%d", hive.calls)
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
	if err != nil || out.Outcome != "conflict" || out.State != hive.current {
		t.Fatalf("out=%#v err=%v", out, err)
	}
	if open.calls != 1 || hive.calls != 0 || hive.currentCalls != 1 {
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

func progressRequest(t *testing.T, requestID, batchID string, generation uint64, previous string) sddprogress.AdvanceRequest {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "issue-653", BatchID: batchID,
		Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: generation, Revision: generation, PreviousDigest: previous,
		TaskManifestSHA256: "0000000000000000000000000000000000000000000000000000000000000000", Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sddprogress.AdvanceRequest{RequestID: requestID, ExpectedGeneration: generation - 1, ExpectedRevision: generation - 1, ExpectedDigest: previous, Batches: []applyprogress.Batch{batch}, Snapshot: snapshot}
}
