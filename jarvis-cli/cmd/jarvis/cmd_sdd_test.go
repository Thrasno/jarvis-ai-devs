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
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestBuildStatusRoutesOnlyAuthenticatedCompletedSuccessor(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "openspec", "changes", "old")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "- [ ] 1.1 original\n"
	revised := "- [ ] 1.1 revised\n"
	if err := os.WriteFile(filepath.Join(old, "tasks.md"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	initial, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "jarvis-dev", Change: "old", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	store := sddprogress.OpenSpec{Root: old}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "initial", Snapshot: initial}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "tasks.md"), []byte(revised), 0o600); err != nil {
		t.Fatal(err)
	}
	_, targetManifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "revised"}})
	if err != nil {
		t.Fatal(err)
	}
	seal := initial
	seal.Schema, seal.Status, seal.Revision, seal.PreviousDigest = applyprogress.SupersessionSnapshotSchema, applyprogress.StatusSuperseded, 2, initial.Digest
	seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "jarvis-dev", SuccessorChange: "new", SuccessorManifestSHA256: targetManifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal"}
	seal, _, err = applyprogress.SealSnapshot(seal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "seal", ExpectedGeneration: 1, ExpectedRevision: 1, ExpectedDigest: initial.Digest, Snapshot: seal}); err != nil {
		t.Fatal(err)
	}
	src := sddstatus.NewOpenSpecSourceForProject(root, "jarvis-dev")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sdd/changes/old/store-binding" || r.Method != http.MethodGet {
			t.Errorf("unexpected binding request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"not_found"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("HIVE_DAEMON_URL", server.URL)
	writeOpenSpecBinding(t, root, "old", "openspec", "persisted:openspec", nil)
	checkCommandSupersession := func(state, change string) {
		t.Helper()
		for _, command := range []struct {
			name string
			run  func(bool) error
		}{
			{"status", func(json bool) error { return runSddStatus("old", "jarvis-dev", root, json, false) }},
			{"continue", func(json bool) error { return runSddContinue("old", "jarvis-dev", root, json) }},
		} {
			for _, asJSON := range []bool{false, true} {
				output, commandErr := captureSDDStdout(t, func() error { return command.run(asJSON) })
				if asJSON {
					var result sddstatus.ChangeStatus
					if commandErr != nil || json.Unmarshal([]byte(output), &result) != nil || result.Supersession == nil || result.Supersession.State != state || result.Supersession.Change != change || result.NextRecommended != "none" {
						t.Fatalf("%s JSON state=%s: %s, %v", command.name, state, output, commandErr)
					}
				} else if state == sddstatus.SupersessionReady {
					if commandErr != nil || !strings.Contains(output, change) || strings.Contains(output, "sdd-apply ←") {
						t.Fatalf("%s human ready: %q, %v", command.name, output, commandErr)
					}
				} else if command.name == "status" {
					if commandErr != nil || !strings.Contains(output, "superseded_pending_successor") || strings.Contains(output, "sdd-apply ←") {
						t.Fatalf("%s human pending: %q, %v", command.name, output, commandErr)
					}
				} else if commandErr == nil || strings.Contains(output, "sdd-apply") {
					t.Fatalf("continue routed pending successor: %q, %v", output, commandErr)
				}
			}
		}
	}
	status, err := buildStatusWithBinding("old", src, "openspec", []string{root}, nil)
	if err != nil || status.Supersession == nil || status.Supersession.State != sddstatus.SupersessionPending || status.NextRecommended != "none" {
		t.Fatalf("pending status = %#v, err %v", status, err)
	}
	checkCommandSupersession(sddstatus.SupersessionPending, "new")
	target := sddprogress.OpenSpec{Root: filepath.Join(root, "openspec", "changes", "new")}
	if _, err := store.PublishSuccessorGenesis(target); err != nil {
		t.Fatal(err)
	}
	status, err = buildStatusWithBinding("old", src, "openspec", []string{root}, nil)
	if err != nil || status.Supersession == nil || status.Supersession.State != sddstatus.SupersessionReady || status.Supersession.Change != "new" {
		t.Fatalf("published status = %#v, err %v", status, err)
	}
	bHead, err := target.InspectPublication()
	if err != nil || bHead == nil {
		t.Fatalf("B genesis: %v", err)
	}
	thirdTasks := "- [ ] 1.1 third\n"
	if err := os.WriteFile(filepath.Join(target.Root, "tasks.md"), []byte(thirdTasks), 0o600); err != nil {
		t.Fatal(err)
	}
	_, thirdManifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "third"}})
	if err != nil {
		t.Fatal(err)
	}
	bSeal := *bHead
	bSeal.Revision++
	bSeal.PreviousDigest = bHead.Digest
	bSeal.Status = applyprogress.StatusSuperseded
	bSeal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "jarvis-dev", SuccessorChange: "third", SuccessorManifestSHA256: thirdManifest, Actor: "agent", Reason: "replanned again", Timestamp: "2026-01-02T00:00:00Z", OperationID: "seal-2"}
	bSeal, _, err = applyprogress.SealSnapshot(bSeal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Advance(sddprogress.AdvanceRequest{RequestID: "seal-2", ExpectedGeneration: bHead.Generation, ExpectedRevision: bHead.Revision, ExpectedDigest: bHead.Digest, Snapshot: bSeal}); err != nil {
		t.Fatal(err)
	}
	third := sddprogress.OpenSpec{Root: filepath.Join(root, "openspec", "changes", "third")}
	if _, err := target.PublishSuccessorGenesis(third); err != nil {
		t.Fatal(err)
	}
	status, err = buildStatusWithBinding("old", src, "openspec", []string{root}, nil)
	if err != nil || status.Supersession == nil || status.Supersession.Change != "third" || status.Supersession.State != sddstatus.SupersessionReady {
		t.Fatalf("chain tail = %#v, err %v", status, err)
	}
	checkCommandSupersession(sddstatus.SupersessionReady, "third")
}

