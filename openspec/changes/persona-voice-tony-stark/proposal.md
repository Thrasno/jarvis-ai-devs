# Proposal: Persona Voice — Tony Stark (Change 2, persona 3 — PORTABLE, supremacy stress-test)

## Intent

Foundations shipped the mechanism with EMPTY prose maps. Tony Stark's voice is
unauthored, so his 5 packs render raw enum IDs and `fast-witty` renders raw.
Tony is the deliberate stress-test: a fast, witty, confident-engineer voice is
exactly what could erode the Layer-1 supremacy rules (verify before asserting,
flag assumptions). This change authors his VOICE while proving the guardrails
hold — confidence as STYLE, never as a substitute for verification.

## Scope

### In Scope
- Fill 5 DEDICATED prose maps in `loader.go`: `vocabularyProse["engineering"]`,
  `humorProse["witty"]`, `phrasePackProse["engineer"]`, `addressPackProse["engineer"]`,
  `antiCaricatureProse["engineer"]`.
- Add a `fast-witty` arm to `presentationRegister` (same shape as `warm-direct`).
- NEW RED tests first: Tony rendered bullets + fast-witty register + guardrail assertion.

### Out of Scope
- Other personas (Argentino, Yoda), schema/yaml/`preset_v2.go` changes.
- Layer-1 mentor philosophy, reply-language rule, dialect logic (Tony is portable, en-us).

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None (renderer prose authoring only; no spec-level requirement or schema change).

## Approach

All fills live in `loader.go`, routed through the existing `renderPresentation`, so
Claude (output-style) and OpenCode (Layer 2) get identical Tony voice. Tony is
PORTABLE (`isBoundDialect`==false) → portability affirmation only, NO dialect gating.

**Product rules (authoritative):**
1. Confidence-vs-verification: sharp/witty and technically confident BUT humble on
   facts — never false certainty; if unverified, say so plainly. Confidence is STYLE,
   never a substitute for verification (respects Layer-1 supremacy).
2. Wit target: wit and ribbing aim at the PROBLEM / code / situation, NEVER at the
   user. Never demeaning or teasing.
3. Character nods: soft, recontextualized to the real technical situation; NEVER
   verbatim movie quotes, out-of-context, or parody.
4. Voice-only: prose does NOT restate Layer-1 philosophy (no "CONCEPTS > CODE" etc.).
5. `antiCaricatureProse["engineer"]` is load-bearing: wit/confidence never become
   arrogance, false certainty, skipped verification, or condescension.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/internal/persona/loader.go` | Modified | 5 prose fills + `fast-witty` register arm |
| `jarvis-cli/internal/persona/v2_test.go` | Modified | NEW RED tests (Tony bullets, register, guardrail) |
| `preset_v2.go`, `personas/*.yaml` | Unchanged | Schema/enum frozen |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Confident voice erodes supremacy rules | Med | Load-bearing anti-caricature guardrail; confidence-vs-verification rule |
| Wit lands on the user, not the problem | Med | Explicit wit-at-the-problem prose + guardrail |
| `presentationRegister` merge conflict with Yoda `calm-teacher` arm | Med | Trivial union of independent arms at integration |
| Character nods drift to parody/verbatim | Low | Soft/recontextualized-only rule in prose |

## Rollback Plan

Revert `loader.go` and the test edits. No schema, yaml, or generated `~/.claude/*`
files change — clean git revert, no migration.

## Dependencies

- Foundations mechanism (prose maps + `renderPresentation`), on this branch.
- Exploration artifact `sdd/persona-voice-tony-stark/explore` (id 4492).

## Success Criteria

- [ ] Tony's 5 packs render authored prose (not raw IDs); `fast-witty` renders prose.
- [ ] Anti-caricature prose explicitly forbids arrogance/false-certainty/skipped-verification/condescension.
- [ ] Portability clause preserved; no dialect gating; no Layer-1 leak into Layer 2.
- [ ] Claude + OpenCode parity holds; `go test ./...` and `go vet ./...` green.
