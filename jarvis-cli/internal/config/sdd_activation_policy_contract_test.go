package config

import (
	"strings"
	"testing"
)

func readPolicyFile(t *testing.T, rel string) string {
	t.Helper()
	return strings.ToLower(readConfigTestFile(t, rel))
}

func markdownSection(t *testing.T, content, startHeading, nextHeading string) string {
	t.Helper()

	start := strings.Index(content, startHeading)
	if start == -1 {
		t.Fatalf("missing markdown section start %q", startHeading)
	}

	remainder := content[start:]
	end := strings.Index(remainder[len(startHeading):], nextHeading)
	if end == -1 {
		t.Fatalf("missing markdown section end %q after %q", nextHeading, startHeading)
	}

	return remainder[:len(startHeading)+end]
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

func TestSDDOrchestrator_PreflightHardGateCoversAllSDDEntryPoints(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")
	preflightSection := markdownSection(t, orchestrator, "### sdd session preflight", "### review workload guard")

	for _, required := range []string{
		"sdd session preflight (hard gate)",
		"before executing any mutating, planning, init, apply, verify, or archive sdd command or natural-language sdd request",
		"`/sdd-status` is read-only recovery",
		"must not be blocked by missing session preflight",
		"natural-language equivalents",
		"hard gate rules",
		"if the session has no preflight block",
		"ask the localized user-facing preflight prompt",
		"stop and wait",
		"do not run init",
		"do not delegate phases",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator preflight hard gate missing %q", required)
		}
	}

	for _, command := range []string{
		"/sdd-init",
		"/sdd-new",
		"/sdd-ff",
		"/sdd-continue",
		"/sdd-explore",
		"/sdd-apply",
		"/sdd-verify",
		"/sdd-archive",
	} {
		if !strings.Contains(preflightSection, command) {
			t.Fatalf("orchestrator preflight hard gate must cover %s", command)
		}
	}
}

func TestSDDOrchestrator_PreflightOrderingBeforeInitGuard(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")
	entrySection := markdownSection(t, orchestrator, "### sdd entry routing", "### sdd session preflight")
	initSection := markdownSection(t, orchestrator, "### sdd init guard", "### execution mode")

	for _, required := range []string{
		"preflight → init guard",
		"after preflight is complete",
		"run the init guard",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator must document preflight-before-init ordering with %q", required)
		}
	}

	for _, forbidden := range []string{
		"after init/preflight",
		"init/preflight",
		"sdd-init check → sdd-new",
		"proceed with sdd-init check → sdd-new or requested phase",
	} {
		if strings.Contains(entrySection, forbidden) || strings.Contains(initSection, forbidden) {
			t.Fatalf("orchestrator must not document init-before-preflight ordering with %q", forbidden)
		}
	}

	if !strings.Contains(initSection, "/sdd-init") {
		t.Fatalf("init guard must treat /sdd-init as a mutating SDD command covered after preflight")
	}
	if !strings.Contains(initSection, "`/sdd-status` is read-only recovery") {
		t.Fatalf("init guard must preserve /sdd-status as the no-preflight read-only exception")
	}
}

func TestSDDOrchestrator_ExplicitSDDInitIsSatisfiedByInitGuardOnce(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")
	initSection := markdownSection(t, orchestrator, "### sdd init guard", "### execution mode")

	for _, required := range []string{
		"if the requested command is `/sdd-init`, the init guard itself satisfies the request",
		"after delegated init completes, stop and report the init result",
		"do not proceed to run `/sdd-init` again",
		"if the requested command is not `/sdd-init`",
		"proceed with the requested command",
	} {
		if !strings.Contains(initSection, required) {
			t.Fatalf("init guard must document explicit /sdd-init single-execution behavior with %q", required)
		}
	}

	for _, forbidden := range []string{
		"if not found → run `sdd-init` first (delegate to sdd-init sub-agent), then proceed with the requested command",
		"if not found -> run `sdd-init` first (delegate to sdd-init sub-agent), then proceed with the requested command",
	} {
		if strings.Contains(initSection, forbidden) {
			t.Fatalf("init guard must not imply direct /sdd-init is followed by the requested command again; found %q", forbidden)
		}
	}
}

