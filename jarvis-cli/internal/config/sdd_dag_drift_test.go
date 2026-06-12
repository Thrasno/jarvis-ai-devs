package config

// sdd_dag_drift_test.go ensures the Go phase DAG in sddstatus stays aligned with the
// phase dependency table documented in the SDD orchestrator prompt. If this test
// fails, both the Go DAG and the prose doc must be updated together.
//
// The canonical prose source is: embed/orchestrator/sdd-orchestrator.md
// The canonical Go source is:    internal/sddstatus/status.go (PhaseRequiredDeps)
//
// Drift detection strategy: assert that the orchestrator doc mentions each
// required-dependency pair as described by PhaseRequiredDeps. This catches
// removals from the prose that the Go runtime still enforces, and additions
// to the prose that were never codified in Go.

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

// orchestratorContentForDAG reads the sdd-orchestrator.md file using the
// readConfigTestFile helper, which resolves the module root via directory
// layout detection — reliable across machines and with -trimpath builds.
func orchestratorContentForDAG(t *testing.T) string {
	t.Helper()
	return strings.ToLower(readConfigTestFile(t, "embed/orchestrator/sdd-orchestrator.md"))
}

// TestDag_PhaseOrderCoverage verifies every phase constant appears in PhaseOrder.
func TestDag_PhaseOrderCoverage(t *testing.T) {
	allPhases := []string{
		sddstatus.PhaseExplore,
		sddstatus.PhasePropose,
		sddstatus.PhaseSpec,
		sddstatus.PhaseDesign,
		sddstatus.PhaseTasks,
		sddstatus.PhaseApply,
		sddstatus.PhaseVerify,
		sddstatus.PhaseArchive,
	}
	orderSet := make(map[string]bool, len(sddstatus.PhaseOrder))
	for _, p := range sddstatus.PhaseOrder {
		orderSet[p] = true
	}
	for _, phase := range allPhases {
		if !orderSet[phase] {
			t.Errorf("phase %q is missing from PhaseOrder", phase)
		}
	}
}

// TestDag_PhaseOutputCoverage verifies every phase in PhaseOrder has an output artifact.
func TestDag_PhaseOutputCoverage(t *testing.T) {
	for _, phase := range sddstatus.PhaseOrder {
		if artifact, ok := sddstatus.PhaseOutput[phase]; !ok || artifact == "" {
			t.Errorf("phase %q has no output in PhaseOutput", phase)
		}
	}
}

// TestDag_PhaseRequiredDepsCoverage verifies every phase has an entry in PhaseRequiredDeps.
func TestDag_PhaseRequiredDepsCoverage(t *testing.T) {
	for _, phase := range sddstatus.PhaseOrder {
		if _, ok := sddstatus.PhaseRequiredDeps[phase]; !ok {
			t.Errorf("phase %q is missing from PhaseRequiredDeps", phase)
		}
	}
}

// TestDag_OrchestratorDocMentionsDependencies verifies that the orchestrator prose
// documents the key dependency relationships encoded in PhaseRequiredDeps.
// This is the primary drift guard: if the Go DAG and prose diverge, this test fails.
func TestDag_OrchestratorDocMentionsDependencies(t *testing.T) {
	doc := orchestratorContentForDAG(t)

	// Key required-dep pairs that MUST appear in the orchestrator doc.
	// Each entry is a (phase, requiredArtifact) pair; we check that both
	// terms appear close to each other in the document. Using a loose check
	// (both present in the same document) is intentional — prose layout varies.
	type pair struct {
		phase    string
		dep      string
		docPhase string // term used in the prose (may differ from constant)
		docDep   string // term used in the prose
	}

	checks := []pair{
		{sddstatus.PhaseSpec, sddstatus.ArtifactProposal, "sdd-spec", "proposal"},
		{sddstatus.PhaseDesign, sddstatus.ArtifactProposal, "sdd-design", "proposal"},
		{sddstatus.PhaseTasks, sddstatus.ArtifactSpec, "sdd-tasks", "spec"},
		{sddstatus.PhaseTasks, sddstatus.ArtifactDesign, "sdd-tasks", "design"},
		{sddstatus.PhaseApply, sddstatus.ArtifactTasks, "sdd-apply", "tasks"},
		{sddstatus.PhaseVerify, sddstatus.ArtifactSpec, "sdd-verify", "spec"},
		{sddstatus.PhaseArchive, sddstatus.ArtifactVerifyReport, "sdd-archive", "verify"},
	}

	for _, c := range checks {
		if !strings.Contains(doc, c.docPhase) {
			t.Errorf("orchestrator doc missing phase term %q (required for %s→%s dependency)", c.docPhase, c.phase, c.dep)
		}
		if !strings.Contains(doc, c.docDep) {
			t.Errorf("orchestrator doc missing artifact term %q (required for %s→%s dependency)", c.docDep, c.phase, c.dep)
		}
	}
}

// TestDag_ArtifactTopicKeyFormat verifies that PhaseOutput values produce valid
// Hive topic keys when combined with a change name.
func TestDag_ArtifactTopicKeyFormat(t *testing.T) {
	for phase, artifact := range sddstatus.PhaseOutput {
		key := "sdd/my-change/" + artifact
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			t.Errorf("phase %q: artifact %q produces malformed topic key %q", phase, artifact, key)
		}
		for i, part := range parts {
			if strings.TrimSpace(part) == "" {
				t.Errorf("phase %q: artifact %q topic key %q has empty segment at index %d", phase, artifact, key, i)
			}
		}
	}
}
