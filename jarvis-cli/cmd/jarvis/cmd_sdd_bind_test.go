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

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

const bindChange = "change"

type bindDaemonBinding struct {
	mode       string
	provenance string
}

// bindDaemon is a stateful post-#798 daemon for one change: store-binding and
// apply-progress routes fold the spelling and resolve aliases at ingress, and
// a binding is stored once under the effective key and never overwritten.
type bindDaemon struct {
	t       *testing.T
	aliases map[string]string

	mu       sync.Mutex
	bindings map[string]bindDaemonBinding
	// heads holds a committed apply-progress head per effective key.
	heads    map[string][]byte
	adopts   []string
	projects []string
	// adoptStatus, when set, rejects every adoption with that HTTP status.
	adoptStatus int
	// raceWinner, when set, is stored by another worktree right before this
	// client's first adoption reaches the daemon.
	raceWinner *bindDaemonBinding
}

func newBindDaemon(t *testing.T) *bindDaemon {
	daemon := &bindDaemon{t: t, bindings: map[string]bindDaemonBinding{}, heads: map[string][]byte{}}
	server := httptest.NewServer(daemon)
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	t.Setenv("JARVIS_SDD_STORE_MODE", "")
	return daemon
}

func (d *bindDaemon) resolve(raw string) string {
	key := hiveclient.CanonicalProjectKey(raw)
	if target, ok := d.aliases[key]; ok {
		return target
	}
	return key
}

func (d *bindDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	project := r.URL.Query().Get("project")
	if r.Method == http.MethodGet {
		d.projects = append(d.projects, project)
	}
	base := "/sdd/changes/" + bindChange
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
		d.projects = append(d.projects, request.Project)
		d.adopts = append(d.adopts, request.Project+":"+request.Mode+":"+request.Provenance)
		key := d.resolve(request.Project)
		if d.adoptStatus != 0 {
			w.WriteHeader(d.adoptStatus)
			_, _ = fmt.Fprint(w, `{"code":"project_blocked"}`)
			return
		}
		if d.raceWinner != nil {
			d.bindings[key], d.raceWinner = *d.raceWinner, nil
		}
		existing, found := d.bindings[key]
		if found && (existing.mode != request.Mode || existing.provenance != request.Provenance) {
			w.WriteHeader(http.StatusConflict)
			_, _ = fmt.Fprintf(w, `{"code":"binding_conflict","existing":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":%q,"created_at":"2026-08-01T10:00:00Z"},"requested":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":%q}}`, key, bindChange, existing.mode, existing.provenance, key, bindChange, request.Mode, request.Provenance)
			return
		}
		if found && (existing.mode != request.Mode || existing.provenance != request.Provenance) {
			d.t.Errorf("adopt %s/%s over stored %s/%s", request.Mode, request.Provenance, existing.mode, existing.provenance)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !found {
			d.bindings[key] = bindDaemonBinding{mode: request.Mode, provenance: request.Provenance}
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":%q,"created_at":"2026-08-01T10:00:00Z"},"created":%t}`, key, bindChange, request.Mode, request.Provenance, !found)
	case r.Method == http.MethodGet && r.URL.Path == base+"/store-binding":
		key := d.resolve(project)
		binding, found := d.bindings[key]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"code":"not_found"}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"binding":{"project":%q,"change":%q,"schema_version":"1","mode":%q,"provenance":%q,"created_at":"2026-08-01T10:00:00Z"}}`, key, bindChange, binding.mode, binding.provenance)
	case r.Method == http.MethodGet && r.URL.Path == base+"/artifacts":
		if _, found := d.heads[d.resolve(project)]; found {
			_, _ = fmt.Fprint(w, `{"artifacts":[{"artifact":"tasks","content":"- [ ] 1.1 task\n"}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"artifacts":[]}`)
	case r.Method == http.MethodGet && r.URL.Path == base+"/apply-progress":
		key := d.resolve(project)
		head, found := d.heads[key]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"outcome":"not_found","code":"not_found","project_key":%q}`, key)
			return
		}
		var snapshot struct {
			Generation uint64 `json:"generation"`
			Revision   uint64 `json:"revision"`
			Digest     string `json:"digest"`
		}
		if err := json.Unmarshal(head, &snapshot); err != nil {
			d.t.Fatal(err)
		}
		_, _ = fmt.Fprintf(w, `{"outcome":"committed","code":"ok","project_key":%q,"state":{"generation":%d,"revision":%d,"digest":%q,"snapshot":%s}}`, key, snapshot.Generation, snapshot.Revision, snapshot.Digest, head)
	default:
		d.t.Errorf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *bindDaemon) stored(key string) (bindDaemonBinding, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	binding, found := d.bindings[key]
	return binding, found
}

