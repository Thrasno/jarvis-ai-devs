package sddstatus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestHiveSourceFetchArtifactsUsesDedicatedCompleteRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/governance/memories" {
			t.Fatal("HiveSource must not use generic governance memories")
		}
		if r.URL.EscapedPath() != "/sdd/changes/epic-06/artifacts" {
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
		if got := r.URL.Query().Get("project"); got != "jarvis-dev" {
			t.Fatalf("project query = %q", got)
		}
		_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"explore","content":"# Exploration","created_at":"2026-06-22T10:00:00Z"},{"artifact":"proposal","content":"# Proposal","created_at":"2026-06-22T10:05:00Z"}]}`))
	}))
	t.Cleanup(server.Close)
	source := newHiveSource(t, server.URL)

	artifacts, contents, err := source.FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if artifacts[sddstatus.ArtifactExplore] != sddstatus.ArtifactDone || artifacts[sddstatus.ArtifactProposal] != sddstatus.ArtifactDone {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	if contents[sddstatus.ArtifactProposal] != "# Proposal" {
		t.Fatalf("contents = %#v", contents)
	}
	status := sddstatus.ComputeStatus("epic-06", "hive", sddstatus.Input{Artifacts: artifacts, Contents: contents})
	if status.NextRecommended == sddstatus.PhaseExplore {
		t.Fatalf("valid exploration produced spurious %q recommendation", status.NextRecommended)
	}
}

func TestOpenSpecSourceClassifiesApplyProgressWithExplicitAndLegacyMarkers(t *testing.T) {
	tests := []struct {
		name     string
		progress string
		tasks    string
		want     sddstatus.ArtifactState
	}{
		{name: "explicit partial marker", progress: "status: partial\n- [x] T1\n", tasks: "- [x] T1\n", want: sddstatus.ArtifactPartial},
		{name: "explicit complete marker", progress: "status: complete\n", tasks: "- [x] T1\n- [ ] T2\n", want: sddstatus.ArtifactDone},
		{name: "malformed marker fails closed", progress: "status:complete\n", tasks: "- [x] T1\n", want: sddstatus.ArtifactPartial},
		{name: "conflicting markers fail closed", progress: "status: complete\nstatus: partial\n", tasks: "- [x] T1\n", want: sddstatus.ArtifactPartial},
		{name: "legacy progress needs deterministic task completion", progress: "legacy progress\n", tasks: "- [x] T1\n- [x] T2\n", want: sddstatus.ArtifactDone},
		{name: "legacy progress with incomplete tasks fails closed", progress: "legacy progress\n", tasks: "- [x] T1\n- [ ] T2\n", want: sddstatus.ArtifactPartial},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeDir := filepath.Join(root, "openspec", "changes", "epic-06")
			if err := os.MkdirAll(changeDir, 0o755); err != nil {
				t.Fatalf("create change directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(changeDir, "apply-progress.md"), []byte(tt.progress), 0o644); err != nil {
				t.Fatalf("write apply progress: %v", err)
			}
			if err := os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte(tt.tasks), 0o644); err != nil {
				t.Fatalf("write tasks: %v", err)
			}

			artifacts, contents, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress state = %q, want %q", got, tt.want)
			}
			if got := contents[sddstatus.ArtifactApplyProgress]; got != tt.progress {
				t.Fatalf("apply-progress content = %q, want preserved content %q", got, tt.progress)
			}
		})
	}
}

func TestOpenSpecSourcePreservesV2ManifestMismatch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "epic-06")
	if err := os.MkdirAll(filepath.Join(dir, "apply-evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{}})
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "stale task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, snapshotData, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = snapshot
	for path, data := range map[string][]byte{filepath.Join(dir, "apply-progress.md"): snapshotData, filepath.Join(dir, "apply-evidence", batch.BatchID+".json"): batchData, filepath.Join(dir, "tasks.md"): []byte("- [ ] 1.1 current task\n")} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedManifestMismatch {
		t.Fatalf("apply-progress = %q, want manifest mismatch", got)
	}
}

func TestHiveSourceClassifiesExplicitPartialApplyProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"apply-progress","content":"status: partial\n- [x] T1\n"}]}`))
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("apply-progress state = %q, want partial", got)
	}
}

