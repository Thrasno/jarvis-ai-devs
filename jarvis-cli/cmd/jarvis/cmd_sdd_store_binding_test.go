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
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
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
			writerStarted := make(chan struct{}, 1)
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
					writerStarted <- struct{}{}
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

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
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
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("writer error = %v, want context deadline exceeded", err)
			}
			select {
			case <-writerStarted:
			default:
				t.Fatal("writer adapter was not reached")
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