func (d *bindDaemon) adoptions() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.adopts...)
}

func (d *bindDaemon) writeHead(key string, head []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.heads[key] = head
}

func runSddBindCommand(t *testing.T, workspace string, args ...string) (string, error) {
	t.Helper()
	t.Chdir(workspace)
	command := newSddBindCommand()
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err := command.Execute()
	return out.String(), err
}

func bindStatus(t *testing.T, project, workspace string) sddstatus.ChangeStatus {
	t.Helper()
	raw, err := captureSDDStdout(t, func() error { return runSddStatus(bindChange, project, workspace, true, false) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var status sddstatus.ChangeStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatalf("decode status: %v\n%s", err, raw)
	}
	return status
}

func readOpenSpecBindingState(t *testing.T, workspace string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "openspec", "changes", bindChange, "state.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func TestSddBindHybridThenStatusWithoutEnvironmentKeepsHybrid(t *testing.T) {
	daemon := newBindDaemon(t)
	workspace := canonicalSddTestWorkspace(t)

	if _, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "hybrid"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	status := bindStatus(t, "project", workspace)
	if status.ArtifactStore != "hybrid" || status.StoreBinding == nil || status.StoreBinding.Mode != "hybrid" || status.StoreBinding.Provenance != sddbinding.ProvenancePreflight || !status.StoreBinding.Persisted {
		t.Fatalf("status binding = %#v (store %q), want persisted hybrid/%s", status.StoreBinding, status.ArtifactStore, sddbinding.ProvenancePreflight)
	}
	if stored, _ := daemon.stored("project"); stored != (bindDaemonBinding{mode: "hybrid", provenance: sddbinding.ProvenancePreflight}) {
		t.Fatalf("Hive binding = %#v", stored)
	}
	if state, found := readOpenSpecBindingState(t, workspace); !found || !strings.Contains(state, "mode: hybrid") || !strings.Contains(state, "provenance: "+sddbinding.ProvenancePreflight) {
		t.Fatalf("OpenSpec binding state = %q (found %t), want hybrid mirror", state, found)
	}
}

func TestSddBindPersistsEveryModeThroughItsOwner(t *testing.T) {
	for _, tt := range []struct {
		mode         string
		wantHive     bool
		wantOpenSpec bool
	}{
		{mode: "hive", wantHive: true},
		{mode: "openspec", wantOpenSpec: true},
		{mode: "hybrid", wantHive: true, wantOpenSpec: true},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			daemon := newBindDaemon(t)
			workspace := canonicalSddTestWorkspace(t)

			output, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", tt.mode, "--json")
			if err != nil {
				t.Fatalf("bind --mode %s: %v", tt.mode, err)
			}
			var result sddBindOutput
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatalf("decode bind output %q: %v", output, err)
			}
			if result != (sddBindOutput{Project: "project", Change: bindChange, Mode: tt.mode, Provenance: sddbinding.ProvenancePreflight, Persisted: true}) {
				t.Fatalf("bind output = %#v", result)
			}
			if _, found := daemon.stored("project"); found != tt.wantHive {
				t.Fatalf("Hive binding stored = %t, want %t", found, tt.wantHive)
			}
			if _, found := readOpenSpecBindingState(t, workspace); found != tt.wantOpenSpec {
				t.Fatalf("OpenSpec binding stored = %t, want %t", found, tt.wantOpenSpec)
			}
			if !tt.wantOpenSpec {
				if _, err := os.Stat(filepath.Join(workspace, "openspec")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("hive bind created openspec/: %v", err)
				}
			}
			status := bindStatus(t, "project", workspace)
			if status.StoreBinding == nil || status.StoreBinding.Mode != tt.mode || status.StoreBinding.Provenance != sddbinding.ProvenancePreflight {
				t.Fatalf("status binding = %#v, want %s/%s", status.StoreBinding, tt.mode, sddbinding.ProvenancePreflight)
			}
		})
	}
}

