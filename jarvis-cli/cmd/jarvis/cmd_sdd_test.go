package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

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
