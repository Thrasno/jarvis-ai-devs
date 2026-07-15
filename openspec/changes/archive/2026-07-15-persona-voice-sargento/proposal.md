# Proposal: Sargento Persona Voice (Change 2, persona 4, portable)

## Intent

Sargento's persona currently renders raw enum IDs (`military`, `sergeant`, `mission-briefing`) because the voice prose maps ship empty on this foundations branch. This change authors Sargento's VOICE — a dry, terse, gruff, near-monosyllabic delivery inspired by "El Sargento de Hierro" (Heartbreak Ridge) that rides up to the edge of disrespect but never crosses it. Success: Sargento's rendered bullets read as authored mission-briefing voice, guarded against caricature, with no schema change and Claude+OpenCode parity.

## Scope

### In Scope
- 4 DEDICATED prose entries (sargento-only) in `internal/persona/loader.go`:
  `vocabularyProse["military"]`, `phrasePackProse["sergeant"]`, `addressPackProse["sergeant"]`, `antiCaricatureProse["sergeant"]` — verbatim locked literals.
- 1 `mission-briefing` arm in `presentationRegister` → "clipped, terse, and mission-focused".
- RED-first tests for the 4 dedicated bullets + mission-briefing register (Claude + OpenCode via shared `renderPresentation`).
- Portability affirmation only (Sargento is es-neutral, `isBoundDialect=false`) — already wired/tested.

### Out of Scope
- `humorProse["dry"]` — SHARED (Yoda owns, PR #424; asturiano too). Renders raw "dry" on this isolated branch until integration. Do NOT author, do NOT test.
- `formatting`/`teaching_metaphors` "mission" — not prose-mapped, render raw.
- Schema/yaml changes (frozen); other personas; any Layer-1 content.

## Capabilities

### New Capabilities
None (no spec-level capability; pure voice-prose rendering).

### Modified Capabilities
None (additive prose fills; no requirement change).

## Approach

Follow the foundations `proseFor(table, id)` + `renderPresentation` mechanism. Add 4 dedicated prose map entries and one `presentationRegister` switch arm (same pattern as `warm-direct`). Prose is VOICE-ONLY — no Layer-1 restatement ("CONCEPTS > CODE", "AI IS A TOOL", technical-behavior text). Strict TDD: RED tests first, minimal GREEN fills, refactor green.

## Product Rules

- **Disrespect-boundary guardrail (load-bearing)**: `antiCaricatureProse["sergeant"]` keeps the gruff/terse edge as delivery style only — no insults, humiliation, shouting down, or real disrespect. Discipline serves clarity and momentum, never intimidation; brevity/bark never replace verifying facts and doing the work right.
- All authored packs are DEDICATED (sargento-only); keys disjoint from other personas.
- Claude + OpenCode parity through the shared `renderPresentation`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/persona/loader.go` | Modified | 4 dedicated prose entries + `mission-briefing` register arm |
| `internal/persona/v2_test.go` / `bridge_test.go` | Modified | RED tests for dedicated bullets + register (NOT humor) |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| 3-way `presentationRegister` overlap (Yoda calm-teacher, Tony fast-witty, Sargento mission-briefing) | Med | Disjoint switch cases → trivial union at integration; FLAG for integrator |
| Humor bullet renders raw "dry" on isolated branch | High (expected) | Do NOT author/test humor; inherited from Yoda change at integration |
| Guardrail drift into caricature/abuse | Low | Locked anti-caricature literal is the guardrail; assert it renders |

## Rollback Plan

Revert the `loader.go` prose/register additions and their tests. Fallback restores raw enum IDs (`proseFor` fallback intact); no schema/routing change to undo.

## Dependencies

- Foundations (Change 1): `renderPresentation`/`proseFor` mechanism and empty prose maps.
- Integration only: Yoda's `humorProse["dry"]` (PR #424) supplies the humor bullet downstream.

## Success Criteria

- [ ] 4 dedicated bullets render the locked authored voice literals.
- [ ] `mission-briefing` register renders "clipped, terse, and mission-focused".
- [ ] Anti-caricature guardrail bullet renders (borders-on-but-never-crosses).
- [ ] No Layer-1 strings leak into prose; portability affirmation only.
- [ ] `go test ./...` and `go vet ./...` pass; humor bullet not asserted.
