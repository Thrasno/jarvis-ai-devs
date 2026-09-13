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
