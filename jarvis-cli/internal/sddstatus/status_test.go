package sddstatus_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func artifacts(pairs ...string) map[string]sddstatus.ArtifactState {
	m := make(map[string]sddstatus.ArtifactState, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = sddstatus.ArtifactState(pairs[i+1])
	}
	return m
}

func TestComputeStatus_JSONContractIncludesRuntimeContext(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts:        allPlanningDone(),
		ActionMode:       sddstatus.ActionModeWorkspaceEdit,
		AllowedEditRoots: []string{"/workspace/jarvis-dev"},
	})

	if s.Schema != "jarvis.sdd-status" {
		t.Fatalf("schema = %q, want jarvis.sdd-status", s.Schema)
	}
	if s.PlanningHome != "sdd/my-feature" {
		t.Fatalf("PlanningHome = %q, want sdd/my-feature", s.PlanningHome)
	}
	if s.ChangeRoot != "sdd/my-feature" {
		t.Fatalf("ChangeRoot = %q, want sdd/my-feature", s.ChangeRoot)
	}
	if got := s.ContextFiles[sddstatus.ArtifactSpec]; got != "sdd/my-feature/spec" {
		t.Fatalf("contextFiles[spec] = %q, want sdd/my-feature/spec", got)
	}
	if got := s.AllowedEditRoots; len(got) != 1 || got[0] != "/workspace/jarvis-dev" {
		t.Fatalf("AllowedEditRoots = %#v, want current workspace root", got)
	}
	if s.ActionContext.Mode != sddstatus.ActionModeWorkspaceEdit {
		t.Fatalf("ActionContext.Mode = %q, want %q", s.ActionContext.Mode, sddstatus.ActionModeWorkspaceEdit)
	}
	if got := s.ActionContext.AllowedEditRoots; len(got) != 1 || got[0] != "/workspace/jarvis-dev" {
		t.Fatalf("ActionContext.AllowedEditRoots = %#v, want current workspace root", got)
	}
	if got := s.PhaseInstructions[sddstatus.PhaseApply]; got != "/sdd-apply my-feature" {
		t.Fatalf("PhaseInstructions[apply] = %q, want /sdd-apply my-feature", got)
	}

	foundApplyRelationship := false
	for _, rel := range s.Relationships {
		if rel.Phase == sddstatus.PhaseApply && rel.OutputArtifact == sddstatus.ArtifactApplyProgress {
			foundApplyRelationship = containsAll(rel.Requires, []string{sddstatus.ArtifactSpec, sddstatus.ArtifactDesign, sddstatus.ArtifactTasks})
		}
	}
	if !foundApplyRelationship {
		t.Fatalf("relationships must describe sdd-apply required artifacts; got %#v", s.Relationships)
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	jsonText := string(data)
	for _, key := range []string{"contextFiles", "planningHome", "changeRoot", "actionContext", "allowedEditRoots", "relationships", "phaseInstructions"} {
		if !strings.Contains(jsonText, `"`+key+`"`) {
			t.Fatalf("status JSON missing key %q: %s", key, jsonText)
		}
	}
}

func TestComputeStatus_PartialArtifactStateIsPreservedButNotReady(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: artifacts(
			sddstatus.ArtifactProposal, string(sddstatus.ArtifactDone),
			sddstatus.ArtifactSpec, string(sddstatus.ArtifactPartial),
			sddstatus.ArtifactDesign, string(sddstatus.ArtifactDone),
		),
	})

	if got := s.Artifacts[sddstatus.ArtifactSpec]; got != sddstatus.ArtifactPartial {
		t.Fatalf("Artifacts[spec] = %q, want partial", got)
	}
	if got := s.Dependencies[sddstatus.PhaseTasks]; got != sddstatus.DepBlocked {
		t.Fatalf("tasks dep = %q, want blocked while spec is partial", got)
	}
}

func containsAll(got []string, want []string) bool {
	seen := make(map[string]bool, len(got))
	for _, item := range got {
		seen[item] = true
	}
	for _, item := range want {
		if !seen[item] {
			return false
		}
	}
	return true
}

func containsString(got []string, want string) bool {
	for _, item := range got {
		if item == want {
			return true
		}
	}
	return false
}

