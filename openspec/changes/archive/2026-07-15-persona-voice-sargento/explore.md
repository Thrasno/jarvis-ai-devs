# Exploration — persona-voice-sargento

Change 2, persona 4 (portable). Author Sargento's VOICE on the foundations mechanism.
NO schema/yaml change.

## Current state (foundations base on this branch)
5 prose maps EMPTY; proseFor raw-ID fallback. presentationRegister has ONLY the
warm-direct arm → `mission-briefing` renders raw. isBoundDialect(sargento)==false
(es-neutral) → PORTABLE → Portability affirmation only, no dialect gating.

## Pack ownership (grep-verified)
- `military`, `sergeant`, `mission-briefing` → sargento-only → DEDICATED, author here.
- `dry` (humor) → SHARED across yoda / asturiano / sargento. The YODA change (PR #424)
  owns `humorProse["dry"]`. Do NOT author it here — same key, different text would be a
  real merge CONFLICT. The Humor bullet renders raw "dry" on this isolated branch until
  the stack integrates (expected/acceptable). The Sargento test must NOT assert the humor bullet.
- `mission` (formatting + teaching_metaphors) → not prose-mapped, renders raw → OUT OF SCOPE.

## Fills required (DEDICATED, this change)
- vocabularyProse["military"], phrasePackProse["sergeant"], addressPackProse["sergeant"],
  antiCaricatureProse["sergeant"]; add a `mission-briefing` arm to presentationRegister.

## Test impact
No existing exact-match assertion breaks. NEW RED tests for Sargento's dedicated rendered
bullets + mission-briefing register. Do NOT assert the Humor bullet (dry inherited from Yoda).

## Merge-conflict surface
- presentationRegister: 3-way overlap (Yoda calm-teacher / Tony fast-witty / Sargento
  mission-briefing) — disjoint switch cases, same function → trivial union, FLAG at integration.
- Prose-map keys disjoint (military/sergeant vs others) → trivial union. `dry` untouched → no conflict.

## Voice intent (for design)
Sargento: crisp, disciplined, mission-briefing cadence; brisk, imperative, structured;
orders framed as clear next steps. Anti-caricature (sergeant): firm and disciplined WITHOUT
hostility, insults, or belittling — never demeaning the user; discipline serves clarity and
momentum, not intimidation; brevity/confidence never replace verification. Portability
affirmation. Mentor philosophy stays Layer 1 (not restated). No military glorification or
abusive drill-instructor content.

Engram artifact: `sdd/persona-voice-sargento/explore` (id 4501).
