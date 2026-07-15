# Exploration — persona-voice-galleguinho

Change 2, persona 6/6 (REGIONAL/BOUND, last regional). Author Galleguinho's VOICE on the
foundations mechanism. NO schema/yaml change.

## Current state (foundations base)
5 prose maps EMPTY; proseFor raw-ID fallback. presentationRegister has only warm-direct →
calm-teacher renders raw here. presentationLanguage(es-galician)="Galician". isBoundDialect
fires for regional language + regional pack.

## Classification (grep-verified)
- BOUND: es-galician + galician packs → isBoundDialect==true → dialect-gating renders.
- register `calm-teacher`: used by yoda + galleguinho; OWNED by the Yoda change (PR #424 adds
  the arm). Do NOT author here (merge conflict); Register bullet renders raw until integration;
  do NOT assert it.
- humor `retranca`: galleguinho-ONLY → DEDICATED → author humorProse["retranca"].
- galician packs (vocabulary/phrase/address/anti-caricature): galleguinho-only → DEDICATED.
- `dry` shared (yoda/asturiano/sargento) — out of scope, not used by galleguinho.

## Modeling (Option A, consistent with Asturiano/Argentino — confirmed in product round)
Galician-flavored SPANISH. NO relabel: presentationLanguage("es-galician") stays "Galician";
the foundations test `TestBoundDialectClauseUsesReadableLanguageName` (galleguinho) is
unchanged. Activation is handled by the foundations dialect-gating clause, which reads "...the
Galician dialect layer ... applies only when replying in Spanish". No arm of
`presentationLanguage` changes for this branch (es-asturian is on the asturiano branch,
es-rioplatense on argentino).

## Fills required (DEDICATED)
vocabularyProse["galician"], phrasePackProse["galician"], addressPackProse["galician"],
antiCaricatureProse["galician"], humorProse["retranca"]. NOT the register arm (calm-teacher = Yoda).

## Voice intent (for design)
Galician warmth + signature RETRANCA (dry indirect irony/understatement — answering a question
with a question, gentle ambiguity, "haberlas haylas" spirit) + morriña warmth; light galego
touches (understandable always). teaching_metaphors "journey" renders raw, but Camino de
Santiago / the sea and rías fit as woven flavor. Anti-caricature (galician) grounded: authentic
retranca/warmth as seasoning, no meigas-postcard cliché/parody; flavor never blocks clarity or
verification. Bound: full flavor in native register, neutral fallback outside. Mentor stays Layer 1.

## Test impact
NO CHANGE to TestBoundDialectClauseUsesReadableLanguageName (galleguinho stays "Galician"). NEW
RED test for the 4 galician bullets + retranca humor + dialect-gating "the Galician dialect
layer ... applies only when replying in Spanish". Do NOT assert the Register bullet (calm-teacher
inherited from Yoda).

## Merge-conflict surface
Prose-map keys disjoint (galician/retranca vs others); shared var-literal block → mechanical
union only. NO presentationRegister edit (calm-teacher is Yoda's) → no register 3-way overlap
from this branch. NO presentationLanguage relabel — es-galician arm is untouched, so no shared
helper conflict beyond the prose-map literals.

Engram artifact: `sdd/persona-voice-galleguinho/explore` (id 4519).
