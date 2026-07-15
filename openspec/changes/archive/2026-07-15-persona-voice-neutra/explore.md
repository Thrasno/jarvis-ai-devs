# Exploration — persona-voice-neutra

Change 2, persona 7/7 (NEUTRAL baseline, portable). Author Neutra's generic voice on the
foundations mechanism. NO schema/yaml change.

## Current state (foundations base)
5 prose maps EMPTY; proseFor raw-ID fallback. presentationRegister only warm-direct.
Neutra is es-neutral + generic packs → PORTABLE (portability affirmation only, no dialect gating).

## Shared-token analysis
- register `friendly-professional` is SHARED with the validPresetV2 test fixture; two assertions
  pin the RAW token (bridge_test.go:140 wantRegister, bridge_test.go:236). Adding an arm would
  break both. RECOMMEND: leave friendly-professional RAW (Approach A) — Neutra touches only the
  5 prose maps, zero test churn.
- humor `none` — no assertion pins rendered "none"; neutra-only → SAFE to author humorProse["none"].
- `neutral` / `neutral-spanish` packs — neutra-only (fixture uses plain-technical/peer/plain/grounded).
  SAFE to fill.

## Fill set (all neutra-only → the generic default/fallback set, zero test churn)
vocabularyProse["neutral-spanish"], humorProse["none"], phrasePackProse["neutral"],
addressPackProse["neutral"], antiCaricatureProse["neutral"]. Prose MUST be genuinely
plain/professional — no flavor, no character (this is the fallback a default/custom persona inherits).

## Register decision
Approach A (RECOMMENDED): leave friendly-professional RAW. Neutra touches only the 5 prose maps.
(Approach B — expand it — breaks the two shared-fixture assertions; not worth it for the baseline.)

## Voice intent (for design)
Clear, calm, friendly-professional, neutral — no regional flavor, no strong character; measured,
composed, warm but restrained. Anti-caricature (neutral): stay genuinely neutral/professional,
never regional or theatrical; clarity first; tone never replaces verification. Mentor stays Layer 1.

## Test impact
No existing assertion breaks (Approach A). NEW RED test for the 5 dedicated bullets + portability
(no dialect-gating). humor "none" authored (safe).

## Merge-conflict surface
Prose-map keys disjoint (neutral-spanish/none/neutral vs others) → trivial union. NO
presentationRegister edit (friendly-professional left raw) → zero register conflict.

Engram artifact: `sdd/persona-voice-neutra/explore` (id 4528).