type chainStatusSource struct {
	seal   applyprogress.Snapshot
	length int
	cycle  bool
	noHead bool
}

func (s chainStatusSource) FetchArtifacts(_ context.Context, change string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	data, _ := json.Marshal(s.seal)
	return map[string]sddstatus.ArtifactState{sddstatus.ArtifactApplyProgress: sddstatus.ArtifactSuperseded}, map[string]string{sddstatus.ArtifactApplyProgress: string(data)}, nil
}
func (s chainStatusSource) ListChanges(context.Context) ([]string, error) { return nil, nil }
func (s chainStatusSource) ResolveSupersession(_ context.Context, change string, _ applyprogress.Snapshot) (sddstatus.SupersessionProjection, error) {
	index := 0
	if change != "old" {
		_, _ = fmt.Sscanf(change, "step-%d", &index)
	}
	next := fmt.Sprintf("step-%d", index+1)
	if s.cycle && index == 1 {
		next = "old"
	}
	if s.noHead {
		return sddstatus.SupersessionProjection{State: sddstatus.SupersessionReady, Change: next}, nil
	}
	return sddstatus.SupersessionProjection{State: sddstatus.SupersessionReady, Change: next, Head: &s.seal}, nil
}

func TestBuildStatusRejectsSupersessionCycleAndDepth(t *testing.T) {
	seal, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 2, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusSuperseded, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, SealIntent: &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "step-1", SuccessorManifestSHA256: strings.Repeat("b", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "request-1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		cycle  bool
		noHead bool
		want   string
	}{{"cycle", true, false, "cycle"}, {"depth", false, false, "depth"}, {"ready without head", false, true, "head"}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildStatusWithBinding("old", chainStatusSource{seal: seal, cycle: tc.cycle, noHead: tc.noHead}, "openspec", nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("chain error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestResolveSourceAtBindsOpenSpecProject(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "openspec", "changes", "change")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	_, data, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "foreign", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apply-progress.md"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")
	src, mode, err := resolveSourceAt("jarvis-dev", root)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "openspec" {
		t.Fatalf("mode = %q", mode)
	}
	arts, _, err := src.FetchArtifacts(context.Background(), "change")
	if err != nil {
		t.Fatal(err)
	}
	if arts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactBlockedInvalid {
		t.Fatalf("foreign project = %q", arts[sddstatus.ArtifactApplyProgress])
	}
}

type fakeSddArtifactSource struct {
	artifacts map[string]sddstatus.ArtifactState
	contents  map[string]string
}

func (f fakeSddArtifactSource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	return f.artifacts, f.contents, nil
}

func (f fakeSddArtifactSource) ListChanges(context.Context) ([]string, error) {
	return []string{"my-feature"}, nil
}

func TestSddWorkingDirectoryPreservesTargetWithProjectAlias(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)

	workingDir, err := sddWorkingDirectory("team-project-alias")
	if err != nil {
		t.Fatalf("sddWorkingDirectory: %v", err)
	}
	if workingDir != workspace {
		t.Fatalf("sddWorkingDirectory with --project = %q, want current workspace %q", workingDir, workspace)
	}
}

func TestResolveSddProjectExplicitOverrideWins(t *testing.T) {
	project, err := resolveSddProject("  Legacy_Project  ", filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("resolveSddProject explicit override: %v", err)
	}
	if project != "Legacy_Project" {
		t.Fatalf("resolveSddProject explicit override = %q, want Legacy_Project", project)
	}
}

func TestResolveSddProjectDerivesFromSuppliedWorkingDirectory(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "fallback-project")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("create test repository: %v", err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd := exec.Command("git", "remote", "add", "origin", "git@github.com:Thrasno/origin-project.git")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add origin: %v\n%s", err, out)
	}

	project, err := resolveSddProject("", repo)
	if err != nil {
		t.Fatalf("resolveSddProject implicit origin: %v", err)
	}
	if project != "origin-project" {
		t.Fatalf("resolveSddProject implicit origin = %q, want origin-project", project)
	}

	fallbackDir := filepath.Join(t.TempDir(), "fallback-project")
	if err := os.Mkdir(fallbackDir, 0o755); err != nil {
		t.Fatalf("create fallback directory: %v", err)
	}
	project, err = resolveSddProject(" \t", fallbackDir)
	if err != nil {
		t.Fatalf("resolveSddProject implicit basename: %v", err)
	}
	if project != "fallback-project" {
		t.Fatalf("resolveSddProject implicit basename = %q, want fallback-project", project)
	}
}

func TestResolveSddProjectReturnsSharedDerivationErrors(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		wantErr    error
	}{
		{name: "blank working directory", workingDir: " \t", wantErr: hivederive.ErrEmptyDir},
		{name: "unresolvable working directory", workingDir: filepath.Join(t.TempDir(), "missing"), wantErr: hivederive.ErrPathUnresolvable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := resolveSddProject("", tt.workingDir)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("resolveSddProject error = %v, want %v", err, tt.wantErr)
			}
			if project != "" {
				t.Fatalf("resolveSddProject project = %q, want empty on derivation failure", project)
			}
		})
	}
}

