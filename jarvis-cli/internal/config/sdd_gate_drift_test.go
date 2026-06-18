package config

// sdd_gate_drift_test.go guards executor SKILL.md files and orchestrator prose against
// drift on the ORCHESTRATOR GATE block and the apply-decision keyword contract.
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
}