func allPlanningDone() map[string]sddstatus.ArtifactState {
	return artifacts(
		sddstatus.ArtifactProposal, "done",
		sddstatus.ArtifactSpec, "done",
		sddstatus.ArtifactDesign, "done",
		sddstatus.ArtifactTasks, "done",
	)
}

func TestComputeStatus_Schema(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})
	if s.Schema != sddstatus.StatusSchema {
		t.Errorf("schema = %q, want %q", s.Schema, sddstatus.StatusSchema)
	}
}

func TestComputeStatus_ArtifactPaths(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})
	want := "sdd/my-feature/proposal"
	if got := s.ArtifactPaths[sddstatus.ArtifactProposal]; got != want {
		t.Errorf("artifact path for proposal = %q, want %q", got, want)
	}
}

func TestComputeStatus_NilInput_AllMissing(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})
	for _, artifact := range []string{
		sddstatus.ArtifactExplore, sddstatus.ArtifactProposal,
		sddstatus.ArtifactSpec, sddstatus.ArtifactDesign,
		sddstatus.ArtifactTasks, sddstatus.ArtifactApplyProgress,
		sddstatus.ArtifactVerifyReport, sddstatus.ArtifactArchiveReport,
	} {
		if got := s.Artifacts[artifact]; got != sddstatus.ArtifactMissing {
			t.Errorf("artifact %q = %q, want missing", artifact, got)
		}
	}
}

func TestComputeStatus_DefaultsToPlanningWhenNoAllowedEditRoots(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})

	if got := s.ActionContext.Mode; got != sddstatus.ActionModeWorkspacePlanning {
		t.Fatalf("ActionContext.Mode = %q, want %q", got, sddstatus.ActionModeWorkspacePlanning)
	}
	if len(s.ActionContext.AllowedEditRoots) != 0 {
		t.Fatalf("ActionContext.AllowedEditRoots = %#v, want empty", s.ActionContext.AllowedEditRoots)
	}
	if len(s.AllowedEditRoots) != 0 {
		t.Fatalf("AllowedEditRoots = %#v, want empty", s.AllowedEditRoots)
	}
}

func TestComputeStatus_ExplicitEditModeWithoutAllowedEditRootsStillPlans(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		ActionMode:       sddstatus.ActionModeWorkspaceEdit,
		AllowedEditRoots: []string{""},
	})

	if got := s.ActionContext.Mode; got != sddstatus.ActionModeWorkspacePlanning {
		t.Fatalf("ActionContext.Mode = %q, want %q", got, sddstatus.ActionModeWorkspacePlanning)
	}
	if len(s.AllowedEditRoots) != 0 {
		t.Fatalf("AllowedEditRoots = %#v, want empty", s.AllowedEditRoots)
	}
}

func TestComputeStatus_EmptyChange_ExploreAndProposeReady(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})

	if s.Dependencies[sddstatus.PhaseExplore] != sddstatus.DepReady {
		t.Errorf("explore dep = %q, want ready", s.Dependencies[sddstatus.PhaseExplore])
	}
	if s.Dependencies[sddstatus.PhasePropose] != sddstatus.DepReady {
		t.Errorf("propose dep = %q, want ready", s.Dependencies[sddstatus.PhasePropose])
	}
	if s.NextRecommended != sddstatus.PhaseExplore {
		t.Errorf("nextRecommended = %q, want sdd-explore", s.NextRecommended)
	}
}

func TestComputeStatus_EmptyChange_DownstreamBlocked(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})

	for _, phase := range []string{
		sddstatus.PhaseSpec, sddstatus.PhaseDesign, sddstatus.PhaseTasks,
		sddstatus.PhaseApply, sddstatus.PhaseVerify, sddstatus.PhaseArchive,
	} {
		if s.Dependencies[phase] != sddstatus.DepBlocked {
			t.Errorf("dep[%s] = %q, want blocked", phase, s.Dependencies[phase])
		}
	}
}

