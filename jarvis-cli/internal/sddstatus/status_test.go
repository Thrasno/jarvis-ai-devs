package sddstatus_test

import (
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

