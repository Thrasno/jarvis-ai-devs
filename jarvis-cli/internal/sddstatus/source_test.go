package sddstatus_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestGuardedSourcesRejectForeignIdentity(t *testing.T) {
	for _, tc := range []struct{ name, project, change string }{
		{"foreign project", "foreign", "change"},
		{"foreign change", "jarvis-dev", "foreign"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: tc.project, Change: tc.change, Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "openspec", "changes", "change")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "apply-progress.md"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			root := filepath.Dir(filepath.Dir(filepath.Dir(dir)))
			osSource := sddstatus.NewOpenSpecSource(root)
			if tc.project != "jarvis-dev" {
				osSource = sddstatus.NewOpenSpecSourceForProject(root, "jarvis-dev")
			}
			arts, _, err := osSource.FetchArtifacts(context.Background(), "change")
			if err != nil {
				t.Fatal(err)
			}
			if arts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedInvalid {
				t.Fatalf("OpenSpec foreign identity = %q", arts[sddstatus.ArtifactApplyProgress])
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/change/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\\n"}]}`))
				case "/sdd/changes/change/apply-progress":
					_, _ = fmt.Fprintf(w, `{"outcome":"current","state":{"snapshot":%s,"generation":%d,"revision":%d,"digest":%q}}`, data, snapshot.Generation, snapshot.Revision, snapshot.Digest)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			arts, _, err = newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "change")
			if err != nil {
				t.Fatal(err)
			}
			if arts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedInvalid {
				t.Fatalf("Hive foreign identity = %q", arts[sddstatus.ArtifactApplyProgress])
			}
		})
	}
}

func TestSealedProgressRequiresMatchingAuthenticatedBackends(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "old")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "- [ ] 1.1 original\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "old", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	initial, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "old", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	store := sddprogress.OpenSpec{Root: dir}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "initial-request", Snapshot: initial, Batches: []applyprogress.Batch{batch}}); err != nil {
		t.Fatal(err)
	}
	revised := "- [ ] 1.1 revised\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte(revised), 0o600); err != nil {
		t.Fatal(err)
	}
	_, revisedManifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "revised"}})
	if err != nil {
		t.Fatal(err)
	}
	seal := initial
	seal.Schema = applyprogress.SupersessionSnapshotSchema
	seal.Revision++
	seal.PreviousDigest = initial.Digest
	seal.Status = applyprogress.StatusSuperseded
	seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "jarvis-dev", SuccessorChange: "successor", SuccessorManifestSHA256: revisedManifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal-request"}
	seal, _, err = applyprogress.SealSnapshot(seal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "seal-request", ExpectedGeneration: initial.Generation, ExpectedRevision: initial.Revision, ExpectedDigest: initial.Digest, Snapshot: seal}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		snapshot applyprogress.Snapshot
		batches  []applyprogress.Batch
		want     sddstatus.ArtifactState
	}{
		{"matching", seal, []applyprogress.Batch{batch}, sddstatus.ArtifactSuperseded},
		{"forged coverage", func() applyprogress.Snapshot {
			bad := seal
			bad.Coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "evidence"}}
			bad, _, err = applyprogress.SealSnapshot(bad)
			if err != nil {
				t.Fatal(err)
			}
			return bad
		}(), []applyprogress.Batch{batch}, sddstatus.ArtifactBlockedBackendDiverged},
		{"different seal", func() applyprogress.Snapshot {
			other := seal
			other.SealIntent = &applyprogress.SealIntent{SuccessorProject: "jarvis-dev", SuccessorChange: "different", SuccessorManifestSHA256: revisedManifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "other-request"}
			other, _, err = applyprogress.SealSnapshot(other)
			if err != nil {
				t.Fatal(err)
			}
			return other
		}(), []applyprogress.Batch{batch}, sddstatus.ArtifactBlockedBackendDiverged},
		{"wrong evidence project", seal, []applyprogress.Batch{func() applyprogress.Batch {
			bad := batch
			bad.Project = "other"
			bad, _, _ = applyprogress.SealBatch(bad)
			return bad
		}()}, sddstatus.ArtifactBlockedBackendDiverged},
		{"wrong evidence change", seal, []applyprogress.Batch{func() applyprogress.Batch {
			bad := batch
			bad.Change = "other"
			bad, _, _ = applyprogress.SealBatch(bad)
			return bad
		}()}, sddstatus.ArtifactBlockedBackendDiverged},
		{"wrong evidence digest", seal, []applyprogress.Batch{func() applyprogress.Batch { bad := batch; bad.SHA256 = strings.Repeat("a", 64); return bad }()}, sddstatus.ArtifactBlockedBackendDiverged},
		{"wrong evidence id", seal, []applyprogress.Batch{func() applyprogress.Batch {
			bad := batch
			bad.BatchID = "apb-00000000000000000000000000000002"
			return bad
		}()}, sddstatus.ArtifactBlockedBackendDiverged},
		{"malformed evidence", seal, []applyprogress.Batch{func() applyprogress.Batch { bad := batch; bad.Entries = nil; return bad }()}, sddstatus.ArtifactBlockedBackendDiverged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hiveData, err := json.Marshal(tc.snapshot)
			if err != nil {
				t.Fatal(err)
			}
			batches, err := json.Marshal(tc.batches)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/old/artifacts":
					_, _ = fmt.Fprintf(w, `{"artifacts":[{"artifact":"tasks","content":%q}]}`, revised)
				case "/sdd/changes/old/apply-progress":
					_, _ = fmt.Fprintf(w, `{"outcome":"current","state":{"snapshot":%s,"batches":%s}}`, hiveData, batches)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			hive, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "old")
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "matching" && hive[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactSuperseded {
				t.Fatalf("hive = %q", hive[sddstatus.ArtifactApplyProgress])
			}
			if (strings.Contains(tc.name, "evidence") || tc.name == "forged coverage") && hive[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedInvalid {
				t.Fatalf("corrupt hive = %q", hive[sddstatus.ArtifactApplyProgress])
			}
			artifacts, contents, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "old")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tc.want {
				t.Fatalf("hybrid = %q, want %q", got, tc.want)
			}
			if tc.name == "matching" {
				status := sddstatus.ComputeStatus("old", "hybrid", sddstatus.Input{Artifacts: artifacts, Contents: contents})
				for _, phase := range []string{sddstatus.PhaseApply, sddstatus.PhaseVerify, sddstatus.PhaseArchive} {
					if status.Dependencies[phase] != sddstatus.DepBlocked {
						t.Fatalf("%s not blocked: %#v", phase, status.Dependencies[phase])
					}
				}
			}
		})
	}
}