func TestHybridSourceReclassifiesApplyProgressAfterMergingContents(t *testing.T) {
	tests := []struct {
		name          string
		hiveArtifacts string
		openSpecFiles map[string]string
		want          sddstatus.ArtifactState
	}{
		{
			name:          "hive progress uses OpenSpec task evidence",
			hiveArtifacts: `[{"artifact":"apply-progress","content":"legacy progress"}]`,
			openSpecFiles: map[string]string{"tasks.md": "- [x] T1\n- [x] T2\n"},
			want:          sddstatus.ArtifactDone,
		},
		{
			name:          "OpenSpec progress uses Hive task evidence",
			hiveArtifacts: `[{"artifact":"tasks","content":"- [x] T1\n- [x] T2\n"}]`,
			openSpecFiles: map[string]string{"apply-progress.md": "legacy progress\n"},
			want:          sddstatus.ArtifactDone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"artifacts":` + tt.hiveArtifacts + `}`))
			}))
			t.Cleanup(server.Close)

			root := t.TempDir()
			changeDir := filepath.Join(root, "openspec", "changes", "epic-06")
			if err := os.MkdirAll(changeDir, 0o755); err != nil {
				t.Fatalf("create change directory: %v", err)
			}
			for name, content := range tt.openSpecFiles {
				if err := os.WriteFile(filepath.Join(changeDir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			source := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root))
			artifacts, _, err := source.FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress state = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHiveSourceListChangesConsumesAllKeysetPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/governance/memories" {
			t.Fatal("HiveSource must not use generic governance memories")
		}
		if r.URL.Path != "/sdd/changes" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Fatalf("limit = %q", got)
		}
		requests++
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = w.Write([]byte(`{"changes":["alpha","bravo"],"next_cursor":"opaque-next"}`))
		case "opaque-next":
			_, _ = w.Write([]byte(`{"changes":["charlie"]}`))
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	t.Cleanup(server.Close)
	source := newHiveSource(t, server.URL)

	changes, err := source.ListChanges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0] != "alpha" || changes[1] != "bravo" || changes[2] != "charlie" {
		t.Fatalf("changes = %#v", changes)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestHiveSourceBlocksManifestMismatchedV2Progress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task"},{"artifact":"apply-progress","content":"{\"schema\":\"jarvis.sdd-apply-progress/v2\"}"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"committed","code":"ok","state":{"snapshot":{"status":"partial","task_manifest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedManifestMismatch {
		t.Fatalf("apply-progress = %q, want manifest mismatch", got)
	}
}

func TestHiveSourceClassifiesValidV2PartialAndCompleteSnapshots(t *testing.T) {
	const tasks = "- [ ] 1.1 task\n"
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		status string
		want   sddstatus.ArtifactState
	}{
		{name: "partial", status: "partial", want: sddstatus.ArtifactPartial},
		{name: "complete", status: "complete", want: sddstatus.ArtifactDone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/epic-06/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"},{"artifact":"apply-progress","content":"{\"schema\":\"jarvis.sdd-apply-progress/v2\"}"}]}`))
				case "/sdd/changes/epic-06/apply-progress":
					_, _ = w.Write([]byte(`{"outcome":"committed","code":"ok","state":{"snapshot":{"status":"` + tt.status + `","task_manifest_sha256":"` + manifest + `"}}}`))
				}
			}))
			t.Cleanup(server.Close)
			artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHiveSourcePreservesTypedApplyProgressOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		outcome string
		want    sddstatus.ArtifactState
	}{
		{name: "continuation", outcome: "continuation_required", want: sddstatus.ArtifactBlockedContinuation},
		{name: "conflict", outcome: "conflict", want: sddstatus.ArtifactBlockedConflict},
		{name: "invalid", outcome: "invalid", want: sddstatus.ArtifactBlockedInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/epic-06/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"proposal","content":"proposal"},{"artifact":"spec","content":"spec"},{"artifact":"design","content":"design"},{"artifact":"tasks","content":"- [ ] 1.1 task"},{"artifact":"apply-progress","content":"{\"schema\":\"jarvis.sdd-apply-progress/v2\"}"}]}`))
				case "/sdd/changes/epic-06/apply-progress":
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"outcome":"` + tt.outcome + `","code":"` + tt.outcome + `","recovery":"reconcile"}`))
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)

			artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHybridSourcePreservesBlockedApplyProgress(t *testing.T) {
	for _, tt := range []struct {
		name, outcome string
		want          sddstatus.ArtifactState
	}{
		{"continuation", "continuation_required", sddstatus.ArtifactBlockedBackendDiverged}, {"conflict", "conflict", sddstatus.ArtifactBlockedBackendDiverged}, {"invalid", "invalid", sddstatus.ArtifactBlockedBackendDiverged}, {"manifest mismatch", "committed", sddstatus.ArtifactBlockedBackendDiverged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/sdd/changes/epic-06/artifacts" {
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task"},{"artifact":"apply-progress","content":"{\"schema\":\"jarvis.sdd-apply-progress/v2\"}"}]}`))
					return
				}
				if tt.outcome != "committed" {
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"outcome":"` + tt.outcome + `"}`))
					return
				}
				_, _ = w.Write([]byte(`{"outcome":"committed","state":{"snapshot":{"status":"partial","task_manifest_sha256":"bad"}}}`))
			}))
			t.Cleanup(server.Close)
			root := t.TempDir()
			dir := filepath.Join(root, "openspec", "changes", "epic-06")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "apply-progress.md"), []byte("status: complete\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, contents, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := contents[sddstatus.ArtifactApplyProgress]; ok {
				t.Fatal("divergence retained selected apply-progress content")
			}
			if got[sddstatus.ArtifactApplyProgress] != tt.want {
				t.Fatalf("apply-progress = %q, want %q", got[sddstatus.ArtifactApplyProgress], tt.want)
			}
		})
	}
}

