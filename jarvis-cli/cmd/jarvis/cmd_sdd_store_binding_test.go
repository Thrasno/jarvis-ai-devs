package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func canonicalSddTestWorkspace(t *testing.T) string {
	t.Helper()

	workspace, err := canonicalSddWorkspaceDirectory(t.TempDir())
	if err != nil {
		t.Fatalf("canonicalize test workspace: %v", err)
	}
	return workspace
}

func crossCommandAdvanceRequest(t *testing.T, project, change, requestID string) sddprogress.AdvanceRequest {
	t.Helper()
	request := progressRequest(t, requestID, "apb-00000000000000000000000000000723", 1, "")
	request.Snapshot.Project, request.Snapshot.Change = project, change
	request.Batches[0].Project, request.Batches[0].Change = project, change
	batch, _, err := applyprogress.SealBatch(request.Batches[0])
	if err != nil {
		t.Fatalf("seal cross-command batch: %v", err)
	}
	request.Batches[0] = batch
	request.Snapshot.Batches[0].SHA256 = batch.SHA256
	snapshot, _, err := applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatalf("seal cross-command snapshot: %v", err)
	}
	request.Snapshot = snapshot
	return request
}

func requireCrossCommandAdvanceRequest(t *testing.T, r *http.Request, project, change string, request sddprogress.AdvanceRequest) {
	t.Helper()
	expected, err := json.Marshal(hiveclient.ApplyProgressAdvanceRequest{
		Project: request.Snapshot.Project, Change: request.Snapshot.Change, RequestID: request.RequestID,
		ExpectedGeneration: request.ExpectedGeneration, ExpectedRevision: request.ExpectedRevision, ExpectedDigest: request.ExpectedDigest,
		LegacySourceSHA256: request.LegacySourceSHA256, Snapshot: request.Snapshot, Batches: request.Batches,
	})
	if err != nil {
		t.Fatalf("marshal expected Hive progress request: %v", err)
	}
	requireSDDRequest(t, r, http.MethodPost, "/sdd/changes/"+change+"/apply-progress/advance", "", string(expected))
	if request.Snapshot.Project != project || request.Snapshot.Change != change {
		t.Fatalf("test request coordinates = %s/%s, want %s/%s", request.Snapshot.Project, request.Snapshot.Change, project, change)
	}
}

func crossCommandHybridReceiptPayload(t *testing.T, root, requestID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".apply-progress-hybrid-receipts", requestID+".json"))
	if err != nil {
		t.Fatalf("read durable hybrid receipt: %v", err)
	}
	var receipt struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil || receipt.Payload == "" {
		t.Fatalf("decode durable hybrid receipt = %s, err=%v", data, err)
	}
	return receipt.Payload
}

func crossCommandCommittedHiveResult(t *testing.T, root string, request sddprogress.AdvanceRequest) hiveclient.ApplyProgressResult {
	t.Helper()
	return hiveclient.ApplyProgressResult{
		Outcome: "committed",
		State: hiveclient.ApplyProgressState{
			Generation: request.Snapshot.Generation,
			Revision:   request.Snapshot.Revision,
			Digest:     request.Snapshot.Digest,
			Snapshot:   request.Snapshot,
			Batches:    request.Batches,
		},
		Receipt: hiveclient.ApplyProgressReceipt{
			RequestID:     request.RequestID,
			PayloadSHA256: crossCommandHybridReceiptPayload(t, root, request.RequestID),
		},
	}
}

func crossCommandMismatchedPayload(payload string) string {
	if strings.HasPrefix(payload, "0") {
		return "1" + payload[1:]
	}
	return "0" + payload[1:]
}

