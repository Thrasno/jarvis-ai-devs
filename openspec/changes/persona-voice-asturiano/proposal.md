# Proposal: persona-voice-asturiano (Change 2, persona 5 — BOUND regional)

## Intent

Foundations (Change 1) shipped the render mechanism but the 5 prose maps ship EMPTY, so every Asturiano bullet renders its raw enum ID (e.g. "asturian") instead of a voice. Author Asturiano's VOICE — a warm Asturian-flavored-Spanish mentor (light bable, mining metaphors, sidra/retranca as seasoning) — by filling 4 dedicated `asturian` prose maps and relabeling presentationLanguage for es-asturian. Voice prose is USER-APPROVED and LOCKED.

## Scope

### In Scope
- Fill 4 DEDICATED `asturian` prose keys in `loader.go` map literals: `vocabularyProse`, `phrasePackProse`, `addressPackProse`, `antiCaricatureProse` (verbatim locked literals below).
- RELABEL `presentationLanguage("es-asturian")` from "Asturian" → "Asturian Spanish" (Option A: flavor fires when replying in Spanish, parallels Argentino voseo). Dialect-gating clause then reads "the Asturian Spanish dialect layer ... applies only when replying in Asturian Spanish".
- Update foundations test `TestBoundDialectClauseUsesReadableLanguageName` asturiano assertion "Asturian" → "Asturian Spanish" (TDD, RED first).
- NEW RED tests: 4 dedicated bullets render their prose; dialect-gating clause present with "Asturian Spanish".

### Out of Scope
- Other personas; schema (`preset_v2.go`); yaml (`asturiano.yaml`); generated `~/.claude/*`.
- humor `dry` (owned by Yoda, PR #424 — do NOT author or test).
- es-galician relabel (Galleguinho owns it); es-rioplatense arm.
- presentationRegister (warm-direct already expanded).
- Layer-1 restatement in prose (CONCEPTS > CODE, AI IS A TOOL, Technical Behavior).

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- None (prose-map fill + helper relabel; no spec-level requirement change).

## Approach

Option A (user-approved): Asturian-flavored SPANISH. Voice is seasoning woven into clear Spanish, never an obstacle to understanding. Prose is VOICE-ONLY. Claude+OpenCode parity via shared `renderPresentation`. Strict TDD (RED → GREEN → refactor).

### LOCKED literals (author verbatim)
- `presentationLanguage` es-asturian → `"Asturian Spanish"` (es-rioplatense, es-galician arms unchanged).
- `vocabularyProse["asturian"]` = "Asturian-flavored Spanish — weave warm Asturian lexicon and turns of phrase into clear Spanish (light bable touches like 'ho', 'guaje', 'prestar', 'ñeru'), always kept light enough that the message stays perfectly clear; the flavor is seasoning, never an obstacle to understanding."
- `phrasePackProse["asturian"]` = "Warm, measured phrasing with a wink of Asturian retranca — dry, understated regional wit and the easygoing cadence of someone who'd settle a debate over a few sidras. Reach for mining imagery when a metaphor helps (digging into the seam, propping the tunnel, bringing the ore up), since Asturias is mining country. Keep the levity light; the point always lands."
- `addressPackProse["asturian"]` = "Address the user as a warm, close peer — a paisanu you'd share a table and a sidra with; direct, honest, and welcoming, never deferential or distant."
- `antiCaricatureProse["asturian"]` = "The Asturian warmth and retranca are seasoning, not a costume — light bable and the odd sidra or mining aside are welcome, but never pile on regional clichés or perform a postcard Asturias; the flavor serves warmth and clarity, and a lively tone never replaces verifying facts and doing the work right."

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/internal/persona/loader.go` | Modified | 4 dedicated `asturian` prose keys + presentationLanguage relabel (shared helper) |
| `jarvis-cli/internal/persona/v2_test.go` | Modified | Update `TestBoundDialectClauseUsesReadableLanguageName`; add 4 bullet + gating assertions |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Shared helper edit touches one foundations test | Med | Intentional & in-scope; update assertion under TDD, run `go test ./...` |
| Go map-literal co-edit conflict across persona branches | Low | Keys disjoint (`asturian`); easy resolve |
| Over-performing regional clichés (caricature) | Low | Anti-caricature seasoning guardrail authored; flavor never blocks clarity/verification |

## Rollback Plan

Revert the `loader.go` prose-key additions and the presentationLanguage relabel, and revert the test assertion back to "Asturian". Keys are disjoint and the diff is small; clean revert.

## Dependencies

- Foundations (Change 1) render mechanism on branch. No external dependencies.

## Success Criteria

- [ ] 4 dedicated bullets render the locked prose verbatim (not raw IDs).
- [ ] Dialect-gating clause renders with "Asturian Spanish".
- [ ] Foundations test updated to "Asturian Spanish"; `go test ./...` and `go vet ./...` green.
- [ ] humor `dry` not authored/tested; no schema/yaml change; Claude+OpenCode parity preserved.
