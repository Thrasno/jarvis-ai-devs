package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestSupersedeRouteRejectsMissingCoordinatesBeforeEffects(t *testing.T) {
	cmd := newSddSupersedeCommand()
	cmd.SetArgs([]string{"--successor", "next", "--actor", "maintainer", "--reason", "revised scope"})
	cmd.SetIn(strings.NewReader("next\n"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "predecessor") {
		t.Fatalf("expected missing predecessor rejection, got %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unexpected output before coordinate validation: %q", output.String())
	}
}

func TestSupersedeRouteOpenSpecConsentAndZeroCredit(t *testing.T) {
	for _, scenario := range []struct {
		name, answer  string
		zero, success bool
	}{
		{name: "decline", answer: "wrong\n"}, {name: "eof", answer: ""}, {name: "zero-credit", zero: true, success: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			workspace, root, original, _ := preflightOpenSpec(t, sddruntime.StoreModeOpenSpec)
			if scenario.zero {
				tasksOld := "- [ ] 1.1 original\n"
				tasksNew := "- [ ] 1.1 replacement\n"
				if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(tasksOld), 0600); err != nil {
					t.Fatal(err)
				}
				// A fresh isolated store is required: do not overwrite a signed credited head.
				workspace = canonicalSddTestPath(t, t.TempDir())
				root = filepath.Join(workspace, "openspec", "changes", "issue-653")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(tasksOld), 0600); err != nil {
					t.Fatal(err)
				}
				_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "original"}})
				if err != nil {
					t.Fatal(err)
				}
				original, _, err = applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = (sddprogress.OpenSpec{Root: root}).Advance(sddprogress.AdvanceRequest{RequestID: "original-request", Snapshot: original}); err != nil {
					t.Fatal(err)
				}
				binding, err := sddbinding.New(sddruntime.StoreModeOpenSpec, "cli")
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = sddbinding.AdoptOpenSpec(root, binding); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(tasksNew), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected Hive mutation %s", r.Method)
					w.WriteHeader(405)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/store-binding") {
					w.WriteHeader(404)
					_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found"})
					return
				}
				t.Errorf("unexpected Hive endpoint %s", r.URL.Path)
				w.WriteHeader(404)
			}))
			defer server.Close()
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			cmd := newSddSupersedeCommand()
			cmd.SetArgs([]string{"--change", "issue-653", "--successor", "next", "--root", root, "--project", "jarvis-dev", "--actor", "maintainer", "--reason", "revised scope"})
			cmd.SetIn(strings.NewReader(scenario.answer))
			var output bytes.Buffer
			cmd.SetOut(&output)
			err := cmd.Execute()
			if scenario.success && err != nil {
				t.Fatal(err)
			}
			if !scenario.success && err == nil {
				t.Fatal("expected declined consent")
			}
			var head *applyprogress.Snapshot
			var readErr error
			if scenario.success {
				head, readErr = (sddprogress.OpenSpec{Root: root}).InspectPublication()
			} else {
				head, readErr = (sddprogress.OpenSpec{Root: root}).InspectSealablePredecessor("jarvis-dev", "issue-653")
			}
			if readErr != nil {
				t.Fatal(readErr)
			}
			target := filepath.Join(workspace, "openspec", "changes", "next")
			if scenario.success {
				if head.Status != applyprogress.StatusSuperseded {
					t.Fatalf("head status %s", head.Status)
				}
				genesis, err := (sddprogress.OpenSpec{Root: target}).InspectPublication()
				if err != nil || genesis == nil {
					t.Fatalf("genesis: %v", err)
				}
				if err := applyprogress.ValidateSuccessorGenesisPair(*head, *genesis); err != nil {
					t.Fatal(err)
				}
				binding, err := sddbinding.ReadOpenSpec(target)
				if err != nil || binding == nil || binding.Mode() != sddruntime.StoreModeOpenSpec {
					t.Fatalf("successor binding %v %v", binding, err)
				}
			} else {
				if head.Digest != original.Digest {
					t.Fatal("decline modified head")
				}
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("decline created target: %v", err)
				}
			}
		})
	}
}

// mutateOnRead simulates a user editing tasks while answering the consent prompt.
type mutateOnRead struct {
	answer io.Reader
	mutate func() error
	read   bool
}

func (r *mutateOnRead) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		if err := r.mutate(); err != nil {
			return 0, err
		}
	}
	return r.answer.Read(p)
}

func TestSupersedeRouteOpenSpecCreditedConsent(t *testing.T) {
	workspace, root, original, batch := preflightOpenSpec(t, sddruntime.StoreModeOpenSpec)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/store-binding") {
			t.Errorf("unexpected Hive call %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found"})
	}))
	defer server.Close()
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	target := filepath.Join(workspace, "openspec", "changes", "next")
	cmd := newSddSupersedeCommand()
	cmd.SetArgs([]string{"--change", "issue-653", "--successor", "next", "--root", root, "--project", "jarvis-dev", "--actor", "maintainer", "--reason", "revised scope"})
	cmd.SetIn(&mutateOnRead{answer: strings.NewReader("next\n"), mutate: func() error {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Errorf("target existed before consent: %v", err)
		}
		return nil
	}})
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "1") || !strings.Contains(output.String(), "next") {
		t.Fatalf("credited consent prompt missing count or successor: %q", output.String())
	}
	sealed, err := (sddprogress.OpenSpec{Root: root}).InspectPublication()
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Status != applyprogress.StatusSuperseded || sealed.TaskManifestSHA256 != original.TaskManifestSHA256 || !reflect.DeepEqual(sealed.Coverage, original.Coverage) || !reflect.DeepEqual(sealed.Batches, original.Batches) {
		t.Fatalf("credited predecessor evidence changed: %+v", sealed)
	}
	if sealed.Batches[0].SHA256 != batch.SHA256 {
		t.Fatal("credited batch receipt changed")
	}
	genesis, err := (sddprogress.OpenSpec{Root: target}).InspectPublication()
	if err != nil || genesis == nil {
		t.Fatalf("successor: %v", err)
	}
	if err := applyprogress.ValidateSuccessorGenesisPair(*sealed, *genesis); err != nil {
		t.Fatal(err)
	}
	if genesis.Generation != 1 || genesis.Revision != 1 || len(genesis.Batches) != 0 || len(genesis.Coverage) != 0 {
		t.Fatalf("successor inherited credit: %+v", genesis)
	}
	binding, err := sddbinding.ReadOpenSpec(target)
	if err != nil || binding == nil || binding.Mode() != sddruntime.StoreModeOpenSpec {
		t.Fatalf("successor binding: %v %v", binding, err)
	}
}

