package skills

import (
	"strings"
	"testing"
)

func TestCatalogContract_ApplyProgressLifecycleGuidanceIsShared(t *testing.T) {
	const sharedPath = "embed/skills/_shared/apply-progress.md"
	content := readEmbeddedSkillAsset(t, sharedPath)
	for _, snippet := range []string{
		"Continuation Lifecycle",
		"stream_sha256",
		"next_entry_index",
		"next_entry_id",
		"atomic `stream_sha256`, `next_entry_index`, and `next_entry_id` continuation group",
		"advances the cursor",
		"ordered references",
		"atomic continuation group",
		"never repairs or recomputes evidence",
	} {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to contain apply-progress guidance %q", sharedPath, snippet)
		}
	}

	for path, reference := range map[string]string{
		"embed/skills/sdd-apply/SKILL.md":   "skills/_shared/apply-progress.md",
		"embed/skills/sdd-archive/SKILL.md": "skills/_shared/apply-progress.md",
		"embed/skills/sdd-verify/SKILL.md":  "../_shared/apply-progress.md",
	} {
		if !strings.Contains(readEmbeddedSkillAsset(t, path), reference) {
			t.Fatalf("expected %s to reference shared apply-progress guidance %q", path, reference)
		}
	}
}

// sectionBetween returns the text from start up to end, or to the end of
// content when end is empty. Callers pass CRLF-normalized content.
func sectionBetween(t *testing.T, content, start, end string) string {
	t.Helper()
	from := strings.Index(content, start)
	if from < 0 {
		t.Fatalf("expected section start %q", start)
	}
	section := content[from:]
	if end == "" {
		return section
	}
	to := strings.Index(section[len(start):], end)
	if to < 0 {
		t.Fatalf("expected section end %q after %q", end, start)
	}
	return section[:len(start)+to]
}

func readNormalizedAsset(t *testing.T, path string) string {
	t.Helper()
	if strings.HasPrefix(path, "embed/orchestrator/") {
		return strings.ReplaceAll(readLocalOrEmbeddedAsset(t, path), "\r\n", "\n")
	}
	return strings.ReplaceAll(readEmbeddedSkillAsset(t, path), "\r\n", "\n")
}

// TestCatalogContract_ApplyCheckpointsOncePerBatchWithTaskRecords pins the lean
// apply flow: one guarded checkpoint per apply batch built from task_records,
// CLI-derived identities, and outcome handling delegated to the returned next.
func TestCatalogContract_ApplyCheckpointsOncePerBatchWithTaskRecords(t *testing.T) {
	apply := readNormalizedAsset(t, "embed/skills/sdd-apply/SKILL.md")
	persist := sectionBetween(t, apply, "### Step 5: Persist Progress", "### Step 6: Mark Tasks Complete")
	mark := sectionBetween(t, apply, "### Step 6: Mark Tasks Complete", "### Step 7: Return Summary")

	requireAllTerms(t, persist,
		"Checkpoint ONCE per apply batch, not per task",
		"`task_records`",
		"One record per task this batch covers",
		"Omit `request_id`, `batch_id`, and `stream_sha256`",
		"`expected_generation`, `expected_revision`, `expected_digest`",
		"Always send `base` and the expected coordinates from the progress read",
		"follow `next.action` and `next.instruction`",
		"#### Recovery",
		"`run_upgrade_continuation`",
		"Never invent coverage",
	)
	requireAllTerms(t, mark,
		"in one edit",
		"exactly the tasks",
		"`committed`",
	)
	for _, forbidden := range []string{
		"for every normal executor checkpoint",
		"On `continuation_required`, the maximal prefix is committed",
		"Use a new request ID and new batch ID only after `continuation_required`",
		"preserve and supply `stream_sha256` unchanged",
		"Historical pre-continuation reconstruction",
		"Partial continuation cursors must equal their appended entry counts",
	} {
		if strings.Contains(apply, forbidden) {
			t.Fatalf("expected sdd-apply not to retain per-checkpoint ceremony %q", forbidden)
		}
	}

	strict := readNormalizedAsset(t, "embed/skills/sdd-apply/strict-tdd.md")
	if lines := strings.Count(strings.TrimRight(strict, "\n"), "\n") + 1; lines > 220 {
		t.Fatalf("expected strict-tdd.md to stay within 220 lines, got %d", lines)
	}
	requireAllTerms(t, strict,
		"once per affected package at batch start",
		"ONE `task_records` item per task",
		"one run after refactoring",
		"`skip_reason`",
		"`strict-tdd-unrunnable`",
		"batch checkpoint",
	)
	for _, forbidden := range []string{
		"6. Mark task complete [x]",
		"EXECUTE tests after EACH refactoring step",
		"Run existing tests for files being modified",
		"Emit distinct entries for the safety net",
	} {
		if strings.Contains(strict, forbidden) {
			t.Fatalf("expected strict-tdd.md not to retain per-task ceremony %q", forbidden)
		}
	}

	orchestrator := readNormalizedAsset(t, "embed/orchestrator/sdd-orchestrator.md")
	bounded := sectionBetween(t, orchestrator, "## Bounded Apply-Progress Continuation (MANDATORY)", "#### Hive Topic Key Format")
	requireAllTerms(t, bounded,
		"`task_records`",
		"`next.action`",
		"`continue_stream`",
		"`continue_tasks`",
		"`stop_*`",
		"verbatim",
	)
	for _, forbidden := range []string{
		"On `stream_preflight_required`, STOP without a write",
		"On `checkpoint_consolidation_required`, STOP without a write",
		"preserve and supply `stream_sha256` unchanged",
	} {
		if strings.Contains(bounded, forbidden) {
			t.Fatalf("expected orchestrator continuation not to duplicate CLI outcome handling %q", forbidden)
		}
	}

	shared := readNormalizedAsset(t, "embed/skills/_shared/apply-progress.md")
	requireAllTerms(t, shared, "`task_records`", "`next`", "once per apply batch")
}

// TestCatalogContract_ApplyNextHandlingFailsClosedAndIsBounded pins the safety rails
// around `next`: one owner for continue_stream, fail-closed handling when `next` is
// missing or unknown (older CLI), and a bounded retry_identical loop.
func TestCatalogContract_ApplyNextHandlingFailsClosedAndIsBounded(t *testing.T) {
	apply := readNormalizedAsset(t, "embed/skills/sdd-apply/SKILL.md")
	requireAllTerms(t, apply,
		"`continue_stream` is yours to rerun now with the returned cursor (the orchestrator does not relaunch for it)",
		"Fail closed: if the response has no `next`, or its action is not one named here or in Recovery, STOP and return `blocked`",
		"retry at most twice; if it still does not commit, STOP and report `code` and `recovery`",
		"a `skip_reason` triangulation did not run",
	)
	orchestrator := readNormalizedAsset(t, "embed/orchestrator/sdd-orchestrator.md")
	requireAllTerms(t, orchestrator,
		"The executor reruns `continue_stream` itself within the same launch. Relaunch `sdd-apply` only for `continue_tasks`",
	)
}