func TestComputeStatus_ProposalDone_SpecAndDesignReady(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: artifacts(sddstatus.ArtifactProposal, "done"),
	})

	if s.Dependencies[sddstatus.PhaseSpec] != sddstatus.DepReady {
		t.Errorf("spec dep = %q, want ready", s.Dependencies[sddstatus.PhaseSpec])
	}
	if s.Dependencies[sddstatus.PhaseDesign] != sddstatus.DepReady {
		t.Errorf("design dep = %q, want ready", s.Dependencies[sddstatus.PhaseDesign])
	}
	if s.Dependencies[sddstatus.PhaseTasks] != sddstatus.DepBlocked {
		t.Errorf("tasks dep = %q, want blocked (spec missing)", s.Dependencies[sddstatus.PhaseTasks])
	}
}

func TestComputeStatus_PlanningComplete_ApplyReady(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: allPlanningDone(),
	})

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready", s.Dependencies[sddstatus.PhaseApply])
	}
	if s.NextRecommended != sddstatus.PhaseApply {
		t.Errorf("nextRecommended = %q, want sdd-apply", s.NextRecommended)
	}
}

func TestComputeStatus_PlanningComplete_VerifyBlockedWithoutApplyProgress(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: allPlanningDone(),
	})

	if s.Dependencies[sddstatus.PhaseVerify] != sddstatus.DepBlocked {
		t.Errorf("verify dep = %q, want blocked (no apply-progress)", s.Dependencies[sddstatus.PhaseVerify])
	}
}

func TestComputeStatus_VerifyReadyWithApplyProgress(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
	})

	if s.Dependencies[sddstatus.PhaseVerify] != sddstatus.DepReady {
		t.Errorf("verify dep = %q, want ready", s.Dependencies[sddstatus.PhaseVerify])
	}
}

func TestComputeStatus_VerifyBlockedWithPartialApplyProgress(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "partial"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
	})

	if got := s.Artifacts[sddstatus.ArtifactApplyProgress]; got != sddstatus.ArtifactPartial {
		t.Fatalf("Artifacts[apply-progress] = %q, want partial", got)
	}
	if got := s.Dependencies[sddstatus.PhaseVerify]; got != sddstatus.DepBlocked {
		t.Errorf("verify dep = %q, want blocked while apply-progress is partial", got)
	}
}

func TestComputeStatus_VerifyBlockedWithPartialApplyProgressEvenWhenAllTasksDone(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "partial"
	const tasksContent = "- [x] T1\n- [x] T2\n"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: tasksContent},
	})

	if got := s.Dependencies[sddstatus.PhaseVerify]; got != sddstatus.DepBlocked {
		t.Errorf("verify dep = %q, want blocked while apply-progress is partial even when all tasks are done", got)
	}
	if got := s.Dependencies[sddstatus.PhaseApply]; got != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready while apply-progress is partial even when all tasks are done", got)
	}
	if got := s.NextRecommended; got != sddstatus.PhaseApply {
		t.Errorf("nextRecommended = %q, want sdd-apply until partial apply-progress is complete", got)
	}
	if !containsString(s.BlockedReasons, "phase sdd-verify blocked — apply-progress is partial; complete or reconcile sdd-apply before verification") {
		t.Fatalf("BlockedReasons = %#v, want clear partial apply-progress blocker", s.BlockedReasons)
	}
}

func TestComputeStatus_VerifyReadyWhenAllTasksDone(t *testing.T) {
	arts := allPlanningDone()
	const tasksContent = "- [x] T1\n- [x] T2\n"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: tasksContent},
	})

	if s.Dependencies[sddstatus.PhaseVerify] != sddstatus.DepReady {
		t.Errorf("verify dep = %q, want ready (all tasks done)", s.Dependencies[sddstatus.PhaseVerify])
	}
}

func TestComputeStatus_VerifyBlockedWhenTasksPartiallyDone(t *testing.T) {
	arts := allPlanningDone()
	const tasksContent = "- [x] T1\n- [ ] T2\n"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: tasksContent},
	})

	if s.Dependencies[sddstatus.PhaseVerify] != sddstatus.DepBlocked {
		t.Errorf("verify dep = %q, want blocked (tasks partial)", s.Dependencies[sddstatus.PhaseVerify])
	}
}

func TestComputeStatus_ArchiveBlockedWithFailingVerify(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"

	const failingVerify = "## Verify Report\n\nStatus: CRITICAL\n\n3 failed tests."

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactVerifyReport: failingVerify},
	})

	if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepBlocked {
		t.Errorf("archive dep = %q, want blocked (failing verify)", s.Dependencies[sddstatus.PhaseArchive])
	}
}

