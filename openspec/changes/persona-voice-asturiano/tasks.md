# Tasks: persona-voice-asturiano

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~35-50 (4 prose literals + 1 kept-as-is arm confirmation + 1 modified assertion + 1 new test) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Fill 4 asturian prose keys, keep es-asturian labeled "Asturian" (no relabel), RED→GREEN | PR 1 (single-pr) | `go test ./internal/persona/... -run 'Asturian|BoundDialectClause'` | N/A — pure text-rendering unit test, no external runtime scenario | Revert confined to `internal/persona/loader.go` + `internal/persona/v2_test.go` |

## Phase 1: Existing Test — Readable Language Name

- [x] 1.1 In `jarvis-cli/internal/persona/v2_test.go`, keep `TestBoundDialectClauseUsesReadableLanguageName` asturiano expectation as `"asturiano": "Asturian"` (corrected foundations already renders "Asturian"; the earlier redundant relabel is reverted). `rawEnums` still asserts `"es-asturian"` absent.
- [x] 1.2 Run `go test ./internal/persona/... -run TestBoundDialectClauseUsesReadableLanguageName` — confirm GREEN against the `"Asturian"` return in loader.go.

## Phase 2: RED — New Authored-Voice Test

- [x] 2.1 In `jarvis-cli/internal/persona/v2_test.go`, add `TestAsturianoPresentationRendersAuthoredVoice`: load builtin `asturiano.yaml` via `ValidateAndDecode`, and for BOTH `RenderLayer2(preset)` and `RenderOutputStyle(preset)` assert presence of these label-prefix substrings (per design/spec, verbatim):
  - `"- Vocabulary: Asturian-flavored Spanish"`
  - `"- Phrase pack: Warm, measured phrasing with a wink of Asturian retranca"`
  - `"- Address pack: Address the user as a warm, close peer"`
  - `"- Anti-caricature: The Asturian warmth and retranca are seasoning"`
  - `"- Dialect gating: the Asturian dialect layer"`
  - `"applies only when replying in Spanish"`
- [x] 2.2 In the same test, assert absence of forbidden Layer-1 strings (supremacy clause / reply-language leakage), per foundations Layer-1 invariant. Do NOT assert the Humor bullet (raw `dry` stays out of scope).
- [x] 2.3 Run `go test ./internal/persona/... -run TestAsturianoPresentationRendersAuthoredVoice` — confirm RED (prose maps are still empty, output falls back to raw enum IDs).

## Phase 3: GREEN — Fill Prose Maps

- [x] 3.1 In `jarvis-cli/internal/persona/loader.go`, keep the `case "es-asturian":` arm of `presentationLanguage` returning `"Asturian"` (the earlier redundant relabel is reverted; corrected foundations gates on "Spanish"). Leave `es-rioplatense` and `es-galician` arms untouched.
- [x] 3.2 In `jarvis-cli/internal/persona/loader.go`, set `vocabularyProse["asturian"]` to the locked literal: `"Asturian-flavored Spanish — weave warm Asturian lexicon and turns of phrase into clear Spanish (light bable touches like 'ho', 'guaje', 'prestar', 'ñeru'), always kept light enough that the message stays perfectly clear; the flavor is seasoning, never an obstacle to understanding."`
- [x] 3.3 Set `phrasePackProse["asturian"]` to the locked literal: `"Warm, measured phrasing with a wink of Asturian retranca — dry, understated regional wit and the easygoing cadence of someone who'd settle a debate over a few sidras. Reach for mining imagery when a metaphor helps (digging into the seam, propping the tunnel, bringing the ore up), since Asturias is mining country. Keep the levity light; the point always lands."`
- [x] 3.4 Set `addressPackProse["asturian"]` to the locked literal: `"Address the user as a warm, close peer — a paisanu you'd share a table and a sidra with; direct, honest, and welcoming, never deferential or distant."`
- [x] 3.5 Set `antiCaricatureProse["asturian"]` to the locked literal: `"The Asturian warmth and retranca are seasoning, not a costume — light bable and the odd sidra or mining aside are welcome, but never pile on regional clichés or perform a postcard Asturias; the flavor serves warmth and clarity, and a lively tone never replaces verifying facts and doing the work right."`
- [x] 3.6 Do NOT touch `humorProse["asturian"]` (stays unmapped, out of scope) or the `warm-direct` register arm.

## Phase 4: Verification

- [x] 4.1 Run `go test ./internal/persona/... -run 'Asturian|BoundDialectClause'` — confirm both tests are GREEN.
- [x] 4.2 Run `go test ./...` — confirm no regressions across the module.
- [x] 4.3 Run `go vet ./...` — confirm clean.
- [x] 4.4 Run `gofmt -l jarvis-cli/internal/persona/loader.go jarvis-cli/internal/persona/v2_test.go` — confirm no formatting diffs.