func TestSddBindIdenticalReplaySucceeds(t *testing.T) {
	for _, mode := range []string{"hive", "openspec", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			daemon := newBindDaemon(t)
			workspace := canonicalSddTestWorkspace(t)
			args := []string{"--project", "project", "--change", bindChange, "--mode", mode}

			if _, err := runSddBindCommand(t, workspace, args...); err != nil {
				t.Fatalf("first bind: %v", err)
			}
			adoptions := daemon.adoptions()
			state, _ := readOpenSpecBindingState(t, workspace)
			if _, err := runSddBindCommand(t, workspace, args...); err != nil {
				t.Fatalf("replayed bind: %v", err)
			}
			if got := daemon.adoptions(); len(got) != len(adoptions) {
				t.Fatalf("replay adopted again: %q, want %q", got, adoptions)
			}
			if replayed, _ := readOpenSpecBindingState(t, workspace); replayed != state {
				t.Fatalf("replay rewrote OpenSpec state:\n%s\nwant:\n%s", replayed, state)
			}
		})
	}
}

func TestSddBindReplaysSameModeAdoptedByEarlierStatus(t *testing.T) {
	daemon := newBindDaemon(t)
	workspace := canonicalSddTestWorkspace(t)
	_ = bindStatus(t, "project", workspace)

	output, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "hive", "--json")
	if err != nil {
		t.Fatalf("bind after default adoption: %v", err)
	}
	var result sddBindOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "hive" || result.Provenance != "initial:default:hive" {
		t.Fatalf("bind output = %#v, want the stored hive/initial:default:hive binding", result)
	}
	if got := daemon.adoptions(); len(got) != 1 {
		t.Fatalf("adoptions = %q, want only the status adoption", got)
	}
}

func TestSddBindConflictingModeLeavesStoredBindingUnchanged(t *testing.T) {
	for _, tt := range []struct {
		first, second string
	}{
		{first: "hive", second: "hybrid"},
		{first: "hybrid", second: "hive"},
		{first: "openspec", second: "hybrid"},
		{first: "hybrid", second: "openspec"},
	} {
		t.Run(tt.first+"->"+tt.second, func(t *testing.T) {
			daemon := newBindDaemon(t)
			workspace := canonicalSddTestWorkspace(t)
			if _, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", tt.first); err != nil {
				t.Fatalf("first bind: %v", err)
			}
			hive, hiveFound := daemon.stored("project")
			state, stateFound := readOpenSpecBindingState(t, workspace)
			adoptions := daemon.adoptions()

			_, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", tt.second)
			var conflict *sddbinding.ConflictError
			if !errors.As(err, &conflict) || !errors.Is(err, sddbinding.ErrBindingConflict) {
				t.Fatalf("conflicting bind error = %v, want a binding conflict", err)
			}
			for _, want := range []string{"bound to " + tt.first, "requested " + tt.second} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("conflict error %q does not name %q", err, want)
				}
			}
			if got, found := daemon.stored("project"); got != hive || found != hiveFound {
				t.Fatalf("Hive binding = %#v (%t), want unchanged %#v (%t)", got, found, hive, hiveFound)
			}
			if got, found := readOpenSpecBindingState(t, workspace); got != state || found != stateFound {
				t.Fatalf("OpenSpec state = %q (%t), want unchanged %q (%t)", got, found, state, stateFound)
			}
			if got := daemon.adoptions(); len(got) != len(adoptions) {
				t.Fatalf("conflicting bind adopted: %q", got)
			}
			if status := bindStatus(t, "project", workspace); status.StoreBinding == nil || status.StoreBinding.Mode != tt.first {
				t.Fatalf("status binding = %#v, want %s", status.StoreBinding, tt.first)
			}
		})
	}
}