func TestSupersedeRouteOpenSpecTasksChangeDuringConsent(t *testing.T) {
	workspace, root, original, batch := preflightOpenSpec(t, sddruntime.StoreModeOpenSpec)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/store-binding") {
			t.Errorf("unexpected Hive call %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found"})
	}))
	defer server.Close()
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	cmd := newSddSupersedeCommand()
	cmd.SetArgs([]string{"--change", "issue-653", "--successor", "next", "--root", root, "--project", "jarvis-dev", "--actor", "maintainer", "--reason", "revised scope"})
	cmd.SetIn(&mutateOnRead{answer: strings.NewReader("next\n"), mutate: func() error {
		return os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 another revision\n- [ ] 1.2 remaining\n"), 0600)
	}})
	var output bytes.Buffer
	cmd.SetOut(&output)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "preflight changed after consent") || output.Len() == 0 {
		t.Fatalf("post-consent change not rejected: %v prompt %q", err, output.String())
	}
	head, err := (sddprogress.OpenSpec{Root: root}).InspectSealablePredecessor("jarvis-dev", "issue-653")
	if err != nil || head.Digest != original.Digest || !reflect.DeepEqual(head.Batches, original.Batches) || !reflect.DeepEqual(head.Coverage, original.Coverage) || head.Batches[0].SHA256 != batch.SHA256 {
		t.Fatalf("predecessor receipt changed: %v %+v", err, head)
	}
	target := filepath.Join(workspace, "openspec", "changes", "next")
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("created target or binding: %v", err)
	}
}