func TestSddStatusAndContinueShareProjectDerivationFailures(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "status",
			run: func() error {
				return runSddStatus("my-feature", "", workingDir, false, false)
			},
		},
		{
			name: "continue",
			run: func() error {
				return runSddContinue("my-feature", "", workingDir, false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if !errors.Is(err, hivederive.ErrPathUnresolvable) {
				t.Fatalf("command error = %v, want ErrPathUnresolvable before Hive access", err)
			}
		})
	}
}

func TestBuildStatus_DoesNotUseCurrentDirectoryAsAllowedEditRoot(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)

	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactProposal: sddstatus.ArtifactDone},
	}, "hive", nil)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	if got := status.AllowedEditRoots; len(got) != 0 {
		t.Fatalf("AllowedEditRoots = %#v, want empty for unvalidated current directory", got)
	}
	if got := status.ActionContext.AllowedEditRoots; len(got) != 0 {
		t.Fatalf("ActionContext.AllowedEditRoots = %#v, want empty for unvalidated current directory", got)
	}
	if got := status.ActionContext.Mode; got != sddstatus.ActionModeWorkspacePlanning {
		t.Fatalf("ActionContext.Mode = %q, want %q", got, sddstatus.ActionModeWorkspacePlanning)
	}
}

func TestResolveSource_NoneModeDoesNotConnectToHive(t *testing.T) {
	t.Setenv("JARVIS_SDD_STORE_MODE", "none")

	src, storeMode, err := resolveSource("jarvis-dev")
	if err != nil {
		t.Fatalf("resolveSource none mode: %v", err)
	}
	if storeMode != "none" {
		t.Fatalf("storeMode = %q, want none", storeMode)
	}
	changes, err := src.ListChanges(context.Background())
	if err != nil {
		t.Fatalf("none source ListChanges: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("none source changes = %v, want empty", changes)
	}
	artifacts, contents, err := src.FetchArtifacts(context.Background(), "any-change")
	if err != nil {
		t.Fatalf("none source FetchArtifacts: %v", err)
	}
	if !reflect.DeepEqual(artifacts, map[string]sddstatus.ArtifactState{}) || len(contents) != 0 {
		t.Fatalf("none source artifacts=%v contents=%v, want empty inline-only source", artifacts, contents)
	}
}

func TestBuildStatus_IncludesValidatedAllowedEditRoot(t *testing.T) {
	workspace := t.TempDir()

	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactProposal: sddstatus.ArtifactDone},
	}, "hive", []string{workspace})
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	if got := status.AllowedEditRoots; len(got) != 1 || got[0] != workspace {
		t.Fatalf("AllowedEditRoots = %#v, want %q", got, workspace)
	}
	if got := status.ActionContext.AllowedEditRoots; len(got) != 1 || got[0] != workspace {
		t.Fatalf("ActionContext.AllowedEditRoots = %#v, want %q", got, workspace)
	}
	if got := status.ActionContext.Mode; got != sddstatus.ActionModeWorkspaceEdit {
		t.Fatalf("ActionContext.Mode = %q, want %q", got, sddstatus.ActionModeWorkspaceEdit)
	}
}