func TestComputeStatus_BlocksLifecycleForBackendDivergence(t *testing.T) {
	arts := map[string]sddstatus.ArtifactState{
		sddstatus.ArtifactProposal:      sddstatus.ArtifactDone,
		sddstatus.ArtifactSpec:          sddstatus.ArtifactDone,
		sddstatus.ArtifactDesign:        sddstatus.ArtifactDone,
		sddstatus.ArtifactTasks:         sddstatus.ArtifactDone,
		sddstatus.ArtifactApplyProgress: sddstatus.ArtifactBlockedBackendDiverged,
		sddstatus.ArtifactVerifyReport:  sddstatus.ArtifactDone,
	}
	status := sddstatus.ComputeStatus("epic-06", "hybrid", sddstatus.Input{
		Artifacts: arts, ActionMode: sddstatus.ActionModeWorkspaceEdit, AllowedEditRoots: []string{"/workspace"},
		Contents: map[string]string{sddstatus.ArtifactVerifyReport: "All checks passed."},
	})
	for _, phase := range []string{sddstatus.PhaseApply, sddstatus.PhaseVerify, sddstatus.PhaseArchive} {
		if status.Dependencies[phase] != sddstatus.DepBlocked {
			t.Fatalf("dependency[%s] = %q, want blocked", phase, status.Dependencies[phase])
		}
	}
}

func newHiveSource(t *testing.T, baseURL string) *sddstatus.HiveSource {
	t.Helper()
	client, err := hiveclient.New(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	return sddstatus.NewHiveSource(client, "jarvis-dev")
}
