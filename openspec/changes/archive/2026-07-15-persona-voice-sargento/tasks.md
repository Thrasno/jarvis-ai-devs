# Tasks: Sargento Persona Voice (Change 2, persona 4, portable)

Spec: `openspec/changes/persona-voice-sargento/specs/persona-presentation-rendering/spec.md`
Design: `openspec/changes/persona-voice-sargento/design.md`
Delivery: single-pr. Strict TDD (RED -> GREEN -> refactor-green).

## 1. RED — add the failing authored-voice test

- [x] 1.1 In `jarvis-cli/internal/persona/v2_test.go`, add
      `TestSargentoPresentationRendersAuthoredVoice`.
  - Load the built-in preset via `fs.ReadFile(jarvis.PersonaFS, "embed/personas/sargento.yaml")`
    + `ValidateAndDecode`, matching the existing pattern used by
    `TestBuiltinPresetsRenderPortabilityAndGateDialectOnlyWhenBound` (loader.go:360-391).
  - For **both** `RenderLayer2(preset)` and `RenderOutputStyle(preset)`, assert
    `strings.Contains` for all 5 stable label-prefix substrings (verbatim from design):
    - `- Register: clipped, terse, and mission-focused`
    - `- Vocabulary: Operational, military vocabulary`
    - `- Address pack: Address the user curtly and directly`
    - `- Phrase pack: Extremely terse, near-monosyllabic delivery`
    - `- Anti-caricature: The gruff, terse edge is delivery style only`
  - Assert **absence** of forbidden Layer-1 strings (e.g. `"CONCEPTS > CODE"`,
    `"AI IS A TOOL"`) and absence of the raw `display_name` value in the rendered output.
  - Do **NOT** assert the Humor bullet (raw `dry` is inherited from Yoda's PR #424;
    out of scope on this branch per design/spec).
  - Run `go test ./jarvis-cli/internal/persona/...` and confirm this new test **fails**
    (raw-ID fallback renders instead of authored prose) before touching loader.go.
  - Satisfies: spec requirements "Sargento's Dedicated Prose Packs Render Authored
    Voice", "Mission-Briefing Register Renders Authored Prose", "Anti-Caricature
    Guardrail...", "Voice-Only Rendering — No Layer-1 Leak, Claude/OpenCode Parity".
  - Parallel: no (must land before task 2; task depends on nothing else).

## 2. GREEN — fill the 4 prose-map literals + register arm

- [x] 2.1 In `jarvis-cli/internal/persona/loader.go`, fill the 4 dedicated map
      entries VERBATIM from the design's Locked Literals table:
      `vocabularyProse["military"]`, `phrasePackProse["sergeant"]`,
      `addressPackProse["sergeant"]`, `antiCaricatureProse["sergeant"]`.
  - Do **NOT** touch `humorProse` (SHARED key `"dry"`, owned by Yoda/PR #424).
  - Keys are additive/disjoint — no existing map entry for any other persona changes.
  - Satisfies: spec requirements "Sargento's Dedicated Prose Packs...", "Anti-Caricature
    Guardrail...", "Authored Packs Are Dedicated to Sargento".
  - Parallel: sequential after 1.1 (same file, same function block as 2.2 — do
    together in one commit-worthy edit, but list separately for review clarity).
- [x] 2.2 In the same file, convert `presentationRegister`'s single-arm `if
      register == "warm-direct"` to a `switch`, keeping the `warm-direct` case
      output byte-identical, and add:
      `case "mission-briefing": return "clipped, terse, and mission-focused"`.
  - Satisfies: spec requirement "Mission-Briefing Register Renders Authored Prose".
  - Note for integrator: Yoda (`calm-teacher`) and Tony (`fast-witty`) each add
    their own disjoint arm in separate changes; the 3-way switch union is a
    trivial, conflict-free merge (flagged in design's Open Questions).
  - Parallel: sequential after 2.1 (same function edit sequence).
- [x] 2.3 Confirm no schema/YAML change was introduced (`git diff` should touch
      only `loader.go` production code + `v2_test.go`); confirm `isBoundDialect`
      untouched so Sargento stays portable (`Language: es-neutral`, generic packs).
  - Satisfies: spec requirement "Sargento Classifies Portable — No Dialect Gating".
  - Parallel: no (verification gate).

## 3. Verify — full test + vet pass

- [x] 3.1 Run `go test ./...` from repo root; confirm
      `TestSargentoPresentationRendersAuthoredVoice` is green and no existing
      test regresses (in particular `TestPresentationValuesResolveNonEmptyWithRawIDFallback`
      and `TestBuiltinPresetsRenderPortabilityAndGateDialectOnlyWhenBound`).
  - Parallel: no (final gate).
- [x] 3.2 Run `go vet ./...`; confirm clean.
  - Parallel: can run alongside 3.1 (independent, no shared mutable state).
- [x] 3.3 Manual spot-check: diff `RenderLayer2` vs `RenderOutputStyle` output for
      the `sargento` preset — "Presentation" and "Language Behavior" bodies must
      be identical (only output-style front matter differs); `RenderOutputStyle`
      output includes `keep-coding-instructions: true`.
  - Satisfies: spec requirement "Voice-Only Rendering — No Layer-1 Leak,
    Claude/OpenCode Parity".
  - Parallel: no (final gate).

## Constraints recap (do not violate)

- No schema/YAML change.
- `humorProse["dry"]` is explicitly out of scope — not authored, not asserted.
- All 4 authored packs are dedicated (disjoint) keys, no shared-key edits.
- Prose is voice-only: no Layer-1 restatement, no `display_name` leak.
- Sargento stays portable: no `isBoundDialect` change.
- Claude (RenderOutputStyle) and OpenCode (RenderLayer2) parity is structural
  (both call the same `renderPresentation`) — no per-surface work needed.
- Source of truth only: `jarvis-cli/embed/` + `jarvis-cli/internal/persona/`.

## Review Workload Forecast

- Diff size: ~6 new/changed lines of production code (4 map literals + 1 switch
  case + if->switch conversion) in `loader.go`, plus one new test function
  (~30-40 lines) in `v2_test.go`. Well under the 400-line "hot path" threshold.
- Risk tier: **standard** — pure in-memory string rendering, no auth/security/
  payments/shell/process-integration surface (per design's Threat Matrix: N/A).
- Recommended lens: single dominant-risk lens, `review-reliability` (behavior/
  test/regression risk: does the switch conversion preserve `warm-direct` byte-
  identical output; do disjoint map keys avoid touching other personas).
- Integration note to flag for the reviewer/integrator: `presentationRegister`
  will receive 2 more disjoint arms (Yoda `calm-teacher`, Tony `fast-witty`)
  from sibling changes — the 3-way switch union is a trivial, non-conflicting
  merge, but the reviewer should confirm no arm silently overlaps.
