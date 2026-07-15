# Proposal: Persona Voice — Argentino

## Intent

The foundations mechanism (Change 1) ships with five EMPTY Layer-2 prose maps in `loader.go`; every persona voice renders as bare enum IDs (e.g. `- Humor: warm`). Argentino must sound like gentle-ai's warm Rioplatense mentor. This change authors that VOICE by filling prose-map entries only — no schema/yaml/mechanism change. The mentor philosophy (Layer 1) already exists and stays invariant.

## Scope

### In Scope
- Fill exactly 5 prose-map entries in `internal/persona/loader.go` (lines 168-174).
- Update 6 exact-match test assertions RED-first (bridge_test.go:239/245/246/247, claude_test.go:156, opencode_test.go:87).

### Out of Scope
- Yoda and other personas; any schema/yaml/mechanism change (preset_v2.go/yaml frozen).
- Restating or touching Layer-1 mentor philosophy.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None (spec-level behavior unchanged; renderer prose only).

## Approach — Pack Authoring Plan (signature vs generic)

- `rioplatense` (vocabulary) = **THE SIGNATURE**. All Argentine color here: full voseo always (vos/tenés/podés/mirá/fijate/dale); rich Rioplatense lexicon (boludo as affectionate colleague address, "posta", "un toque", "bárbaro/joya", emphatic "lo hacemos mierda"/"hacela pelota") as warm-mentor SEASONING (Option A — not every line), directed at the problem, never insulting the user. Expressive patterns here too (rhetorical questions, repetition, close-with-impact) with a FEW marked example phrases. CAPS only when really needed. MUST NOT contain literal enum strings es-rioplatense/es-asturian/es-galician.
- `warm` (humor) = GENERIC: warm/passionate energy from caring; never sarcastic. No Argentine content.
- `peer` (address) = GENERIC: capable colleague; never deferential or bossy.
- `plain` (phrase) = GENERIC and genuinely plain: clear, direct, unadorned; no regional flavor.
- `grounded` (anti-caricature) = GENERIC: authentic color, never stereotype/spectacle; tone never substitutes for verification.

Shared packs stay generic because future/custom personas inherit them. Argentino's distinctiveness = combination + signature: rioplatense + es-rioplatense dialect gating + warm-direct register + energetic cadence + Layer-1 mentor soul. Out-of-Spanish behavior (drop voseo, keep tone/soul) is already rendered by the existing dialect-gating Language Behavior clause — confirm only.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/persona/loader.go:168-174` | Modified | 5 prose-map literals populated |
| `internal/persona/bridge_test.go` | Modified | 4 assertions updated |
| `internal/agent/{claude,opencode}_test.go` | Modified | 2 assertions updated |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Generic packs inherited by future personas | Low | Intended; affects no current builtin but argentino |
| Raw enum leak in rioplatense prose | Low | Avoid es-* strings; TestBoundDialectClause guards |
| Layer-1 restatement trips forbidden-string tests | Low | Keep prose voice-only; no CONCEPTS>CODE etc. |

## Rollback Plan

Revert loader.go prose entries to empty maps and restore the 6 assertions; renderer falls back to raw IDs. Single-file logic revert.

## Dependencies

- `feat/persona-voice-foundations` mechanism (present on branch).

## Success Criteria

- [ ] All 5 packs render prose, not raw IDs.
- [ ] rioplatense carries voseo + Rioplatense seasoning; shared packs stay generic.
- [ ] 6 assertions updated; full suite green; forbidden-string/dialect-gating invariants hold.