func TestBuildStatus_WorkspacePlanningBlocksMutatingPhases(t *testing.T) {
	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{
			sddstatus.ArtifactProposal: sddstatus.ArtifactDone,
		},
	}, "hive", nil)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	for _, phase := range []string{sddstatus.PhaseApply, sddstatus.PhaseVerify, sddstatus.PhaseArchive} {
		if got := status.Dependencies[phase]; got != sddstatus.DepBlocked {
			t.Errorf("dependency[%s] = %q, want blocked without validated workspace authority", phase, got)
		}
		if !containsSddStatusReason(status.BlockedReasons, "phase "+phase+" blocked — workspace-edit mode with non-empty allowed edit roots required") {
			t.Errorf("BlockedReasons = %#v, want workspace authority blocker for %s", status.BlockedReasons, phase)
		}
	}

	for _, phase := range []string{sddstatus.PhaseSpec, sddstatus.PhaseDesign} {
		if got := status.Dependencies[phase]; got != sddstatus.DepReady {
			t.Errorf("dependency[%s] = %q, want planning phase to remain ready", phase, got)
		}
	}
}

func containsSddStatusReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func TestValidatedEditRootsForProjectUsesValidatedTargetAndPermitsProjectAliases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jarvis-dev")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create workspace root: %v", err)
	}

	canonicalRoot := canonicalSddTestPath(t, root)
	for _, tt := range []struct {
		name        string
		projectName string
		root        string
		wantRoot    bool
	}{
		{name: "derived project identity", projectName: "jarvis-dev", root: root, wantRoot: true},
		{name: "explicit project alias", projectName: "team-project-alias", root: root, wantRoot: true},
		{name: "empty project identity", projectName: "", root: root, wantRoot: true},
		{name: "missing root", projectName: "jarvis-dev", root: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := validatedEditRootsForProject(tt.projectName, tt.root)
			if tt.wantRoot {
				if len(got) != 1 || got[0] != canonicalRoot {
					t.Fatalf("validatedEditRootsForProject(%q, %q) = %#v, want %q", tt.projectName, tt.root, got, canonicalRoot)
				}
				return
			}
			if len(got) != 0 {
				t.Fatalf("validatedEditRootsForProject(%q, %q) = %#v, want empty", tt.projectName, tt.root, got)
			}
		})
	}
}

func TestBuildStatus_ApplyReadyUsesWorktreeRootAsAllowedEditRoot(t *testing.T) {
	worktreeRoot := filepath.Join(t.TempDir(), "epic-06-projects-repository-api")
	if err := os.Mkdir(worktreeRoot, 0o755); err != nil {
		t.Fatalf("create worktree root: %v", err)
	}
	editRoots := validatedEditRootsForProject("jarvis-dev", worktreeRoot)
	canonicalWorktreeRoot := canonicalSddTestPath(t, worktreeRoot)

	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{
			sddstatus.ArtifactProposal: sddstatus.ArtifactDone,
			sddstatus.ArtifactSpec:     sddstatus.ArtifactDone,
			sddstatus.ArtifactDesign:   sddstatus.ArtifactDone,
			sddstatus.ArtifactTasks:    sddstatus.ArtifactDone,
		},
	}, "hive", editRoots)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	if got := status.Dependencies[sddstatus.PhaseApply]; got != sddstatus.DepReady {
		t.Fatalf("sdd-apply dependency = %q, want %q", got, sddstatus.DepReady)
	}
	if got := status.ActionContext.Mode; got != sddstatus.ActionModeWorkspaceEdit {
		t.Fatalf("ActionContext.Mode = %q, want %q", got, sddstatus.ActionModeWorkspaceEdit)
	}
	if got := status.ActionContext.AllowedEditRoots; len(got) != 1 || got[0] != canonicalWorktreeRoot {
		t.Fatalf("ActionContext.AllowedEditRoots = %#v, want [%q]", got, canonicalWorktreeRoot)
	}
	if got := status.AllowedEditRoots; len(got) != 1 || got[0] != canonicalWorktreeRoot {
		t.Fatalf("AllowedEditRoots = %#v, want [%q]", got, canonicalWorktreeRoot)
	}
}

// TestRunSddContinue_BlockedWhenProposalMissing proves that when no planning artifacts
// exist, the spec phase is blocked (proposal missing) and the continue routing reflects
// this by surfacing a blocked reason referencing both sdd-spec and proposal.
// This tests the CLI-level enforcement of spec scenario "sdd-spec blocked without proposal".
func TestRunSddContinue_BlockedWhenProposalMissing(t *testing.T) {
	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{},
	}, "hive", nil)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	if status.Dependencies[sddstatus.PhaseSpec] != sddstatus.DepBlocked {
		t.Errorf("sdd-spec dep = %q, want blocked when proposal is missing", status.Dependencies[sddstatus.PhaseSpec])
	}

	hasReason := false
	for _, r := range status.BlockedReasons {
		if strings.Contains(r, sddstatus.PhaseSpec) && strings.Contains(r, sddstatus.ArtifactProposal) {
			hasReason = true
		}
	}
	if !hasReason {
		t.Errorf("BlockedReasons must reference both sdd-spec and proposal; got: %v", status.BlockedReasons)
	}
}