func TestCrossCommandBoundSddProgressSurvivesEnvironmentRestart(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)

	for _, tt := range []struct {
		name                string
		initialMode         string
		changedMode         string
		wantFirstErr        bool
		wantSecondErr       bool
		wantBinding         string
		wantAdopts          int
		wantPosts           int
		wantArtifacts       int
		wantProgress        int
		hivePayloadMismatch bool
	}{
		{name: "Hive", initialMode: "hive", changedMode: "invalid", wantBinding: "hive", wantAdopts: 1, wantPosts: 1, wantArtifacts: 1, wantProgress: 1},
		{name: "OpenSpec", initialMode: "openspec", changedMode: "hive", wantBinding: "openspec", wantAdopts: 0, wantPosts: 0, wantArtifacts: 1, wantProgress: 1},
		{name: "Hybrid receipt recovery", initialMode: "hybrid", changedMode: "invalid", wantFirstErr: true, wantBinding: "hybrid", wantAdopts: 1, wantPosts: 2, wantArtifacts: 2, wantProgress: 1},
		{name: "Hybrid blocks mismatched Hive acknowledgement", initialMode: "hybrid", changedMode: "invalid", wantFirstErr: true, wantSecondErr: true, wantBinding: "hybrid", wantAdopts: 1, wantPosts: 2, wantArtifacts: 2, wantProgress: 1, hivePayloadMismatch: true},
		{name: "None", initialMode: "none", changedMode: "invalid", wantFirstErr: true, wantSecondErr: true, wantBinding: "", wantAdopts: 0, wantPosts: 0, wantArtifacts: 2, wantProgress: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			root := filepath.Join(workspace, "openspec", "changes", change)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(workspace)

			request := crossCommandAdvanceRequest(t, project, change, "cross-command-"+strings.ToLower(strings.ReplaceAll(tt.name, " ", "-")))
			requestData, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			requestPath := filepath.Join(workspace, "request.json")
			if err := os.WriteFile(requestPath, requestData, 0o600); err != nil {
				t.Fatal(err)
			}

			var backend struct {
				binding      string
				adoptions    int
				advancePosts int
				artifacts    int
				progress     int
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/" + change + "/store-binding":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					if backend.binding == "" {
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
						return
					}
					_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":"initial:environment:JARVIS_SDD_STORE_MODE","created_at":"2026-08-01T10:00:00Z"}}`, project, change, backend.binding)
				case "/sdd/changes/" + change + "/store-binding/adopt":
					requireSDDRequest(t, r, http.MethodPost, r.URL.Path, "", fmt.Sprintf(`{"project":%q,"mode":%q,"provenance":"initial:environment:JARVIS_SDD_STORE_MODE"}`, project, tt.initialMode))
					if backend.binding != "" || tt.wantBinding == "openspec" || tt.wantBinding == "" {
						t.Fatalf("unexpected Hive binding adoption for %s", tt.name)
					}
					backend.binding = tt.wantBinding
					backend.adoptions++
					w.WriteHeader(http.StatusCreated)
					_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":"initial:environment:JARVIS_SDD_STORE_MODE","created_at":"2026-08-01T10:00:00Z"},"created":true}`, project, change, backend.binding)
				case "/sdd/changes/" + change + "/artifacts":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					backend.artifacts++
					_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`)
				case "/sdd/changes/" + change + "/apply-progress":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					backend.progress++
					_, _ = fmt.Fprint(w, `{}`)
				case "/sdd/changes/" + change + "/apply-progress/advance":
					requireCrossCommandAdvanceRequest(t, r, project, change, request)
					backend.advancePosts++
					if tt.wantBinding == "hybrid" && backend.advancePosts == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = fmt.Fprint(w, `{"outcome":"unavailable","code":"offline"}`)
						return
					}
					if tt.wantBinding == "hybrid" {
						result := crossCommandCommittedHiveResult(t, root, request)
						if tt.hivePayloadMismatch {
							result.Receipt.PayloadSHA256 = crossCommandMismatchedPayload(result.Receipt.PayloadSHA256)
						}
						if err := json.NewEncoder(w).Encode(result); err != nil {
							t.Fatalf("encode Hive progress result: %v", err)
						}
						return
					}
					_, _ = fmt.Fprint(w, `{"outcome":"committed"}`)
				default:
					t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			t.Setenv("JARVIS_SDD_STORE_MODE", tt.initialMode)

			firstRoot := root
			if tt.wantBinding == "hive" || tt.wantBinding == "" {
				firstRoot = ""
			}
			if tt.wantBinding == "hybrid" {
				first := newBoundSddProgressCommand(resolveBoundProgressStore)
				first.SetOut(&bytes.Buffer{})
				first.SetErr(&bytes.Buffer{})
				first.SetArgs([]string{"advance", "--root", root, "--request", requestPath})
				if err := first.Execute(); (err != nil) != tt.wantFirstErr {
					t.Fatalf("first command error = %v, want error=%t", err, tt.wantFirstErr)
				}
				first = nil // The recovery invocation must construct a fresh command and Hive client.
			} else {
				if _, err := resolveBoundProgressStore(context.Background(), firstRoot, project, change); (err != nil) != tt.wantFirstErr {
					t.Fatalf("first resolution error = %v, want error=%t", err, tt.wantFirstErr)
				}
				// The first factory result is intentionally discarded before constructing
				// the second command, so its client cannot be reused in this test.
			}

			beforeFilesystem := snapshotBoundArchiveFilesystem(t, workspace)
			beforeBackend := backend
			t.Setenv("JARVIS_SDD_STORE_MODE", tt.changedMode)
			second := newBoundSddProgressCommand(resolveBoundProgressStore)
			second.SetOut(&bytes.Buffer{})
			second.SetErr(&bytes.Buffer{})
			args := []string{"advance", "--request", requestPath}
			if tt.wantBinding == "openspec" || tt.wantBinding == "hybrid" {
				args = append([]string{"advance", "--root", root}, args[1:]...)
			}
			second.SetArgs(args)
			err = second.Execute()
			if (err != nil) != tt.wantSecondErr {
				t.Fatalf("second command error = %v, want error=%t", err, tt.wantSecondErr)
			}

			wantBackendBinding := tt.wantBinding
			if wantBackendBinding == "openspec" {
				wantBackendBinding = ""
			}
			if backend.binding != wantBackendBinding || backend.adoptions != tt.wantAdopts || backend.advancePosts != tt.wantPosts || backend.artifacts != tt.wantArtifacts || backend.progress != tt.wantProgress {
				t.Fatalf("request classes = binding:%q adopts:%d posts:%d artifacts:%d progress:%d, want binding:%q adopts:%d posts:%d artifacts:%d progress:%d", backend.binding, backend.adoptions, backend.advancePosts, backend.artifacts, backend.progress, wantBackendBinding, tt.wantAdopts, tt.wantPosts, tt.wantArtifacts, tt.wantProgress)
			}

			switch tt.wantBinding {
			case "hive", "":
				if after := snapshotBoundArchiveFilesystem(t, workspace); after != beforeFilesystem {
					t.Fatalf("non-OpenSpec filesystem changed:\nwant %q\n got %q", beforeFilesystem, after)
				}
			case "openspec":
				if backend.advancePosts != beforeBackend.advancePosts || backend.adoptions != beforeBackend.adoptions || backend.binding != beforeBackend.binding {
					t.Fatalf("OpenSpec-bound restart mutated Hive backend: before=%#v after=%#v", beforeBackend, backend)
				}
			case "hybrid":
				receiptDir := filepath.Join(root, ".apply-progress-hybrid-receipts")
				receipts, err := os.ReadDir(receiptDir)
				if err != nil || len(receipts) != 1 {
					t.Fatalf("hybrid receipts = %v, err=%v; want exactly the recovered receipt", receipts, err)
				}
				data, err := os.ReadFile(filepath.Join(receiptDir, receipts[0].Name()))
				if err != nil {
					t.Fatal(err)
				}
				var receipt struct {
					Payload     string `json:"payload"`
					OpenSpec    string `json:"openspec"`
					Hive        string `json:"hive"`
					OpenSpecAck *struct {
						Generation uint64 `json:"generation"`
						Revision   uint64 `json:"revision"`
						Digest     string `json:"digest"`
						Payload    string `json:"payload"`
					} `json:"openspec_ack"`
					HiveAck *struct {
						Generation uint64 `json:"generation"`
						Revision   uint64 `json:"revision"`
						Digest     string `json:"digest"`
						Payload    string `json:"payload"`
					} `json:"hive_ack"`
				}
				if err := json.Unmarshal(data, &receipt); err != nil || receipt.Payload == "" || receipt.OpenSpec != "committed" || receipt.OpenSpecAck == nil {
					t.Fatalf("recovered receipt = %s, err=%v", data, err)
				}
				if receipt.OpenSpecAck.Generation != request.Snapshot.Generation || receipt.OpenSpecAck.Revision != request.Snapshot.Revision || receipt.OpenSpecAck.Digest != request.Snapshot.Digest || receipt.OpenSpecAck.Payload != receipt.Payload {
					t.Fatalf("OpenSpec acknowledgement = %#v, want request generation=%d revision=%d digest=%q payload=%q", receipt.OpenSpecAck, request.Snapshot.Generation, request.Snapshot.Revision, request.Snapshot.Digest, receipt.Payload)
				}
				if tt.hivePayloadMismatch {
					if receipt.Hive != "failed" || receipt.HiveAck != nil {
						t.Fatalf("mismatched Hive acknowledgement did not block recovery: receipt=%s", data)
					}
					break
				}
				if receipt.Hive != "committed" || receipt.HiveAck == nil {
					t.Fatalf("recovered receipt = %s, want committed Hive acknowledgement", data)
				}
				if receipt.HiveAck.Generation != request.Snapshot.Generation || receipt.HiveAck.Revision != request.Snapshot.Revision || receipt.HiveAck.Digest != request.Snapshot.Digest || receipt.HiveAck.Payload != receipt.Payload {
					t.Fatalf("Hive acknowledgement = %#v, want request generation=%d revision=%d digest=%q payload=%q", receipt.HiveAck, request.Snapshot.Generation, request.Snapshot.Revision, request.Snapshot.Digest, receipt.Payload)
				}
			}
			if tt.wantBinding == "openspec" && reflect.DeepEqual(beforeBackend, backend) == false {
				t.Fatalf("OpenSpec-bound backend snapshot changed: before=%#v after=%#v", beforeBackend, backend)
			}
		})
	}
}

func snapshotBoundArchiveFilesystem(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := rel + "|" + info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += "|" + string(data)
		}
		entries = append(entries, entry)
		return nil
	}); err != nil {
		t.Fatalf("snapshot filesystem: %v", err)
	}
	return strings.Join(entries, "\n")
}

func TestBoundSddArchiveHivePersistedBindingIgnoresEnvironmentWithoutFilesystemMutation(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)
	workspace := canonicalSddTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(workspace, "sentinel.txt"), []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotBoundArchiveFilesystem(t, workspace)
	t.Chdir(workspace)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/" + change + "/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":"hive","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change)
		case "/sdd/changes/" + change + "/artifacts":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"proposal","content":"# Proposal"},{"artifact":"spec","content":"# Spec"},{"artifact":"design","content":"# Design"},{"artifact":"tasks","content":"- [x] 1.1 task\n"},{"artifact":"apply-progress","content":"status: complete\n"},{"artifact":"verify-report","content":"## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"},{"artifact":"archive-report","content":"# Archive report\n"}]}`)
		case "/sdd/changes/" + change + "/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"outcome":"unavailable","code":"compatibility"}`)
		default:
			if r.Method == http.MethodPost {
				posts++
			}
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "invalid")

	for attempt := 1; attempt <= 2; attempt++ {
		command := newBoundSddArchiveCommand()
		command.SetArgs([]string{"--project", project, "--change", change})
		if err := command.Execute(); err != nil {
			t.Fatalf("Hive logical closure attempt %d: %v", attempt, err)
		}
	}
	if posts != 0 {
		t.Fatalf("POST requests = %d, want 0", posts)
	}
	if after := snapshotBoundArchiveFilesystem(t, workspace); after != before {
		t.Fatalf("filesystem changed during Hive logical archive:\nwant %q\n got %q", before, after)
	}
}

