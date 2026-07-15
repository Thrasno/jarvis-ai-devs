# Exploration — persona-voice-tony-stark

Change 2, persona 3 (portable, the supremacy stress-test). Author Tony Stark's VOICE
by filling pack prose + a register arm on the foundations mechanism. NO schema/yaml change.

## Current state (foundations base on this branch)
5 prose maps EMPTY; proseFor raw-ID fallback. presentationRegister has ONLY the
warm-direct arm → `fast-witty` renders RAW. isBoundDialect(tony-stark)==false (en-us) →
PORTABLE → Portability affirmation only, NO dialect gating. preset_v2.go already accepts
fast-witty/engineering/witty/engineer → no schema change.

## Pack ownership — ALL DEDICATED to Tony
Grep across embed/personas/*.yaml: nobody else uses `fast-witty`, `engineering`, `witty`,
or `engineer` (custom.yaml.tmpl lists them only as commented enum options). So author
Tony-specific confident-engineer voice; NO generic/shared authoring needed.
`teaching_metaphors: engineering` is Tony-only too but renders raw (not prose-mapped).

## Fills required
- vocabularyProse["engineering"], humorProse["witty"], phrasePackProse["engineer"],
  addressPackProse["engineer"], antiCaricatureProse["engineer"].
- A `fast-witty` arm in presentationRegister (same shape as warm-direct).

## Test impact
No existing exact-match assertion changes (existing tests target validPresetV2 / rioplatense).
Portability test already asserts tony-stark:false. NEW RED tests: Tony's rendered bullets +
fast-witty register render authored prose; anti-caricature guardrail present; no Layer-1 leak.

## Merge-conflict surface
Prose-map keys disjoint across Tony/Argentino/Yoda → trivial union. ONE real risk:
presentationRegister — Yoda's branch adds a `calm-teacher` arm and Tony adds `fast-witty`
to the same function → overlapping-hunk conflict, resolved by trivial union at integration.

## KEY tension (the reason Tony was the deferred stress-test)
Tony's confident/witty voice is exactly what could erode the Layer-1 supremacy rules
(verify before asserting, flag assumptions). `antiCaricatureProse["engineer"]` is the
load-bearing guardrail: wit/confidence NEVER become arrogance, false certainty, or skipped
verification; never condescending or at the user's expense. Mentor philosophy stays Layer 1
(not restated). Character nods only soft/recontextualized, never verbatim/parody.

## Recommendation
Fill 5 dedicated prose entries + add the fast-witty register arm; RED tests first. No
schema/yaml change. Author the anti-caricature guardrail most carefully.

Engram artifact: `sdd/persona-voice-tony-stark/explore` (id 4492).
