package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

type sddBindingTestSource struct {
	changes []string
	err     error
}

func (s sddBindingTestSource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	return map[string]sddstatus.ArtifactState{}, map[string]string{}, nil
}

func (s sddBindingTestSource) ListChanges(context.Context) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.changes, nil
}

func requireSDDRequest(t *testing.T, r *http.Request, method, path, query, body string) {
	t.Helper()
	if r.Method != method || r.URL.Path != path || r.URL.RawQuery != query {
		t.Fatalf("request = %s %s?%s, want %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery, method, path, query)
	}
	gotBody, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if body == "" {
		if len(gotBody) != 0 {
			t.Fatalf("request body = %q, want empty", gotBody)
		}
		return
	}
	var got, want any
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("decode request body %q: %v", gotBody, err)
	}
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatalf("decode expected request body %q: %v", body, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request body = %#v, want %#v", got, want)
	}
}

func captureSDDStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		_ = writer.Close()
		_ = reader.Close()
	}()

	type readResult struct {
		output []byte
		err    error
	}
	readDone := make(chan readResult, 1)
	go func() {
		output, err := io.ReadAll(reader)
		readDone <- readResult{output: output, err: err}
	}()

	runErr := run()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	os.Stdout = original
	result := <-readDone
	if result.err != nil {
		t.Fatalf("read stdout pipe: %v", result.err)
	}
	return string(result.output), runErr
}

func TestCaptureSDDStdoutDrainsLargeOutput(t *testing.T) {
	payload := strings.Repeat("x", 1<<20)
	output, err := captureSDDStdout(t, func() error {
		_, err := os.Stdout.Write([]byte(payload))
		return err
	})
	if err != nil || output != payload {
		t.Fatalf("captured %d bytes, want %d: %v", len(output), len(payload), err)
	}
}

func writeOpenSpecBinding(t *testing.T, workspace, change, mode, provenance string, files map[string]string) {
	t.Helper()
	changeDir := filepath.Join(workspace, "openspec", "changes", change)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("create OpenSpec change directory: %v", err)
	}
	state := "jarvis_sdd_binding:\n  schema_version: 1\n  mode: " + mode + "\n  provenance: " + provenance + "\n"
	if err := os.WriteFile(filepath.Join(changeDir, "state.yaml"), []byte(state), 0o600); err != nil {
		t.Fatalf("write OpenSpec binding: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write OpenSpec artifact %s: %v", name, err)
		}
	}
}

