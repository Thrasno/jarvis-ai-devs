package main

import (
	"bytes"
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

func TestConfiguredSddProgressAdvanceUpgradesLegacyInHybridMode(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hive := &commandProgressBackend{}
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

func TestSddProgressCheckpointCommitsInitialStream(t *testing.T) {
	root := t.TempDir()
	request := checkpointRequest(t, "checkpoint-1", "apb-00000000000000000000000000000001", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "evidence")})
	output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(defaultOpenSpec), root, request)
	if err != nil || output.Outcome != "committed" || output.State.Generation != 1 {
		t.Fatalf("checkpoint = %#v, %v; want initial committed state", output, err)
	}
}

type checkpointBackend struct {
	current      *applyprogress.Snapshot
	calls        int
	currentCalls int
}

func (b *checkpointBackend) Current() (*applyprogress.Snapshot, error) {
	b.currentCalls++
	return b.current, nil
}
func (b *checkpointBackend) Advance(sddprogress.AdvanceRequest) (sddprogress.AdvanceResult, error) {
	b.calls++
	return sddprogress.AdvanceResult{}, errors.New("unexpected advance")
}

func TestSddProgressCheckpointCapacityDoesNotAdvance(t *testing.T) {
	for name, test := range map[string]struct {
		request checkpointInput
		outcome string
	}{
		"evidence item": {checkpointRequest(t, "capacity-entry", "apb-00000000000000000000000000000002", nil, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", strings.Repeat("x", applyprogress.MaxDocumentRunes))}), "evidence_item_too_large"},
		"snapshot":      {checkpointSnapshotCapacityRequest(t), "snapshot_capacity_exhausted"},
	} {
		t.Run(name, func(t *testing.T) {
			request := test.request
			backend := &checkpointBackend{current: request.Base}
			output, err := executeSddProgressCheckpoint(t, newSddProgressCommand(func(string) progressAdvancer { return backend }), t.TempDir(), request)
			if err != nil || backend.calls != 0 || output.Outcome != test.outcome || output.Code != test.outcome {
				t.Fatalf("capacity = %#v, %v; calls=%d", output, err, backend.calls)
			}
		})
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
			if !errors.Is(err, applyprogress.ErrInvalidValue) || output.Outcome != "invalid" || backend.currentCalls != 0 || backend.calls != 0 {
				t.Fatalf("changed continuation = %#v, %v; backend resolve/advance calls=%d/%d", output, err, backend.currentCalls, backend.calls)
			}
		})
	}
}

func TestSddProgressCheckpointDurablyContinues162725Runes(t *testing.T) {
	root := t.TempDir()
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
		if err != nil || output.Snapshot == nil || output.Receipt == nil || output.Receipt.RequestID != request.RequestID || output.State != checkpointState(output.Snapshot) {
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

func checkpointSnapshotCapacityRequest(t *testing.T) checkpointInput {
	t.Helper()
	tasks := []applyprogress.Task{{ID: "1", Text: "task"}}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	base := applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}}
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
	return checkpointRequest(t, "capacity-snapshot", "apb-00000000000000000000000000000003", &base, 0, "entry-1", []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1", "fits")})
}

func TestSddProgressCheckpointRejectsStaleAndChangedRequestID(t *testing.T) {
	root := t.TempDir()
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
	if err != nil || output.Code != "request_id_conflict" {
		t.Fatalf("changed ID = %#v, %v", output, err)
	}
}

func TestSddProgressCheckpointRejectsDifferentIDCandidateReplay(t *testing.T) {
	root := t.TempDir()
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
			if err != nil || output.Outcome != "committed" || output.State != checkpointState(output.Snapshot) {
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
				t.Fatalf("hybrid rewrote committed side: calls=%d/%d", len(open.calls), len(hive.calls))
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
			if err != nil || output.Outcome != "committed" || output.Snapshot == nil || output.Snapshot.Generation != 2 {
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
			if hiveFirst && (len(hive.calls) != 1 || len(open.calls) != 2) {
				t.Fatalf("hive-first calls = %d/%d", len(open.calls), len(hive.calls))
			}
			if !hiveFirst && (len(open.calls) != 1 || len(hive.calls) != 2) {
				t.Fatalf("openspec-first calls = %d/%d", len(open.calls), len(hive.calls))
			}
		})
	}
}

func TestSddProgressCheckpointBlocksDivergentCompletedHybridReceipt(t *testing.T) {
	root := t.TempDir()
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

func TestConfiguredSddProgressCheckpointTreatsHiveNotFoundAsInitial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"outcome":"invalid","code":"validation"}`))
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
	if err != nil || output.Outcome != "committed" || output.State.Generation != 1 {
		t.Fatalf("initial hive checkpoint = %#v, %v", output, err)
	}
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
