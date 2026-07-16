# Exploration — persona-voice-argentino

Change 2, persona 1 of 2. Author Argentino's VOICE by filling pack prose on top
of the foundations mechanism. NO schema/yaml/mechanism change.

## Current state
The foundations mechanism is present on this branch. `internal/persona/loader.go`
`renderPresentation` (shared by RenderLayer2/RenderOutputStyle) routes 5 fields
through `proseFor` against 5 EMPTY prose maps (raw-ID fallback otherwise).
`presentationRegister` expands `warm-direct`; `presentationLanguage` + `isBoundDialect`
produce the `### Language Behavior` block. Argentino classifies as bound (es-rioplatense
+ rioplatense pack).

## Pack IDs Argentino needs (only these 5 hit prose maps)
- `rioplatense` (vocabulary) — DIALECT-SPECIFIC, dialect-gated (voseo / Rioplatense lexicon).
- `warm` (humor), `peer` (address), `plain` (phrase), `grounded` (anti-caricature) —
  SHARED / GENERIC; author with NO Argentine content (other personas reuse them).

NOT needing a map entry: register `warm-direct` (handled by `presentationRegister`),
language `es-rioplatense` (handled by `presentationLanguage` + dialect-gating clause),
and cadence/emotional_range/verbosity/formatting/teaching_metaphors/examples (raw fields).

## Key finding — shared packs affect no other builtin today
Among the 7 builtins, only argentino uses peer/plain/warm/grounded. Filling those maps
changes no other builtin's rendered output; current non-argentino consumers are only test
fixtures. Future/custom personas reusing these IDs inherit the generic prose — intended
(the pack-inheritance payoff). Flag for product confirmation (low risk).

## Test impact — exact-match assertions to update RED-first
Shared-pack fills break: `bridge_test.go:239` (`- Humor: warm`), `:245` (`- Address pack: peer`),
`:246` (`- Phrase pack: plain`), `:247` (`- Anti-caricature: grounded`); `claude_test.go:156`
and `opencode_test.go:87` (both `- Address pack: peer`). The dialect fill `rioplatense`
breaks NO assertion, but the prose must NOT contain the literal enums
`es-rioplatense`/`es-asturian`/`es-galician` (enforced by `TestBoundDialectClauseUsesReadableLanguageName`).
All other tests stay green.

## Merge-conflict surface (future Yoda branch)
Disjoint keys (yoda/dry vs rioplatense/warm/...). Only structural overlap is the 5
empty-map declarations both branches populate — trivial union merge, no semantic clash.

## Confirmations
No schema/yaml/mechanism change needed — only loader.go prose-map literals. Dialect-gating
+ Portability fire for Argentino. `warm-direct` handled outside prose maps.

## Recommendation
Approach 1 — fill all 5 maps (generic shared + gated dialect), update the 6 exact-match
assertions TDD-first.

Engram artifact: `sdd/persona-voice-argentino/explore` (id 4464).