func TestSupersedeRouteHiveWithoutLocalChangeFailsClosedBeforePrompt(t *testing.T) {
	workspace := canonicalSddTestPath(t, t.TempDir())
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Error(err)
		}
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sdd/changes/issue-653/apply-progress" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found"})
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/sdd/changes/issue-653/store-binding" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"binding": map[string]any{"schema_version": "1", "project": "jarvis-dev", "change": "issue-653", "mode": "hive", "provenance": "cli", "created_at": "2026-08-01T10:00:00Z"}})
	}))
	defer server.Close()
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	cmd := newSddSupersedeCommand()
	cmd.SetArgs([]string{"--change", "issue-653", "--successor", "next", "--project", "jarvis-dev", "--actor", "maintainer", "--reason", "revised scope"})
	cmd.SetIn(strings.NewReader("next\n"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not_found") {
		t.Fatalf("expected explicit fail-closed Hive mode, got %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("prompted before Hive implementation: %q", output.String())
	}
	if _, err := os.Lstat(filepath.Join(workspace, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("created OpenSpec path: %v", err)
	}
}

func TestSupersedeRouteHiveNewAndRetry(t *testing.T) {
	for _, scenario := range []struct {
		name, answer                                                                                                                          string
		credited, occupied, success, retry, adoptionInterrupted, missingAttribution, foreign, advanced, forged, malformedGET, inconsistentGET bool
	}{
		{name: "zero-credit", success: true},
		{name: "credited-accept", answer: "next\n", credited: true, success: true},
		{name: "pending-retry", success: true, retry: true},
		{name: "advanced-bound", success: true, advanced: true},
		{name: "advanced-forged-binding", success: true, advanced: true, forged: true},
		{name: "malformed-get", retry: true, malformedGET: true},
		{name: "inconsistent-get", retry: true, inconsistentGET: true},
		{name: "foreign-retry", retry: true, foreign: true},
		{name: "ready-unbound", adoptionInterrupted: true},
		{name: "credited-decline", answer: "wrong\n", credited: true},
		{name: "credited-eof", credited: true},
		{name: "occupied", answer: "next\n", credited: true, occupied: true},
		{name: "missing-attribution", answer: "next\n", credited: true, missingAttribution: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			workspace := canonicalSddTestPath(t, t.TempDir())
			oldDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Chdir(workspace); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(oldDir) })
			_, oldManifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "original"}})
			if err != nil {
				t.Fatal(err)
			}
			predecessor, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: oldManifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
			if err != nil {
				t.Fatal(err)
			}
			var evidence applyprogress.Batch
			tasks := "- [ ] 1.1 replacement\n"
			if scenario.credited {
				predecessor, evidence = preflightEvidence(t)
				tasks = preflightNewTasks
			}
			heads := map[string]applyprogress.Snapshot{"issue-653": predecessor}
			bindings := map[string]hiveclient.SDDStoreBinding{"issue-653": {SchemaVersion: "1", Project: "jarvis-dev", Change: "issue-653", Mode: hiveclient.SDDStoreModeHive, Provenance: "cli", CreatedAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)}}
			advanceCalls, publishCalls := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				reply := func(value any) {
					if err := json.NewEncoder(w).Encode(value); err != nil {
						t.Error(err)
					}
				}
				path := r.URL.Path
				change := "issue-653"
				if strings.HasPrefix(path, "/sdd/changes/next/") {
					change = "next"
				}
				switch {
				case strings.HasSuffix(path, "/store-binding") || strings.HasSuffix(path, "/store-binding/adopt"):
					if r.Method == http.MethodPost {
						var request hiveclient.SDDStoreBindingRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						if change != "next" || heads["next"].Digest == "" {
							t.Error("adopted before publication")
						}
						if scenario.adoptionInterrupted {
							scenario.adoptionInterrupted = false
							w.WriteHeader(503)
							reply(map[string]string{"code": "unavailable"})
							return
						}
						binding, found := bindings[change]
						if !found {
							binding = hiveclient.SDDStoreBinding{SchemaVersion: "1", Project: "jarvis-dev", Change: change, Mode: request.Mode, Provenance: request.Provenance, CreatedAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)}
							bindings[change] = binding
						}
						if !found {
							w.WriteHeader(http.StatusCreated)
						}
						reply(map[string]any{"binding": binding, "created": !found})
						return
					}
					if binding, ok := bindings[change]; ok {
						reply(map[string]any{"binding": binding})
					} else {
						w.WriteHeader(404)
						reply(map[string]string{"code": "not_found"})
					}
				case strings.HasSuffix(path, "/apply-progress/advance"):
					advanceCalls++
					var request hiveclient.ApplyProgressAdvanceRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.Batches == nil || len(request.Batches) != 0 {
						t.Errorf("daemon rejects nil or nonempty seal batches: %#v", request.Batches)
						w.WriteHeader(http.StatusBadRequest)
						reply(map[string]string{"code": "invalid_request"})
						return
					}
					if request.Snapshot.Status != applyprogress.StatusSuperseded || request.Snapshot.PreviousDigest != predecessor.Digest || request.ExpectedDigest != predecessor.Digest {
						t.Errorf("non-exact seal request %+v", request)
					}
					heads[change] = request.Snapshot
					reply(hiveclient.ApplyProgressResult{Outcome: "committed", Code: "ok", State: hiveclient.ApplyProgressState{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, Snapshot: request.Snapshot}, Receipt: daemonFormatReceipt(t, request)})
				case strings.HasSuffix(path, "/apply-progress/publish-successor"):
					publishCalls++
					seal := heads["issue-653"]
					pointer := &applyprogress.SupersedesPointer{Project: seal.Project, Change: seal.Change, SealDigest: seal.Digest, OriginalManifestSHA256: seal.TaskManifestSHA256, Actor: seal.SealIntent.Actor, Reason: seal.SealIntent.Reason, Timestamp: seal.SealIntent.Timestamp, OperationID: seal.SealIntent.OperationID}
					genesis, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: seal.Project, Change: "next", Generation: 1, Revision: 1, TaskManifestSHA256: seal.SealIntent.SuccessorManifestSHA256, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, Supersedes: pointer})
					if err != nil {
						t.Error(err)
					}
					heads["next"] = genesis
					reply(hiveclient.ApplyProgressResult{Outcome: "committed", State: hiveclient.ApplyProgressState{Generation: 1, Revision: 1, Digest: genesis.Digest, Snapshot: genesis}})
				case strings.Contains(path, "/apply-evidence/"):
					reply(map[string]any{"batch": evidence})
				case strings.HasSuffix(path, "/apply-progress"):
					if head, ok := heads[change]; ok {
						state := hiveclient.ApplyProgressState{Generation: head.Generation, Revision: head.Revision, Digest: head.Digest, Snapshot: head}
						if scenario.credited && change == "issue-653" {
							state.Batches = []applyprogress.Batch{evidence}
						}
						if scenario.malformedGET {
							reply(map[string]any{"outcome": "current", "code": "ok", "state": state})
						} else if scenario.inconsistentGET {
							state.Digest = strings.Repeat("f", 64)
							reply(map[string]any{"outcome": "committed", "code": "ok", "state": state})
						} else {
							// GET wire contract: hive-daemon/internal/httpapi/server.go:460.
							reply(map[string]any{"outcome": "committed", "code": "ok", "state": state})
						}
					} else {
						w.WriteHeader(404)
						reply(map[string]string{"code": "not_found"})
					}
				case strings.HasSuffix(path, "/artifacts"):
					if change == "next" && heads["next"].Digest == "" {
						reply(map[string]any{"artifacts": []map[string]string{}})
						return
					}
					if change == "issue-653" {
						reply(map[string]any{"artifacts": []map[string]string{{"artifact": "tasks", "content": tasks, "created_at": "2026-08-01T10:00:01Z"}}})
					} else {
						reply(map[string]any{"artifacts": []map[string]string{{"artifact": "tasks", "content": tasks, "created_at": "2026-08-01T10:00:01Z"}}})
					}
				case strings.HasSuffix(path, "/successor-occupancy"):
					category := ""
					if scenario.occupied {
						category = "head"
					}
					reply(map[string]any{"occupied": scenario.occupied, "category": category})
				default:
					t.Errorf("unexpected wire %s %s", r.Method, path)
					w.WriteHeader(404)
					reply(map[string]string{"code": "not_found"})
				}
			}))
			defer server.Close()
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			if scenario.retry {
				parsed, err := applyprogress.ParseTasksMarkdown(tasks)
				if err != nil {
					t.Fatal(err)
				}
				_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
				if err != nil {
					t.Fatal(err)
				}
				request, err := planNewSupersessionSeal(newSupersessionPreflightResult{Mode: sddruntime.StoreModeHive, Predecessor: predecessor, TaskManifest: manifest, TasksContent: tasks}, "next", "maintainer", "revised scope", time.Unix(1700000000, 0).UTC(), "stored-operation")
				if err != nil {
					t.Fatal(err)
				}
				heads["issue-653"] = request.Snapshot
				if scenario.foreign {
					foreign, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "next", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
					if err != nil {
						t.Fatal(err)
					}
					heads["next"] = foreign
				}
			}
			var output bytes.Buffer
			execute := func(flags ...string) error {
				cmd := newSddSupersedeCommand()
				cmd.SetArgs(append([]string{"--change", "issue-653", "--successor", "next", "--project", "jarvis-dev"}, flags...))
				cmd.SetIn(strings.NewReader(scenario.answer))
				cmd.SetOut(&output)
				return cmd.Execute()
			}
			firstFlags := []string{"--actor", "maintainer", "--reason", "revised scope"}
			if scenario.retry || scenario.missingAttribution {
				firstFlags = nil
			}
			if err := execute(firstFlags...); scenario.malformedGET || scenario.inconsistentGET {
				if err == nil || advanceCalls != 0 || publishCalls != 0 {
					t.Fatalf("malformed GET permitted mutation: %v", err)
				}
				return
			} else if scenario.foreign {
				if err == nil || advanceCalls != 0 || publishCalls != 0 || bindings["next"].Mode != "" {
					t.Fatalf("foreign target mutated: %v", err)
				}
				return
			} else if scenario.name == "ready-unbound" {
				if err == nil || heads["next"].Digest == "" || bindings["next"].Mode != "" {
					t.Fatalf("expected published unbound genesis: %v", err)
				}
				if err = execute(); err != nil {
					t.Fatalf("retry adoption: %v", err)
				}
				if bindings["next"].Mode != hiveclient.SDDStoreModeHive || publishCalls != 1 || advanceCalls != 2 {
					t.Fatal("retry failed to adopt exact publication")
				}
				return
			} else if !scenario.success {
				if err == nil {
					t.Fatal("accepted declined or occupied successor")
				}
				if advanceCalls != 0 || publishCalls != 0 || heads["issue-653"].Digest != predecessor.Digest || heads["next"].Digest != "" || bindings["next"].Mode != "" {
					t.Fatal("pre-consent mutation")
				}
				if (scenario.occupied || scenario.missingAttribution) && output.Len() != 0 {
					t.Fatalf("prompted for occupied target: %q", output.String())
				}
				return
			} else if err != nil {
				t.Fatal(err)
			}
			seal := heads["issue-653"]
			genesis := heads["next"]
			if err := applyprogress.ValidateSuccessorGenesisPair(seal, genesis); err != nil {
				t.Fatal(err)
			}
			if bindings["next"].Mode != hiveclient.SDDStoreModeHive || advanceCalls != 1 || publishCalls != 1 {
				t.Fatalf("binding/mutations: %+v %d %d", bindings, advanceCalls, publishCalls)
			}
			if scenario.advanced {
				advanced := genesis
				advanced.Revision = 2
				advanced.PreviousDigest = genesis.Digest
				advanced.StreamSHA256 = strings.Repeat("a", 64)
				advanced.NextEntryID = "entry-next"
				advanced, _, err = applyprogress.SealSnapshot(advanced)
				if err != nil {
					t.Fatal(err)
				}
				heads["next"] = advanced
				if scenario.forged {
					binding := bindings["next"]
					binding.Provenance = "foreign"
					bindings["next"] = binding
				}
				err = execute()
				if scenario.forged {
					if err == nil || advanceCalls != 1 || publishCalls != 1 {
						t.Fatalf("forged binding accepted: %v", err)
					}
					return
				}
				if err != nil || advanceCalls != 1 || publishCalls != 1 {
					t.Fatalf("advanced replay mutated: %v", err)
				}
				return
			}
			if err := execute(); err != nil {
				t.Fatalf("exact retry: %v", err)
			}
			if advanceCalls != 2 || publishCalls != 1 || heads["next"].Digest != genesis.Digest {
				t.Fatalf("replay mutated genesis: %d %d", advanceCalls, publishCalls)
			}
			if err := execute("--actor", "different"); err == nil {
				t.Fatal("accepted changed signed actor")
			}
			if err := execute("--reason", "different"); err == nil {
				t.Fatal("accepted changed signed reason")
			}
			if advanceCalls != 2 {
				t.Fatal("mismatched retry mutated head")
			}
			if _, err := os.Lstat(filepath.Join(workspace, "openspec")); !os.IsNotExist(err) {
				t.Fatalf("created local artifacts: %v", err)
			}
		})
	}
}