func TestBoundSddArchiveUnboundAdoptsBindingBeforeBlockingLifecycleMutation(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)
	workspace := canonicalSddTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(workspace, "sentinel.txt"), []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotBoundArchiveFilesystem(t, workspace)
	t.Chdir(workspace)

	bindingPosts, unexpectedPosts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/" + change + "/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
		case "/sdd/changes/" + change + "/store-binding/adopt":
			if r.Method != http.MethodPost || r.URL.RawQuery != "" {
				t.Fatalf("binding adoption request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			}
			var request struct {
				Project    string `json:"project"`
				Mode       string `json:"mode"`
				Provenance string `json:"provenance"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode binding adoption: %v", err)
			}
			if request.Project != project || request.Mode != "hive" || request.Provenance != "initial:environment:JARVIS_SDD_STORE_MODE" {
				t.Fatalf("binding adoption = %#v", request)
			}
			bindingPosts++
			_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":"hive","provenance":%q,"created_at":"2026-08-01T10:00:00Z"}}`, project, change, request.Provenance)
		case "/sdd/changes/" + change + "/artifacts":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprint(w, `{"artifacts":[]}`)
		case "/sdd/changes/" + change + "/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprint(w, `{}`)
		default:
			if r.Method == http.MethodPost {
				unexpectedPosts++
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")

	command := newBoundSddArchiveCommand()
	command.SetArgs([]string{"--project", project, "--change", change})
	if err := command.Execute(); err == nil {
		t.Fatal("unready unbound archive succeeded")
	}
	if bindingPosts != 1 || unexpectedPosts != 0 {
		t.Fatalf("POST requests = binding adoption %d, lifecycle/other %d; want 1/0", bindingPosts, unexpectedPosts)
	}
	if after := snapshotBoundArchiveFilesystem(t, workspace); after != before {
		t.Fatalf("filesystem changed while archive was blocked:\nwant %q\n got %q", before, after)
	}
}

func TestBoundSddArchiveHiveBlocksMissingBlankOrUnreadyClosure(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)
	blankReport := " \n\t"
	validReport := "# Archive report\n"
	for _, tt := range []struct {
		name          string
		archiveReport *string
		tasks         string
	}{
		{name: "missing archive report", tasks: "- [x] 1.1 task\n"},
		{name: "blank archive report", archiveReport: &blankReport, tasks: "- [x] 1.1 task\n"},
		{name: "incomplete readiness", archiveReport: &validReport, tasks: "- [ ] 1.1 task\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			t.Chdir(workspace)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/" + change + "/store-binding":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":"hive","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change)
				case "/sdd/changes/" + change + "/artifacts":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					artifacts := []map[string]string{
						{"artifact": "proposal", "content": "# Proposal"},
						{"artifact": "spec", "content": "# Spec"},
						{"artifact": "design", "content": "# Design"},
						{"artifact": "tasks", "content": tt.tasks},
						{"artifact": "apply-progress", "content": "status: complete\n"},
						{"artifact": "verify-report", "content": "## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"},
					}
					if tt.archiveReport != nil {
						artifacts = append(artifacts, map[string]string{"artifact": "archive-report", "content": *tt.archiveReport})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": artifacts})
				case "/sdd/changes/" + change + "/apply-progress":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"outcome":"unavailable","code":"compatibility"}`)
				default:
					t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")

			command := newBoundSddArchiveCommand()
			command.SetArgs([]string{"--project", project, "--change", change})
			if err := command.Execute(); err == nil {
				t.Fatal("Hive archive succeeded without a complete logical closure")
			}
			if _, err := os.Stat(filepath.Join(workspace, "openspec")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("OpenSpec path error = %v, want no filesystem mutation", err)
			}
		})
	}
}

func writeBoundArchiveReadyOpenSpec(t *testing.T, workspace, change, archiveReport string) (string, sddprogress.AdvanceRequest) {
	t.Helper()
	root := filepath.Join(workspace, "openspec", "changes", change)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := progressRequest(t, "bound-archive", "apb-00000000000000000000000000000001", 1, "")
	request.Snapshot.Project, request.Snapshot.Change = "jarvis-dev", change
	request.Batches[0].Project, request.Batches[0].Change = "jarvis-dev", change
	request.Batches[0].Entries[0].TaskIDs = []string{"1.1"}
	request.Batches[0].Entries[0].CompletesTaskIDs = []string{"1.1"}
	var err error
	request.Batches[0], _, err = applyprogress.SealBatch(request.Batches[0])
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	request.Snapshot.Status = applyprogress.StatusComplete
	request.Snapshot.TaskManifestSHA256 = manifest
	request.Snapshot.Coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: request.Batches[0].BatchID, EntryID: "entry"}}
	request.Snapshot.Batches[0].SHA256 = request.Batches[0].SHA256
	request.Snapshot, _, err = applyprogress.SealSnapshot(request.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (sddprogress.OpenSpec{Root: root}).Advance(request); err != nil {
		t.Fatalf("persist complete OpenSpec progress: %v", err)
	}
	files := map[string]string{
		"proposal.md":      "# Proposal\n",
		"spec.md":          "# Spec\n",
		"design.md":        "# Design\n",
		"verify-report.md": "## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n",
		filepath.Join("specs", "delivery", "spec.md"): "# Delivery Delta\n",
	}
	if archiveReport != "" {
		files["archive-report.md"] = archiveReport
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, request
}

func TestBoundSddArchiveOpenSpecPersistedBindingIgnoresHiveEnvironmentAndRenames(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-653"
	)
	workspace := canonicalSddTestWorkspace(t)
	root, _ := writeBoundArchiveReadyOpenSpec(t, workspace, change, "# Archive report\n")
	writeOpenSpecBinding(t, workspace, change, "openspec", "persisted:test", nil)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/"+change+"/store-binding", "project="+project, "")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")

	destination := filepath.Join(workspace, "openspec", "archive", change)
	command := newBoundSddArchiveCommand()
	command.SetArgs([]string{"--root", root, "--destination", destination, "--project", project})
	if err := command.Execute(); err != nil {
		t.Fatalf("archive OpenSpec binding: %v", err)
	}
	if requests != 1 {
		t.Fatalf("binding requests = %d, want 1", requests)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active OpenSpec root error = %v, want renamed", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "archive-report.md")); err != nil {
		t.Fatalf("archived report: %v", err)
	}
}

func TestBoundSddArchiveHybridRequiresMatchingReportsBeforeRename(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-653"
	)
	for _, tt := range []struct {
		name         string
		localReport  string
		hiveReport   string
		artifactCode int
		wantSuccess  bool
	}{
		{name: "equal", localReport: "# Archive report\n", hiveReport: "# Archive report\n", wantSuccess: true},
		{name: "different", localReport: "# Archive report\n", hiveReport: "# Different report\n"},
		{name: "one sided", localReport: "", hiveReport: "# Archive report\n"},
		{name: "Hive error", localReport: "# Archive report\n", artifactCode: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			root, request := writeBoundArchiveReadyOpenSpec(t, workspace, change, tt.localReport)
			writeOpenSpecBinding(t, workspace, change, "hybrid", "persisted:test", nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/" + change + "/store-binding":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":"hybrid","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change)
				case "/sdd/changes/" + change + "/artifacts":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					if tt.artifactCode != 0 {
						w.WriteHeader(tt.artifactCode)
						return
					}
					_, _ = fmt.Fprintf(w, `{"artifacts":[{"artifact":"proposal","content":"# Proposal\n"},{"artifact":"spec","content":"# Spec\n"},{"artifact":"design","content":"# Design\n"},{"artifact":"tasks","content":"- [x] 1.1 task\n"},{"artifact":"verify-report","content":"## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"},{"artifact":"archive-report","content":%q}]}`, tt.hiveReport)
				case "/sdd/changes/" + change + "/apply-progress":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					_ = json.NewEncoder(w).Encode(map[string]any{"outcome": "committed", "state": map[string]any{"generation": request.Snapshot.Generation, "revision": request.Snapshot.Revision, "digest": request.Snapshot.Digest, "snapshot": request.Snapshot, "batches": request.Batches}})
				default:
					t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			t.Setenv("JARVIS_SDD_STORE_MODE", "invalid")

			destination := filepath.Join(workspace, "openspec", "archive", change)
			command := newBoundSddArchiveCommand()
			command.SetArgs([]string{"--root", root, "--destination", destination, "--project", project})
			err := command.Execute()
			if (err == nil) != tt.wantSuccess {
				t.Fatalf("archive error = %v, want success=%t", err, tt.wantSuccess)
			}
			if tt.wantSuccess {
				if _, err := os.Stat(filepath.Join(destination, "archive-report.md")); err != nil {
					t.Fatalf("hybrid archive destination: %v", err)
				}
				return
			}
			if _, err := os.Stat(root); err != nil {
				t.Fatalf("source moved after blocked hybrid archive: %v", err)
			}
		})
	}
}

func TestBoundSddArchiveHybridUsesOnlyComparedViews(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-653"
	)
	workspace := canonicalSddTestWorkspace(t)
	root, request := writeBoundArchiveReadyOpenSpec(t, workspace, change, "# Archive report\n")
	writeOpenSpecBinding(t, workspace, change, "hybrid", "persisted:test", nil)

	hiveArtifactReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/" + change + "/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":"hybrid","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change)
		case "/sdd/changes/" + change + "/artifacts":
			hiveArtifactReads++
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"proposal","content":"# Proposal\n"},{"artifact":"spec","content":"# Spec\n"},{"artifact":"design","content":"# Design\n"},{"artifact":"tasks","content":"- [x] 1.1 task\n"},{"artifact":"verify-report","content":"## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"},{"artifact":"archive-report","content":"# Archive report\n"}]}`)
		case "/sdd/changes/" + change + "/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_ = json.NewEncoder(w).Encode(map[string]any{"outcome": "committed", "state": map[string]any{"generation": request.Snapshot.Generation, "revision": request.Snapshot.Revision, "digest": request.Snapshot.Digest, "snapshot": request.Snapshot, "batches": request.Batches}})
		default:
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)

	destination := filepath.Join(workspace, "openspec", "archive", change)
	command := newBoundSddArchiveCommand()
	command.SetArgs([]string{"--root", root, "--destination", destination, "--project", project})
	if err := command.Execute(); err != nil {
		t.Fatalf("archive hybrid binding: %v", err)
	}
	if hiveArtifactReads != 2 {
		t.Fatalf("Hive artifact reads = %d, want 2 for prevalidation and locked callback only", hiveArtifactReads)
	}
}

