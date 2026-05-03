package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPolicyFile(t *testing.T, rel string) string {
	t.Helper()
	root := "/home/andres/Desarrollo/Proyectos/jarvis-dev/jarvis-cli"
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return strings.ToLower(string(b))
}

func TestSDDOrchestrator_ActivationPolicyContract(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	if !strings.Contains(orchestrator, "force_sdd") || !strings.Contains(orchestrator, "force_inline") || !strings.Contains(orchestrator, "recommendation_only") {
		t.Fatalf("orchestrator policy must include decision tokens force_sdd|force_inline|recommendation_only")
	}

	explicitIdx := strings.Index(orchestrator, "explicit override")
	heuristicIdx := strings.Index(orchestrator, "complexity")
	if explicitIdx == -1 || heuristicIdx == -1 {
		t.Fatalf("orchestrator policy must describe explicit override and complexity heuristic")
	}
	if explicitIdx > heuristicIdx {
		t.Fatalf("explicit override must be documented before complexity heuristic (precedence order)")
	}

	if !strings.Contains(orchestrator, "warning-only") {
		t.Fatalf("orchestrator policy must explicitly define warning-only pushback")
	}

	// Precedence contract: decision order must be mandatory and deterministic
	if !strings.Contains(orchestrator, "decision order") || !strings.Contains(orchestrator, "mandatory") {
		t.Fatalf("orchestrator policy must enforce mandatory decision order (explicit first, heuristics second)")
	}
	if !strings.Contains(orchestrator, "deterministic") {
		t.Fatalf("orchestrator policy must enforce deterministic decision order")
	}
}

func TestSDDOrchestrator_BilingualOverrideVocabulary(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, phrase := range []string{"use sdd", "usa sdd", "let's use sdd", "quiero sdd"} {
		if !strings.Contains(orchestrator, phrase) {
			t.Fatalf("missing explicit sdd override phrase: %q", phrase)
		}
	}

	for _, phrase := range []string{"do it inline", "do it directly", "hacelo directo", "sin sdd"} {
		if !strings.Contains(orchestrator, phrase) {
			t.Fatalf("missing explicit inline override phrase: %q", phrase)
		}
	}

	// Normalization must be deterministic and precise enough for test coverage
	for _, normRule := range []string{"lowercase", "strip", "whitespace", "accent", "exact phrase match"} {
		if !strings.Contains(orchestrator, normRule) {
			t.Fatalf("missing normalization rule keyword: %q (normalization must be deterministic)", normRule)
		}
	}
	// Normalization must specify HOW to strip accents, not just "strip accents"
	if !strings.Contains(orchestrator, "á") || !strings.Contains(orchestrator, "spanish accent") {
		t.Fatalf("normalization rules must include concrete accent mapping (e.g., á→a) for deterministic implementation")
	}
	// Punctuation scope must clarify leading/trailing ONLY, never internal
	if !strings.Contains(orchestrator, "leading/trailing punctuation") || !strings.Contains(orchestrator, "not internal") {
		t.Fatalf("normalization rules must explicitly clarify punctuation scope: leading/trailing only, never internal punctuation")
	}
	// Order dependency must be explicit where needed
	if !strings.Contains(orchestrator, "order dependency") || !strings.Contains(orchestrator, "accent removal happens before") {
		t.Fatalf("normalization rules must explicitly state order dependency (accent removal before punctuation)")
	}
}

func TestSDDOrchestrator_ComplexityFixturesAndScopeGuardrails(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, fixture := range []string{"trivial copy tweak", "single-file bugfix", "multi-artifact feature"} {
		if !strings.Contains(orchestrator, fixture) {
			t.Fatalf("missing complexity fixture: %q", fixture)
		}
	}

	if !strings.Contains(orchestrator, "mixed") || !strings.Contains(orchestrator, "inline recommendation") {
		t.Fatalf("policy must force mixed/unclear complexity to inline recommendation")
	}

	if !strings.Contains(orchestrator, "must not redesign runtime hardening") {
		t.Fatalf("policy must include scope guardrail excluding runtime hardening redesign")
	}

	layer1 := readPolicyFile(t, "internal/config/layer1.md")
	if !(strings.Contains(layer1, "complexity check") || strings.Contains(layer1, "complexity_check")) || !strings.Contains(layer1, "recommendation") {
		t.Fatalf("layer1 must describe complexity_check as recommendation-only guidance")
	}
	if !strings.Contains(layer1, "explicit user command") || !strings.Contains(layer1, "takes precedence") {
		t.Fatalf("layer1 must enforce explicit user-command precedence over heuristics")
	}
}