func TestSupersedeRouteHybridOccupiedBeforeConsent(t *testing.T) {
	workspace, root, head, batch := preflightOpenSpec(t, sddruntime.StoreModeHybrid)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation %s", r.Method)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		reply := func(value any) { _ = json.NewEncoder(w).Encode(value) }
		switch r.URL.Path {
		case "/sdd/changes/issue-653/store-binding":
			reply(map[string]any{"binding": map[string]any{"schema_version": "1", "project": "jarvis-dev", "change": "issue-653", "mode": "hybrid", "provenance": "cli", "created_at": "2026-08-01T10:00:00Z"}})
		case "/sdd/changes/issue-653/apply-progress":
			// GET wire contract: hive-daemon/internal/httpapi/server.go:460.
			reply(map[string]any{"outcome": "committed", "code": "ok", "state": hiveclient.ApplyProgressState{Generation: head.Generation, Revision: head.Revision, Digest: head.Digest, Snapshot: head, Batches: []applyprogress.Batch{batch}}})
		case "/sdd/changes/issue-653/artifacts":
			reply(map[string]any{"artifacts": []map[string]string{{"artifact": "tasks", "content": preflightNewTasks, "created_at": "2026-08-01T10:00:01Z"}}})
		case "/sdd/changes/next/successor-occupancy":
			reply(map[string]any{"occupied": true, "category": "head"})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	cmd := newSddSupersedeCommand()
	cmd.SetArgs([]string{"--root", root, "--change", "issue-653", "--successor", "next", "--project", "jarvis-dev", "--actor", "maintainer", "--reason", "revised scope"})
	cmd.SetIn(strings.NewReader("next\n"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("expected occupied target error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("prompted for occupied target: %q", output.String())
	}
	if _, err := os.Lstat(filepath.Join(workspace, "openspec", "changes", "next")); !os.IsNotExist(err) {
		t.Fatalf("created target: %v", err)
	}
}

func TestSupersedeRouteHybridNewAndPartialSealReplay(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                                                             string
		zero, decline, remoteSealed, localSealed, publishInterrupted, adoptionInterrupted, mirrorInterrupted, rejectAttribution, foreign bool
	}{
		{name: "new-zero-credit", zero: true}, {name: "advanced-bound", zero: true}, {name: "advanced-diverged", zero: true}, {name: "advanced-forged-binding", zero: true}, {name: "credited-decline", decline: true}, {name: "credited-accept"},
		{name: "one-sided-genesis", zero: true, publishInterrupted: true},
		{name: "published-unbound", zero: true, adoptionInterrupted: true},
		{name: "partial-binding-mirror", zero: true, mirrorInterrupted: true},
		{name: "wrong-signed-attribution", localSealed: true, rejectAttribution: true},
		{name: "foreign-retry-target", localSealed: true, foreign: true},
		{name: "remote-sealed", remoteSealed: true}, {name: "remote-genesis-ready", remoteSealed: true}, {name: "local-sealed", localSealed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace, root, original, batch := preflightOpenSpec(t, sddruntime.StoreModeHybrid)
			tasks := preflightNewTasks
			if tc.zero {
				workspace = canonicalSddTestPath(t, t.TempDir())
				root = filepath.Join(workspace, "openspec", "changes", "issue-653")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 original\n"), 0600); err != nil {
					t.Fatal(err)
				}
				_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "original"}})
				if err != nil {
					t.Fatal(err)
				}
				original, _, err = applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "issue-653", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = (sddprogress.OpenSpec{Root: root}).Advance(sddprogress.AdvanceRequest{RequestID: "initial-hybrid", Snapshot: original}); err != nil {
					t.Fatal(err)
				}
				binding, err := sddbinding.New(sddruntime.StoreModeHybrid, "cli")
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = sddbinding.AdoptOpenSpec(root, binding); err != nil {
					t.Fatal(err)
				}
				tasks = "- [ ] 1.1 replacement\n"
				if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte(tasks), 0600); err != nil {
					t.Fatal(err)
				}
			}
			heads := map[string]applyprogress.Snapshot{"issue-653": original}
			bindings := map[string]hiveclient.SDDStoreBinding{"issue-653": {SchemaVersion: "1", Project: "jarvis-dev", Change: "issue-653", Mode: hiveclient.SDDStoreModeHybrid, Provenance: "cli", CreatedAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)}}
			sealRequest := sddprogress.AdvanceRequest{}
			if tc.localSealed || tc.remoteSealed {
				parsed, err := applyprogress.ParseTasksMarkdown(tasks)
				if err != nil {
					t.Fatal(err)
				}
				_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
				if err != nil {
					t.Fatal(err)
				}
				sealRequest, err = planNewSupersessionSeal(newSupersessionPreflightResult{Mode: sddruntime.StoreModeHybrid, Predecessor: original, TaskManifest: manifest, TasksContent: tasks}, "next", "maintainer", "revised scope", time.Unix(1700000000, 0).UTC(), "hybrid-retry-operation")
				if err != nil {
					t.Fatal(err)
				}
				if tc.localSealed {
					if _, err = (sddprogress.OpenSpec{Root: root}).Advance(sealRequest); err != nil {
						t.Fatal(err)
					}
				}
				if tc.remoteSealed {
					heads["issue-653"] = sealRequest.Snapshot
				}
				if tc.foreign {
					target := filepath.Join(workspace, "openspec", "changes", "next")
					if err := os.Mkdir(target, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(target, "foreign"), []byte("occupied"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if tc.name == "remote-genesis-ready" {
					seal := sealRequest.Snapshot
					pointer := &applyprogress.SupersedesPointer{Project: seal.Project, Change: seal.Change, SealDigest: seal.Digest, OriginalManifestSHA256: seal.TaskManifestSHA256, Actor: seal.SealIntent.Actor, Reason: seal.SealIntent.Reason, Timestamp: seal.SealIntent.Timestamp, OperationID: seal.SealIntent.OperationID}
					genesis, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: seal.Project, Change: "next", Generation: 1, Revision: 1, TaskManifestSHA256: seal.SealIntent.SuccessorManifestSHA256, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, Supersedes: pointer})
					if err != nil {
						t.Fatal(err)
					}
					heads["next"] = genesis
				}
			}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				reply := func(v any) {
					if err := json.NewEncoder(w).Encode(v); err != nil {
						t.Error(err)
					}
				}
				change := "issue-653"
				if strings.HasPrefix(r.URL.Path, "/sdd/changes/next/") {
					change = "next"
				}
				path := r.URL.Path
				switch {
				case strings.HasSuffix(path, "/store-binding") || strings.HasSuffix(path, "/store-binding/adopt"):
					if r.Method == http.MethodPost {
						writes++
						var request hiveclient.SDDStoreBindingRequest
						_ = json.NewDecoder(r.Body).Decode(&request)
						if tc.adoptionInterrupted {
							tc.adoptionInterrupted = false
							w.WriteHeader(503)
							reply(map[string]string{"code": "unavailable"})
							return
						}
						binding, found := bindings[change]
						if !found {
							binding = hiveclient.SDDStoreBinding{SchemaVersion: "1", Project: "jarvis-dev", Change: change, Mode: request.Mode, Provenance: request.Provenance, CreatedAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)}
							bindings[change] = binding
							if tc.mirrorInterrupted {
								target := filepath.Join(workspace, "openspec", "changes", "next")
								if err := os.Chmod(target, 0500); err != nil {
									t.Error(err)
								}
							}
							w.WriteHeader(201)
						}
						reply(map[string]any{"binding": binding, "created": !found})
						return
					}
					if binding, found := bindings[change]; found {
						reply(map[string]any{"binding": binding})
					} else {
						w.WriteHeader(404)
						reply(map[string]string{"code": "not_found"})
					}
				case strings.HasSuffix(path, "/apply-progress/advance"):
					writes++
					var request hiveclient.ApplyProgressAdvanceRequest
					_ = json.NewDecoder(r.Body).Decode(&request)
					if request.Batches == nil || len(request.Batches) != 0 {
						t.Errorf("daemon rejects nil or nonempty seal batches: %#v", request.Batches)
						w.WriteHeader(http.StatusBadRequest)
						reply(map[string]string{"code": "invalid_request"})
						return
					}
					if request.ExpectedDigest != original.Digest || request.Snapshot.Status != applyprogress.StatusSuperseded {
						t.Errorf("not exact seal: %+v", request)
					}
					heads[change] = request.Snapshot
					reply(hiveclient.ApplyProgressResult{Outcome: "committed", Code: "ok", State: hiveclient.ApplyProgressState{Generation: request.Snapshot.Generation, Revision: request.Snapshot.Revision, Digest: request.Snapshot.Digest, Snapshot: request.Snapshot}, Receipt: daemonFormatReceipt(t, request)})
				case strings.HasSuffix(path, "/apply-progress/publish-successor"):
					writes++
					if tc.publishInterrupted {
						tc.publishInterrupted = false
						w.WriteHeader(503)
						reply(map[string]string{"code": "unavailable"})
						return
					}
					seal := heads["issue-653"]
					pointer := &applyprogress.SupersedesPointer{Project: seal.Project, Change: seal.Change, SealDigest: seal.Digest, OriginalManifestSHA256: seal.TaskManifestSHA256, Actor: seal.SealIntent.Actor, Reason: seal.SealIntent.Reason, Timestamp: seal.SealIntent.Timestamp, OperationID: seal.SealIntent.OperationID}
					genesis, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: seal.Project, Change: "next", Generation: 1, Revision: 1, TaskManifestSHA256: seal.SealIntent.SuccessorManifestSHA256, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, Supersedes: pointer})
					if err != nil {
						t.Error(err)
					}
					heads["next"] = genesis
					reply(hiveclient.ApplyProgressResult{Outcome: "committed", State: hiveclient.ApplyProgressState{Generation: 1, Revision: 1, Digest: genesis.Digest, Snapshot: genesis}})
				case strings.HasSuffix(path, "/apply-progress"):
					if head, found := heads[change]; found {
						state := hiveclient.ApplyProgressState{Generation: head.Generation, Revision: head.Revision, Digest: head.Digest, Snapshot: head}
						if !tc.zero && change == "issue-653" {
							state.Batches = []applyprogress.Batch{batch}
						}
						// GET wire contract: hive-daemon/internal/httpapi/server.go:460.
						reply(map[string]any{"outcome": "committed", "code": "ok", "state": state})
					} else {
						w.WriteHeader(404)
						reply(map[string]string{"code": "not_found"})
					}
				case strings.Contains(path, "/apply-evidence/"):
					reply(map[string]any{"batch": batch})
				case strings.HasSuffix(path, "/artifacts"):
					if change == "next" && heads["next"].Digest == "" {
						reply(map[string]any{"artifacts": []map[string]string{}})
					} else {
						reply(map[string]any{"artifacts": []map[string]string{{"artifact": "tasks", "content": tasks, "created_at": "2026-08-01T10:00:01Z"}}})
					}
				case strings.HasSuffix(path, "/successor-occupancy"):
					reply(map[string]any{"occupied": false, "category": ""})
				default:
					t.Errorf("unexpected endpoint %s %s", r.Method, path)
					w.WriteHeader(404)
					reply(map[string]string{"code": "not_found"})
				}
			}))
			defer server.Close()
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			args := []string{"--root", root, "--change", "issue-653", "--successor", "next", "--project", "jarvis-dev"}
			if !tc.localSealed && !tc.remoteSealed {
				args = append(args, "--actor", "maintainer", "--reason", "revised scope")
			}
			if tc.rejectAttribution {
				args = append(args, "--reason", "different")
			}
			cmd := newSddSupersedeCommand()
			cmd.SetArgs(args)
			answer := ""
			if tc.decline {
				answer = "wrong\n"
			}
			if tc.name == "credited-accept" {
				answer = "next\n"
			}
			cmd.SetIn(strings.NewReader(answer))
			var output bytes.Buffer
			cmd.SetOut(&output)
			err := cmd.Execute()
			if tc.rejectAttribution || tc.foreign {
				if err == nil || writes != 0 || heads["issue-653"].Digest != original.Digest {
					t.Fatalf("changed signed retry: %v writes %d", err, writes)
				}
				return
			}
			if tc.decline {
				if err == nil || writes != 0 || heads["issue-653"].Digest != original.Digest || output.Len() == 0 {
					t.Fatalf("decline changed stores or skipped consent: %v writes %d output %q", err, writes, output.String())
				}
				if _, statErr := os.Lstat(filepath.Join(workspace, "openspec", "changes", "next")); !os.IsNotExist(statErr) {
					t.Fatalf("created target: %v", statErr)
				}
				return
			}
			if tc.name == "one-sided-genesis" || tc.name == "published-unbound" || tc.name == "partial-binding-mirror" {
				if err == nil {
					t.Fatal("expected interruption before binding")
				}
				if tc.name == "one-sided-genesis" && heads["next"].Digest != "" {
					t.Fatal("Hive published despite injected interruption")
				}
				if tc.mirrorInterrupted {
					if bindings["next"].Mode != hiveclient.SDDStoreModeHybrid {
						t.Fatal("Hive mirror missing after partial adoption")
					}
					target := filepath.Join(workspace, "openspec", "changes", "next")
					if err := os.Chmod(target, 0700); err != nil {
						t.Fatal(err)
					}
				}
				retry := newSddSupersedeCommand()
				retry.SetArgs([]string{"--root", root, "--change", "issue-653", "--successor", "next", "--project", "jarvis-dev"})
				retry.SetIn(strings.NewReader(""))
				retry.SetOut(&output)
				if err = retry.Execute(); err != nil {
					t.Fatalf("hybrid retry: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			local, err := (sddprogress.OpenSpec{Root: root}).InspectPublication()
			if err != nil || local.Digest != heads["issue-653"].Digest {
				t.Fatalf("hybrid seals differ: %v", err)
			}
			target := filepath.Join(workspace, "openspec", "changes", "next")
			genesis, err := (sddprogress.OpenSpec{Root: target}).InspectPublication()
			if err != nil || genesis == nil || genesis.Digest != heads["next"].Digest || applyprogress.ValidateSuccessorGenesisPair(*local, *genesis) != nil {
				t.Fatalf("hybrid genesis differs: %v", err)
			}
			localBinding, err := sddbinding.ReadOpenSpec(target)
			if err != nil || localBinding == nil || localBinding.Mode() != sddruntime.StoreModeHybrid || bindings["next"].Mode != hiveclient.SDDStoreModeHybrid {
				t.Fatalf("hybrid binding incomplete: %v", err)
			}
			if tc.name == "advanced-bound" || tc.name == "advanced-diverged" || tc.name == "advanced-forged-binding" {
				advanced := *genesis
				advanced.Revision = 2
				advanced.PreviousDigest = genesis.Digest
				advanced.StreamSHA256 = strings.Repeat("a", 64)
				advanced.NextEntryID = "entry-next"
				advanced, _, err = applyprogress.SealSnapshot(advanced)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = (sddprogress.OpenSpec{Root: target}).Advance(sddprogress.AdvanceRequest{RequestID: "advanced-hybrid", ExpectedGeneration: 1, ExpectedRevision: 1, ExpectedDigest: genesis.Digest, Snapshot: advanced}); err != nil {
					t.Fatal(err)
				}
				if tc.name != "advanced-diverged" {
					heads["next"] = advanced
				}
				if tc.name == "advanced-forged-binding" {
					binding := bindings["next"]
					binding.Provenance = "foreign"
					bindings["next"] = binding
				}
				beforeRetryWrites := writes
				output.Reset()
				retry := newSddSupersedeCommand()
				retry.SetArgs([]string{"--root", root, "--change", "issue-653", "--successor", "next", "--project", "jarvis-dev"})
				retry.SetIn(strings.NewReader(""))
				retry.SetOut(&output)
				err = retry.Execute()
				if tc.name == "advanced-diverged" {
					localHead, readErr := (sddprogress.OpenSpec{Root: target}).InspectPublication()
					if err == nil || writes != beforeRetryWrites || output.Len() != 0 || readErr != nil || localHead.Digest != advanced.Digest || heads["next"].Digest != genesis.Digest {
						t.Fatalf("divergent hybrid replay accepted or mutated: %v writes %d→%d output %q", err, beforeRetryWrites, writes, output.String())
					}
					return
				}
				if tc.name == "advanced-forged-binding" {
					if err == nil {
						t.Fatal("accepted forged hybrid binding")
					}
					return
				}
				if err != nil {
					t.Fatalf("advanced hybrid replay: %v", err)
				}
				current, readErr := (sddprogress.OpenSpec{Root: target}).InspectPublication()
				if readErr != nil || current.Digest != advanced.Digest || heads["next"].Digest != advanced.Digest {
					t.Fatalf("advanced hybrid changed: %v", readErr)
				}
			}
			if tc.name == "new-zero-credit" {
				repeated := newSddSupersedeCommand()
				repeated.SetArgs([]string{"--root", root, "--change", "issue-653", "--successor", "next", "--project", "jarvis-dev"})
				repeated.SetIn(strings.NewReader(""))
				repeated.SetOut(&output)
				if err := repeated.Execute(); err != nil {
					t.Fatalf("hybrid exact replay: %v", err)
				}
				if heads["next"].Digest != genesis.Digest {
					t.Fatal("exact replay replaced genesis")
				}
			}
		})
	}
}