func TestBoundSddArchiveHybridBlocksReportMutationInLockedCallback(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-653"
	)
	workspace := canonicalSddTestWorkspace(t)
	root, request := writeBoundArchiveReadyOpenSpec(t, workspace, change, "# Archive report\n")
	writeOpenSpecBinding(t, workspace, change, "hybrid", "persisted:test", nil)

	hiveArtifactReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/" + change + "/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":"hybrid","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change)
		case "/sdd/changes/" + change + "/artifacts":
			hiveArtifactReads++
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			report := "# Archive report\n"
			if hiveArtifactReads > 1 {
				report = "# Changed archive report\n"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"artifacts": []map[string]string{
				{"artifact": "proposal", "content": "# Proposal\n"},
				{"artifact": "spec", "content": "# Spec\n"},
				{"artifact": "design", "content": "# Design\n"},
				{"artifact": "tasks", "content": "- [x] 1.1 task\n"},
				{"artifact": "verify-report", "content": "## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"},
				{"artifact": "archive-report", "content": report},
			}})
		case "/sdd/changes/" + change + "/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
			_ = json.NewEncoder(w).Encode(map[string]any{"outcome": "committed", "state": map[string]any{"generation": request.Snapshot.Generation, "revision": request.Snapshot.Revision, "digest": request.Snapshot.Digest, "snapshot": request.Snapshot, "batches": request.Batches}})
		default:
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)

	destination := filepath.Join(workspace, "openspec", "archive", change)
	command := newBoundSddArchiveCommand()
	command.SetArgs([]string{"--root", root, "--destination", destination, "--project", project})
	if err := command.Execute(); err == nil {
		t.Fatal("archive succeeded after Hive report changed in locked callback")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("source changed after callback rejection: %v", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination changed after callback rejection: %v", err)
	}
}