func TestResolveBoundStatusSourceAt_NormalizesExplicitChangeExactlyOnce(t *testing.T) {
	workspace := t.TempDir()
	const change = "change with spaces"
	writeOpenSpecBinding(t, workspace, change, "openspec", "persisted:openspec", map[string]string{"proposal.md": "# Proposal\n"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/change with spaces/store-binding", "project=project", "")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)

	gotChange, source, binding, err := resolveBoundStatusSourceAt(context.Background(), "project", "  "+change+"  ", workspace)
	if err != nil {
		t.Fatalf("resolve bound status source: %v", err)
	}
	if gotChange != change {
		t.Fatalf("change = %q, want %q", gotChange, change)
	}
	if _, ok := source.(*sddstatus.OpenSpecSource); !ok {
		t.Fatalf("source = %T, want *sddstatus.OpenSpecSource", source)
	}
	if binding.Mode != sddruntime.StoreModeOpenSpec || binding.Provenance != "persisted:openspec" || !binding.Persisted {
		t.Fatalf("binding = %#v", binding)
	}
	status, err := buildStatusWithBinding(gotChange, source, string(binding.Mode), nil, bindingStatus(binding))
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.ChangeName != change || status.Artifacts[sddstatus.ArtifactProposal] != sddstatus.ArtifactDone {
		t.Fatalf("status = %#v, want canonical OpenSpec coordinate", status)
	}
}

func TestNormalizeExplicitChangeName_RejectsBlankExplicitName(t *testing.T) {
	change, explicit, err := normalizeExplicitChangeName(" \t ")
	if err == nil || !explicit || change != "" || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("normalize explicit blank = change:%q explicit:%t err:%v", change, explicit, err)
	}
}

func TestResolveBoundStatusSourceAt_PersistedHiveBindingIgnoresInvalidEnvironmentWithoutOpenSpecDirectory(t *testing.T) {
	workspace := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireSDDRequest(t, r, http.MethodGet, "/sdd/changes/change/store-binding", "project=project", "")
		_, _ = fmt.Fprint(w, `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "invalid")

	change, source, binding, err := resolveBoundStatusSourceAt(context.Background(), "project", "change", workspace)
	if err != nil {
		t.Fatalf("resolve bound status source: %v", err)
	}
	if change != "change" {
		t.Fatalf("change = %q, want change", change)
	}
	if _, ok := source.(*sddstatus.HiveSource); !ok {
		t.Fatalf("source = %T, want *sddstatus.HiveSource", source)
	}
	if binding.Mode != sddruntime.StoreModeHive || binding.Provenance != "persisted:test" || !binding.Persisted {
		t.Fatalf("binding = %#v", binding)
	}
	if _, err := os.Stat(filepath.Join(workspace, "openspec", "changes", "change")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenSpec change directory error = %v, want absence", err)
	}
}

func TestResolveBoundStatusSourceAt_SelectsEffectiveStoreAcrossBindings(t *testing.T) {
	const change = "change"
	tests := []struct {
		name            string
		environmentMode string
		hiveBinding     string
		localBinding    string
		provenance      string
		localFiles      map[string]string
		hiveArtifacts   string
		wantSource      string
		wantArtifacts   []string
		wantPersisted   bool
		wantAdoption    bool
	}{
		{
			name:            "Hive",
			environmentMode: "hive",
			provenance:      "initial:environment:JARVIS_SDD_STORE_MODE",
			hiveArtifacts:   `{"artifacts":[{"artifact":"proposal","content":"# Hive proposal","created_at":"2026-08-01T10:00:00Z"}]}`,
			wantSource:      "hive",
			wantArtifacts:   []string{sddstatus.ArtifactProposal},
			wantPersisted:   true,
			wantAdoption:    true,
		},
		{
			name:            "OpenSpec",
			environmentMode: "invalid",
			localBinding:    "openspec",
			provenance:      "persisted:openspec",
			localFiles:      map[string]string{"design.md": "# OpenSpec design\n"},
			wantSource:      "openspec",
			wantArtifacts:   []string{sddstatus.ArtifactDesign},
			wantPersisted:   true,
		},
		{
			name:            "Hybrid",
			environmentMode: "invalid",
			hiveBinding:     "hybrid",
			localBinding:    "hybrid",
			provenance:      "persisted:hybrid",
			localFiles:      map[string]string{"design.md": "# OpenSpec design\n"},
			hiveArtifacts:   `{"artifacts":[{"artifact":"proposal","content":"# Hive proposal","created_at":"2026-08-01T10:00:00Z"}]}`,
			wantSource:      "hybrid",
			wantArtifacts:   []string{sddstatus.ArtifactProposal, sddstatus.ArtifactDesign},
			wantPersisted:   true,
		},
		{
			name:            "None",
			environmentMode: "none",
			provenance:      "initial:environment:JARVIS_SDD_STORE_MODE",
			wantSource:      "none",
			wantPersisted:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			if tt.localBinding != "" {
				writeOpenSpecBinding(t, workspace, change, tt.localBinding, tt.provenance, tt.localFiles)
			}
			postRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/change/store-binding":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=project", "")
					if tt.hiveBinding == "" {
						w.WriteHeader(http.StatusNotFound)
						_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
						return
					}
					_, _ = fmt.Fprintf(w, `{"binding":{"project":"project","change":"change","schema_version":"1","mode":%q,"provenance":%q,"created_at":"2026-08-01T10:00:00Z"}}`, tt.hiveBinding, tt.provenance)
				case "/sdd/changes/change/store-binding/adopt":
					postRequests++
					requireSDDRequest(t, r, http.MethodPost, r.URL.Path, "", `{"project":"project","mode":"hive","provenance":"initial:environment:JARVIS_SDD_STORE_MODE"}`)
					w.WriteHeader(http.StatusCreated)
					_, _ = fmt.Fprint(w, `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"initial:environment:JARVIS_SDD_STORE_MODE","created_at":"2026-08-01T10:00:00Z"},"created":true}`)
				case "/sdd/changes/change/artifacts":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=project", "")
					if tt.hiveArtifacts == "" {
						_, _ = fmt.Fprint(w, `{"artifacts":[]}`)
						return
					}
					_, _ = fmt.Fprint(w, tt.hiveArtifacts)
				case "/sdd/changes/change/apply-progress":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=project", "")
					_, _ = fmt.Fprint(w, `{"outcome":"none"}`)
				default:
					t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			t.Setenv("JARVIS_SDD_STORE_MODE", tt.environmentMode)

			gotChange, source, binding, err := resolveBoundStatusSourceAt(context.Background(), "project", change, workspace)
			if err != nil {
				t.Fatalf("resolve bound status source: %v", err)
			}
			if gotChange != change || binding.Mode != sddruntime.StoreMode(tt.wantSource) || binding.Provenance != tt.provenance || binding.Persisted != tt.wantPersisted {
				t.Fatalf("resolution = change:%q binding:%#v", gotChange, binding)
			}
			switch tt.wantSource {
			case "hive":
				if _, ok := source.(*sddstatus.HiveSource); !ok {
					t.Fatalf("source = %T, want *sddstatus.HiveSource", source)
				}
			case "openspec":
				if _, ok := source.(*sddstatus.OpenSpecSource); !ok {
					t.Fatalf("source = %T, want *sddstatus.OpenSpecSource", source)
				}
			case "hybrid":
				if _, ok := source.(*sddstatus.HybridSource); !ok {
					t.Fatalf("source = %T, want *sddstatus.HybridSource", source)
				}
			case "none":
				if _, ok := source.(noneArtifactSource); !ok {
					t.Fatalf("source = %T, want noneArtifactSource", source)
				}
			}
			status, err := buildStatusWithBinding(gotChange, source, string(binding.Mode), nil, bindingStatus(binding))
			if err != nil {
				t.Fatalf("build status: %v", err)
			}
			if status.ArtifactStore != tt.wantSource {
				t.Fatalf("status artifact store = %q, want %q", status.ArtifactStore, tt.wantSource)
			}
			if status.StoreBinding == nil || status.StoreBinding.Mode != tt.wantSource || status.StoreBinding.Provenance != tt.provenance || status.StoreBinding.Persisted != tt.wantPersisted {
				t.Fatalf("status store binding = %#v", status.StoreBinding)
			}
			for _, artifact := range tt.wantArtifacts {
				if status.Artifacts[artifact] != sddstatus.ArtifactDone {
					t.Fatalf("status artifact %q = %q, want done", artifact, status.Artifacts[artifact])
				}
			}
			if tt.wantSource == "none" && status.StoreBinding.Persisted {
				t.Fatalf("none binding persisted = true, want false")
			}
			if (postRequests > 0) != tt.wantAdoption {
				t.Fatalf("POST adoption requests = %d, want adoption=%t", postRequests, tt.wantAdoption)
			}
		})
	}
}

func TestSddStatusAndContinue_UsePersistedBindingBeforeInvalidEnvironmentSelection(t *testing.T) {
	workspace := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/change/store-binding":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=project", "")
			_, _ = fmt.Fprint(w, `{"binding":{"project":"project","change":"change","schema_version":"1","mode":"hive","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`)
		case "/sdd/changes/change/artifacts":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=project", "")
			_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"proposal","content":"# Proposal","created_at":"2026-08-01T10:00:00Z"},{"artifact":"apply-progress","content":"legacy progress","created_at":"2026-08-01T10:00:00Z"}]}`)
		case "/sdd/changes/change/apply-progress":
			requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=project", "")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"outcome":"unavailable","code":"compatibility"}`)
		default:
			requireSDDRequest(t, r, "", "", "", "")
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "invalid")

	for _, run := range []struct {
		name string
		run  func() error
	}{
		{name: "status", run: func() error { return runSddStatus("change", "project", workspace, false, false) }},
		{name: "continue", run: func() error { return runSddContinue("change", "project", workspace, false) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			output, err := captureSDDStdout(t, run.run)
			if err != nil {
				t.Fatalf("%s: %v", run.name, err)
			}
			switch run.name {
			case "status":
				for _, want := range []string{"Store binding: hive", "provenance: persisted:test", "persisted: true"} {
					if !strings.Contains(output, want) {
						t.Fatalf("status output missing %q:\n%s", want, output)
					}
				}
			case "continue":
				if output != "sdd-spec\n" {
					t.Fatalf("continue output = %q, want prior routing contract", output)
				}
			}
		})
	}
}