func TestOpenSpecSourceRecognizesCanonicalV3Successor(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "successor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tasks := "- [ ] 1.1 revised task\n"
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte(tasks), 0o600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "revised task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: "jarvis-dev", Change: "successor", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, Supersedes: &applyprogress.SupersedesPointer{Project: "jarvis-dev", Change: "old", SealDigest: strings.Repeat("a", 64), OriginalManifestSHA256: strings.Repeat("b", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "operation-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apply-progress.md"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/successor/artifacts":
			_, _ = fmt.Fprintf(w, `{"artifacts":[{"artifact":"tasks","content":%q}]}`, tasks)
		case "/sdd/changes/successor/apply-progress":
			_, _ = fmt.Fprintf(w, `{"outcome":"current","state":{"snapshot":%s,"generation":%d,"revision":%d,"digest":%q}}`, data, snapshot.Generation, snapshot.Revision, snapshot.Digest)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	hiveArtifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "successor")
	if err != nil {
		t.Fatal(err)
	}
	if got := hiveArtifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("Hive v3 successor = %q, want partial", got)
	}
	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "successor")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("v3 successor = %q, want partial", got)
	}
}

func TestOpenSpecSourceRejectsNoncanonicalJSONProgress(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "malformed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"tasks.md":          "- [x] 1.1 finished\n",
		"apply-progress.md": "{\"schema\" : \"jarvis.sdd-apply-progress/v3\", \"status\":\"complete\"}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "malformed")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("malformed JSON progress = %q, want blocked invalid", got)
	}
}

func TestOpenSpecSourceStatusDetectsStagingWithoutApplyProgressHead(t *testing.T) {
	root := t.TempDir()
	change := filepath.Join(root, "openspec", "changes", "epic-06")
	stage := filepath.Join(change, "apply-evidence", ".apply-progress-stage-anonymous")
	if err := os.MkdirAll(filepath.Dir(stage), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("unbound residue"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedPublicationInterrupted {
		t.Fatalf("apply-progress state = %q, want publication interruption", got)
	}
}

func TestHiveSourceRejectsJSONLegacyProgressOnTypedNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/legacy/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 finished\n"},{"artifact":"apply-progress","content":"[]"}]}`))
		case "/sdd/changes/legacy/apply-progress":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("JSON array progress = %q, want blocked invalid", got)
	}
}

