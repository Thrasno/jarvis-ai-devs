package main

import (
	"context"
	"errors"
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
			sddstatus.ArtifactTasks:        "- [x] T1\n- [ ] T2\n",
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

func TestSddArchiveCommandMovesValidatedTopology(t *testing.T) {
	root := t.TempDir()
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
	if err := os.WriteFile(filepath.Join(root, "tasks.md"), []byte("- [ ] 1.1 task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "issue-653")
	command := newSddArchiveCommand(func(root string) sddArchiver { return sddprogress.OpenSpec{Root: root} }, archiveReadyStatus)
	command.SetArgs([]string{"--root", root, "--destination", archive})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(archive, "apply-progress.md")); err != nil {
		t.Fatalf("archived snapshot: %v", err)
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
			status := &sddstatus.ChangeStatus{ArtifactStore: tt.store, Artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactApplyProgress: tt.progress}, Dependencies: map[string]sddstatus.DependencyState{sddstatus.PhaseArchive: tt.dependency}}
			err := runSddArchive(archiveFunc(func(string) error { calls++; return nil }), status, "archive/change")
			if (err != nil) != tt.wantErr || calls != map[bool]int{true: 0, false: 1}[tt.wantErr] {
				t.Fatalf("runSddArchive() error=%v calls=%d", err, calls)
			}
		})
	}
}

func archiveReadyStatus(string, string) (*sddstatus.ChangeStatus, error) {
	return &sddstatus.ChangeStatus{ArtifactStore: "openspec", Artifacts: map[string]sddstatus.ArtifactState{sddstatus.ArtifactApplyProgress: sddstatus.ArtifactDone}, Dependencies: map[string]sddstatus.DependencyState{sddstatus.PhaseArchive: sddstatus.DepReady}}, nil
}

type archiveFunc func(string) error

func (f archiveFunc) Archive(destination string) error { return f(destination) }

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
