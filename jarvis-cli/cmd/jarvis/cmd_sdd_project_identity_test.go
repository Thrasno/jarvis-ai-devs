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
	"strings"
	"sync"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

const identityChange = "change"

// identityDaemon models the post-#798 daemon contract: apply-progress and
// store-binding routes fold the spelling and resolve aliases at ingress and
// report the effective key, while the artifact routes still only fold spelling.
type identityDaemon struct {
	t         *testing.T
	effective string
	aliases   map[string]string
	mode      string
	snapshot  []byte
	// noProgress serves a typed not_found head carrying the effective key.
	noProgress bool
	// omitProjectKey models an apply-progress route that does not report the
	// effective key, as older daemons do.
	omitProjectKey bool
	// unbound starts without a stored binding; adoption stores it under the
	// effective key, as the daemon does for a retired alias.
	unbound bool

	adopts []string

	mu       sync.Mutex
	projects []string
	advances []hiveclient.ApplyProgressAdvanceRequest
}

func (d *identityDaemon) resolve(raw string) string {
	key := hiveclient.CanonicalProjectKey(raw)
	if target, ok := d.aliases[key]; ok {
		return target
	}
	return key
}

func (d *identityDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	project := r.URL.Query().Get("project")
	if r.Method == http.MethodGet {
		d.projects = append(d.projects, project)
	}
	base := "/sdd/changes/" + identityChange
	switch {
	case r.Method == http.MethodPost && r.URL.Path == base+"/store-binding/adopt":
		var request struct {
			Project    string `json:"project"`
			Mode       string `json:"mode"`
			Provenance string `json:"provenance"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			d.t.Fatal(err)
		}
		key := d.resolve(request.Project)
		d.adopts = append(d.adopts, request.Project)
		created := d.unbound
		d.unbound, d.mode = false, request.Mode
		if created {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":%q,"created_at":"2026-08-01T10:00:00Z"},"created":%t}`, key, identityChange, request.Mode, request.Provenance, created)
	case r.Method == http.MethodGet && r.URL.Path == base+"/store-binding":
		if d.unbound || d.resolve(project) != d.effective {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, d.effective, identityChange, d.mode)
	case r.Method == http.MethodGet && r.URL.Path == base+"/artifacts":
		if hiveclient.CanonicalProjectKey(project) != d.effective {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error":"project not found"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"explore","content":"# Explore"},{"artifact":"proposal","content":"# Proposal"},{"artifact":"spec","content":"# Spec"},{"artifact":"design","content":"# Design"},{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`)
	case r.Method == http.MethodGet && r.URL.Path == base+"/apply-progress":
		key := d.resolve(project)
		if key != d.effective {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"outcome":"not_found","code":"project_not_found"}`)
			return
		}
		if d.noProgress || d.snapshot == nil {
			w.WriteHeader(http.StatusNotFound)
			if d.omitProjectKey {
				_, _ = fmt.Fprint(w, `{"outcome":"not_found","code":"not_found"}`)
				return
			}
			_, _ = fmt.Fprintf(w, `{"outcome":"not_found","code":"not_found","project_key":%q}`, key)
			return
		}
		var snapshot applyprogress.Snapshot
		if err := json.Unmarshal(d.snapshot, &snapshot); err != nil {
			d.t.Fatal(err)
		}
		_, _ = fmt.Fprintf(w, `{"outcome":"committed","code":"ok","project_key":%q,"state":{"generation":%d,"revision":%d,"digest":%q,"snapshot":%s}}`, key, snapshot.Generation, snapshot.Revision, snapshot.Digest, d.snapshot)
	case r.Method == http.MethodPost && r.URL.Path == base+"/apply-progress/advance":
		var request hiveclient.ApplyProgressAdvanceRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			d.t.Fatal(err)
		}
		key := d.resolve(request.Project)
		if key != d.effective || request.Snapshot.Project != key {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = fmt.Fprintf(w, `{"outcome":"invalid","code":"validation","project_key":%q}`, key)
			return
		}
		d.advances = append(d.advances, request)
		result := hiveclient.ApplyProgressResult{Outcome: "committed", Code: "ok", State: hiveclient.ApplyProgressState{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, Snapshot: request.Snapshot}, Receipt: daemonFormatReceipt(d.t, request)}
		body, err := json.Marshal(result)
		if err != nil {
			d.t.Fatal(err)
		}
		_, _ = w.Write(bytes.Replace(body, []byte(`{"outcome"`), []byte(`{"project_key":"`+key+`","outcome"`), 1))
	default:
		d.t.Errorf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *identityDaemon) requireOnlyCanonicalProjects(t *testing.T) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, project := range d.projects {
		if project != hiveclient.CanonicalProjectKey(project) {
			t.Fatalf("daemon received non-canonical project spelling %q (all: %q)", project, d.projects)
		}
	}
}

func identityPartialHead(t *testing.T, project string) []byte {
	t.Helper()
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	_, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: project, Change: identityChange, Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// identityWorkspace prepares the local side of a bound change. Hybrid needs the
// same authenticated head published through the OpenSpec writer.
func identityWorkspace(t *testing.T, mode string, head []byte) string {
	t.Helper()
	workspace := canonicalSddTestWorkspace(t)
	if mode != "hybrid" {
		return workspace
	}
	writeOpenSpecBinding(t, workspace, identityChange, "hybrid", "persisted:test", map[string]string{"tasks.md": "- [ ] 1.1 task\n"})
	var snapshot applyprogress.Snapshot
	if err := json.Unmarshal(head, &snapshot); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(workspace, "openspec", "changes", identityChange)
	if _, err := (sddprogress.OpenSpec{Root: root}).Advance(sddprogress.AdvanceRequest{RequestID: "identity-head", Snapshot: snapshot, Batches: []applyprogress.Batch{}}); err != nil {
		t.Fatalf("publish OpenSpec head: %v", err)
	}
	return workspace
}

type identityRun struct {
	status    sddstatus.ChangeStatus
	raw       string
	continued string
	err       error
}

func runIdentityStatusAndContinue(t *testing.T, projectFlag, workspace string) identityRun {
	t.Helper()
	var run identityRun
	raw, err := captureSDDStdout(t, func() error { return runSddStatus(identityChange, projectFlag, workspace, true, false) })
	if err != nil {
		t.Fatalf("status --project %q: %v", projectFlag, err)
	}
	if err := json.Unmarshal([]byte(raw), &run.status); err != nil {
		t.Fatalf("decode status --project %q: %v\n%s", projectFlag, err, raw)
	}
	run.raw = raw
	run.continued, run.err = captureSDDStdout(t, func() error { return runSddContinue(identityChange, projectFlag, workspace, false) })
	return run
}

func TestSddStatusAndContinueCanonicalizeProjectSpellings(t *testing.T) {
	for _, mode := range []string{"hive", "hybrid"} {
		for _, group := range []struct {
			canonical string
			spellings []string
		}{
			{canonical: "myproject", spellings: []string{"MyProject", " MYPROJECT ", "myproject"}},
			{canonical: "my-project", spellings: []string{"my_project", "My.Project", "my-project"}},
		} {
			t.Run(mode+"/"+group.canonical, func(t *testing.T) {
				head := identityPartialHead(t, group.canonical)
				workspace := identityWorkspace(t, mode, head)
				daemon := &identityDaemon{t: t, effective: group.canonical, mode: mode, snapshot: head}
				server := httptest.NewServer(daemon)
				t.Cleanup(server.Close)
				t.Setenv("HIVE_DAEMON_URL", server.URL)

				baseline := runIdentityStatusAndContinue(t, group.canonical, workspace)
				if got := baseline.status.Artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
					t.Fatalf("canonical apply-progress = %q, want partial", got)
				}
				if got := baseline.status.Dependencies[sddstatus.PhaseApply]; got != sddstatus.DepReady {
					t.Fatalf("canonical sdd-apply = %q, want ready", got)
				}
				for _, spelling := range group.spellings {
					got := runIdentityStatusAndContinue(t, spelling, workspace)
					if got.raw != baseline.raw {
						t.Fatalf("status --project %q differs from canonical:\n%s\nwant:\n%s", spelling, got.raw, baseline.raw)
					}
					if got.continued != baseline.continued || (got.err == nil) != (baseline.err == nil) {
						t.Fatalf("continue --project %q = %q (%v), want %q (%v)", spelling, got.continued, got.err, baseline.continued, baseline.err)
					}
				}
				daemon.requireOnlyCanonicalProjects(t)
			})
		}
	}
}

func TestSddStatusRetiredAliasResolvesTargetBindingAndProgress(t *testing.T) {
	for _, mode := range []string{"hive", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			head := identityPartialHead(t, "target")
			workspace := identityWorkspace(t, mode, head)
			daemon := &identityDaemon{t: t, effective: "target", aliases: map[string]string{"retired": "target"}, mode: mode, snapshot: head}
			server := httptest.NewServer(daemon)
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)

			baseline := runIdentityStatusAndContinue(t, "target", workspace)
			if got := baseline.status.Dependencies[sddstatus.PhaseApply]; got != sddstatus.DepReady {
				t.Fatalf("target sdd-apply = %q, want ready", got)
			}
			for _, spelling := range []string{"retired", "Retired"} {
				got := runIdentityStatusAndContinue(t, spelling, workspace)
				if got.raw != baseline.raw || got.continued != baseline.continued {
					t.Fatalf("--project %q status/continue differ from target:\n%s%s\nwant:\n%s%s", spelling, got.raw, got.continued, baseline.raw, baseline.continued)
				}
			}
		})
	}
}

func TestSddStatusRetiredAliasAdoptsBindingUnderTarget(t *testing.T) {
	daemon := &identityDaemon{t: t, effective: "target", aliases: map[string]string{"retired": "target"}, snapshot: identityPartialHead(t, "target"), unbound: true}
	server := httptest.NewServer(daemon)
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "hive")

	run := runIdentityStatusAndContinue(t, "Retired", canonicalSddTestWorkspace(t))
	if run.status.StoreBinding == nil || !run.status.StoreBinding.Persisted || run.status.Artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactPartial {
		t.Fatalf("status = %s", run.raw)
	}
	if run.status.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Fatalf("sdd-apply = %q, want ready", run.status.Dependencies[sddstatus.PhaseApply])
	}
	// The daemon redirects the single adoption to the target; the CLI confirms
	// the effective key and reads the stored binding back without a second write.
	if len(daemon.adopts) != 1 || daemon.unbound {
		t.Fatalf("adoptions = %q, want exactly one stored adoption", daemon.adopts)
	}
}

func TestSddStatusReportsProjectMismatchInsteadOfInvalid(t *testing.T) {
	daemon := &identityDaemon{t: t, effective: "my-project", mode: "hive", snapshot: identityPartialHead(t, "foreign")}
	server := httptest.NewServer(daemon)
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	workspace := canonicalSddTestWorkspace(t)

	raw, err := captureSDDStdout(t, func() error { return runSddStatus(identityChange, "My_Project", workspace, true, false) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var status sddstatus.ChangeStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatal(err)
	}
	if got := status.Artifacts[sddstatus.ArtifactApplyProgress]; got != "blocked:project_mismatch" {
		t.Fatalf("apply-progress = %q, want blocked:project_mismatch", got)
	}
	if _, err := captureSDDStdout(t, func() error { return runSddContinue(identityChange, "my-project", workspace, false) }); err == nil {
		t.Fatal("continue routed a project mismatch")
	}
}

func runIdentityCheckpoint(t *testing.T, workspace string, request checkpointInput) (checkpointOutput, error) {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	command := newBoundSddProgressCommand(resolveBoundProgressStore)
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"checkpoint", "--request", path})
	runErr := command.Execute()
	var output checkpointOutput
	if strings.TrimSpace(out.String()) != "" {
		if err := json.Unmarshal(out.Bytes(), &output); err != nil {
			t.Fatalf("decode checkpoint output %q: %v", out.String(), err)
		}
	}
	return output, runErr
}

func identityCheckpointRequest(project string) checkpointInput {
	return checkpointInput{Project: project, Change: identityChange, Tasks: []applyprogress.Task{{ID: "1.1", Text: "task"}}, Entries: []applyprogress.EvidenceEntry{checkpointEntry("entry-1", "1.1", "green")}, EntryID: "entry-1"}
}

func TestSddProgressCheckpointWritesUnderEffectiveProjectKey(t *testing.T) {
	for _, tt := range []struct {
		name      string
		project   string
		effective string
		aliases   map[string]string
	}{
		{name: "canonical key without alias", project: "my-project", effective: "my-project"},
		{name: "non-canonical spelling", project: "My_Project", effective: "my-project"},
		{name: "retired alias", project: "Retired", effective: "target", aliases: map[string]string{"retired": "target"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemon := &identityDaemon{t: t, effective: tt.effective, aliases: tt.aliases, mode: "hive", noProgress: true}
			server := httptest.NewServer(daemon)
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)

			output, err := runIdentityCheckpoint(t, canonicalSddTestWorkspace(t), identityCheckpointRequest(tt.project))
			if err != nil {
				t.Fatalf("checkpoint: %v (output %#v)", err, output)
			}
			if output.Outcome != string(applyprogress.PlanCommitted) || output.Snapshot == nil || output.Snapshot.Project != tt.effective {
				t.Fatalf("output = %#v, want committed snapshot under %q", output, tt.effective)
			}
			if len(daemon.advances) != 1 || daemon.advances[0].Project != tt.effective || daemon.advances[0].Snapshot.Project != tt.effective {
				t.Fatalf("advances = %#v, want one write under %q", daemon.advances, tt.effective)
			}
			daemon.requireOnlyCanonicalProjects(t)
		})
	}
}

func TestSddProgressCheckpointReportsProjectMismatchWithoutWriting(t *testing.T) {
	head := identityPartialHead(t, "foreign")
	var snapshot applyprogress.Snapshot
	if err := json.Unmarshal(head, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		withBase bool
	}{
		{name: "base from another project", withBase: true},
		{name: "current head from another project"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemon := &identityDaemon{t: t, effective: "my-project", mode: "hive", snapshot: head}
			server := httptest.NewServer(daemon)
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)

			request := identityCheckpointRequest("my-project")
			if tt.withBase {
				request.Base = &snapshot
				request.ExpectedGeneration, request.ExpectedRevision, request.ExpectedDigest = snapshot.Generation, snapshot.Revision, snapshot.Digest
			}
			output, err := runIdentityCheckpoint(t, canonicalSddTestWorkspace(t), request)
			if !errors.Is(err, sddstatus.ErrProjectMismatch) || output.Code != "project_mismatch" || output.Next == nil || output.Next.Action != checkpointNextStopBlocked {
				t.Fatalf("output = %#v, err = %v; want project_mismatch with stop_blocked", output, err)
			}
			if len(daemon.advances) != 0 {
				t.Fatalf("project mismatch wrote progress: %#v", daemon.advances)
			}
		})
	}
}

func TestSddProgressAdvanceReportsProjectMismatchWithoutWriting(t *testing.T) {
	for _, tt := range []struct {
		name            string
		snapshotProject string
		aliases         map[string]string
	}{
		{name: "snapshot signed under a non-canonical spelling", snapshotProject: "My_Project"},
		{name: "snapshot signed under a retired key", snapshotProject: "retired", aliases: map[string]string{"retired": "my-project"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemon := &identityDaemon{t: t, effective: "my-project", aliases: tt.aliases, mode: "hive", noProgress: true}
			server := httptest.NewServer(daemon)
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			workspace := canonicalSddTestWorkspace(t)
			t.Chdir(workspace)

			request := crossCommandAdvanceRequest(t, tt.snapshotProject, identityChange, "identity-advance")
			data, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "advance.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			command := newBoundSddProgressCommand(resolveBoundProgressStore)
			var out bytes.Buffer
			command.SetOut(&out)
			command.SetErr(&bytes.Buffer{})
			command.SetArgs([]string{"advance", "--request", path})
			err = command.Execute()
			var output progressAdvanceOutput
			if decodeErr := json.Unmarshal(out.Bytes(), &output); decodeErr != nil {
				t.Fatalf("decode advance output %q: %v", out.String(), decodeErr)
			}
			if !errors.Is(err, sddstatus.ErrProjectMismatch) || output.Code != "project_mismatch" {
				t.Fatalf("output = %#v, err = %v; want project_mismatch", output, err)
			}
			if len(daemon.advances) != 0 {
				t.Fatalf("project mismatch wrote progress: %#v", daemon.advances)
			}
		})
	}
}

func TestSddStatusRejectsUnconfirmedBindingRedirect(t *testing.T) {
	daemon := &identityDaemon{t: t, effective: "target", aliases: map[string]string{"retired": "target"}, mode: "hive", noProgress: true, omitProjectKey: true}
	server := httptest.NewServer(daemon)
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)

	_, err := captureSDDStdout(t, func() error {
		return runSddStatus(identityChange, "retired", canonicalSddTestWorkspace(t), true, false)
	})
	if !errors.Is(err, sddstatus.ErrProjectMismatch) {
		t.Fatalf("err = %v, want project mismatch for an unconfirmed redirect", err)
	}
}

func TestBoundSddArchiveCanonicalizesProjectSpelling(t *testing.T) {
	const change = "issue-723"
	for _, flag := range []string{"my-project", "My_Project", " MY.PROJECT "} {
		t.Run(flag, func(t *testing.T) {
			workspace := canonicalSddTestWorkspace(t)
			t.Chdir(workspace)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sdd/changes/" + change + "/store-binding":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=my-project", "")
					_, _ = fmt.Fprintf(w, `{"binding":{"project":"my-project","change":%q,"schema_version":"1","mode":"hive","provenance":"persisted:test","created_at":"2026-08-01T10:00:00Z"}}`, change)
				case "/sdd/changes/" + change + "/artifacts":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=my-project", "")
					_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"proposal","content":"# Proposal"},{"artifact":"spec","content":"# Spec"},{"artifact":"design","content":"# Design"},{"artifact":"tasks","content":"- [x] 1.1 task\n"},{"artifact":"apply-progress","content":"status: complete\n"},{"artifact":"verify-report","content":"## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"},{"artifact":"archive-report","content":"# Archive report\n"}]}`)
				case "/sdd/changes/" + change + "/apply-progress":
					requireSDDRequest(t, r, http.MethodGet, r.URL.Path, "project=my-project", "")
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"outcome":"unavailable","code":"compatibility"}`)
				default:
					t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("HIVE_DAEMON_URL", server.URL)

			command := newBoundSddArchiveCommand()
			command.SetArgs([]string{"--project", flag, "--change", change})
			if err := command.Execute(); err != nil {
				t.Fatalf("archive --project %q: %v", flag, err)
			}
		})
	}
}
