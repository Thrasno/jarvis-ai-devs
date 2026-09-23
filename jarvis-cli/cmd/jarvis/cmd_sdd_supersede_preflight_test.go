package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

const preflightOldTasks = "- [ ] 1.1 task\n- [ ] 1.2 remaining\n"
const preflightNewTasks = "- [ ] 1.1 revised\n- [ ] 1.2 remaining\n"

func preflightEvidence(t *testing.T) (applyprogress.Snapshot, applyprogress.Batch) {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "issue-653", BatchID: "apb-00000000000000000000000000000001",
		Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}, {ID: "1.2", Text: "remaining"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1,
		TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial,
		Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "entry"}},
		Batches:  []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, batch
}

func preflightOpenSpec(t *testing.T, mode sddruntime.StoreMode) (string, string, applyprogress.Snapshot, applyprogress.Batch) {
	t.Helper()
	workspace := canonicalSddTestPath(t, t.TempDir())
	root := filepath.Join(workspace, "openspec", "changes", "issue-653")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	tasks := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasks, []byte(preflightOldTasks), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, batch := preflightEvidence(t)
	// OpenSpec requires a cursor into the signed evidence stream for a partial head.
	next := batch.Entries[0]
	next.EntryID = "entry-next"
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{batch.Entries[0], next})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.StreamSHA256, snapshot.NextEntryIndex, snapshot.NextEntryID = stream, 1, next.EntryID
	snapshot, _, err = applyprogress.SealSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (sddprogress.OpenSpec{Root: root}).Advance(sddprogress.AdvanceRequest{
		RequestID: "preflight-request", ExpectedRevision: 0, Batches: []applyprogress.Batch{batch}, Snapshot: snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := sddbinding.New(mode, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sddbinding.AdoptOpenSpec(root, binding); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasks, []byte(preflightNewTasks), 0600); err != nil {
		t.Fatal(err)
	}
	return workspace, root, snapshot, batch
}

func preflightClient(t *testing.T, mode string, snapshot applyprogress.Snapshot, batch applyprogress.Batch, tasks string, occupied bool, requests *[]string, later ...applyprogress.Snapshot) *hiveclient.Client {
	t.Helper()
	progressReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation: %s", r.Method)
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		write := func(value any) {
			t.Helper()
			if err := json.NewEncoder(w).Encode(value); err != nil {
				t.Error(err)
			}
		}
		switch r.URL.Path {
		case "/sdd/changes/issue-653/store-binding":
			if mode == "" {
				w.WriteHeader(404)
				write(map[string]string{"code": "not_found"})
				return
			}
			write(map[string]any{"binding": map[string]any{"schema_version": "1", "project": "jarvis-dev", "change": "issue-653", "mode": mode, "provenance": "cli", "created_at": "2026-08-01T10:00:00Z"}})
		case "/sdd/changes/issue-653/apply-progress":
			progressReads++
			head := snapshot
			if progressReads > 1 && len(later) != 0 {
				head = later[0]
			}
			// GET wire contract: hive-daemon/internal/httpapi/server.go:460.
			write(map[string]any{"outcome": "committed", "code": "ok", "state": hiveclient.ApplyProgressState{Generation: head.Generation, Revision: head.Revision, Digest: head.Digest, Snapshot: head}})
		case "/sdd/changes/issue-653/apply-evidence/" + batch.BatchID:
			write(map[string]any{"batch": batch})
		case "/sdd/changes/issue-653/artifacts":
			write(map[string]any{"artifacts": []map[string]string{{"artifact": "tasks", "content": tasks, "created_at": "2026-08-01T10:00:01Z"}}})
		case "/sdd/changes/next/successor-occupancy":
			category := ""
			if occupied {
				category = "head"
			}
			write(map[string]any{"occupied": occupied, "category": category})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewSupersessionPreflightOpenSpec(t *testing.T) {
	workspace, root, snapshot, batch := preflightOpenSpec(t, sddruntime.StoreModeOpenSpec)
	var requests []string
	client := preflightClient(t, "", snapshot, batch, preflightNewTasks, false, &requests)
	stateBefore, err := os.ReadFile(filepath.Join(root, "state.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	headBefore, err := os.ReadFile(filepath.Join(root, "apply-progress.md"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != sddruntime.StoreModeOpenSpec || result.Predecessor.Digest != snapshot.Digest || result.TaskManifest == snapshot.TaskManifestSHA256 || result.TasksContent != preflightNewTasks {
		t.Fatalf("result = %+v", result)
	}
	target := filepath.Join(workspace, "openspec", "changes", "next")
	for _, kind := range []string{"directory", "file"} {
		if kind == "directory" {
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(target, []byte("occupied"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", client); err == nil {
			t.Fatalf("accepted occupied %s", kind)
		}
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(preflightOldTasks), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", client); err == nil {
		t.Fatal("accepted historical tasks")
	}
	for _, path := range []string{filepath.Join(root, "state.yaml"), filepath.Join(root, "apply-progress.md")} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := stateBefore
		if strings.HasSuffix(path, "apply-progress.md") {
			want = headBefore
		}
		if string(got) != string(want) {
			t.Fatalf("mutated %s", path)
		}
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("created successor: %v", err)
	}
	receipts, err := os.ReadDir(filepath.Join(root, ".apply-progress-receipts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("preflight changed receipt count: %d", len(receipts))
	}
	for _, request := range requests {
		if !strings.HasPrefix(request, "GET ") {
			t.Fatalf("mutation: %s", request)
		}
	}
}

func TestNewSupersessionPreflightRejectsUnsafeIdentity(t *testing.T) {
	var requests []string
	client := preflightClient(t, "", applyprogress.Snapshot{}, applyprogress.Batch{}, "", false, &requests)
	workspace := canonicalSddTestPath(t, t.TempDir())
	for _, path := range []string{workspace + "/.", filepath.Join(workspace, "absent"), "relative"} {
		if _, err := newSupersessionPreflight(context.Background(), path, "jarvis-dev", "issue-653", "next", client); err == nil {
			t.Fatalf("accepted workspace %q", path)
		}
	}
	if _, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "../unsafe", "next", client); err == nil {
		t.Fatal("accepted unsafe predecessor")
	}
	if _, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", client); err == nil {
		t.Fatal("accepted missing binding")
	}
	if len(requests) != 1 || requests[0] != "GET /sdd/changes/issue-653/store-binding" {
		t.Fatalf("unexpected reads or writes: %v", requests)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("created binding path: %v", err)
	}
}

func TestNewSupersessionPreflightRejectsHiveHeadChangingDuringTasksRead(t *testing.T) {
	workspace := canonicalSddTestPath(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(workspace, "openspec", "changes", "issue-653"), 0700); err != nil {
		t.Fatal(err)
	}
	snapshot, batch := preflightEvidence(t)
	foreign := snapshot
	foreign.Project = "foreign"
	var err error
	foreign, _, err = applyprogress.SealSnapshot(foreign)
	if err != nil {
		t.Fatal(err)
	}
	var requests []string
	client := preflightClient(t, "hive", snapshot, batch, preflightNewTasks, false, &requests, foreign)
	if _, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", client); err == nil {
		t.Fatal("accepted foreign head on revised tasks read")
	}
	progressReads := 0
	for _, request := range requests {
		if request == "GET /sdd/changes/issue-653/apply-progress" {
			progressReads++
		}
		if !strings.HasPrefix(request, "GET ") {
			t.Fatalf("mutation: %s", request)
		}
	}
	if progressReads != 2 {
		t.Fatalf("wanted initial and tasks-read heads, got %v", requests)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "openspec", "changes", "next")); !os.IsNotExist(err) {
		t.Fatalf("created target: %v", err)
	}
}

func TestNewSupersessionPreflightHiveAndHybrid(t *testing.T) {
	workspace, _, snapshot, batch := preflightOpenSpec(t, sddruntime.StoreModeHybrid)
	for _, tc := range []struct {
		name, mode, tasks   string
		occupied, wantError bool
		head                applyprogress.Snapshot
	}{
		{name: "matching hybrid", mode: "hybrid", tasks: preflightNewTasks, head: snapshot},
		{name: "divergent hybrid tasks", mode: "hybrid", tasks: "- [ ] 1.1 other\n- [ ] 1.2 remaining\n", head: snapshot, wantError: true},
		{name: "occupied hybrid", mode: "hybrid", tasks: preflightNewTasks, head: snapshot, occupied: true, wantError: true},
		{name: "completed revised task", mode: "hybrid", tasks: "- [x] 1.1 revised\n- [ ] 1.2 remaining\n", head: snapshot, wantError: true},
		{name: "missing Hive tasks", mode: "hybrid", tasks: "", head: snapshot, wantError: true},
		{name: "divergent hybrid head", mode: "hybrid", tasks: preflightNewTasks, head: func() applyprogress.Snapshot {
			copy := snapshot
			copy.Revision++
			copy, _, _ = applyprogress.SealSnapshot(copy)
			return copy
		}(), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			client := preflightClient(t, tc.mode, tc.head, batch, tc.tasks, tc.occupied, &requests)
			result, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", client)
			if (err != nil) != tc.wantError {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			for _, r := range requests {
				if !strings.HasPrefix(r, "GET ") {
					t.Fatalf("mutation: %s", r)
				}
			}
		})
	}
	// One-sided OpenSpec occupancy blocks Hybrid before the Hive occupancy GET.
	target := filepath.Join(workspace, "openspec", "changes", "next")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	var occupiedRequests []string
	occupiedClient := preflightClient(t, "hybrid", snapshot, batch, preflightNewTasks, false, &occupiedRequests)
	if _, err := newSupersessionPreflight(context.Background(), workspace, "jarvis-dev", "issue-653", "next", occupiedClient); err == nil {
		t.Fatal("accepted OpenSpec-only occupancy")
	}
	for _, request := range occupiedRequests {
		if strings.Contains(request, "successor-occupancy") {
			t.Fatalf("queried Hive after occupied OpenSpec target: %v", occupiedRequests)
		}
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	// Hive-only has no local binding or OpenSpec predecessor.
	hiveWorkspace := canonicalSddTestPath(t, t.TempDir())
	for _, occupied := range []bool{false, true} {
		var requests []string
		client := preflightClient(t, "hive", snapshot, batch, preflightNewTasks, occupied, &requests)
		result, err := newSupersessionPreflight(context.Background(), hiveWorkspace, "jarvis-dev", "issue-653", "next", client)
		if (err != nil) != occupied {
			t.Fatalf("occupied=%v result=%+v err=%v", occupied, result, err)
		}
		for _, r := range requests {
			if !strings.HasPrefix(r, "GET ") {
				t.Fatalf("mutation: %s", r)
			}
		}
	}
}