func TestSddBindBeforeHiveProgressPreventsLegacyHiveInference(t *testing.T) {
	daemon := newBindDaemon(t)
	workspace := canonicalSddTestWorkspace(t)
	if _, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "hybrid"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	// The Hive progress write lands before any OpenSpec progress exists.
	daemon.writeHead("project", identityPartialHead(t, "project"))

	ctx := t.Context()
	_, _, binding, err := resolveBoundStatusSourceAt(ctx, "project", bindChange, workspace)
	if err != nil {
		t.Fatalf("resolve binding after Hive progress: %v", err)
	}
	if binding.Mode != "hybrid" || binding.Provenance != sddbinding.ProvenancePreflight {
		t.Fatalf("binding = %#v, want hybrid/%s, not %s", binding, sddbinding.ProvenancePreflight, sddbinding.ProvenanceLegacyHiveProgress)
	}
	if got := daemon.adoptions(); len(got) != 1 {
		t.Fatalf("adoptions = %q, want only the bind adoption", got)
	}
}

func TestSddBindAfterHiveProgressRejectsDifferentModeWithoutWriting(t *testing.T) {
	daemon := newBindDaemon(t)
	workspace := canonicalSddTestWorkspace(t)
	daemon.writeHead("project", identityPartialHead(t, "project"))

	_, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "hybrid")
	if !errors.Is(err, sddbinding.ErrBindingConflict) || !strings.Contains(err.Error(), sddbinding.ProvenanceLegacyHiveProgress) {
		t.Fatalf("bind over Hive progress error = %v, want conflict naming %s", err, sddbinding.ProvenanceLegacyHiveProgress)
	}
	if got := daemon.adoptions(); len(got) != 0 {
		t.Fatalf("bind over Hive progress adopted: %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "openspec")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected bind created openspec/: %v", err)
	}
}

func TestSddBindRejectsInvalidModeWithoutWriting(t *testing.T) {
	for _, mode := range []string{"none", "memory", "", " "} {
		t.Run(fmt.Sprintf("%q", mode), func(t *testing.T) {
			daemon := newBindDaemon(t)
			workspace := canonicalSddTestWorkspace(t)
			_, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", mode)
			if err == nil || !strings.Contains(err.Error(), "hive, openspec, or hybrid") {
				t.Fatalf("bind --mode %q error = %v, want a mode validation error", mode, err)
			}
			daemon.mu.Lock()
			requests := len(daemon.projects)
			daemon.mu.Unlock()
			if requests != 0 {
				t.Fatalf("invalid mode reached the daemon %d times", requests)
			}
			if _, err := os.Stat(filepath.Join(workspace, "openspec")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid mode created openspec/: %v", err)
			}
		})
	}
}

func TestSddBindRequiresChangeAndMode(t *testing.T) {
	newBindDaemon(t)
	workspace := canonicalSddTestWorkspace(t)
	for _, args := range [][]string{
		{"--mode", "hive"},
		{"--change", bindChange},
		{"--change", " ", "--mode", "hive"},
	} {
		if _, err := runSddBindCommand(t, workspace, args...); err == nil {
			t.Fatalf("bind %q succeeded, want a usage error", args)
		}
	}
}

func TestSddBindWritesUnderCanonicalProjectKey(t *testing.T) {
	for _, tt := range []struct {
		name      string
		project   string
		effective string
		aliases   map[string]string
	}{
		{name: "non-canonical spelling", project: "My_Project", effective: "my-project"},
		{name: "padded upper case", project: " MY.PROJECT ", effective: "my-project"},
		{name: "retired alias", project: "Retired", effective: "target", aliases: map[string]string{"retired": "target"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemon := newBindDaemon(t)
			daemon.aliases = tt.aliases
			workspace := canonicalSddTestWorkspace(t)

			output, err := runSddBindCommand(t, workspace, "--project", tt.project, "--change", bindChange, "--mode", "hybrid", "--json")
			if err != nil {
				t.Fatalf("bind --project %q: %v", tt.project, err)
			}
			var result sddBindOutput
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if result.Project != tt.effective || result.Mode != "hybrid" {
				t.Fatalf("bind output = %#v, want hybrid under %q", result, tt.effective)
			}
			if stored, found := daemon.stored(tt.effective); !found || stored.mode != "hybrid" || stored.provenance != sddbinding.ProvenancePreflight {
				t.Fatalf("binding under %q = %#v (%t)", tt.effective, stored, found)
			}
			daemon.mu.Lock()
			for _, project := range daemon.projects {
				if project != hiveclient.CanonicalProjectKey(project) {
					daemon.mu.Unlock()
					t.Fatalf("daemon received non-canonical project spelling %q", project)
				}
			}
			daemon.mu.Unlock()
			if status := bindStatus(t, tt.effective, workspace); status.StoreBinding == nil || status.StoreBinding.Mode != "hybrid" {
				t.Fatalf("status under %q = %#v", tt.effective, status.StoreBinding)
			}
		})
	}
}