func TestResolveBoundSddArchiveCoordinatesRejectsNonCanonicalRootSpellings(t *testing.T) {
	workspace := canonicalSddTestWorkspace(t)
	root := filepath.Join(workspace, "openspec", "changes", "issue-653")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)

	for _, root := range []string{
		root + string(filepath.Separator) + ".",
		root + string(filepath.Separator) + ".." + string(filepath.Separator) + "issue-653",
		"openspec/changes/issue-653",
	} {
		if _, err := resolveBoundSddArchiveCoordinates(root, "jarvis-dev", ""); err == nil {
			t.Fatalf("accepted non-canonical root spelling %q", root)
		}
	}
}

func TestBoundSddArchiveRejectsNoneAndNonCanonicalCoordinatesBeforeEffects(t *testing.T) {
	workspace := canonicalSddTestWorkspace(t)
	root := filepath.Join(workspace, "openspec", "changes", "issue-653")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "whitespace change", args: []string{"--change", " issue-653 ", "--project", "jarvis-dev"}},
		{name: "whitespace root", args: []string{"--root", " " + root + " ", "--destination", filepath.Join(workspace, "archive"), "--project", "jarvis-dev"}},
		{name: "root change mismatch", args: []string{"--root", root, "--change", "issue-654", "--destination", filepath.Join(workspace, "archive"), "--project", "jarvis-dev"}},
		{name: "non canonical project", args: []string{"--root", root, "--destination", filepath.Join(workspace, "archive"), "--project", "Jarvis-Dev"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := newBoundSddArchiveCommand()
			command.SetArgs(tt.args)
			if err := command.Execute(); err == nil {
				t.Fatal("archive accepted invalid coordinates")
			}
		})
	}

	t.Chdir(workspace)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/issue-653/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=jarvis-dev", "")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
		case "/sdd/changes/issue-653/artifacts":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=jarvis-dev", "")
			_, _ = fmt.Fprint(w, `{"artifacts":[]}`)
		case "/sdd/changes/issue-653/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=jarvis-dev", "")
			_, _ = fmt.Fprint(w, `{}`)
		default:
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "none")
	command := newBoundSddArchiveCommand()
	command.SetArgs([]string{"--change", "issue-653", "--project", "jarvis-dev"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "binding is none") {
		t.Fatalf("none archive error = %v", err)
	}
}