func TestSDDOrchestrator_TrivialExplicitSDD_RecommendInlineButAllowOverride(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	if !strings.Contains(orchestrator, "inline/direct as lower-friction guidance") || !strings.Contains(orchestrator, "first response only") {
		t.Fatalf("policy must require inline/direct recommendation in FIRST response only for trivial explicit sdd requests")
	}
	if !strings.Contains(orchestrator, "immediately start sdd flow") || !strings.Contains(orchestrator, "without further pushback") {
		t.Fatalf("policy must keep trivial-work inline recommendation non-blocking and allow immediate SDD after reconfirmation")
	}
}

func TestSDDOrchestrator_TrivialExplicitSDD_UserReconfirmsThenProceedsSDD(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	// Behavior transition must define WHAT counts as reconfirmation
	if !strings.Contains(orchestrator, "reconfirmation detector") && !strings.Contains(orchestrator, "what counts as") {
		t.Fatalf("policy must explicitly define what counts as user reconfirmation (detector contract)")
	}
	// Must define when inline suggestions stop
	if !strings.Contains(orchestrator, "stop suggesting inline") || !strings.Contains(orchestrator, "without further pushback") {
		t.Fatalf("policy must explicitly define that inline suggestions stop after reconfirmation")
	}
	// Must define immediate SDD flow trigger after confirmation
	if !strings.Contains(orchestrator, "immediately") || !strings.Contains(orchestrator, "proceed") || !strings.Contains(orchestrator, "sdd") {
		t.Fatalf("policy must explicitly define immediate sdd flow after confirmation")
	}
	// Behavior transition must be unambiguous and testable
	if !strings.Contains(orchestrator, "affirmative") || (!strings.Contains(orchestrator, "yes") && !strings.Contains(orchestrator, "continue")) {
		t.Fatalf("policy must include affirmative intent keywords (yes, continue) as reconfirmation triggers")
	}
	// Reconfirmation must use SAME normalization pipeline as explicit override detection
	if !strings.Contains(orchestrator, "same normalization pipeline") {
		t.Fatalf("policy must explicitly state that reconfirmation detection uses the same normalization pipeline as explicit override detection")
	}
}

func TestSDDActivationPolicy_Layer1DriftGuard(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")
	layer1 := readPolicyFile(t, "internal/config/layer1.md")

	// Layer1 must reference orchestrator as canonical source
	if !strings.Contains(layer1, "canonical source") || !strings.Contains(layer1, "sdd-orchestrator.md") {
		t.Fatalf("layer1.md must reference sdd-orchestrator.md as canonical source to prevent drift")
	}
	// Layer1 must DEFER to orchestrator for normalization, vocabulary, decision order
	if !strings.Contains(layer1, "defers") || !strings.Contains(layer1, "orchestrator is authoritative") {
		t.Fatalf("layer1.md must explicitly defer to orchestrator for critical decision contracts (normalization, vocabulary, order)")
	}

	// Core decision concepts must be present in both files (semantic alignment)
	// Orchestrator uses "decision order" while layer1 uses "precedence" — both valid
	orchestratorConcepts := []string{"recommendation", "explicit", "warning", "decision order"}
	layer1Concepts := []string{"recommendation", "explicit", "warning", "precedence"}
	
	for _, concept := range orchestratorConcepts {
		if !strings.Contains(orchestrator, concept) {
			t.Fatalf("orchestrator missing core decision concept: %q", concept)
		}
	}
	
	for _, concept := range layer1Concepts {
		if !strings.Contains(layer1, concept) {
			t.Fatalf("layer1.md missing core decision concept: %q (drift from orchestrator)", concept)
		}
	}

	// Decision order description must align semantically
	if strings.Contains(orchestrator, "deterministic") && !strings.Contains(layer1, "deterministic") {
		t.Fatalf("layer1.md must enforce same deterministic decision order as orchestrator")
	}
}
