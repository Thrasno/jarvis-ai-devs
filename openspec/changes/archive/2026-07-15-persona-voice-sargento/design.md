# Design: Sargento Persona Voice (Change 2, persona 4, portable)

## Technical Approach

Fill the empty foundations prose maps and the `presentationRegister` switch in
`internal/persona/loader.go` with DEDICATED, sargento-only literals so Sargento
renders authored mission-briefing voice instead of raw enum IDs. VOICE-ONLY: no
Layer-1 restatement. No schema/YAML change. Claude and OpenCode reach the same
output through the single shared `renderPresentation` (called by both
`RenderOutputStyle` and `RenderLayer2`), so parity is structural — no per-surface
work. Strict TDD: RED test first, minimal GREEN fills, refactor green.

## Architecture Decisions

### Decision: Fill prose maps + register arm; reuse the foundations mechanism

**Choice**: Add 4 map entries (`vocabularyProse["military"]`,
`phrasePackProse["sergeant"]`, `addressPackProse["sergeant"]`,
`antiCaricatureProse["sergeant"]`) and one `mission-briefing` register arm —
mirroring the existing `warm-direct` pattern.
**Alternatives considered**: new per-persona render path; schema field for prose.
**Rationale**: `proseFor(table, id)` + `renderPresentation` already exist and
fall back to raw IDs. Filling maps is the minimal additive change; disjoint keys
keep other personas untouched.

### Decision: presentationRegister `if` → `switch`

**Choice**: Convert the single-arm `if register == "warm-direct"` to a `switch`,
adding the `mission-briefing` case. `warm-direct` output stays byte-identical.
**Alternatives considered**: chained `if`/`else if`.
**Rationale**: 3 personas (Yoda calm-teacher, Tony fast-witty, Sargento
mission-briefing) each add one disjoint arm. A `switch` makes the union trivial
and readable. Yoda/Tony arms are added by their own changes; at integration the
three arms merge with no logic conflict — FLAG for the integrator.

### Decision: Sargento stays PORTABLE (no dialect gating)

**Choice**: No change to `isBoundDialect`. Sargento is `es-neutral`, uses generic
packs (`military`/`sergeant`), so `isBoundDialect` returns false → Portability
affirmation renders, no dialect-gating line.
**Rationale**: The voice is language-independent discipline, not a regional
dialect. Already wired and tested by foundations.

### Decision: Humor `dry` is OUT OF SCOPE

**Choice**: Do NOT author or assert `humorProse["dry"]`. The Humor bullet renders
raw `dry` on this isolated branch.
**Rationale**: `dry` is SHARED (Yoda owns it, PR #424; asturiano also). Same key
with different text would be a real merge conflict. Inherited at integration.

## Locked Literals (VERBATIM — final Go string values)

| Map key / register token | Exact literal | Rationale |
|---|---|---|
| `presentationRegister` `mission-briefing` | `clipped, terse, and mission-focused` | Register cadence: short, functional, objective-driven. |
| `vocabularyProse["military"]` | `Operational, military vocabulary — frame the work as a mission with objectives, targets, and next moves; terse and functional, no filler, no soft edges. Name the task, name the step, move on.` | Mission framing without glorification. |
| `phrasePackProse["sergeant"]` | `Extremely terse, near-monosyllabic delivery — short, clipped sentences and blunt imperatives. Orders framed as clear next steps: 'Guard the index. Run the tests. Move.' No pleasantries, no hedging, no wind-up. Say it once, say it straight.` | Near-monosyllabic, imperative cadence. |
| `addressPackProse["sergeant"]` | `Address the user curtly and directly, as a capable operator who gets clear orders — brusque, no coddling, no small talk. It rides right up to the edge of disrespect but never crosses it: no insults, no humiliation, never actually demeaning.` | Curt address, bounded at the edge of disrespect. |
| `antiCaricatureProse["sergeant"]` | `The gruff, terse edge is delivery style only: it may border on brusque, but it never crosses into insults, humiliation, shouting the user down, or real disrespect. The discipline serves clarity and momentum, never intimidation; the bark and the brevity never replace verifying facts and doing the work right.` | Load-bearing guardrail: gruffness is style, never abuse. |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `jarvis-cli/internal/persona/loader.go` | Modify | Fill 4 prose map entries; add `mission-briefing` arm (if→switch) |
| `jarvis-cli/internal/persona/v2_test.go` or `bridge_test.go` | Modify | NEW RED test `TestSargentoPresentationRendersAuthoredVoice` |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Both `RenderLayer2` and `RenderOutputStyle` of the built-in sargento preset contain the 5 label-prefix substrings | New RED test; stable prefixes below |
| Unit | Absence of forbidden Layer-1 strings ("CONCEPTS > CODE", "AI IS A TOOL", technical-behavior copy) | Negative assertions |

Asserted label-prefix substrings (stable, not full literal):
- `- Register: clipped, terse, and mission-focused`
- `- Vocabulary: Operational, military vocabulary`
- `- Address pack: Address the user curtly and directly`
- `- Phrase pack: Extremely terse, near-monosyllabic delivery`
- `- Anti-caricature: The gruff, terse edge is delivery style only`

Do NOT assert the Humor bullet (`dry` inherited from Yoda). No existing exact-match
assertion breaks (disjoint keys; `warm-direct` output byte-identical).

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file
classification, or process-integration boundary. Pure in-memory string rendering.

## Migration / Rollout

No migration required. Revert loader.go prose/register + tests to roll back;
`proseFor` fallback restores raw enum IDs. No schema/routing to undo.

## Open Questions

- [ ] None blocking. Integration note: presentationRegister 3-way arm union
  (Yoda/Tony/Sargento) is a trivial disjoint merge — FLAG for integrator.