func TestComputeStatus_ArchiveBlockedWithPendingVerify(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"

	const pendingVerify = "## Verify Report\n\nSome tests are pending and untested."

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactVerifyReport: pendingVerify},
	})

	if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepBlocked {
		t.Errorf("archive dep = %q, want blocked (pending verify)", s.Dependencies[sddstatus.PhaseArchive])
	}
}

func TestComputeStatus_ArchiveReadyWithPassingVerify(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"

	const passingVerify = "## Verify Report\n\nAll checks passed. No issues found."

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactVerifyReport: passingVerify},
	})

	if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepReady {
		t.Errorf("archive dep = %q, want ready", s.Dependencies[sddstatus.PhaseArchive])
	}
	if s.NextRecommended != sddstatus.PhaseArchive {
		t.Errorf("nextRecommended = %q, want sdd-archive", s.NextRecommended)
	}
}

func TestComputeStatus_ApplyAllDoneWhenAllTasksComplete(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	const tasksContent = "- [x] T1\n- [x] T2\n"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: tasksContent},
	})

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepAllDone {
		t.Errorf("apply dep = %q, want all_done", s.Dependencies[sddstatus.PhaseApply])
	}
}

func TestComputeStatus_ExploreAllDone(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: artifacts(sddstatus.ArtifactExplore, "done"),
	})

	if s.Dependencies[sddstatus.PhaseExplore] != sddstatus.DepAllDone {
		t.Errorf("explore dep = %q, want all_done", s.Dependencies[sddstatus.PhaseExplore])
	}
	if s.NextRecommended != sddstatus.PhasePropose {
		t.Errorf("nextRecommended = %q, want sdd-propose", s.NextRecommended)
	}
}

func TestComputeStatus_AllDone_NextIsNone(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactExplore] = "done"
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"
	arts[sddstatus.ArtifactArchiveReport] = "done"

	const passingVerify = "## Verify Report\n\nAll checks passed."

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactVerifyReport: passingVerify},
	})

	if s.NextRecommended != "none" {
		t.Errorf("nextRecommended = %q, want none", s.NextRecommended)
	}
	if len(s.BlockedReasons) != 0 {
		t.Errorf("blockedReasons = %v, want empty", s.BlockedReasons)
	}
}

func TestComputeStatus_TaskProgress_Parsed(t *testing.T) {
	arts := allPlanningDone()
	const tasksContent = "- [x] T1 done\n- [x] T2 done\n- [ ] T3 pending\n"

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: tasksContent},
	})

	if s.TaskProgress == nil {
		t.Fatal("TaskProgress is nil")
	}
	if s.TaskProgress.Total != 3 {
		t.Errorf("total = %d, want 3", s.TaskProgress.Total)
	}
	if s.TaskProgress.Completed != 2 {
		t.Errorf("completed = %d, want 2", s.TaskProgress.Completed)
	}
	if s.TaskProgress.AllDone {
		t.Error("AllDone = true, want false")
	}
}

func TestComputeStatus_ApplyState_NilWhenMissing(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})
	if s.ApplyState != nil {
		t.Errorf("ApplyState = %+v, want nil", s.ApplyState)
	}
}

func TestComputeStatus_ApplyState_PresentWhenProgressExists(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: artifacts(sddstatus.ArtifactApplyProgress, "done"),
	})
	if s.ApplyState == nil {
		t.Fatal("ApplyState is nil")
	}
	if !s.ApplyState.HasProgress {
		t.Error("HasProgress = false, want true")
	}
}

func TestComputeStatus_BlockedReasons_IncludesMissingDeps(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{})

	hasSpecReason := false
	for _, r := range s.BlockedReasons {
		if strings.Contains(r, sddstatus.ArtifactProposal) && strings.Contains(r, sddstatus.PhaseSpec) {
			hasSpecReason = true
		}
	}
	if !hasSpecReason {
		t.Errorf("blocked reasons missing spec→proposal dependency; got: %v", s.BlockedReasons)
	}
}