func TestHiveSourceFetchArtifactsUsesDedicatedCompleteRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("HiveSource status request method = %s, want GET", r.Method)
		}
		if r.URL.Path == "/sdd/changes/epic-06/apply-progress" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"outcome":"not_found","code":"not_found"}`))
			return
		}
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

func TestOpenSpecSourceDiscoversNestedCanonicalDeltaSpecs(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "openspec", "changes", "epic-06")
	specPath := filepath.Join(changeDir, "specs", "platform", "authentication", "spec.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# Delta Specification\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactSpec]; got != sddstatus.ArtifactDone {
		t.Fatalf("nested delta spec state = %q, want done", got)
	}
}

func TestOpenSpecSourceClassifiesApplyProgressWithExplicitAndLegacyMarkers(t *testing.T) {
	tests := []struct {
		name     string
		progress string
		tasks    string
		want     sddstatus.ArtifactState
	}{
		{name: "explicit partial marker", progress: "status: partial\n- [x] 1\n", tasks: "- [x] 1\n", want: sddstatus.ArtifactPartial},
		{name: "explicit complete marker with incomplete tasks fails closed", progress: "status: complete\n", tasks: "- [x] 1.1 first\n- [ ] 1.2 second\n", want: sddstatus.ArtifactPartial},
		{name: "explicit complete marker with duplicate task is ambiguous", progress: "status: complete\n", tasks: "- [x] 1.1 first\n- [x] 1.1 duplicate\n", want: sddstatus.ArtifactPartial},
		{name: "malformed marker fails closed", progress: "status:complete\n", tasks: "- [x] 1\n", want: sddstatus.ArtifactPartial},
		{name: "malformed task marker fails closed", progress: "status: complete\n", tasks: "- [x] 1.1 task\n- [z] 1.2 malformed\n", want: sddstatus.ArtifactPartial},
		{name: "one-field task row fails closed", progress: "status: complete\n", tasks: "- [x] 1.1 task\n- [x] garbage\n", want: sddstatus.ArtifactPartial},
		{name: "nondigit multiword task ID fails closed", progress: "status: complete\n", tasks: "- [x] 1.1 task\n- [x] garbage words\n", want: sddstatus.ArtifactPartial},
		{name: "conflicting markers fail closed", progress: "status: complete\nstatus: partial\n", tasks: "- [x] 1\n", want: sddstatus.ArtifactPartial},
		{name: "JSON array is not legacy completion", progress: "[]\n", tasks: "- [x] 1.1 task\n", want: sddstatus.ArtifactBlockedInvalid},
		{name: "legacy progress needs deterministic task completion", progress: "legacy progress\n", tasks: "- [x] 1.1 task\n- [x] 1.2 task\n", want: sddstatus.ArtifactDone},
		{name: "legacy phase labels exclude parent actions", progress: "legacy progress\n", tasks: "## Compatibility\n- [x] RED: reproduce\n- [x] GREEN: fix\n- [x] TRIANGULATE: alternate\n- [x] REFACTOR: clarify\n\n## Parent Actions\n- [ ] RED: workflow prose\n", want: sddstatus.ArtifactDone},
		{name: "legacy progress with incomplete tasks fails closed", progress: "legacy progress\n", tasks: "- [x] 1.1 task\n- [ ] 1.2 task\n", want: sddstatus.ArtifactPartial},
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

func TestOpenSpecSourceReportsPublicationInterruptionWithoutMutatingStages(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "epic-06")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000009", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, snapshotData, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	store := sddprogress.OpenSpec{Root: dir}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "interrupted-status", Snapshot: snapshot, Batches: []applyprogress.Batch{batch}}); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(dir, ".apply-progress-receipts", ".apply-progress-stage-crash")
	if err := os.WriteFile(stage, []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactState("blocked:publication_interrupted") {
		t.Fatalf("apply-progress = %q, want publication interruption", got)
	}
	if _, err := os.Stat(stage); err != nil {
		t.Fatalf("status mutated retained stage: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":` + string(snapshotData) + `,"batches":[` + string(batchData) + `]}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	artifacts, _, err = sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactState("blocked:publication_interrupted") {
		t.Fatalf("hybrid apply-progress = %q, want publication interruption", got)
	}
}

func TestOpenSpecSourcePreservesV2ManifestMismatch(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "epic-06")
	if err := os.MkdirAll(filepath.Join(dir, "apply-evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
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

func TestHiveSourcePreservesTypedValidationCodeAndDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"outcome":"invalid","code":"legacy_migration","detail":"legacy source sha256","recovery":"repair progress"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, contents, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactState("blocked:legacy_migration") {
		t.Fatalf("apply-progress state = %q, want typed legacy migration block", got)
	}
	if got := contents[sddstatus.ArtifactApplyProgress]; got != "legacy source sha256" {
		t.Fatalf("apply-progress validation detail = %q", got)
	}
}

func TestHiveSourcePropagatesEvidenceUnavailableAsRetryableFetchError(t *testing.T) {
	fixture := sharedApplyProgressGetFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/change/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 first task\n- [ ] 1.2 second task\n"}]}`))
		case "/sdd/changes/change/apply-progress":
			_, _ = w.Write(fixture)
		case "/sdd/changes/change/apply-evidence/apb-00000000000000000000000000000001":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"outcome":"unavailable","code":"unavailable","recovery":"retry"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	_, _, err := hiveSourceForProject(t, server.URL, "project").FetchArtifacts(context.Background(), "change")
	var typed *hiveclient.ApplyProgressError
	if !errors.As(err, &typed) || typed.Result.Code != "unavailable" {
		t.Fatalf("FetchArtifacts() error = %#v, want retryable unavailable ApplyProgressError", err)
	}
}

func TestHiveSourceReadsTypedV2HeadWithoutLegacyArtifactProjection(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(hiveclient.ApplyProgressResult{Outcome: "current", Code: "ok", State: hiveclient.ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot, Batches: []applyprogress.Batch{}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write(state)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("apply-progress = %q, want partial from typed head", got)
	}
}

func TestOpenSpecSourceReportsPartialV2HeadWithFullCoverageAndContinuation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "epic-06")
	if err := os.MkdirAll(filepath.Join(dir, "apply-evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	tasks := []applyprogress.Task{{ID: "1.1", Text: "task"}}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000002", Entries: []applyprogress.EvidenceEntry{{EntryID: "complete", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, snapshotData, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "complete"}}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}, StreamSHA256: strings.Repeat("a", 64), NextEntryIndex: 1, NextEntryID: "trailing"})
	if err != nil {
		t.Fatal(err)
	}
	_ = snapshot
	for path, data := range map[string][]byte{filepath.Join(dir, "apply-progress.md"): snapshotData, filepath.Join(dir, "apply-evidence", batch.BatchID+".json"): batchData, filepath.Join(dir, "tasks.md"): []byte("- [ ] 1.1 task\n")} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	artifacts, _, err := sddstatus.NewOpenSpecSource(root).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("apply-progress = %q, want partial while trailing continuation evidence remains", got)
	}
}

