package config

// sdd_gate_drift_test.go guards executor SKILL.md files and orchestrator prose against
// drift on the ORCHESTRATOR GATE block and native phase authority.
//
// Canonical executor skills: embed/skills/sdd-{explore,propose,spec,design,tasks,apply,verify,archive}/SKILL.md
// Canonical orchestrator:     embed/orchestrator/sdd-orchestrator.md

import (
	"strings"
	"testing"
)

// executorSkills is the exhaustive list of SDD executor skill directories.
// All eight must carry the ORCHESTRATOR GATE + Executor Override block.
var executorSkills = []string{
	"sdd-explore",
	"sdd-propose",
	"sdd-spec",
	"sdd-design",
	"sdd-tasks",
	"sdd-apply",
	"sdd-verify",
	"sdd-archive",
}

// TestGateDrift_ExecutorSkillsHaveOrchestratorGate verifies every executor SKILL.md
// contains the ORCHESTRATOR GATE block and the Executor Override section, and that
// the gate appears before the override (correct ordering).
func TestGateDrift_ExecutorSkillsHaveOrchestratorGate(t *testing.T) {
	for _, skill := range executorSkills {
		skill := skill
		t.Run(skill, func(t *testing.T) {
			content := readConfigTestFile(t, "embed/skills/"+skill+"/SKILL.md")

			if !strings.Contains(content, "ORCHESTRATOR GATE") {
				t.Errorf("%s/SKILL.md missing 'ORCHESTRATOR GATE' block", skill)
			}
			if !strings.Contains(content, "Executor Override") {
				t.Errorf("%s/SKILL.md missing 'Executor Override' section", skill)
			}

			gateIdx := strings.Index(content, "ORCHESTRATOR GATE")
			overrideIdx := strings.Index(content, "Executor Override")
			if gateIdx >= 0 && overrideIdx >= 0 && gateIdx >= overrideIdx {
				t.Errorf("%s/SKILL.md: 'ORCHESTRATOR GATE' must appear before 'Executor Override' (gate at %d, override at %d)", skill, gateIdx, overrideIdx)
			}
		})
	}
}

func TestGateDrift_OrchestratorRequiresNativeWorkspaceAuthorityForMutatingPhases(t *testing.T) {
	content := readConfigTestFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, required := range []string{
		"`sdd-apply`, `sdd-verify`, or `sdd-archive`",
		"`schema` field equals `jarvis.sdd-status`",
		"dependencies[phase] == `ready`",
		"actionContext.mode == `workspace-edit`",
		"actionContext.allowedEditRoots` is non-empty",
		"Manual recovery cannot invent workspace-edit authority",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("sdd-orchestrator.md missing native workspace-authority contract %q", required)
		}
	}
}

// TestGateDrift_OrchestratorRunsStatusOncePerTransitionAndForwardsReferences verifies
// that the orchestrator runs native status once per mutating phase transition, applies
// the shared Native Status Gate, and forwards the status JSON plus the artifact
// references it already holds so executors do not cold-start.
func TestGateDrift_OrchestratorRunsStatusOncePerTransitionAndForwardsReferences(t *testing.T) {
	content := strings.ReplaceAll(readConfigTestFile(t, "embed/orchestrator/sdd-orchestrator.md"), "\r\n", "\n")

	for _, required := range []string{
		"run `jarvis sdd status <change> --json` exactly once for that phase transition and apply the Native Status Gate (Section G of `_shared/sdd-phase-common.md`)",
		"Forward that status JSON verbatim to the executor together with the artifact references you already hold: Hive observation IDs or OpenSpec paths for proposal, spec, design, and tasks, plus the progress snapshot reference.",
		"This one run also serves routing, the Review Workload Guard, and the Automatic Mode Gatekeeper for that transition; do not run status again before the launch.",
		"orchestrator passes artifact references (Hive observation IDs when known, otherwise topic keys, or OpenSpec file paths), NOT content itself",
		"When the launch prompt forwards an observation ID, the sub-agent calls `mem_get_observation(id)` directly and skips `mem_search`.",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("sdd-orchestrator.md missing single-status-run contract %q", required)
		}
	}

	if got := strings.Count(content, "actionContext.mode == `workspace-edit`"); got != 1 {
		t.Fatalf("sdd-orchestrator.md must state the native authority checks exactly once, got %d", got)
	}

	for _, forbidden := range []string{
		"the orchestrator MUST verify native authority from the current status",
		"If `jarvis sdd status <change> --json` is available and reports",
		"prefer native `jarvis sdd status <change> --json` `nextRecommended`",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("sdd-orchestrator.md must not repeat status-run instructions %q", forbidden)
		}
	}
}

// TestGateDrift_OrchestratorDocNamesApplyDecisionKeyword verifies that the orchestrator
// prose names the exact keyword that the native apply-decision gate enforces, so the two
// cannot drift silently.
func TestGateDrift_OrchestratorDocNamesApplyDecisionKeyword(t *testing.T) {
	content := readConfigTestFile(t, "embed/orchestrator/sdd-orchestrator.md")
	lower := strings.ToLower(content)

	if !strings.Contains(lower, "decision needed before apply") {
		t.Errorf("sdd-orchestrator.md missing 'Decision needed before apply' keyword required by the native apply-decision gate")
	}
	if !strings.Contains(lower, "dependencies") {
		t.Errorf("sdd-orchestrator.md missing 'dependencies' term (required for native status gate reference)")
	}
	if !strings.Contains(lower, "sdd-apply") {
		t.Errorf("sdd-orchestrator.md missing 'sdd-apply' term (required for apply-decision gate section)")
	}
	// The fallback must describe the same whole-line resolution rule as the native gate.
	for _, required := range []string{
		"resolved only by a whole line with exactly one value",
		"`chain strategy: size:exception`",
		"`chain strategy: pending`",
		"bare `size:exception` token do not resolve",
		// The native gate activates on any decision line, including an unfilled placeholder.
		"any `decision needed before apply:` line (including `yes` or an unfilled placeholder)",
	} {
		if !strings.Contains(lower, required) {
			t.Errorf("sdd-orchestrator.md missing whole-line apply-decision fallback rule %q", required)
		}
	}
}
