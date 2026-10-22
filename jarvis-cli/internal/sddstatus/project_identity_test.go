package sddstatus_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

// identityProgressServer serves one guarded partial head signed under
// snapshotProject. projectKey is the effective key the daemon reports; an empty
// value models an older daemon that omits project_key. Artifacts are served
// only to artifactsProject, as a daemon without alias-aware artifact routes does.
func identityProgressServer(t *testing.T, snapshotProject, projectKey, artifactsProject string) *httptest.Server {
	t.Helper()
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: snapshotProject, Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	keyField := ""
	if projectKey != "" {
		keyField = fmt.Sprintf(`"project_key":%q,`, projectKey)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/change/artifacts":
			if got := r.URL.Query().Get("project"); got != artifactsProject {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"project not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`))
		case "/sdd/changes/change/apply-progress":
			_, _ = fmt.Fprintf(w, `{"outcome":"committed","code":"ok",%s"state":{"generation":%d,"revision":%d,"digest":%q,"snapshot":%s}}`, keyField, snapshot.Generation, snapshot.Revision, snapshot.Digest, data)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestHiveSourceComparesProgressAgainstDaemonEffectiveKey(t *testing.T) {
	for _, tt := range []struct {
		name             string
		requested        projectidentity.Key
		snapshotProject  string
		projectKey       string
		artifactsProject string
		want             sddstatus.ArtifactState
	}{
		{name: "canonical key without alias", requested: "my-project", snapshotProject: "my-project", projectKey: "my-project", artifactsProject: "my-project", want: sddstatus.ArtifactPartial},
		{name: "older daemon without project_key", requested: "my-project", snapshotProject: "my-project", artifactsProject: "my-project", want: sddstatus.ArtifactPartial},
		{name: "retired alias reads target progress", requested: "retired", snapshotProject: "target", projectKey: "target", artifactsProject: "target", want: sddstatus.ArtifactPartial},
		{name: "snapshot differs from effective key", requested: "my-project", snapshotProject: "foreign", projectKey: "my-project", artifactsProject: "my-project", want: sddstatus.ArtifactBlockedProjectMismatch},
		{name: "older daemon snapshot differs from requested key", requested: "my-project", snapshotProject: "foreign", artifactsProject: "my-project", want: sddstatus.ArtifactBlockedProjectMismatch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := identityProgressServer(t, tt.snapshotProject, tt.projectKey, tt.artifactsProject)
			artifacts, _, err := hiveSourceForProject(t, server.URL, tt.requested).FetchArtifacts(context.Background(), "change")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress = %q, want %q", got, tt.want)
			}
			if artifacts[sddstatus.ArtifactTasks] != sddstatus.ArtifactDone {
				t.Fatalf("tasks = %q, want artifacts read under the effective key", artifacts[sddstatus.ArtifactTasks])
			}
		})
	}
}

func TestHiveInspectSealablePredecessorUsesEffectiveKey(t *testing.T) {
	for _, tt := range []struct {
		name            string
		snapshotProject string
		projectKey      string
		wantMismatch    bool
	}{
		{name: "retired alias resolves to target head", snapshotProject: "target", projectKey: "target"},
		{name: "head differs from effective key", snapshotProject: "foreign", projectKey: "target", wantMismatch: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := identityProgressServer(t, tt.snapshotProject, tt.projectKey, "target")
			snapshot, err := hiveSourceForProject(t, server.URL, "retired").InspectSealablePredecessor(context.Background(), "change")
			if tt.wantMismatch {
				if !errors.Is(err, sddstatus.ErrProjectMismatch) {
					t.Fatalf("err = %v, want project mismatch", err)
				}
				return
			}
			if err != nil || snapshot.Project != "target" {
				t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
			}
		})
	}
}

func writeIdentityOpenSpecHead(t *testing.T, project string) string {
	t.Helper()
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	_, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: project, Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "change")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"apply-progress.md": data, "tasks.md": []byte("- [ ] 1.1 task\n")} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestOpenSpecSourceReportsProjectMismatch(t *testing.T) {
	for _, tt := range []struct {
		name     string
		project  string
		expected projectidentity.Key
		want     sddstatus.ArtifactState
	}{
		{name: "matching canonical key", project: "my-project", expected: "my-project", want: sddstatus.ArtifactPartial},
		{name: "foreign project", project: "foreign", expected: "my-project", want: sddstatus.ArtifactBlockedProjectMismatch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := writeIdentityOpenSpecHead(t, tt.project)
			artifacts, _, err := sddstatus.NewOpenSpecSourceForProject(root, tt.expected).FetchArtifacts(context.Background(), "change")
			if err != nil {
				t.Fatal(err)
			}
			if got := artifacts[sddstatus.ArtifactApplyProgress]; got != tt.want {
				t.Fatalf("apply-progress = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHybridSourceSurfacesProjectMismatch(t *testing.T) {
	root := writeIdentityOpenSpecHead(t, "foreign")
	server := identityProgressServer(t, "my-project", "my-project", "my-project")
	hybrid := sddstatus.NewHybridSource(hiveSourceForProject(t, server.URL, "my-project"), sddstatus.NewOpenSpecSourceForProject(root, "my-project"))
	artifacts, _, err := hybrid.FetchArtifacts(context.Background(), "change")
	if err != nil {
		t.Fatal(err)
	}
	if got := artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactBlockedProjectMismatch {
		t.Fatalf("apply-progress = %q, want project mismatch", got)
	}
}