func TestSDDOrchestrator_StatusCommandIsDirectReadOnlyNotAutocompleteSkill(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")
	commandsSection := markdownSection(t, orchestrator, "### commands", "### sdd init guard")
	skillsSection := markdownSection(t, commandsSection, "skills (appear in autocomplete):", "meta-commands")
	metaStart := strings.Index(commandsSection, "meta-commands")
	if metaStart == -1 {
		t.Fatalf("commands section must include meta/direct orchestrator handling")
	}
	metaSection := commandsSection[metaStart:]

	if strings.Contains(skillsSection, "/sdd-status") {
		t.Fatalf("/sdd-status must not appear under autocomplete skills; status is native/direct orchestrator handling")
	}

	for _, required := range []string{
		"/sdd-status [change]",
		"read-only status",
		"direct orchestrator handling",
		"native `jarvis sdd status`",
		"won't appear in autocomplete",
	} {
		if !strings.Contains(metaSection, required) {
			t.Fatalf("/sdd-status meta/direct handling contract missing %q", required)
		}
	}
}

func TestSDDOrchestrator_PreflightDirectCommandPrecedenceIsNotContradictory(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, required := range []string{
		"read-only status may run without session preflight",
		"mutating, planning, apply, verify, and archive sdd commands require session preflight",
		"unless all four preflight choices were already provided",
		"the sdd session preflight hard gate takes precedence over direct-command bypass wording",
		"outside this sdd hard gate",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator preflight precedence contract missing %q", required)
		}
	}

	if strings.Contains(orchestrator, "never block direct user commands. warnings are advisory only") {
		t.Fatalf("orchestrator must not keep broad direct-command bypass wording that contradicts the SDD preflight hard gate")
	}
}

func TestSDDOrchestrator_PreflightPromptsAndMappingsAreLocalizedAndCanonical(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, required := range []string{
		"before continuing with sdd, choose one option per group.",
		"reply with \"use recommended\" or with codes like: a1, b1, c1, d1.",
		"a. pace",
		"b. artifacts",
		"c. prs",
		"d. review",
		"antes de continuar con sdd, elija una opción por grupo.",
		"responda con \"usar recomendado\" o con códigos como: a1, b1, c1, d1.",
		"a. ritmo",
		"b. artefactos",
		"c. prs",
		"d. revisión",
		"do not mix languages inside one preflight prompt",
		"headings, option titles, descriptions, and follow-up text",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator preflight prompt contract missing %q", required)
		}
	}

	mappings := map[string]string{
		"a1/interactive":  "`interactive`",
		"a2/automatic":    "`auto`",
		"b1/hive":         "`hive`",
		"b2/openspec":     "`openspec`",
		"b3/hybrid":       "`hybrid`",
		"b4/none":         "`none`",
		"c1/ask me":       "`ask-on-risk`",
		"c2/auto-chain":   "`auto-chain`",
		"c3/single pr":    "`single-pr`",
		"c4/exception-ok": "`exception-ok`",
	}
	for code, canonical := range mappings {
		if !strings.Contains(orchestrator, code) || !strings.Contains(orchestrator, canonical) {
			t.Fatalf("orchestrator preflight mapping must include %s -> %s", code, canonical)
		}
	}
	for _, reviewMapping := range []string{
		"d1/400 lines -> `review_budget_lines: 400`",
		"d2/800 lines -> `review_budget_lines: 800`",
		"d3/other -> ask one follow-up for the number",
	} {
		if !strings.Contains(orchestrator, reviewMapping) {
			t.Fatalf("orchestrator preflight review mapping missing %q", reviewMapping)
		}
	}
}