func TestComputeStatus_PhaseOrderCoverage(t *testing.T) {
	s := sddstatus.ComputeStatus("x", "hive", sddstatus.Input{})
	for _, phase := range sddstatus.PhaseOrder {
		if _, ok := s.Dependencies[phase]; !ok {
			t.Errorf("phase %q missing from Dependencies map", phase)
		}
	}
}

func TestComputeStatus_ArchiveBlocked_EmptyVerifyContent_HasDescriptiveReason(t *testing.T) {
	// When verify-report artifact exists but has no content, the blocked reason must
	// clearly indicate the content is empty — not generically say "must pass".
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"
	// Contents deliberately omitted for verify-report — artifact exists, body is empty.
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
	})
	if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepBlocked {
		t.Fatalf("archive dep = %q, want blocked (empty verify content)", s.Dependencies[sddstatus.PhaseArchive])
	}
	hasEmptyReason := false
	for _, r := range s.BlockedReasons {
		if strings.Contains(r, "empty") {
			hasEmptyReason = true
		}
	}
	if !hasEmptyReason {
		t.Errorf("BlockedReasons should indicate empty content; got: %v", s.BlockedReasons)
	}
}

func TestComputeStatus_ArchiveBlocked_HasBlockedReason(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"
	const failingVerify = "## Verify Report\n\nStatus: CRITICAL\n\n3 failed tests."
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactVerifyReport: failingVerify},
	})
	if len(s.BlockedReasons) == 0 {
		t.Error("BlockedReasons must be non-empty when archive is blocked by failing verify")
	}
}

func TestComputeStatus_VerifyBlocked_HasBlockedReason(t *testing.T) {
	arts := allPlanningDone()
	// No apply-progress, no all-tasks-done — verify should have a soft-blocker reason.
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
	})
	if s.Dependencies[sddstatus.PhaseVerify] != sddstatus.DepBlocked {
		t.Fatalf("verify dep = %q, want blocked", s.Dependencies[sddstatus.PhaseVerify])
	}
	hasVerifyReason := false
	for _, r := range s.BlockedReasons {
		if strings.Contains(r, "sdd-verify") {
			hasVerifyReason = true
		}
	}
	if !hasVerifyReason {
		t.Errorf("BlockedReasons must mention sdd-verify when verify is soft-blocked; got: %v", s.BlockedReasons)
	}
}

func TestVerifyBlockPatterns_ZeroFailedDoesNotBlock(t *testing.T) {
	// "0 failed" must NOT block archive.
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"
	const passingVerify = "## Verify Report\n\n0 failed, all checks passed."
	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactVerifyReport: passingVerify},
	})
	if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepReady {
		t.Errorf("archive dep = %q, want ready when verify says '0 failed'", s.Dependencies[sddstatus.PhaseArchive])
	}
}

func TestVerifyBlockPatterns_PluralForms(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"

	for _, content := range []string{
		"## Verify Report\n\n2 failures found.",
		"## Verify Report\n\nSome blockers remain.",
	} {
		s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
			Artifacts: arts,
			Contents:  map[string]string{sddstatus.ArtifactVerifyReport: content},
		})
		if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepBlocked {
			t.Errorf("archive dep = %q, want blocked for content %q", s.Dependencies[sddstatus.PhaseArchive], content)
		}
	}
}

// --- Slice B: Apply Delivery Decision Gate tests (B1–B7) ---

// applyDecisionBlockedContent is the minimum tasks content that triggers the apply-decision gate.
const applyDecisionBlockedContent = "Decision needed before apply: Yes\n"

// allPlanningDoneWithTasksContent returns allPlanningDone() artifacts plus tasks content in Contents.
func allPlanningDoneWithTasksContent(content string) sddstatus.Input {
	return sddstatus.Input{
		Artifacts: allPlanningDone(),
		Contents:  map[string]string{sddstatus.ArtifactTasks: content},
	}
}

// TestApplyDecisionGate_BlockedWhenDecisionRequired covers spec scenario
// "apply blocked when tasks declare unresolved decision".
func TestApplyDecisionGate_BlockedWhenDecisionRequired(t *testing.T) {
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(applyDecisionBlockedContent))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepBlocked {
		t.Errorf("apply dep = %q, want blocked when decision required and unresolved", s.Dependencies[sddstatus.PhaseApply])
	}

	hasReason := false
	for _, r := range s.BlockedReasons {
		if strings.Contains(r, "Decision needed before apply") {
			hasReason = true
		}
	}
	if !hasReason {
		t.Errorf("BlockedReasons must mention 'Decision needed before apply'; got: %v", s.BlockedReasons)
	}
}

