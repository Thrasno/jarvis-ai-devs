# Tasks: Persona Voice — Argentino

Strict TDD. Sequential (single small file + its tests; no parallelism justified).

## 1. RED — update the 6 exact-match assertions to expect authored prose

Files: `jarvis-cli/internal/persona/bridge_test.go`, `jarvis-cli/internal/persona/claude_test.go`,
`jarvis-cli/internal/persona/opencode_test.go`.

Assert only the stable substring (label prefix + first clause), not the whole
bullet, so future prose polish does not churn tests.

- [x] 1.1 `bridge_test.go:239` — change expected substring from
      `- Humor: warm` to `- Humor: Warmth and humor that come from genuinely caring`.
      Satisfies: spec "Exhaustive Prose Fallback" scenario "Argentino's 5 fields
      render authored prose, not raw IDs".
- [x] 1.2 `bridge_test.go:245` — change expected substring from
      `- Address pack: peer` to `- Address pack: Address the user as a capable colleague`.
      Satisfies: same scenario as 1.1.
- [x] 1.3 `bridge_test.go:246` — change expected substring from
      `- Phrase pack: plain` to `- Phrase pack: Plain, clear, direct phrasing`.
      Satisfies: same scenario as 1.1.
- [x] 1.4 `bridge_test.go:247` — change expected substring from
      `- Anti-caricature: grounded` to `- Anti-caricature: Express character and regional color authentically`.
      Satisfies: same scenario as 1.1.
- [x] 1.5 `claude_test.go:156` — change expected substring from
      `- Address pack: peer` to `- Address pack: Address the user as a capable colleague`.
      Satisfies: spec "Claude/OpenCode Parity" scenario "Argentino prose matches
      across both render paths" (Claude side).
- [x] 1.6 `opencode_test.go:87` — change expected substring from
      `- Address pack: peer` to `- Address pack: Address the user as a capable colleague`.
      Satisfies: same parity scenario (OpenCode side).
- [x] 1.7 Do NOT touch `bridge_test.go:237` (`- Vocabulary: plain-technical`) —
      out of scope, `plain-technical` has no prose entry and keeps raw-ID fallback.
- [x] 1.8 Confirm RED: run `go test ./jarvis-cli/internal/persona/...` and verify
      the 6 updated assertions fail against the still-empty prose maps (expected
      failure before step 2).

Can run in parallel: 1.1–1.6 are independent edits across 3 files but small
enough to do as one sequential pass by a single writer (avoids partial-file
merge risk); no separate agents needed.

## 2. GREEN — fill the 5 prose-map literals in loader.go

File: `jarvis-cli/internal/persona/loader.go:168-174`.

- [x] 2.1 `vocabularyProse["rioplatense"]` — exact literal from design.md
      (Rioplatense voseo + lexicon signature, no `es-*` literals).
      Satisfies: spec "Rioplatense Signature Prose" (both scenarios).
- [x] 2.2 `humorProse["warm"]` — exact literal from design.md (generic warm
      caring energy, no sarcasm). Satisfies: spec "Shared Pack Generic
      Neutrality".
- [x] 2.3 `addressPackProse["peer"]` — exact literal from design.md (peer
      stance, no deference/bossiness). Satisfies: spec "Shared Pack Generic
      Neutrality".
- [x] 2.4 `phrasePackProse["plain"]` — exact literal from design.md (plain,
      unadorned, no regional flavor). Satisfies: spec "Shared Pack Generic
      Neutrality".
- [x] 2.5 `antiCaricatureProse["grounded"]` — exact literal from design.md
      (authentic color, never a stereotype/spectacle). Satisfies: spec
      "Layer-2 Voice-Only Boundary" + "Shared Pack Generic Neutrality".
- [x] 2.6 Leave `renderPresentation`, `proseFor`, `isBoundDialect`, and the
      Language Behavior/Portability rendering untouched — no schema/yaml/
      mechanism change.

Sequential — all 5 edits land in the same map block in one file; do as a
single atomic edit.

## 3. Verify

- [x] 3.1 Run `go test ./...` — confirm all persona tests green, including the
      6 updated assertions and `TestBoundDialectClauseUsesReadableLanguageName`
      (must stay green: rioplatense prose contains no `es-rioplatense`/
      `es-asturian`/`es-galician`).
- [x] 3.2 Run `go vet ./...` — confirm clean.
- [x] 3.3 Spot-check forbidden-string invariants: authored prose contains no
      `CONCEPTS > CODE`, no `AI IS A TOOL`, no `display_name` leak. Satisfies:
      spec "Layer-2 Voice-Only Boundary".
- [x] 3.4 Spot-check Claude/OpenCode parity: `RenderLayer2` and
      `RenderOutputStyle` render identical prose text for all 5 fields.
      Satisfies: spec "Claude/OpenCode Parity".

## Review Workload Forecast

- Estimated changed lines: ~15-20 (5 map-literal assignments in loader.go,
  6 single-line test-assertion edits, no new files).
- 400-line budget risk: none — well under threshold; standard-tier review
  (single dominant-risk lens), not full 4R.
- Chained recommendation: `review-readability` (pure prose/string content,
  no behavior/security/resilience surface) is the appropriate lens if a
  review is triggered.
- Decision needed: none — design already fixed exact literals and test
  substrings; no open questions per design.md.