func TestResolveBoundProgressStoreSelectsPersistedBindingBeforeEnvironment(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)
	tests := []struct {
		name        string
		mode        string
		localMode   string
		environment string
		wantOpen    int
		wantHive    int
	}{
		{name: "Hive ignores invalid environment", mode: "hive", environment: "invalid", wantHive: 1},
		{name: "OpenSpec ignores Hive environment", localMode: "openspec", environment: "hive", wantOpen: 1},
		{name: "Hybrid ignores OpenSpec environment", mode: "hybrid", localMode: "hybrid", environment: "openspec", wantOpen: 1, wantHive: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			root := filepath.Join(workspace, "openspec", "changes", change)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.localMode != "" {
				writeOpenSpecBinding(t, workspace, change, tt.localMode, "persisted:test", nil)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/"+change+"/store-binding", "project="+project, "")
				if tt.mode == "" {
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
					return
				}
				_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change, tt.mode)
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			t.Setenv("JARVIS_SDD_STORE_MODE", tt.environment)

			open, hive := 0, 0
			oldOpen, oldHiveWithContext := newProgressOpenSpec, newProgressHiveWithContext
			t.Cleanup(func() { newProgressOpenSpec, newProgressHiveWithContext = oldOpen, oldHiveWithContext })
			newProgressOpenSpec = func(string) progressAdvancer { open++; return &commandProgressBackend{} }
			newProgressHiveWithContext = func(context.Context) (progressAdvancer, error) { hive++; return &commandProgressBackend{}, nil }

			store, err := resolveBoundProgressStore(context.Background(), root, project, change)
			if err != nil {
				t.Fatalf("resolve bound store: %v", err)
			}
			if store == nil || open != tt.wantOpen || hive != tt.wantHive {
				t.Fatalf("store=%T open=%d hive=%d, want %d/%d", store, open, hive, tt.wantOpen, tt.wantHive)
			}
		})
	}
}