func TestHiveSourceBlocksPartialV2HeadWithCompleteAuthoritativeCoverage(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"current","code":"ok","state":{"snapshot":{"schema":"jarvis.sdd-apply-progress/v2","status":"partial","task_manifest_sha256":"` + manifest + `","coverage":[{"task_id":"1.1","batch_id":"apb-00000000000000000000000000000001","entry_id":"evidence"}]}}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("apply-progress = %q, want partial complete coverage to block", got)
	}
}

func TestHiveSourceConsumesSharedApplyProgressGetFixture(t *testing.T) {
	fixture := sharedApplyProgressGetFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/change/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 first task\n- [ ] 1.2 second task\n"}]}`))
		case "/sdd/changes/change/apply-progress":
			_, _ = w.Write(fixture)
		case "/sdd/changes/change/apply-evidence/apb-00000000000000000000000000000001":
			_, _ = w.Write(sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000001"))
		case "/sdd/changes/change/apply-evidence/apb-00000000000000000000000000000002":
			_, _ = w.Write(sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000002"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := hiveSourceForProject(t, server.URL, "project").FetchArtifacts(context.Background(), "change")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("apply-progress = %q, want partial", got)
	}
}

func TestHybridSourceAcceptsMatchingSharedV2Fixture(t *testing.T) {
	fixture := sharedApplyProgressGetFixture(t)
	var progress hiveclient.ApplyProgressResult
	if err := json.Unmarshal(fixture, &progress); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	changeDir := filepath.Join(root, "openspec", "changes", "change")
	if err := os.MkdirAll(filepath.Join(changeDir, "apply-evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(progress.State.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "apply-progress.md"), snapshot, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("- [ ] 1.1 first task\n- [ ] 1.2 second task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, ref := range progress.State.Snapshot.Batches {
		var envelope struct {
			Batch applyprogress.Batch `json:"batch"`
		}
		if err := json.Unmarshal(sharedApplyProgressEvidenceFixture(t, ref.BatchID), &envelope); err != nil {
			t.Fatal(err)
		}
		_, batch, err := applyprogress.SealBatch(envelope.Batch)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(changeDir, "apply-evidence", ref.BatchID+".json"), batch, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/change/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 first task\n- [ ] 1.2 second task\n"}]}`))
		case "/sdd/changes/change/apply-progress":
			_, _ = w.Write(fixture)
		case "/sdd/changes/change/apply-evidence/apb-00000000000000000000000000000001":
			_, _ = w.Write(sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000001"))
		case "/sdd/changes/change/apply-evidence/apb-00000000000000000000000000000002":
			_, _ = w.Write(sharedApplyProgressEvidenceFixture(t, "apb-00000000000000000000000000000002"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := sddstatus.NewHybridSource(hiveSourceForProject(t, server.URL, "project"), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "change")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("apply-progress = %q, want matching partial v2 state", got)
	}
}

func TestHiveSourceRejectsInvalidGuardedAttributionAndBatchOrder(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*applyprogress.Snapshot, *[]applyprogress.Batch)
	}{
		{
			name: "unknown task attribution",
			edit: func(snapshot *applyprogress.Snapshot, batches *[]applyprogress.Batch) {
				second := (*batches)[1]
				second.Entries[0].TaskIDs = []string{"unknown"}
				sealed, _, err := applyprogress.SealBatch(second)
				if err != nil {
					t.Fatal(err)
				}
				(*batches)[1] = sealed
				snapshot.Batches[1].SHA256 = sealed.SHA256
				sealedSnapshot, _, err := applyprogress.SealSnapshot(*snapshot)
				if err != nil {
					t.Fatal(err)
				}
				*snapshot = sealedSnapshot
			},
		},
		{
			name: "reversed referenced batch order",
			edit: func(_ *applyprogress.Snapshot, batches *[]applyprogress.Batch) {
				(*batches)[0], (*batches)[1] = (*batches)[1], (*batches)[0]
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, batches := guardedHiveValidationFixture(t)
			tt.edit(&snapshot, &batches)
			state, err := json.Marshal(hiveclient.ApplyProgressResult{Outcome: "current", Code: "ok", State: hiveclient.ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot, Batches: batches}})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/epic-06/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 first\n- [ ] 1.2 second\n"}]}`))
				case "/sdd/changes/epic-06/apply-progress":
					_, _ = w.Write(state)
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)

			artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
				t.Fatalf("apply-progress = %q, want invalid guarded state", got)
			}
		})
	}
}

func guardedHiveValidationFixture(t *testing.T) (applyprogress.Snapshot, []applyprogress.Batch) {
	t.Helper()
	tasks := []applyprogress.Task{{ID: "1.1", Text: "first"}, {ID: "1.2", Text: "second"}}
	_, manifest, err := applyprogress.TaskManifest(tasks)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema:  applyprogress.EvidenceSchema,
		Project: "jarvis-dev",
		Change:  "epic-06",
		BatchID: "apb-00000000000000000000000000000011",
		Entries: []applyprogress.EvidenceEntry{{
			EntryID:          "entry-1",
			TaskIDs:          []string{"1.1"},
			CompletesTaskIDs: []string{"1.1"},
			Kind:             applyprogress.EvidenceGreen,
			Summary:          "first",
			Command:          "go test",
			Outcome:          applyprogress.OutcomePass,
			Files:            []string{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := applyprogress.SealBatch(applyprogress.Batch{
		Schema:  applyprogress.EvidenceSchema,
		Project: "jarvis-dev",
		Change:  "epic-06",
		BatchID: "apb-00000000000000000000000000000012",
		Entries: []applyprogress.EvidenceEntry{{
			EntryID:          "entry-2",
			TaskIDs:          []string{"1.2"},
			CompletesTaskIDs: []string{},
			Kind:             applyprogress.EvidenceVerification,
			Summary:          "second",
			Command:          "go test",
			Outcome:          applyprogress.OutcomePass,
			Files:            []string{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema:             applyprogress.SnapshotSchema,
		Project:            "jarvis-dev",
		Change:             "epic-06",
		Generation:         1,
		Revision:           1,
		TaskManifestSHA256: manifest,
		Status:             applyprogress.StatusPartial,
		Coverage:           []applyprogress.Coverage{{TaskID: "1.1", BatchID: first.BatchID, EntryID: "entry-1"}},
		Batches: []applyprogress.BatchRef{
			{BatchID: first.BatchID, SHA256: first.SHA256},
			{BatchID: second.BatchID, SHA256: second.SHA256},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, []applyprogress.Batch{first, second}
}

func TestHiveSourceFailsClosedWhenGuardedBatchCannotValidateSnapshot(t *testing.T) {
	const tasks = "- [ ] 1.1 task\n"
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000099", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	tampered := batch
	tampered.Entries[0].Summary = "tampered evidence"
	state, err := json.Marshal(hiveclient.ApplyProgressResult{Outcome: "current", Code: "ok", State: hiveclient.ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot, Batches: []applyprogress.Batch{tampered}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write(state)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("apply-progress = %q, want invalid after full batch validation", got)
	}
}

func TestHiveSourceFailsClosedForCompleteHeadWithoutValidTaskCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":{"schema":"jarvis.sdd-apply-progress/v2","status":"complete"}}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("apply-progress = %q, want fail-closed invalid state", got)
	}
}

func TestHiveSourceBlocksCompleteV2HeadWhenAuthoritativeTasksUnchecked(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":{"schema":"jarvis.sdd-apply-progress/v2","status":"complete","task_manifest_sha256":"` + manifest + `","coverage":[{"task_id":"1.1","batch_id":"apb-00000000000000000000000000000001","entry_id":"evidence"}]}}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	artifacts, _, err := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("apply-progress = %q, want unchecked authoritative tasks to block completion", got)
	}
}

func TestHiveSourceClassifiesExplicitPartialApplyProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"apply-progress","content":"status: partial\n- [x] 1\n"}]}`))
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
			openSpecFiles: map[string]string{"tasks.md": "- [x] 1.1 task\n- [x] 1.2 task\n"},
			want:          sddstatus.ArtifactDone,
		},
		{
			name:          "OpenSpec progress uses Hive task evidence",
			hiveArtifacts: `[{"artifact":"tasks","content":"- [x] 1.1 task\n- [x] 1.2 task\n"}]`,
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
			_, _ = w.Write([]byte(`{"outcome":"current","code":"ok","state":{"snapshot":{"schema":"jarvis.sdd-apply-progress/v2","status":"partial","project":"jarvis-dev","change":"epic-06","task_manifest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`))
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
		t.Fatalf("apply-progress = %q, want task manifest mismatch", got)
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
			tasksContent := "- [ ] 1.1 task\n"
			coverage := []applyprogress.Coverage{}
			refs := []applyprogress.BatchRef{}
			batches := []applyprogress.Batch{}
			if tt.status == "complete" {
				tasksContent = "- [x] 1.1 task\n"
				batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
				if err != nil {
					t.Fatal(err)
				}
				coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "evidence"}}
				refs, batches = []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}, []applyprogress.Batch{batch}
			}
			snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.Status(tt.status), Coverage: coverage, Batches: refs})
			if err != nil {
				t.Fatal(err)
			}
			state, err := json.Marshal(hiveclient.ApplyProgressResult{Outcome: "current", Code: "ok", State: hiveclient.ApplyProgressState{Generation: snapshot.Generation, Revision: snapshot.Revision, Digest: snapshot.Digest, Snapshot: snapshot, Batches: batches}})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/epic-06/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":` + fmt.Sprintf("%q", tasksContent) + `},{"artifact":"apply-progress","content":"{\"schema\":\"jarvis.sdd-apply-progress/v2\"}"}]}`))
				case "/sdd/changes/epic-06/apply-progress":
					_, _ = w.Write(state)
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
				_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":{"status":"partial","task_manifest_sha256":"bad"}}}`))
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

