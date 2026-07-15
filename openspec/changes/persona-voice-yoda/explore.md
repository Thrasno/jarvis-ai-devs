# Exploration — persona-voice-yoda

Change 2, persona 2 of 2. Author Yoda's VOICE by filling pack prose on the
foundations mechanism. NO schema/yaml change (one optional small register arm).

## Current state
Foundations mechanism present; 5 prose maps EMPTY on this branch (Argentino's fills
live on its own branch). `renderPresentation` routes Vocabulary/Humor/Address/Phrase/
Anti-caricature through `proseFor`; raw-ID fallback otherwise.

## Portability — CONFIRMED
`isBoundDialect(yoda) == false` (es-neutral not in regionalLanguages → short-circuits).
Yoda renders ONLY the Portability affirmation clause, NO dialect-gating clause. It is
the PORTABLE exemplar — character applies in any language. Asserted green at v2_test.go:364.

## Pack IDs — dedicated vs shared
- DEDICATED to Yoda (author Yoda-specific): vocabulary `yoda`, phrase_pack `yoda`,
  address_pack `yoda`, anti_caricature `yoda`.
- SHARED/generic: humor `dry` — ALSO used by sargento and asturiano. Author generically
  (not Yoda-flavored); filling it changes their `- Humor:` bullet too (safe, no test asserts it).
- Render raw / out of scope: cadence `reflective`, emotional_range `calm`, verbosity
  `concise`, formatting `compact`, teaching_metaphors `roots`, examples `concise`.

## calm-teacher register — recommendation: EXTEND
`presentationRegister` only expands `warm-direct`; `calm-teacher` renders raw. Recommend
adding a `calm-teacher` switch arm (same pattern), written GENERICALLY (galleguinho also
uses calm-teacher). Small shared-mechanism touch, no schema change. No test asserts the
rendered register for calm-teacher. Leaving raw is acceptable but inconsistent.

## Test impact
No existing exact-match assertion breaks (all target the validPresetV2 fixture or the
rioplatense variant, never Yoda's packs). Author NEW RED tests for Yoda's rendered bullets
(+ calm-teacher register if extended) first. TestPresentationValuesResolveNonEmptyWithRawIDFallback
already passes with any non-empty prose.

## Merge-conflict surface with Argentino branch
Disjoint keys (yoda/dry vs rioplatense/warm/peer/plain/grounded) → semantic union, no
logical conflict; only a textual conflict on the map-literal lines. Fill each map as a
multi-line literal (one key per line) for clean merges. NOTE: `dry` and `calm-teacher` are
shared — Yoda authors them here; future personas (sargento/asturiano/galleguinho) must
INHERIT, not re-author them.

## Voice intent (for design)
Clause inversion for emphasis; reflective/measured cadence; occasional "Hmm." (sparingly,
never a tic); metaphors of roots and patience; NO verbatim movie quotes. Portability
affirmation. Anti-caricature (yoda): clarity beats mysticism — drop inversion if it hurts
comprehension; calm never becomes vagueness or false certainty. Mentor philosophy stays in
Layer 1 (do NOT restate).

## Confirmations
No schema/yaml change; only loader.go prose maps (yoda ×4 + dry) + optional calm-teacher
register arm.

Engram artifact: `sdd/persona-voice-yoda/explore`.