// TestRunSddContinue_BlockedWhenApplyDecisionUnresolved proves that when all planning
// artifacts are done but the tasks artifact declares an unresolved delivery decision,
// sdd-apply is blocked and the continue routing surfaces a descriptive reason.
// This tests the CLI-level enforcement of spec scenario "apply blocked when tasks declare
// unresolved decision".
func TestRunSddContinue_BlockedWhenApplyDecisionUnresolved(t *testing.T) {
	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{
			sddstatus.ArtifactProposal: sddstatus.ArtifactDone,
			sddstatus.ArtifactSpec:     sddstatus.ArtifactDone,
			sddstatus.ArtifactDesign:   sddstatus.ArtifactDone,
			sddstatus.ArtifactTasks:    sddstatus.ArtifactDone,
		},
		contents: map[string]string{
			sddstatus.ArtifactTasks: "Decision needed before apply: Yes\n",
		},
	}, "hive", nil)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	if status.Dependencies[sddstatus.PhaseApply] != sddstatus.DepBlocked {
		t.Errorf("sdd-apply dep = %q, want blocked when apply-decision is unresolved", status.Dependencies[sddstatus.PhaseApply])
	}

	hasReason := false
	for _, r := range status.BlockedReasons {
		if strings.Contains(r, "Decision needed before apply") {
			hasReason = true
		}
	}
	if !hasReason {
		t.Errorf("BlockedReasons must mention 'Decision needed before apply'; got: %v", status.BlockedReasons)
	}
}

func TestRunSddContinue_IncompleteTasksBlockArchiveRouting(t *testing.T) {
	status, err := buildStatus("my-feature", fakeSddArtifactSource{
		artifacts: map[string]sddstatus.ArtifactState{
			sddstatus.ArtifactProposal:      sddstatus.ArtifactDone,
			sddstatus.ArtifactSpec:          sddstatus.ArtifactDone,
			sddstatus.ArtifactDesign:        sddstatus.ArtifactDone,
			sddstatus.ArtifactTasks:         sddstatus.ArtifactDone,
			sddstatus.ArtifactApplyProgress: sddstatus.ArtifactDone,
			sddstatus.ArtifactVerifyReport:  sddstatus.ArtifactDone,
			sddstatus.ArtifactArchiveReport: sddstatus.ArtifactDone,
		},
		contents: map[string]string{
			sddstatus.ArtifactTasks:        "- [x] 1\n- [ ] 2\n",
			sddstatus.ArtifactVerifyReport: "All checks passed.",
		},
	}, "hive", nil)
	if err != nil {
		t.Fatalf("buildStatus: %v", err)
	}

	if got := status.Dependencies[sddstatus.PhaseArchive]; got != sddstatus.DepBlocked {
		t.Fatalf("sdd-archive dependency = %q, want blocked", got)
	}
	if got := status.NextRecommended; got != "" {
		t.Fatalf("nextRecommended = %q, want empty so continue reports the blocker", got)
	}
	const wantReason = "phase sdd-archive blocked — task progress is incomplete (1/2 tasks complete)"
	if !containsStatusReason(status.BlockedReasons, wantReason) {
		t.Fatalf("blockedReasons = %#v, want %q", status.BlockedReasons, wantReason)
	}
}

func containsStatusReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

// TestPrintStatusHuman_BlockedWithNoNextRecommended guards against the regression
// where printStatusHuman showed "all phases complete ✓" while also showing blocked
// reasons — contradictory output that occurred when NextRecommended was "" and
// BlockedReasons was non-empty (e.g., archive soft-blocked by empty verify content).
func TestPrintStatusHuman_BlockedWithNoNextRecommended(t *testing.T) {
	s := &sddstatus.ChangeStatus{
		Schema:        sddstatus.StatusSchema,
		ChangeName:    "my-feature",
		ArtifactStore: "hive",
		ArtifactPaths: map[string]string{},
		Artifacts:     map[string]sddstatus.ArtifactState{},
		Dependencies:  map[string]sddstatus.DependencyState{},
		// NextRecommended empty + BlockedReasons non-empty: the blocked-but-no-next state.
		NextRecommended: "",
		BlockedReasons:  []string{"phase sdd-archive blocked — verify report is empty (re-run sdd-verify to generate content)"},
	}

	for _, phase := range sddstatus.PhaseOrder {
		s.Dependencies[phase] = sddstatus.DepAllDone
		s.Artifacts[sddstatus.PhaseOutput[phase]] = sddstatus.ArtifactDone
	}
	s.Dependencies[sddstatus.PhaseArchive] = sddstatus.DepBlocked
	s.Artifacts[sddstatus.ArtifactArchiveReport] = sddstatus.ArtifactMissing

	var buf strings.Builder
	printStatusHuman(&buf, s, false)
	out := buf.String()

	if strings.Contains(out, "all phases complete") {
		t.Errorf("printStatusHuman must NOT say 'all phases complete' when BlockedReasons is non-empty; got:\n%s", out)
	}
	if !strings.Contains(out, "blocked") {
		t.Errorf("printStatusHuman must say 'blocked' in the next-recommended line; got:\n%s", out)
	}
}