// TestApplyDecisionGate_ReadyWhenDecisionNo covers spec scenario
// "apply ready when tasks carry only the No resolution token" (gate inactive: required=false).
// A tasks artifact with only "No" means the author explicitly flagged this as not needing
// a decision — the gate does not trigger.
func TestApplyDecisionGate_ReadyWhenDecisionNo(t *testing.T) {
	content := "Decision needed before apply: No\n"
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready when only 'Decision needed before apply: No' present (gate inactive)", s.Dependencies[sddstatus.PhaseApply])
	}
}

// TestApplyDecisionGate_ReadyWhenDecisionYesAndNoPresent tests the conflict-resolution
// behavior: when both Yes and No are present (e.g., a tasks artifact that evolved),
// the No resolution token wins and apply is unblocked. Required=true, Resolved=true.
func TestApplyDecisionGate_ReadyWhenDecisionYesAndNoPresent(t *testing.T) {
	// Contradictory content: Yes triggers the gate, No resolves it.
	content := "Decision needed before apply: Yes\nDecision needed before apply: No\n"
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready when both Yes and No are present (No wins)", s.Dependencies[sddstatus.PhaseApply])
	}
	if s.ApplyDecision == nil {
		t.Fatal("ApplyDecision must not be nil when tasks content has decision flags")
	}
	if !s.ApplyDecision.Required {
		t.Errorf("ApplyDecision.Required = false, want true (Yes was present)")
	}
	if !s.ApplyDecision.Resolved {
		t.Errorf("ApplyDecision.Resolved = false, want true (No resolves Yes)")
	}
}

// TestApplyDecisionGate_ReadyWhenChainStrategyStackedToMain covers B3.
func TestApplyDecisionGate_ReadyWhenChainStrategyStackedToMain(t *testing.T) {
	content := "Decision needed before apply: Yes\nChain strategy: stacked-to-main\n"
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready when 'Chain strategy: stacked-to-main' present", s.Dependencies[sddstatus.PhaseApply])
	}
}

// TestApplyDecisionGate_ReadyWhenChainStrategyFeatureBranchChain covers B4.
func TestApplyDecisionGate_ReadyWhenChainStrategyFeatureBranchChain(t *testing.T) {
	content := "Decision needed before apply: Yes\nChain strategy: feature-branch-chain\n"
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready when 'Chain strategy: feature-branch-chain' present", s.Dependencies[sddstatus.PhaseApply])
	}
}

// TestApplyDecisionGate_ReadyWhenSizeException covers B5.
func TestApplyDecisionGate_ReadyWhenSizeException(t *testing.T) {
	content := "Decision needed before apply: Yes\nsize:exception\n"
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready when 'size:exception' present", s.Dependencies[sddstatus.PhaseApply])
	}
}

// TestApplyDecisionGate_ReadyWhenNoDecisionFlag covers spec scenario
// "apply ready when tasks have no decision flag" (B6).
func TestApplyDecisionGate_ReadyWhenNoDecisionFlag(t *testing.T) {
	content := "## Tasks\n\n- [ ] T1\n- [ ] T2\n"
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content))

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepReady {
		t.Errorf("apply dep = %q, want ready when no 'Decision needed before apply' flag", s.Dependencies[sddstatus.PhaseApply])
	}
}