func TestHybridSourceDoesNotTreatAbsentActiveOpenSpecAsArchivedWithoutEvidence(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000003", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusComplete, Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "evidence"}}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 task"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"committed","state":{"snapshot":` + string(data) + `,"batches":[` + string(batchData) + `]}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(t.TempDir())).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedBackendDiverged || artifacts[sddstatus.ArtifactArchiveReport] == sddstatus.ArtifactDone {
		t.Fatalf("absent active topology = %#v; snapshot=%s", artifacts, snapshot.Digest)
	}
}

func TestHybridSourceRejectsFakeArchivedTopologyWithCompleteHiveHead(t *testing.T) {
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000004", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusComplete, Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "evidence"}}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 task"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"committed","state":{"snapshot":` + string(data) + `,"batches":[` + string(batchData) + `]}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	archive := filepath.Join(root, "openspec", "changes", "archive", "2026-09-10-epic-06")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"archive-report.md", "proposal.md", "design.md", "tasks.md", "apply-progress.md", "verify-report.md"} {
		if err := os.WriteFile(filepath.Join(archive, name), []byte("evidence\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedBackendDiverged || artifacts[sddstatus.ArtifactArchiveReport] == sddstatus.ArtifactDone {
		t.Fatalf("fake archived topology = %#v", artifacts)
	}
}

func TestHybridSourceArchivedTopologyRequiresReceiptsAndDeltaSpecs(t *testing.T) {
	for _, tt := range []struct {
		name           string
		omit           string
		corruptReceipt bool
	}{
		{name: "missing receipts directory", omit: "receipts"},
		{name: "missing archived delta specs", omit: "specs"},
		{name: "corrupt JSON receipt", corruptReceipt: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tasks := "- [x] 1.1 task\n"
			parsed, err := applyprogress.ParseTasksMarkdown(tasks)
			if err != nil {
				t.Fatal(err)
			}
			_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
			if err != nil {
				t.Fatal(err)
			}
			batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000002", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
			if err != nil {
				t.Fatal(err)
			}
			_, snapshotData, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusComplete, Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "evidence"}}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
			if err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(root, "openspec", "changes", "archive", "2026-09-10-epic-06")
			if err := os.MkdirAll(filepath.Join(archive, "apply-evidence"), 0o755); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string][]byte{"archive-report.md": []byte("archive\n"), "proposal.md": []byte("proposal\n"), "design.md": []byte("design\n"), "tasks.md": []byte(tasks), "apply-progress.md": snapshotData, "verify-report.md": []byte("verify\n"), filepath.Join("apply-evidence", batch.BatchID+".json"): batchData} {
				if err := os.WriteFile(filepath.Join(archive, name), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.omit != "receipts" {
				if err := os.MkdirAll(filepath.Join(archive, ".apply-progress-receipts"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(archive, ".apply-progress-receipts", "request.json"), []byte(`{"payload":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if tt.corruptReceipt {
					if err := os.WriteFile(filepath.Join(archive, ".apply-progress-receipts", "torn.json"), []byte(`{"payload":`), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tt.omit != "specs" {
				if err := os.MkdirAll(filepath.Join(archive, "specs", "bounded-apply-progress"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(archive, "specs", "bounded-apply-progress", "spec.md"), []byte("# Delta Specification\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/epic-06/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 task\n"}]}`))
				case "/sdd/changes/epic-06/apply-progress":
					_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":` + string(snapshotData) + `,"batches":[` + string(batchData) + `]}}`))
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedBackendDiverged {
				t.Fatalf("apply-progress = %q, want missing archive topology to diverge", got)
			}
		})
	}
}

func TestHybridSourceAcceptsValidatedArchivedTopologyMatchingHive(t *testing.T) {
	root := t.TempDir()
	tasks := "- [x] 1.1 task\n"
	parsed, err := applyprogress.ParseTasksMarkdown(tasks)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	batch, batchData, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{"1.1"}, CompletesTaskIDs: []string{"1.1"}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, snapshotData, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusComplete, Coverage: []applyprogress.Coverage{{TaskID: "1.1", BatchID: batch.BatchID, EntryID: "evidence"}}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(root, "openspec", "changes", "epic-06")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(active, "tasks.md"), []byte(tasks), 0o600); err != nil {
		t.Fatal(err)
	}
	// Advance produces the canonical receipt bytes that archive validation must accept.
	store := sddprogress.OpenSpec{Root: active}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "archive-request", Snapshot: snapshot, Batches: []applyprogress.Batch{batch}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(active, "specs", "bounded-apply-progress"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{
		filepath.Join(active, "archive-report.md"): []byte("archive\n"), filepath.Join(active, "proposal.md"): []byte("proposal\n"), filepath.Join(active, "design.md"): []byte("design\n"), filepath.Join(active, "verify-report.md"): []byte("## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"), filepath.Join(active, "specs", "bounded-apply-progress", "spec.md"): []byte("# Delta Specification\n"),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(root, "openspec", "changes", "archive", "2026-09-10-epic-06")
	if err := store.Archive(archive); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/epic-06/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] 1.1 task\n"}]}`))
		case "/sdd/changes/epic-06/apply-progress":
			_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":` + string(snapshotData) + `,"batches":[` + string(batchData) + `]}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactDone || artifacts[sddstatus.ArtifactArchiveReport] != sddstatus.ArtifactDone || snapshot.Digest == "" {
		t.Fatalf("validated archive = %#v", artifacts)
	}
	if err := os.WriteFile(filepath.Join(archive, "apply-evidence", ".apply-progress-stage-archive"), []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts, _, err = sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedBackendDiverged {
		t.Fatalf("archived staging topology = %q, want backend divergence", got)
	}
	if err := os.Remove(filepath.Join(archive, "apply-evidence", ".apply-progress-stage-archive")); err != nil {
		t.Fatal(err)
	}

	receiptPath := filepath.Join(archive, ".apply-progress-receipts", "archive-request.json")
	receiptData, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Payload  string                 `json:"payload"`
		Snapshot applyprogress.Snapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(receiptData, &receipt); err != nil {
		t.Fatal(err)
	}
	t.Run("unresolved successor receipt", func(t *testing.T) {
		unresolved := receipt
		unresolved.Snapshot.Revision++
		unresolved.Snapshot.PreviousDigest = receipt.Snapshot.Digest
		var err error
		unresolved.Snapshot, _, err = applyprogress.SealSnapshot(unresolved.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(unresolved)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(archive, ".apply-progress-receipts", "unresolved.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
		if err != nil {
			t.Fatal(err)
		}
		if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedBackendDiverged {
			t.Fatalf("unresolved archived receipt = %q, want backend divergence", got)
		}
	})
	mismatched := receipt
	mismatched.Snapshot.Project = "other-project"
	mismatched.Snapshot, _, err = applyprogress.SealSnapshot(mismatched.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedData, err := json.Marshal(mismatched)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"corrupt": []byte(`{"payload":`), "mismatched identity": mismatchedData} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(receiptPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedBackendDiverged {
				t.Fatalf("%s archived receipt = %q, want backend divergence", name, got)
			}
		})
	}

	t.Run("mismatched digest", func(t *testing.T) {
		mismatched := receipt
		mismatched.Snapshot.Revision++
		mismatched.Snapshot.PreviousDigest = receipt.Snapshot.Digest
		mismatched.Snapshot, _, err = applyprogress.SealSnapshot(mismatched.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(mismatched)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(receiptPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		artifacts, _, err := sddstatus.NewHybridSource(newHiveSource(t, server.URL), sddstatus.NewOpenSpecSource(root)).FetchArtifacts(context.Background(), "epic-06")
		if err != nil {
			t.Fatal(err)
		}
		if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedBackendDiverged {
			t.Fatalf("mismatched digest archived receipt = %q, want backend divergence", got)
		}
	})
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

func TestHiveSourceUsesTolerantTaskParsingOnlyForImportedLegacyManifest(t *testing.T) {
	tasksContent := "- [x] RED: reproduce\n"
	legacyParsed, err := applyprogress.ParseLegacyTasksMarkdown(tasksContent)
	if err != nil {
		t.Fatal(err)
	}
	legacyTasks, legacyManifest, err := applyprogress.LegacyTaskManifest(legacyParsed.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	nativeManifestTasks := []applyprogress.Task{{ID: "RED", Text: "RED: reproduce"}}
	_, nativeManifest, err := applyprogress.TaskManifest(nativeManifestTasks)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		tasks    []applyprogress.Task
		manifest string
		want     sddstatus.ArtifactState
	}{
		{name: "imported legacy manifest", tasks: legacyTasks, manifest: legacyManifest, want: sddstatus.ArtifactDone},
		{name: "native manifest remains strict", tasks: nativeManifestTasks, manifest: nativeManifest, want: sddstatus.ArtifactBlockedInvalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			batch, batchData, sealErr := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "epic-06", BatchID: "apb-00000000000000000000000000000999", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{tt.tasks[0].ID}, CompletesTaskIDs: []string{tt.tasks[0].ID}, Kind: applyprogress.EvidenceImported, Summary: "imported", Command: "migration", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			_, snapshotData, sealErr := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "epic-06", Generation: 1, Revision: 1, TaskManifestSHA256: tt.manifest, Status: applyprogress.StatusComplete, Coverage: []applyprogress.Coverage{{TaskID: tt.tasks[0].ID, BatchID: batch.BatchID, EntryID: "evidence"}}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/sdd/changes/epic-06/artifacts":
					_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [x] RED: reproduce\n"}]}`))
				case "/sdd/changes/epic-06/apply-progress":
					_, _ = w.Write([]byte(`{"outcome":"current","state":{"snapshot":` + string(snapshotData) + `,"batches":[` + string(batchData) + `]}}`))
				default:
					t.Fatalf("unexpected path %q", request.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			artifacts, _, fetchErr := newHiveSource(t, server.URL).FetchArtifacts(context.Background(), "epic-06")
			if fetchErr != nil {
				t.Fatal(fetchErr)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress state = %q, want %q", got, tt.want)
			}
		})
	}
}

// sharedApplyProgressGetFixture resolves from this source file, not the test process
// working directory, because Go runs packages from different module subdirectories.
func sharedApplyProgressEvidenceFixture(t *testing.T, batchID string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture source path")
	}
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", "testdata", "sdd-progress", "apply-evidence-"+batchID+".json")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sharedApplyProgressGetFixture(t *testing.T) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture source path")
	}
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", "testdata", "sdd-progress", "apply-progress-get.json")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestObserveLegacyProgressUsesOnlyProtectedProgress(t *testing.T) {
	source := legacyProgressSource{
		artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactProposal: sddstatus.ArtifactDone},
		contents:  map[string]string{sddstatus.ArtifactProposal: "proposal"},
	}
	observation, err := sddstatus.ObserveLegacyProgress(context.Background(), source, "change")
	if err != nil {
		t.Fatal(err)
	}
	if observation.Present || observation.State != "" || observation.Content != "" {
		t.Fatalf("observation = %#v, want absent protected progress", observation)
	}

	source.artifacts[sddstatus.ArtifactApplyProgress] = sddstatus.ArtifactBlockedInvalid
	source.contents[sddstatus.ArtifactApplyProgress] = "invalid progress"
	observation, err = sddstatus.ObserveLegacyProgress(context.Background(), source, "change")
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Present || observation.State != sddstatus.ArtifactBlockedInvalid || observation.Content != "invalid progress" {
		t.Fatalf("observation = %#v", observation)
	}
}

func TestObserveLegacyProgressPropagatesSourceFailure(t *testing.T) {
	want := errors.New("source unavailable")
	_, err := sddstatus.ObserveLegacyProgress(context.Background(), legacyProgressSource{err: want}, "change")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want source failure", err)
	}
}