func TestSDDOrchestrator_SpanishPreflightDoesNotExposeEnglishOptionLabels(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	spanishStart := strings.Index(orchestrator, "if the user's current language is spanish")
	if spanishStart == -1 {
		t.Fatalf("orchestrator must include a Spanish localized preflight shape")
	}
	spanishEnd := strings.Index(orchestrator[spanishStart:], "map answers to canonical values")
	if spanishEnd == -1 {
		t.Fatalf("Spanish localized preflight shape must end before canonical mapping")
	}
	spanishPrompt := orchestrator[spanishStart : spanishStart+spanishEnd]

	for _, required := range []string{
		"b3 híbrido",
		"b4 ninguno",
		"c2 encadenar automáticamente",
		"c4 excepción aprobada",
	} {
		if !strings.Contains(spanishPrompt, required) {
			t.Fatalf("Spanish preflight prompt must localize user-facing label %q", required)
		}
	}

	for _, forbidden := range []string{"b3 hybrid", "b4 none", "c2 auto-chain", "c4 exception-ok"} {
		if strings.Contains(spanishPrompt, forbidden) {
			t.Fatalf("Spanish preflight prompt must not expose English user-facing label %q", forbidden)
		}
	}
}

func TestSDDOrchestrator_NativeStatusJSONIsRoutingAuthority(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, required := range []string{
		"native `jarvis.sdd-status` json is authoritative",
		"route only by `nextrecommended` and `dependencies`",
		"never infer routing from prose",
		"blockedreasons stop only the blocked phase or action",
		"do not override a safe `nextrecommended` for a different ready phase",
		"launch the `nextrecommended` phase only when that phase dependency is `ready`",
		"report the relevant `blockedreasons` and stop only for the blocked phase or terminal action",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator routing contract missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"`blockedreasons`: non-empty array blocks apply, verify, and archive",
		"if `blockedreasons` is non-empty, do not launch apply, verify, archive, or terminal work",
	} {
		if strings.Contains(orchestrator, forbidden) {
			t.Fatalf("orchestrator routing contract must not use broad blockedReasons wording %q", forbidden)
		}
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
	if !strings.Contains(orchestrator, "continue the sdd path") || !strings.Contains(orchestrator, "without further inline pushback") {
		t.Fatalf("policy must keep trivial-work inline recommendation non-blocking while allowing SDD after reconfirmation")
	}
	for _, forbidden := range []string{
		"any sdd trigger phrase → start sdd flow",
		"sdd-init check → sdd-new",
		"immediately start sdd flow without further pushback",
		"proceed with sdd-init check → sdd-new or requested phase",
	} {
		if strings.Contains(orchestrator, forbidden) {
			t.Fatalf("recommendation acceptance must not bypass session preflight with %q", forbidden)
		}
	}
}

func TestSDDOrchestrator_ExecutionModePreflightHasNoSilentDefault(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	for _, required := range []string{
		"execution mode is collected by `sdd session preflight`",
		"missing execution-mode choice means preflight is incomplete",
		"ask the localized preflight prompt and stop",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("execution-mode guidance must defer collection to session preflight with %q", required)
		}
	}

	for _, forbidden := range []string{
		"if the user doesn't specify, default to **interactive**",
		"default to **interactive**",
		"default to interactive",
	} {
		if strings.Contains(orchestrator, forbidden) {
			t.Fatalf("execution-mode guidance must not silently default after the hard gate; found %q", forbidden)
		}
	}
}

func TestSDDOrchestrator_TrivialExplicitSDD_UserReconfirmsThenProceedsSDD(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	// Behavior transition must define WHAT counts as reconfirmation
	if !strings.Contains(orchestrator, "reconfirmation detector") && !strings.Contains(orchestrator, "what counts as") {
		t.Fatalf("policy must explicitly define what counts as user reconfirmation (detector contract)")
	}
	// Must define when inline suggestions stop
	if !strings.Contains(orchestrator, "stop suggesting inline") || !strings.Contains(orchestrator, "without further inline pushback") {
		t.Fatalf("policy must explicitly define that inline suggestions stop after reconfirmation")
	}
	// Must define that confirmed natural-language SDD still goes through preflight before init/planning/delegation
	for _, required := range []string{
		"reconfirmation does not satisfy session preflight",
		"before any init guard, planning phase, requested phase, or delegation",
		"session preflight must already be complete",
		"if session preflight is missing, ask the localized preflight prompt and stop",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("trivial explicit-SDD reconfirmation contract missing hard-gate wording %q", required)
		}
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