func TestSddBindHumanOutputNamesBinding(t *testing.T) {
	newBindDaemon(t)
	workspace := canonicalSddTestWorkspace(t)
	output, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "openspec")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"project/" + bindChange, "openspec", sddbinding.ProvenancePreflight} {
		if !strings.Contains(output, want) {
			t.Fatalf("human output %q missing %q", output, want)
		}
	}
}

func TestSddCommandRegistersBind(t *testing.T) {
	for _, command := range sddCmd.Commands() {
		if command.Name() == "bind" {
			return
		}
	}
	t.Fatal("jarvis sdd bind is not registered")
}

func TestSddBindFailedDaemonAdoptionLeavesWorkspaceUnchanged(t *testing.T) {
	for _, status := range []int{http.StatusLocked, http.StatusInternalServerError} {
		for _, existing := range [][]string{nil, {"openspec"}} {
			t.Run(fmt.Sprintf("%d/%q", status, existing), func(t *testing.T) {
				daemon := newBindDaemon(t)
				daemon.adoptStatus = status
				workspace := canonicalSddTestWorkspace(t)
				for _, dir := range existing {
					if err := os.Mkdir(filepath.Join(workspace, dir), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "hybrid"); err == nil {
					t.Fatal("bind succeeded although the daemon rejected the adoption")
				}
				entries, err := os.ReadDir(workspace)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != len(existing) {
					t.Fatalf("workspace entries = %v, want only %q", entries, existing)
				}
				for _, dir := range existing {
					nested, err := os.ReadDir(filepath.Join(workspace, dir))
					if err != nil || len(nested) != 0 {
						t.Fatalf("%s = %v (%v), want the existing empty directory", dir, nested, err)
					}
				}
			})
		}
	}
}

func TestSddBindLosingARaceReportsTheWinningBinding(t *testing.T) {
	for _, tt := range []struct {
		name         string
		winner       bindDaemonBinding
		wantConflict bool
		wantMirror   bool
	}{
		{name: "another worktree bound a different mode", winner: bindDaemonBinding{mode: "hive", provenance: "initial:default:hive"}, wantConflict: true},
		{name: "another worktree bound the same mode", winner: bindDaemonBinding{mode: "hybrid", provenance: "initial:environment:JARVIS_SDD_STORE_MODE"}, wantMirror: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemon := newBindDaemon(t)
			winner := tt.winner
			daemon.raceWinner = &winner
			workspace := canonicalSddTestWorkspace(t)

			_, err := runSddBindCommand(t, workspace, "--project", "project", "--change", bindChange, "--mode", "hybrid")
			if tt.wantConflict {
				if !errors.Is(err, sddbinding.ErrBindingConflict) || !strings.Contains(err.Error(), "bound to hive") || !strings.Contains(err.Error(), "requested hybrid") {
					t.Fatalf("error = %v, want a binding conflict naming hive and hybrid", err)
				}
				if _, err := os.Stat(filepath.Join(workspace, "openspec")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("losing bind created openspec/: %v", err)
				}
			} else if err != nil {
				t.Fatalf("bind of the same mode after losing the race: %v", err)
			}
			if stored, _ := daemon.stored("project"); stored != tt.winner {
				t.Fatalf("Hive binding = %#v, want the winner %#v", stored, tt.winner)
			}
			state, found := readOpenSpecBindingState(t, workspace)
			if found != tt.wantMirror || (tt.wantMirror && !strings.Contains(state, "provenance: "+tt.winner.provenance)) {
				t.Fatalf("OpenSpec state = %q (found %t), want mirror %t of the winner", state, found, tt.wantMirror)
			}
		})
	}
}