// TestApplyDecisionGate_ApplyDecisionFieldPopulated covers B7.
func TestApplyDecisionGate_ApplyDecisionFieldPopulated(t *testing.T) {
	// Case 1: Required=true, Resolved=false.
	s := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(applyDecisionBlockedContent))
	if s.ApplyDecision == nil {
		t.Fatal("ApplyDecision must not be nil when tasks content is non-empty and has decision flag")
	}
	if !s.ApplyDecision.Required {
		t.Errorf("ApplyDecision.Required = false, want true")
	}
	if s.ApplyDecision.Resolved {
		t.Errorf("ApplyDecision.Resolved = true, want false")
	}

	// Case 2: Required=true, Resolved=true (Decision needed before apply: No).
	content2 := "Decision needed before apply: Yes\nDecision needed before apply: No\n"
	s2 := sddstatus.ComputeStatus("my-feature", "hive",
		allPlanningDoneWithTasksContent(content2))
	if s2.ApplyDecision == nil {
		t.Fatal("ApplyDecision must not be nil for case 2")
	}
	if !s2.ApplyDecision.Required {
		t.Errorf("ApplyDecision.Required = false, want true (case 2)")
	}
	if !s2.ApplyDecision.Resolved {
		t.Errorf("ApplyDecision.Resolved = false, want true (case 2)")
	}

	// Case 3: nil when tasks content is empty.
	s3 := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: allPlanningDone(),
	})
	if s3.ApplyDecision != nil {
		t.Errorf("ApplyDecision = %+v, want nil when tasks content is empty", s3.ApplyDecision)
	}
}

// TestApplyDecisionGate_ApplyProgressDoneBypassesGate pins the behavior that
// when apply-progress is already done, the delivery-decision gate is not evaluated.
// This is intentional: if apply completed in a prior session, the gate was resolved then.
func TestApplyDecisionGate_ApplyProgressDoneBypassesGate(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = sddstatus.ArtifactDone
	content := "Decision needed before apply: Yes\n" // gate would block if evaluated

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: content},
	})

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepAllDone {
		t.Errorf("apply dep = %q, want all_done when apply-progress is done (gate bypassed intentionally)", s.Dependencies[sddstatus.PhaseApply])
	}
}

// TestApplyDecisionGate_BlockedWhenPartialProgressAndDecisionUnresolved pins the
// ordering invariant: the delivery-decision gate fires before the apply-progress=partial
// check, so an in-progress apply batch remains blocked when the gate is unresolved.
func TestApplyDecisionGate_BlockedWhenPartialProgressAndDecisionUnresolved(t *testing.T) {
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = sddstatus.ArtifactPartial

	s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
		Artifacts: arts,
		Contents:  map[string]string{sddstatus.ArtifactTasks: applyDecisionBlockedContent},
	})

	if s.Dependencies[sddstatus.PhaseApply] != sddstatus.DepBlocked {
		t.Errorf("apply dep = %q, want blocked: delivery-decision gate must fire before apply-progress=partial check", s.Dependencies[sddstatus.PhaseApply])
	}

	hasReason := false
	for _, r := range s.BlockedReasons {
		if strings.Contains(r, "Decision needed before apply") {
			hasReason = true
		}
	}
	if !hasReason {
		t.Errorf("BlockedReasons must mention 'Decision needed before apply'; got: %v", s.BlockedReasons)
	}
}

func TestVerifyBlockPatterns_NegatedNounFormsDoNotBlock(t *testing.T) {
	// Common CI summary phrases that contain failure/blocker words in a zero/negated context
	// must NOT block archive. These were false-positives before the verifyNegatedForms strip.
	arts := allPlanningDone()
	arts[sddstatus.ArtifactApplyProgress] = "done"
	arts[sddstatus.ArtifactVerifyReport] = "done"

	passingContents := []string{
		"## Verify Report\n\n0 failures, all checks passed.",
		"## Verify Report\n\nNo failures detected. No blockers remaining.",
		"## Verify Report\n\nAll green. 0 blockers, 0 failures.",
		"## Verify Report\n\nno failure, no blocker found.",
		// non-critical: hyphen creates word boundary before "critical", so \bcritical\b matches
		// without the negated-forms strip. All of these must pass.
		"## Verify Report\n\nOnly non-critical warnings remain.",
		"## Verify Report\n\nAll items are non-critical. Ship it.",
		// no pending / no untested also need to pass
		"## Verify Report\n\nNo pending items. Zero untested paths.",
	}

	for _, content := range passingContents {
		s := sddstatus.ComputeStatus("my-feature", "hive", sddstatus.Input{
			Artifacts: arts,
			Contents:  map[string]string{sddstatus.ArtifactVerifyReport: content},
		})
		if s.Dependencies[sddstatus.PhaseArchive] != sddstatus.DepReady {
			t.Errorf("archive dep = %q, want ready for content %q", s.Dependencies[sddstatus.PhaseArchive], content)
		}
	}
}
