# Exploration — persona-voice-asturiano

Change 2, persona 5 (REGIONAL/BOUND, like Argentino). Author Asturiano's VOICE on the
foundations mechanism. NO schema/yaml change.

## Current state (foundations base)
5 prose maps EMPTY; proseFor raw-ID fallback. presentationRegister("warm-direct") already
returns "warm, energetic, and direct". presentationLanguage("es-asturian") already returns
"Asturian". isBoundDialect fires for regional language + regional pack.

## Classification (confirmed)
es-asturian ∈ regionalLanguages AND asturian ∈ regionalPacks ⇒ isBoundDialect == true → BOUND
(same shape as Argentino). Renders the dialect-gating clause with native name "Asturian"
(TestBoundDialectClauseUsesReadableLanguageName). register warm-direct ALREADY expanded → NO
register change, NO 3-way presentationRegister overlap. humor `dry` SHARED (Yoda owns, PR #424)
→ NOT authored here; renders raw until integration; not tested.

## Fills needed (DEDICATED asturian keys)
vocabularyProse["asturian"], phrasePackProse["asturian"], addressPackProse["asturian"],
antiCaricatureProse["asturian"]. Grep confirms `asturian` pack IDs are asturiano-only.

## KEY MODELING QUESTION (for the product round)
Asturian (asturianu/bable) is arguably its own language, but the intended persona parallels
Argentino: warm regional flavor woven into SPANISH. The gating clause says "the Asturian
dialect layer ... applies only when replying in Asturian." Options:
  (a) Asturian-flavored SPANISH — flavor fires when the user writes Spanish (parallels
      Argentino's voseo, where "Rioplatense Spanish (voseo)" is still Spanish). RECOMMENDED.
  (b) the distinct Asturian language — flavor almost never appears.
If (a), the vocabulary prose should make explicit it is Asturian-flavored Spanish; optionally
relabel presentationLanguage("es-asturian")→"Asturian Spanish" (touches a shared helper +
the existing "Asturian" test — out of minimal scope, decide in product round).

## Voice intent (for design)
Warm, grounded Asturian mentor: measured, workshop metaphors, warm regional expressions/lexicon;
anti-caricature (asturian) grounded-style — authentic Asturian warmth, no regional parody/cliché,
flavor is seasoning; tone never replaces verification. Bound: full flavor in native register,
neutral fallback outside. Mentor philosophy stays Layer 1 (not restated).

## Test impact
No existing exact-match assertion breaks. NEW RED tests: 4 dedicated-bullet prose assertions +
dialect-gating "Asturian" presence. Do NOT assert humor.

## Merge-conflict surface
Prose-map keys disjoint (asturian vs others). NO presentationRegister edit (warm-direct reused)
→ no 3-way overlap. humor `dry` not authored → no key collision. Smaller surface than sargento/tony/yoda.

Engram artifact: `sdd/persona-voice-asturiano/explore` (id 4510).