func TestResolveBoundProgressStoreNoneDoesNotPersistOrCreateOpenSpec(t *testing.T) {
	workspace := canonicalSddTestWorkspace(t)
	postRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/issue-723/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=jarvis-dev", "")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
		case "/sdd/changes/issue-723/artifacts":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=jarvis-dev", "")
			_, _ = fmt.Fprint(w, `{"artifacts":[]}`)
		case "/sdd/changes/issue-723/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=jarvis-dev", "")
			// A legacy daemon's successful non-v2 payload leaves protected progress
			// absent when the general artifact endpoint also has no projection.
			_, _ = fmt.Fprint(w, `{}`)
		default:
			if r.Method == http.MethodPost {
				postRequests++
			}
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "none")
	t.Chdir(workspace)

	_, err := resolveBoundProgressStore(context.Background(), "", "jarvis-dev", "issue-723")
	if err == nil || err.Error() != "SDD progress store is disabled" {
		t.Fatalf("resolve none error = %v", err)
	}
	if postRequests != 0 {
		t.Fatalf("binding POST requests = %d, want 0", postRequests)
	}
	if _, err := os.Stat(filepath.Join(workspace, "openspec")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenSpec directory error = %v, want absence", err)
	}
}

func TestResolveBoundProgressStoreFailsBeforeWritesForInvalidRootOrBinding(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)
	for _, tt := range []struct {
		name    string
		change  string
		binding string
	}{
		{name: "spaced request coordinate", change: " issue-723 "},
		{name: "root coordinate mismatch", change: "other-change"},
		{name: "unsupported binding protocol", change: change, binding: "none"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			root := filepath.Join(workspace, "openspec", "changes", change)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if tt.binding == "" {
					t.Fatalf("unexpected request before root validation: %s %s", r.Method, r.URL.Path)
				}
				requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/"+change+"/store-binding", "project="+project, "")
				_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change, tt.binding)
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)

			_, err := resolveBoundProgressStore(context.Background(), root, project, tt.change)
			if err == nil {
				t.Fatal("resolve bound store succeeded")
			}
			if tt.binding == "" && requests != 0 {
				t.Fatalf("requests = %d, want 0 before coordinate validation", requests)
			}
			if tt.binding != "" && requests != 1 {
				t.Fatalf("requests = %d, want only binding GET", requests)
			}
		})
	}
}

func TestResolveBoundProgressStoreBindingReadFailureDoesNotAdopt(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-723"
	)
	workspace := canonicalSddTestWorkspace(t)
	root := filepath.Join(workspace, "openspec", "changes", change)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
			t.Fatalf("unexpected write after binding read failure: %s %s", r.Method, r.URL.Path)
		}
		requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/"+change+"/store-binding", "project="+project, "")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"code":"unavailable"}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")

	_, err := resolveBoundProgressStore(context.Background(), root, project, change)
	if err == nil {
		t.Fatal("resolve store succeeded after binding read failure")
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want 0", writes)
	}
}

func TestProgressStoreForBindingRejectsNone(t *testing.T) {
	if _, err := progressStoreForBinding(sddruntime.StoreModeNone, canonicalSddTestWorkspace(t)); err == nil {
		t.Fatal("none store resolved without error")
	}
}

func TestResolveBoundProgressStoreRejectsNonCanonicalProjectAndRootBeforeBinding(t *testing.T) {
	const (
		canonicalProject = "jarvis-dev"
		change           = "issue-723"
	)
	nonCanonicalProject := "Jarvis-Dev"
	if !applyprogress.ValidID(nonCanonicalProject) || hiveclient.CanonicalProjectKey(nonCanonicalProject) == nonCanonicalProject {
		t.Fatalf("test setup project %q must be a valid non-canonical identifier", nonCanonicalProject)
	}

	workspace := canonicalSddTestWorkspace(t)
	root := filepath.Join(workspace, "openspec", "changes", change)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("unexpected request before canonical coordinate validation: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)

	for _, tt := range []struct {
		name    string
		root    string
		project string
	}{
		{name: "canonical project spelling", root: root, project: nonCanonicalProject},
		{name: "whitespace root", root: "   ", project: canonicalProject},
		{name: "surrounded root", root: " " + root + " ", project: canonicalProject},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveBoundProgressStore(context.Background(), tt.root, tt.project, change)
			if err == nil {
				t.Fatal("resolved non-canonical input")
			}
		})
	}
	if requests != 0 {
		t.Fatalf("binding requests = %d, want 0", requests)
	}
}

