# Exploration — persona-voice-foundations

Change 1 of 2. Mechanism + correctness for persona rendering. NO concrete persona
authoring (Argentino/Yoda/Stark) — that is Change 2.

## Current state (verified against code)

Two-layer model confirmed:
- **Layer 1** = `jarvis-cli/embed/technical-contract.md` — persona-INVARIANT technical/educational soul. There is NO `gentleman.yaml`.
- **Layer 2** = `jarvis-cli/embed/personas/*.yaml`, rendered by `internal/persona/loader.go`.

`renderPresentation(preset, outputStyle bool)` (loader.go:90-115) emits a terse bullet
list of the 13 presentation fields. Only 2 are expanded to prose, each for one enum value:
- `presentationLanguage()` (117-122): `es-rioplatense` → "Rioplatense Spanish (voseo)"; else raw.
- `presentationRegister()` (124-129): `warm-direct` → "warm, energetic, and direct"; else raw.

`RenderLayer2` = renderPresentation(…,false). `RenderOutputStyle` = renderPresentation(…,true)
+ frontmatter incl. `keep-coding-instructions: true`.

**Parity note:** output-style is Claude-only (`ClaudeAgent.WriteOutputStyle`);
`OpenCodeAgent.WriteOutputStyle` is a no-op — OpenCode gets persona only via Layer 2 in
AGENTS.md (`ApplyProfile → RenderLayer2 → WriteInstructions`). Therefore all four fixes
MUST live inside `renderPresentation` (shared) and `technical-contract.md`, NOT in
RenderOutputStyle-only frontmatter, or Claude/OpenCode parity breaks.

The misleading `- Language: <X>` bullet originates at loader.go:101 and is the only
language signal to the model.

## Corrections to prior assumptions

1. **The reply-language rule does NOT exist** as a general directive. Repo-wide grep found
   it only in `sdd-orchestrator.md:111-115`, scoped to the SDD preflight prompt.
   `technical-contract.md` has no general reply-language rule. → Fix #2 must CREATE an
   authoritative rule (home: Layer 1), not relocate one.
2. **Contract-supremacy clause confirmed absent** (grep override|precede|conflict|wins|
   subordinate|supremacy = 0 hits). The existing "Persona Scope and Artifact Language"
   section scopes persona to replies/artifacts but sets no precedence over epistemic rules.
3. **Golden files confirmed absent** — no `*.golden`, no persona fixtures. De-risks change.

## Personas + enums

7 builtins + `custom.yaml.tmpl`.
- Regional language: argentino (es-rioplatense), asturiano (es-asturian), galleguinho (es-galician).
- es-neutral: neutra, yoda, sargento. en-us: tony-stark.

Enum universe frozen in `preset_v2.go` `v2AllowedPresentationValues`
(language set: es-rioplatense, es-neutral, es-asturian, es-galician, en-us).

**Portability (fix #3, renderer-side only):** regional language + regional pack ⇒ bound;
else portable. → argentino/asturiano/galleguinho = bound; neutra/yoda/sargento/tony-stark
= portable. Computable from the in-memory `Presentation` struct; zero changes to
preset_v2.go or yaml.
⚠ OPEN QUESTION: es-neutral personas (yoda/sargento/neutra) classify portable despite a
Spanish neutral flavor — confirm intended.

## Test invariants constraining new prose (persona package)

- FORBIDDEN strings in both render outputs: "Persona Scope (CRITICAL)",
  "Always propose alternatives with tradeoffs", "Technical Behavior", "CONCEPTS > CODE",
  "AI IS A TOOL", "workflow_rules", "Response Length Contract", "## Notes"; user
  `display_name` must never leak (renderer uses `## Persona: <TitleCase(slug)>`).
- `TestRenderV2PresentationRendersEverySelectedTrait` and
  `TestRenderV2PresentationKeepsPolicyOutOfPresentationSurfaces` assert EXACT bullet strings.
  Any bullet/format change (fix #2/#4) requires updating these assertions FIRST (strict TDD).
- `keep-coding-instructions: true` must remain.
- `TestBuiltinProfilesV2MatchPresentationMatrix` pins each builtin tuple at asset level.

## Affected areas

- `jarvis-cli/embed/technical-contract.md` — fix #1 + fix #2.
- `jarvis-cli/internal/persona/loader.go` — fix #2, #3, #4.
- `jarvis-cli/internal/persona/bridge_test.go`, `v2_test.go`, `apply_test.go` — update exact
  trait assertions; preserve forbidden-string invariants.
- `docs/personas.md` — pinned by tests for Argentine contract strings; verify no contradiction.
- Unchanged: `preset_v2.go`, all `personas/*.yaml`, claude.go/opencode.go/apply.go.

## Approaches

1. **All four fixes in renderer + Layer 1, schema frozen — RECOMMENDED.** Single source of
   truth in Go/Layer 1, zero schema churn, automatic Claude+OpenCode parity, no goldens.
   Cons: exact-match assertions updated in lockstep; prose maps must be exhaustive. Medium.
2. Push prose/portability into yaml — REJECTED. Violates "no schema change", reintroduces
   injection surface.

## Recommendation

Approach 1. One work unit per fix under strict TDD, each well under the 400-line budget:
#1 supremacy clause → #2 reply-language rule + remove Language imperative → #3 portability
classifier + affirmation/fallback clause (table-driven over 7 builtins) → #4 exhaustive
prose maps with raw-ID fallback.

## Risks

- Exact-match trait assertions break on any bullet-format change — update tests first.
- Prose maps must be exhaustive or fall back; never render empty.
- Reply-language rule is newly created — single-source in Layer 1; don't contradict the
  orchestrator preflight.
- Custom user personas flow through the same renderer — handle arbitrary valid enum combos
  without asserting caricature.
- es-neutral personas classify portable — confirm intended.
- Everything reaching OpenCode must live in `renderPresentation` (shared).
- Never edit generated `~/.claude/*` files; source of truth is `embed/` + `internal/`.

Engram artifact: `sdd/persona-voice-foundations/explore` (id 4453).
