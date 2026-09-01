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

func TestValidatedEditRootsForProjectRequiresKnownProjectAndWorkspaceRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jarvis-dev")

	got := validatedEditRootsForProject("jarvis-dev", root)
	if len(got) != 1 || got[0] != root {
		t.Fatalf("validatedEditRootsForProject matching root = %#v, want %q", got, root)
	}

	mismatchedWorktreeRoot := filepath.Join(t.TempDir(), "epic-06-projects-repository-api")
	got = validatedEditRootsForProject("jarvis-dev", mismatchedWorktreeRoot)
	if len(got) != 1 || got[0] != mismatchedWorktreeRoot {
		t.Fatalf("validatedEditRootsForProject worktree root = %#v, want %q", got, mismatchedWorktreeRoot)
	}

	for _, tt := range []struct {
		name        string
		projectName string
		root        string
	}{
		{name: "missing project", projectName: "", root: root},
		{name: "missing root", projectName: "jarvis-dev", root: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatedEditRootsForProject(tt.projectName, tt.root); len(got) != 0 {
				t.Fatalf("validatedEditRootsForProject(%q, %q) = %#v, want empty", tt.projectName, tt.root, got)
			}
		})
	}
}

func TestBuildStatus_ApplyReadyUsesWorktreeRootAsAllowedEditRoot(t *testing.T) {
	worktreeRoot := filepath.Join(t.TempDir(), "epic-06-projects-repository-api")
	editRoots := validatedEditRootsForProject("jarvis-dev", worktreeRoot)

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
	if got := status.ActionContext.AllowedEditRoots; len(got) != 1 || got[0] != worktreeRoot {
		t.Fatalf("ActionContext.AllowedEditRoots = %#v, want [%q]", got, worktreeRoot)
	}
	if got := status.AllowedEditRoots; len(got) != 1 || got[0] != worktreeRoot {
		t.Fatalf("AllowedEditRoots = %#v, want [%q]", got, worktreeRoot)
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
