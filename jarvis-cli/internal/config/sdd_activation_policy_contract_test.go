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

	// Explicit commands must take precedence over complexity heuristics.
	explicitIdx := strings.Index(orchestrator, "explicit user commands take precedence")
	heuristicIdx := strings.Index(orchestrator, "multiple deliverables or cross-component impact")
	if explicitIdx == -1 {
		t.Fatalf("orchestrator policy must state that explicit user commands take precedence")
	}
	if heuristicIdx == -1 {
		t.Fatalf("orchestrator policy must describe complexity-based SDD recommendation")
	}
	if explicitIdx > heuristicIdx {
		t.Fatalf("explicit override rule must be documented before complexity heuristic (precedence order)")
	}

	// Must document that the model waits before proceeding when recommending SDD.
	if !strings.Contains(orchestrator, "do not write code or plans until the user responds") {
		t.Fatalf("orchestrator policy must require waiting for user response before writing code")
	}

	// Confirmation must not bypass session preflight.
	if !strings.Contains(orchestrator, "confirmation does not satisfy preflight") {
		t.Fatalf("orchestrator policy must state that SDD confirmation does not satisfy session preflight")
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
		"never mix languages in a single preflight prompt",
		"headings, option titles, and descriptions together",
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
}

func TestSDDOrchestrator_ComplexityFixturesAndScopeGuardrails(t *testing.T) {
	orchestrator := readPolicyFile(t, "embed/orchestrator/sdd-orchestrator.md")

	// Policy must describe complexity-based recommendation behavior.
	if !strings.Contains(orchestrator, "multiple deliverables or cross-component impact") {
		t.Fatalf("policy must describe complexity signal for SDD recommendation")
	}

	// Policy must describe trivial explicit-SDD handling.
	if !strings.Contains(orchestrator, "trivial request explicitly invokes sdd") {
		t.Fatalf("policy must describe handling of trivial requests that explicitly invoke SDD")
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

	// Must suggest inline once in the first response only, then allow SDD.
	if !strings.Contains(orchestrator, "suggest inline once in the first response only") {
		t.Fatalf("policy must suggest inline once in first response only for trivial explicit SDD requests")
	}
	if !strings.Contains(orchestrator, "follow sdd without further inline pushback") {
		t.Fatalf("policy must allow SDD without further inline pushback after user confirms")
	}
	// Confirmation must still go through preflight.
	if !strings.Contains(orchestrator, "subject to the session preflight hard gate") {
		t.Fatalf("policy must require session preflight after trivial-SDD confirmation")
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

	// Must document that inline suggestions stop after user confirms SDD.
	if !strings.Contains(orchestrator, "follow sdd without further inline pushback") {
		t.Fatalf("policy must stop inline suggestions after user confirms SDD")
	}
	// Confirmation must not satisfy session preflight.
	if !strings.Contains(orchestrator, "confirmation does not satisfy preflight") {
		t.Fatalf("policy must state that SDD confirmation does not satisfy session preflight")
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

	// Core decision concepts must be present in both files (semantic alignment).
	orchestratorConcepts := []string{"recommendation", "explicit", "warning"}
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