// TestPrintStatusHuman_AllDone_ShowsComplete verifies the "all phases complete ✓"
// message is still shown correctly when there are no blocked reasons.
func TestSddArchiveHelpUsesFlagOnlySyntax(t *testing.T) {
	command := newSddArchiveCommand(func(string) sddArchiver { return archiveFunc(func(string) error { return nil }) }, archiveReadyStatus)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "jarvis sdd archive --root <change-root> --destination <archive-destination>") || strings.Contains(got, "archive [change]") {
		t.Fatalf("archive help = %q", got)
	}

	invalid := newSddArchiveCommand(func(string) sddArchiver { return archiveFunc(func(string) error { return nil }) }, archiveReadyStatus)
	invalid.SetArgs([]string{"unexpected", "--root", "change", "--destination", "archive/change"})
	if err := invalid.Execute(); err == nil {
		t.Fatal("archive accepted a positional change")
	}
}

func TestSddArchiveInvokesProductionArchiver(t *testing.T) {
	var gotRoot, gotDestination string
	command := newSddArchiveCommand(func(root string) sddArchiver {
		gotRoot = root
		return archiveFunc(func(destination string) error { gotDestination = destination; return nil })
	}, archiveReadyStatus)
	command.SetArgs([]string{"--root", "change", "--destination", "archive/change"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotRoot != "change" || gotDestination != "archive/change" {
		t.Fatalf("archive call = root %q destination %q", gotRoot, gotDestination)
	}
}

func TestSddArchiveCommandRejectsNoncanonicalRootBeforeArchive(t *testing.T) {
	workspace := canonicalSddTestPath(t, t.TempDir())
	change := "issue-653"
	validatedRoot := filepath.Join(workspace, "openspec", "changes", change)
	if err := os.MkdirAll(validatedRoot, 0o755); err != nil {
		t.Fatalf("create validated change root: %v", err)
	}
	for name, content := range map[string]string{
		"proposal.md":      "proposal",
		"spec.md":          "spec",
		"design.md":        "design",
		"tasks.md":         "- [x] 1.1 task\n",
		"verify-report.md": "## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n",
	} {
		if err := os.WriteFile(filepath.Join(validatedRoot, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	request := progressRequest(t, "archive-ready", "apb-00000000000000000000000000000001", 1, "")
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
	if _, err := (sddprogress.OpenSpec{Root: validatedRoot}).Advance(request); err != nil {
		t.Fatalf("persist complete OpenSpec progress: %v", err)
	}

	noncanonicalRoot := filepath.Join(workspace, "unrelated", "other", "change")
	if err := os.MkdirAll(noncanonicalRoot, 0o755); err != nil {
		t.Fatalf("create noncanonical change root: %v", err)
	}
	t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")

	status, err := archiveStatus(validatedRoot, "jarvis-dev")
	if err != nil {
		t.Fatalf("archiveStatus with canonical root: %v", err)
	}
	if got, want := status.ChangeRoot, canonicalSddTestPath(t, validatedRoot); got != want {
		t.Fatalf("archiveStatus ChangeRoot = %q, want canonical root %q", got, want)
	}

	archived := false
	command := newSddArchiveCommand(func(string) sddArchiver {
		return archiveFunc(func(string) error { archived = true; return nil })
	}, archiveStatus)
	command.SetArgs([]string{"--root", noncanonicalRoot, "--destination", filepath.Join(workspace, "openspec", "archive", "other"), "--project", "jarvis-dev"})

	err = command.Execute()
	if err == nil || !strings.Contains(err.Error(), "canonical OpenSpec change root") {
		t.Fatalf("archive with noncanonical root error = %v, want canonical-root rejection", err)
	}
	if archived {
		t.Fatal("archive invoked with a noncanonical root")
	}

	command = newSddArchiveCommand(func(string) sddArchiver {
		return archiveFunc(func(string) error { archived = true; return nil })
	}, archiveStatus)
	command.SetArgs([]string{"--root", validatedRoot, "--destination", filepath.Join(workspace, "openspec", "archive", change), "--project", "jarvis-dev"})
	if err := command.Execute(); err != nil {
		t.Fatalf("archive with canonical root: %v", err)
	}
	if !archived {
		t.Fatal("archive was not invoked with a canonical root")
	}
}

func TestSddLegacyUpgradeStatusAndArchiveEndToEnd(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "openspec", "changes", "issue-653")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apply-progress.md"), []byte("status: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := progressLegacyRequest(t)
	store := sddprogress.OpenSpec{Root: root}
	if _, upgraded, err := store.UpgradeLegacy(request); err != nil || !upgraded {
		t.Fatalf("UpgradeLegacy() upgraded=%t error=%v", upgraded, err)
	}
	artifacts, _, err := sddstatus.NewOpenSpecSource(workspace).FetchArtifacts(context.Background(), "issue-653")
	if err != nil || artifacts[sddstatus.ArtifactApplyProgress] != sddstatus.ArtifactDone {
		t.Fatalf("status after upgrade = %#v, %v", artifacts, err)
	}
	for name, data := range map[string][]byte{
		"proposal.md":       []byte("# Proposal\n"),
		"design.md":         []byte("# Design\n"),
		"verify-report.md":  []byte("## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"),
		"archive-report.md": []byte("# Archive\n"),
		filepath.Join("specs", "base", "spec.md"): []byte("# Spec\n"),
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(workspace, "openspec", "archive", "issue-653")
	if err := store.Archive(archive); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(archive, "apply-progress.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyprogress.DecodeCanonicalSnapshot(data); err != nil {
		t.Fatalf("archived snapshot = %q: %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(archive, ".apply-progress-receipts", request.RequestID+".json")); err != nil {
		t.Fatalf("archived receipt: %v", err)
	}
}

func TestSddArchiveCommandMovesValidatedTopology(t *testing.T) {
	root := newProgressTestRoot(t)
	for name, data := range map[string][]byte{
		"proposal.md":      []byte("# Proposal\n"),
		"design.md":        []byte("# Design\n"),
		"verify-report.md": []byte("## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"),
		filepath.Join("specs", "base", "spec.md"): []byte("# Spec\n"),
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	request := progressRequest(t, "archive", "apb-00000000000000000000000000000001", 1, "")
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
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(filepath.Dir(root), "archive", "issue-653")
	command := newSddArchiveCommand(func(root string) sddArchiver { return sddprogress.OpenSpec{Root: root} }, archiveReadyStatus)
	command.SetArgs([]string{"--root", root, "--destination", archive})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(archive, "apply-progress.md")); err != nil {
		t.Fatalf("archived snapshot: %v", err)
	}
}

func TestSddArchiveRevalidatesLifecycleReadinessUnderLock(t *testing.T) {
	t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")
	workspace := canonicalSddTestPath(t, t.TempDir())
	root := filepath.Join(workspace, "openspec", "changes", "issue-653")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create change root: %v", err)
	}
	for name, content := range map[string]string{
		"proposal.md":       "# Proposal\n",
		"spec.md":           "# Spec\n",
		"design.md":         "# Design\n",
		"tasks.md":          "- [x] 1.1 task\n",
		"verify-report.md":  "## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n",
		"archive-report.md": "# Archive report\n",
		filepath.Join("specs", "delivery", "spec.md"): "# Delivery Delta\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	request := progressRequest(t, "archive-revalidation", "apb-00000000000000000000000000000001", 1, "")
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
		t.Fatal(err)
	}

	mutated := false
	command := newSddArchiveCommand(func(root string) sddArchiver {
		return sddprogress.OpenSpec{Root: root, BeforeArchiveValidate: func() error {
			mutated = true
			return os.WriteFile(filepath.Join(root, "verify-report.md"), []byte("critical blocker\n"), 0o600)
		}}
	}, archiveStatus)
	command.SetArgs([]string{"--root", root, "--destination", filepath.Join(workspace, "openspec", "archive", "issue-653"), "--project", "jarvis-dev"})

	if err := command.Execute(); err == nil {
		t.Fatal("archive succeeded after verify-report became invalid under the archive lock")
	}
	if !mutated {
		t.Fatal("verify-report was not invalidated under the archive lock")
	}
	if _, err := os.Stat(filepath.Join(root, "apply-progress.md")); err != nil {
		t.Fatalf("archive moved lifecycle after failed revalidation: %v", err)
	}
}

func TestArchiveStatusReadiesProductionArchiveWithNestedDeltaSpecs(t *testing.T) {
	t.Setenv("JARVIS_SDD_STORE_MODE", "openspec")
	workspace := canonicalSddTestPath(t, t.TempDir())
	root := filepath.Join(workspace, "openspec", "changes", "issue-653")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [x] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	request := progressRequest(t, "archive-nested-spec", "apb-00000000000000000000000000000001", 1, "")
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
	store := sddprogress.OpenSpec{Root: root}
	if _, err := store.Advance(request); err != nil {
		t.Fatal(err)
	}

	for path, data := range map[string][]byte{
		"proposal.md":       []byte("# Proposal\n"),
		"design.md":         []byte("# Design\n"),
		"verify-report.md":  []byte("## Verdict\n\n**PASS — archive ready.**\n\n## Critical Findings\n\n0\n\n## Blockers\n\nNone\n"),
		"archive-report.md": []byte("# Archive report\n"),
		filepath.Join("specs", "delivery", "spec.md"):           []byte("# Delivery Delta\n"),
		filepath.Join("specs", "delivery", "nested", "spec.md"): []byte("# Nested Delivery Delta\n"),
	} {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	status, err := archiveStatus(root, "jarvis-dev")
	if err != nil {
		t.Fatal(err)
	}
	if status.Artifacts[sddstatus.ArtifactSpec] != sddstatus.ArtifactDone || (status.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepReady && status.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepAllDone) {
		t.Fatalf("archive status = %#v, want nested delta specs to permit archive", status)
	}

	destination := filepath.Join(workspace, "openspec", "changes", "archive", "2026-09-10-issue-653")
	if err := runSddArchive(store, status, destination); err != nil {
		t.Fatalf("runSddArchive() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(destination, "specs", "delivery", "spec.md"),
		filepath.Join(destination, "specs", "delivery", "nested", "spec.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("archived nested delta spec %q: %v", path, err)
		}
	}
}

func TestRunSddArchiveFailsClosedOnLifecycleState(t *testing.T) {
	for _, tt := range []struct {
		name, store string
		progress    sddstatus.ArtifactState
		dependency  sddstatus.DependencyState
		wantErr     bool
	}{
		{"partial", "openspec", sddstatus.ArtifactPartial, sddstatus.DepBlocked, true},
		{"diverged", "hybrid", sddstatus.ArtifactBlockedBackendDiverged, sddstatus.DepBlocked, true},
		{"hive mode", "hive", sddstatus.ArtifactDone, sddstatus.DepReady, true},
		{"complete", "openspec", sddstatus.ArtifactDone, sddstatus.DepReady, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			status := &sddstatus.ChangeStatus{ArtifactStore: tt.store, ChangeRoot: "change", ActionContext: sddstatus.ActionContext{AllowedEditRoots: []string{"."}}, Artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactApplyProgress: tt.progress}, Dependencies: map[string]sddstatus.DependencyState{sddstatus.PhaseArchive: tt.dependency}}
			err := runSddArchive(archiveFunc(func(string) error { calls++; return nil }), status, "archive/change")
			if (err != nil) != tt.wantErr || calls != map[bool]int{true: 0, false: 1}[tt.wantErr] {
				t.Fatalf("runSddArchive() error=%v calls=%d", err, calls)
			}
		})
	}
}

func archiveReadyStatus(root, _ string) (*sddstatus.ChangeStatus, error) {
	return &sddstatus.ChangeStatus{ArtifactStore: "openspec", ChangeRoot: root, ActionContext: sddstatus.ActionContext{AllowedEditRoots: []string{filepath.Dir(root)}}, Artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactApplyProgress: sddstatus.ArtifactDone}, Dependencies: map[string]sddstatus.DependencyState{sddstatus.PhaseArchive: sddstatus.DepReady}}, nil
}

type archiveFunc func(string) error

func (f archiveFunc) Archive(destination string) error { return f(destination) }
func (f archiveFunc) ArchiveWithLifecycleValidation(destination string, _ func() error) error {
	return f(destination)
}

func TestPrintStatusHuman_AllDone_ShowsComplete(t *testing.T) {
	s := &sddstatus.ChangeStatus{
		Schema:          sddstatus.StatusSchema,
		ChangeName:      "my-feature",
		ArtifactStore:   "hive",
		ArtifactPaths:   map[string]string{},
		Artifacts:       map[string]sddstatus.ArtifactState{},
		Dependencies:    map[string]sddstatus.DependencyState{},
		NextRecommended: "none",
		BlockedReasons:  nil,
	}

	for _, phase := range sddstatus.PhaseOrder {
		s.Dependencies[phase] = sddstatus.DepAllDone
		s.Artifacts[sddstatus.PhaseOutput[phase]] = sddstatus.ArtifactDone
	}

	var buf strings.Builder
	printStatusHuman(&buf, s, false)
	out := buf.String()

	if !strings.Contains(out, "all phases complete") {
		t.Errorf("printStatusHuman must say 'all phases complete' when all done and no blocked reasons; got:\n%s", out)
	}
}