func TestLegacyProgressEquivalent(t *testing.T) {
	canonical := legacyProgressSnapshot(t, "change")
	divergent := legacyProgressSnapshot(t, "other-change")
	for _, tt := range []struct {
		name        string
		left, right sddstatus.LegacyProgressObservation
		want        bool
	}{
		{name: "both absent", want: true},
		{name: "one present", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "complete"}},
		{name: "matching legacy", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "complete"}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "complete"}, want: true},
		{name: "legacy state mismatch", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "complete"}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: "complete"}},
		{name: "legacy content mismatch", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "complete"}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "partial"}},
		{name: "blank legacy content", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone}},
		{name: "whitespace legacy content", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: " \t\n"}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: " \t\n"}},
		{name: "blank legacy state", left: sddstatus.LegacyProgressObservation{Present: true, Content: "complete"}, right: sddstatus.LegacyProgressObservation{Present: true, Content: "complete"}},
		{name: "matching blocked states", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactBlockedInvalid, Content: "same"}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactBlockedInvalid, Content: "same"}},
		{name: "different blocked states", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactBlockedInvalid, Content: "same"}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactBlockedBackendDiverged, Content: "same"}},
		{name: "normalized canonical v2", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: canonical}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactPartial, Content: canonical}, want: true},
		{name: "divergent canonical v2", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: canonical}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: divergent}},
		{name: "canonical and legacy", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: canonical}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: "complete"}},
		{name: "matching malformed v2", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: `{"schema":"jarvis.sdd-apply-progress/v2"`}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: `{"schema":"jarvis.sdd-apply-progress/v2"`}},
		{name: "matching v2 lookalike", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: ` {"schema":"jarvis.sdd-apply-progress/v2"}`}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: ` {"schema":"jarvis.sdd-apply-progress/v2"}`}},
		{name: "matching spaced v2 lookalike", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: `{"schema" : "jarvis.sdd-apply-progress/v2"}`}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: `{"schema" : "jarvis.sdd-apply-progress/v2"}`}},
		{name: "matching escaped v2 lookalike", left: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: `{"schema":"jarvis.sdd-apply-progress\/v2"}`}, right: sddstatus.LegacyProgressObservation{Present: true, State: sddstatus.ArtifactDone, Content: `{"schema":"jarvis.sdd-apply-progress\/v2"}`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sddstatus.LegacyProgressEquivalent(tt.left, tt.right); got != tt.want {
				t.Fatalf("LegacyProgressEquivalent() = %t, want %t", got, tt.want)
			}
		})
	}
}

func legacyProgressSnapshot(t *testing.T, change string) string {
	t.Helper()
	_, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{
		Schema:             applyprogress.SnapshotSchema,
		Project:            "jarvis-dev",
		Change:             change,
		Generation:         1,
		Revision:           1,
		TaskManifestSHA256: strings.Repeat("a", 64),
		Status:             applyprogress.StatusPartial,
		Coverage:           []applyprogress.Coverage{},
		Batches:            []applyprogress.BatchRef{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type legacyProgressSource struct {
	artifacts map[string]sddstatus.ArtifactState
	contents  map[string]string
	err       error
}

func (s legacyProgressSource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	return s.artifacts, s.contents, s.err
}

func (legacyProgressSource) ListChanges(context.Context) ([]string, error) { return nil, nil }

func newHiveSource(t *testing.T, baseURL string) *sddstatus.HiveSource {
	return hiveSourceForProject(t, baseURL, "jarvis-dev")
}

func hiveSourceForProject(t *testing.T, baseURL, project string) *sddstatus.HiveSource {
	t.Helper()
	client, err := hiveclient.New(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	return sddstatus.NewHiveSource(client, project)
}