func TestSupersedeRouteRetryOpenSpec(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      []string
		publish    bool
		foreign    bool
		mismatch   bool
		advanced   bool
		superseded bool
		complete   bool
	}{
		{name: "pending"}, {name: "advanced-bound", advanced: true}, {name: "advanced-superseded", advanced: true, superseded: true}, {name: "advanced-complete", advanced: true, complete: true}, {name: "exact-attribution", flags: []string{"--actor", "maintainer", "--reason", "revised scope"}},
		{name: "wrong-actor", flags: []string{"--actor", "different"}, mismatch: true},
		{name: "wrong-reason", flags: []string{"--reason", "different"}, mismatch: true},
		{name: "foreign-target", foreign: true, mismatch: true},
		{name: "published-unbound", publish: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace, root, original, _ := preflightOpenSpec(t, sddruntime.StoreModeOpenSpec)
			parsed, err := applyprogress.ParseTasksMarkdown(preflightNewTasks)
			if err != nil {
				t.Fatal(err)
			}
			_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
			if err != nil {
				t.Fatal(err)
			}
			request, err := planNewSupersessionSeal(newSupersessionPreflightResult{Mode: sddruntime.StoreModeOpenSpec, Predecessor: original, TaskManifest: manifest, TasksContent: preflightNewTasks}, "next", "maintainer", "revised scope", time.Unix(1700000000, 0).UTC(), "retry-operation")
			if err != nil {
				t.Fatal(err)
			}
			open := sddprogress.OpenSpec{Root: root}
			if _, err = open.Advance(request); err != nil {
				t.Fatal(err)
			}
			target := sddprogress.OpenSpec{Root: filepath.Join(workspace, "openspec", "changes", "next")}
			if tc.publish {
				if _, err = open.PublishSuccessorGenesis(target); err != nil {
					t.Fatal(err)
				}
			}
			if tc.foreign {
				if err = os.Mkdir(target.Root, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(target.Root, "foreign"), []byte("occupied"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/store-binding") {
					t.Errorf("unexpected Hive call %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(404)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found"})
			}))
			defer server.Close()
			t.Setenv("HIVE_DAEMON_URL", server.URL)
			args := append([]string{"--change", "issue-653", "--successor", "next", "--root", root, "--project", "jarvis-dev"}, tc.flags...)
			execute := func() error {
				cmd := newSddSupersedeCommand()
				cmd.SetArgs(args)
				cmd.SetIn(strings.NewReader(""))
				cmd.SetOut(&bytes.Buffer{})
				return cmd.Execute()
			}
			err = execute()
			if tc.mismatch {
				if err == nil {
					t.Fatal("accepted mismatched intent or foreign target")
				}
				head, readErr := open.InspectPublication()
				if readErr != nil || head.Digest != request.Snapshot.Digest {
					t.Fatalf("changed seal: %v", readErr)
				}
				if !tc.foreign {
					if _, statErr := os.Lstat(target.Root); !os.IsNotExist(statErr) {
						t.Fatalf("created target: %v", statErr)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.advanced {
				genesis, readErr := target.InspectPublication()
				if readErr != nil {
					t.Fatal(readErr)
				}
				advanced := *genesis
				advanced.Revision = 2
				advanced.PreviousDigest = genesis.Digest
				advanced.StreamSHA256 = strings.Repeat("a", 64)
				advanced.NextEntryID = "entry-next"
				var completionBatch applyprogress.Batch
				if tc.complete {
					completionBatch, _, readErr = applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "jarvis-dev", Change: "next", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "entry-next", TaskIDs: []string{"1.1", "1.2"}, CompletesTaskIDs: []string{"1.1", "1.2"}, Kind: applyprogress.EvidenceGreen, Summary: "completed", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
					if readErr != nil {
						t.Fatal(readErr)
					}
					advanced.StreamSHA256, readErr = applyprogress.StreamSHA256(completionBatch.Entries)
					if readErr != nil {
						t.Fatal(readErr)
					}
				}
				advanced, _, readErr = applyprogress.SealSnapshot(advanced)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if _, readErr = target.Advance(sddprogress.AdvanceRequest{RequestID: "advanced-progress", ExpectedGeneration: 1, ExpectedRevision: 1, ExpectedDigest: genesis.Digest, Snapshot: advanced}); readErr != nil {
					t.Fatal(readErr)
				}
				if tc.complete {
					completed := advanced
					completed.Revision = 3
					completed.PreviousDigest = advanced.Digest
					completed.Status = applyprogress.StatusComplete
					completed.StreamSHA256 = ""
					completed.NextEntryIndex = 0
					completed.NextEntryID = ""
					completed.Batches = []applyprogress.BatchRef{{BatchID: completionBatch.BatchID, SHA256: completionBatch.SHA256}}
					completed.Coverage = []applyprogress.Coverage{{TaskID: "1.1", BatchID: completionBatch.BatchID, EntryID: "entry-next"}, {TaskID: "1.2", BatchID: completionBatch.BatchID, EntryID: "entry-next"}}
					completed, _, readErr = applyprogress.SealSnapshot(completed)
					if readErr != nil {
						t.Fatal(readErr)
					}
					if _, readErr = target.Advance(sddprogress.AdvanceRequest{RequestID: "completion", ExpectedGeneration: 1, ExpectedRevision: 2, ExpectedDigest: advanced.Digest, Snapshot: completed, Batches: []applyprogress.Batch{completionBatch}}); readErr != nil {
						t.Fatal(readErr)
					}
					advanced = completed
					if err = os.WriteFile(filepath.Join(target.Root, "tasks.md"), []byte("- [x] 1.1 revised\n- [x] 1.2 remaining\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if tc.superseded {
					revised := "- [ ] 1.1 successor revised\n- [ ] 1.2 fresh\n"
					if err = os.WriteFile(filepath.Join(target.Root, "tasks.md"), []byte(revised), 0600); err != nil {
						t.Fatal(err)
					}
					parsed, parseErr := applyprogress.ParseTasksMarkdown(revised)
					if parseErr != nil {
						t.Fatal(parseErr)
					}
					_, manifest, parseErr := applyprogress.TaskManifest(parsed.Tasks)
					if parseErr != nil {
						t.Fatal(parseErr)
					}
					seal, planErr := planNewSupersessionSeal(newSupersessionPreflightResult{Predecessor: advanced, TaskManifest: manifest, TasksContent: revised}, "later", "maintainer", "revised scope", time.Unix(1700000000, 0).UTC(), "later-operation")
					if planErr != nil {
						t.Fatal(planErr)
					}
					if _, planErr = target.Advance(seal); planErr != nil {
						t.Fatal(planErr)
					}
					advanced = seal.Snapshot
				}
				if err = execute(); err != nil {
					t.Fatalf("advanced exact replay: %v", err)
				}
				head, readErr := target.InspectPublication()
				if readErr != nil || head.Digest != advanced.Digest {
					t.Fatalf("advanced head changed: %v", readErr)
				}
				return
			}
			if err = execute(); err != nil {
				t.Fatalf("exact replay: %v", err)
			}
			head, err := target.InspectPublication()
			if err != nil || head == nil || applyprogress.ValidateSuccessorGenesisPair(request.Snapshot, *head) != nil {
				t.Fatalf("invalid genesis: %v", err)
			}
			binding, err := sddbinding.ReadOpenSpec(target.Root)
			if err != nil || binding == nil || binding.Mode() != sddruntime.StoreModeOpenSpec {
				t.Fatalf("missing binding: %v %v", binding, err)
			}
			projection, err := sddstatus.NewOpenSpecSourceForProject(workspace, "jarvis-dev").ResolveSupersession(context.Background(), "issue-653", request.Snapshot)
			if err != nil || projection.State != sddstatus.SupersessionReady {
				t.Fatalf("projection %v %v", projection, err)
			}
		})
	}
}
