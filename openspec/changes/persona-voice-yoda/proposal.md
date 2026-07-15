# Proposal: Persona Voice — Yoda (portable exemplar)

## Intent
Give the Yoda persona a real, readable VOICE on the foundations mechanism (Change 1).
Today the 5 prose maps ship empty, so Yoda's rendered `### Presentation` bullets fall
back to raw enum IDs (`yoda`, `dry`, `calm-teacher`) — unreadable and character-less.
Yoda is the PORTABLE exemplar (es-neutral, `isBoundDialect=false`): its voice must apply
in ANY language the user writes, with no dialect gating. This change authors that voice.

## Scope

### In Scope
- Author Yoda-DEDICATED prose: `vocabularyProse["yoda"]`, `phrasePackProse["yoda"]`,
  `addressPackProse["yoda"]`, `antiCaricatureProse["yoda"]`.
- Author SHARED generic prose: `humorProse["dry"]` (also inherited by sargento/asturiano).
- Extend `presentationRegister` with a `calm-teacher` arm returning generic readable prose
  (also inherited by galleguinho).
- New RED tests (TDD) for Yoda's rendered bullets + the calm-teacher register, then GREEN.

### Out of Scope
- Other personas' signature voices; schema / yaml changes (`preset_v2.go`, `yoda.yaml` frozen).
- Layer-1 mentor philosophy (forbidden strings: `CONCEPTS > CODE`, `AI IS A TOOL`, `Technical Behavior`).
- Dialect-gating logic; raw-rendered bullets (cadence/emotional_range/verbosity/formatting/
  teaching_metaphors/examples stay raw).

## Capabilities
### New Capabilities
None.
### Modified Capabilities
None (voice-only prose fill; no spec-level requirement change).

## Approach
Fill prose maps as multi-line literals (one key per line) so concurrent Argentino edits merge
cleanly. Encode the product rules below into the DEDICATED yoda prose; keep SHARED `dry` and
`calm-teacher` prose generic so inheriting personas read well. Claude+OpenCode parity via the
shared `renderPresentation`.

### Product rules (encode in yoda prose)
- INVERSION: strong/frequent signature — BUT clarity and teaching are a HARD CAP. Invert freely
  on short/medium statements; straighten the sentence whenever inversion would bury the technical
  point. The lesson must always land.
- "Hmm." sporadic thinking beat — never a tic.
- Movie nods: soft, recontextualized echoes only; NEVER verbatim, out-of-context, or parody.
- Metaphors of roots and patience; portability affirmation (character in any language).
- Anti-caricature (yoda): clarity beats mysticism; calm never becomes vagueness or false certainty.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/internal/persona/loader.go` | Modified | 5 prose entries + `calm-teacher` register arm |
| `jarvis-cli/internal/persona/*_test.go` | Modified | New RED→GREEN tests for yoda bullets + register |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Cross-persona bleed (`dry`, `calm-teacher` shared) | Med | Write generically; no persona flavor |
| Layer-1 leak trips forbidden-string tests | Low | Voice-only prose; no philosophy restated |
| Textual merge conflict with Argentino branch | Med | Disjoint keys; one-key-per-line literals |
| Inversion obscures the lesson | Med | Clarity HARD CAP encoded in prose + anti-caricature |

## Rollback Plan
Revert the loader.go edits (empty maps + single register arm) and drop the new tests. Values
fall back to raw enum IDs; no schema/yaml touched, so no migration.

## Dependencies
- `persona-voice-foundations` (Change 1) mechanism present on this branch.

## Success Criteria
- [ ] Yoda renders readable prose for Vocabulary/Humor/Address/Phrase/Anti-caricature + Register.
- [ ] Yoda stays portable (portability clause only, NO dialect-gating clause).
- [ ] `dry` and `calm-teacher` prose read generically for inheriting personas.
- [ ] No forbidden Layer-1 strings in output; `go test ./...` and `go vet ./...` green.