func TestResolveChangeNameAcrossStores_StrictUnionIsDeduplicatedAndDeterministic(t *testing.T) {
	change, err := resolveChangeNameAcrossStores(context.Background(), "", sddBindingTestSource{changes: []string{"change", "change"}}, sddBindingTestSource{changes: []string{"change"}})
	if err != nil {
		t.Fatalf("resolve duplicate change: %v", err)
	}
	if change != "change" {
		t.Fatalf("change = %q, want change", change)
	}

	_, err = resolveChangeNameAcrossStores(context.Background(), "", sddBindingTestSource{changes: []string{"zulu", "alpha"}}, sddBindingTestSource{changes: []string{"zulu", "bravo", "alpha"}})
	if err == nil || !strings.Contains(err.Error(), "[alpha bravo zulu]") {
		t.Fatalf("multiple change error = %v, want sorted deduplicated names", err)
	}
}

func TestResolveChangeNameAcrossStores_ListingErrorsAreNotAbsence(t *testing.T) {
	backendErr := errors.New("backend unavailable")
	for _, tt := range []struct {
		name     string
		hive     sddBindingTestSource
		openSpec sddBindingTestSource
		want     string
	}{
		{name: "Hive", hive: sddBindingTestSource{err: backendErr}, openSpec: sddBindingTestSource{}, want: "list Hive changes"},
		{name: "OpenSpec", hive: sddBindingTestSource{}, openSpec: sddBindingTestSource{err: backendErr}, want: "list OpenSpec changes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveChangeNameAcrossStores(context.Background(), "", tt.hive, tt.openSpec)
			if !errors.Is(err, backendErr) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q wrapping %v", err, tt.want, backendErr)
			}
		})
	}
}

func TestPrintStatusHuman_ShowsStoreBindingProvenance(t *testing.T) {
	status := sddstatus.ComputeStatus("change", "hive", sddstatus.Input{
		StoreBinding: &sddstatus.StoreBindingStatus{Mode: "hive", Provenance: "persisted:test", Persisted: true},
	})
	var output strings.Builder
	printStatusHuman(&output, status, false)
	if got := output.String(); !strings.Contains(got, "binding: hive") || !strings.Contains(got, "provenance: persisted:test") || !strings.Contains(got, "persisted: true") {
		t.Fatalf("human status missing binding details:\n%s", got)
	}
}