func TestBoundSddProgressWriterCancellationUsesCommandContextForHiveAndHybrid(t *testing.T) {
	const (
		project = "jarvis-dev"
		change  = "issue-653"
	)
	for _, mode := range []string{"hive", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			root := filepath.Join(workspace, "openspec", "changes", change)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if mode == "hybrid" {
				binding, err := sddbinding.New(sddruntime.StoreModeHybrid, "persisted:test")
				if err != nil {
					t.Fatal(err)
				}
				if _, adopted, err := sddbinding.AdoptOpenSpec(root, binding); err != nil || !adopted {
					t.Fatalf("persist OpenSpec hybrid binding: adopted=%t err=%v", adopted, err)
				}
			}

			var requests struct {
				sync.Mutex
				bindingGETs         int
				artifactGETs        int
				writerPOSTs         int
				unexpected          int
				late                int
				writerBeforeStore   int
				artifactBeforeStore int
				storeBuilt          bool
				returned            bool
			}
			writerStarted := make(chan struct{})
			var writerStartedOnce sync.Once
			signalWriterStarted := func() { writerStartedOnce.Do(func() { close(writerStarted) }) }
			releaseWriter := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseWriter) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Lock()
				late, storeBuilt := requests.returned, requests.storeBuilt
				requests.Unlock()
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/sdd/changes/"+change+"/store-binding":
					requests.Lock()
					requests.bindingGETs++
					if late {
						requests.late++
					}
					requests.Unlock()
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, project, change, mode)
				case r.Method == http.MethodGet && r.URL.Path == "/sdd/changes/"+change+"/artifacts":
					requests.Lock()
					requests.artifactGETs++
					if late {
						requests.late++
					}
					if !storeBuilt {
						requests.artifactBeforeStore++
					}
					requests.Unlock()
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project="+project, "")
					_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`)
				case r.Method == http.MethodPost && r.URL.Path == "/sdd/changes/"+change+"/apply-progress/advance":
					requests.Lock()
					requests.writerPOSTs++
					if late {
						requests.late++
					}
					if !storeBuilt {
						requests.writerBeforeStore++
					}
					requests.Unlock()
					signalWriterStarted()
					<-releaseWriter
				default:
					requests.Lock()
					requests.unexpected++
					if late {
						requests.late++
					}
					requests.Unlock()
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			t.Cleanup(release)
			t.Setenv("HIVE_DAEMON_URL", server.URL)

			request := progressRequest(t, "context-"+mode, "apb-00000000000000000000000000000099", 1, "")
			data, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			requestPath := filepath.Join(workspace, "request.json")
			if err := os.WriteFile(requestPath, data, 0o600); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cancelerDone := make(chan struct{})
			go func() {
				defer close(cancelerDone)
				select {
				case <-writerStarted:
					cancel()
				case <-ctx.Done():
				}
			}()
			resolve := func(ctx context.Context, root, project, change string) (progressAdvancer, error) {
				store, err := resolveBoundProgressStore(ctx, root, project, change)
				if err == nil {
					requests.Lock()
					requests.storeBuilt = true
					requests.Unlock()
				}
				return store, err
			}
			command := newBoundSddProgressCommand(resolve)
			command.SetContext(ctx)
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs([]string{"advance", "--root", root, "--request", requestPath})
			err = command.Execute()
			requests.Lock()
			requests.returned = true
			requests.Unlock()
			select {
			case <-writerStarted:
			default:
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					t.Fatal("writer did not begin before the watchdog deadline")
				}
				t.Fatal("writer adapter was not reached")
			}
			<-cancelerDone
			if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("writer error = %v, watchdog deadline exceeded", err)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("writer error = %v, want context canceled", err)
			}
			release()
			server.Close()
			requests.Lock()
			bindingGETs, artifactGETs, writerPOSTs, unexpected, late := requests.bindingGETs, requests.artifactGETs, requests.writerPOSTs, requests.unexpected, requests.late
			writerBeforeStore, artifactBeforeStore := requests.writerBeforeStore, requests.artifactBeforeStore
			requests.Unlock()
			wantArtifactGETs := 0
			if mode == "hybrid" {
				wantArtifactGETs = 1
			}
			if bindingGETs != 1 || artifactGETs != wantArtifactGETs || writerPOSTs != 1 || unexpected != 0 || late != 0 || writerBeforeStore != 0 || artifactBeforeStore != 0 {
				t.Fatalf("requests after command return: binding GETs=%d artifact GETs=%d writer POSTs=%d unexpected=%d late=%d writer-before-store=%d artifact-before-store=%d, want 1/%d/1/0/0/0/0", bindingGETs, artifactGETs, writerPOSTs, unexpected, late, writerBeforeStore, artifactBeforeStore, wantArtifactGETs)
			}
		})
	}
}
