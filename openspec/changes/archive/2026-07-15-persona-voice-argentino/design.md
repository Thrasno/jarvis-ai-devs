# Design: Persona Voice — Argentino

## Technical Approach

Fill the 5 empty Layer-2 prose maps in `internal/persona/loader.go:168-174` with
final English descriptive prose. `rioplatense` (vocabulary) carries ALL Argentine
color (signature); `warm`/`peer`/`plain`/`grounded` stay generic so future personas
inherit them. No schema/yaml/mechanism change — `renderPresentation`, `proseFor`,
`isBoundDialect`, and the Language Behavior block are untouched. Both Claude
(`RenderOutputStyle`) and OpenCode (`RenderLayer2`) go through the same
`renderPresentation`, so parity is automatic. Each map value renders as one
bullet: `- Vocabulary: <prose>` etc. Prose is single-line, VOICE-ONLY (no Layer-1
philosophy restatement, no `es-*` enum literals).

## The 5 Prose Literals (final)

### `vocabularyProse["rioplatense"]` — SIGNATURE
```go
"Speak Rioplatense with full voseo always — vos, tenés, podés, mirá, fijate, dale — never tú/tuteo. Season the talk with warm Argentine lexicon (boludo as affectionate address between colleagues, never an insult to the user; posta for real emphasis; un toque for a little; bárbaro/joya for great) and let emphatic turns land on the problem, not the person — lo hacemos mierda, hacela pelota, a la miércoles — as occasional seasoning for warmth and drive, not on every line. Use expressive patterns: rhetorical hooks (e.g., ¿y sabés por qué?), repetition to drive a point home (e.g., se terminó, eso ya está), and close with impact. Reserve CAPS for the rare moment emphasis truly needs it. Treat these phrases as illustrations of the flavor, not a script to repeat."
```
Rationale: Full voseo + Rioplatense seasoning (Option A warm-mentor, not every line), emphatic expressions aimed at the problem, marked example phrases, no `es-*` literals.

### `humorProse["warm"]` — GENERIC
```go
"Warmth and humor that come from genuinely caring about the person and the work — passionate, energetic, encouraging. Never sarcastic, never mocking, never at the user's expense; the energy lifts the collaboration rather than scoring points."
```
Rationale: Generic caring energy; no Argentine content so any persona can reuse.

### `addressPackProse["peer"]` — GENERIC
```go
"Address the user as a capable colleague working alongside you — an equal peer. Never deferential or subservient, never bossy or condescending; assume competence and share ownership of the problem."
```
Rationale: Peer stance, neither deferential nor bossy; no regional flavor.

### `phrasePackProse["plain"]` — GENERIC
```go
"Plain, clear, direct phrasing — say things simply and get to the point. No ornament, no filler, no regional flavor or stylized turns of phrase; unadorned language that communicates without decoration."
```
Rationale: Genuinely plain, unadorned; explicitly no regional flavor (color belongs to the signature vocabulary pack only).

### `antiCaricatureProse["grounded"]` — GENERIC
```go
"Express character and regional color authentically, as a real person would — never perform it as a stereotype or cartoon, and never pile on clichés for show. Color serves clarity and warmth, not spectacle; a lively tone never substitutes for verifying facts and doing the work right."
```
Rationale: Authentic color over spectacle; tone never replaces verification. No forbidden Layer-1 strings.

## Rendering & Invariants

Each bullet renders exactly as `- <Label>: <prose>` via existing `fmt.Fprintf`
lines (loader.go:105/111/112/113/103). Dialect-gating unaffected: `isBoundDialect`
still fires for argentino (es-rioplatense + rioplatense pack), and the Language
Behavior/Portability clauses render unchanged. Because the rioplatense prose
contains no `es-rioplatense`/`es-asturian`/`es-galician` literal,
`TestBoundDialectClauseUsesReadableLanguageName` stays green. Out-of-Spanish
behavior (drop voseo, keep register + mentor soul) is already handled by the
dialect-gating clause — confirmed, no change.

## Test Impact (RED-first, stable substrings)

Update to the leading, distinctive portion of each bullet (robust to tail edits):

| File:Line | Old substring | New substring |
|-----------|---------------|---------------|
| bridge_test.go:239 | `- Humor: warm` | `- Humor: Warmth and humor that come from genuinely caring` |
| bridge_test.go:245 | `- Address pack: peer` | `- Address pack: Address the user as a capable colleague` |
| bridge_test.go:246 | `- Phrase pack: plain` | `- Phrase pack: Plain, clear, direct phrasing` |
| bridge_test.go:247 | `- Anti-caricature: grounded` | `- Anti-caricature: Express character and regional color authentically` |
| claude_test.go:156 | `- Address pack: peer` | `- Address pack: Address the user as a capable colleague` |
| opencode_test.go:87 | `- Address pack: peer` | `- Address pack: Address the user as a capable colleague` |

Note: bridge_test.go:237 `- Vocabulary: plain-technical` stays UNCHANGED — the
`validPresetV2` fixture uses `plain-technical`, which has no prose entry and keeps
its raw-ID fallback. Only `rioplatense` gets vocabulary prose, and no assertion
checks a rioplatense vocabulary bullet, so the signature fill breaks no test.
Guidance: assert the label prefix + first clause only, not the whole block, so
future prose polish does not churn tests.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file
classification, or process-integration boundary. Pure in-memory string data.

## Migration / Rollout

No migration. Rollback = revert the 5 map literals to empty and restore the 6
assertions; renderer falls back to raw IDs.

## Open Questions

None. Product confirmation to surface: filling shared packs establishes generic
prose that future/custom personas inherit (intended; affects no current builtin
besides argentino — low risk).
