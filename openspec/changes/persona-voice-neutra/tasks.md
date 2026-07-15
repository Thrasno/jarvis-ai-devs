# Tasks: persona-voice-neutra (Neutra baseline VOICE)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~40-70 (5 literal map entries + 1 new test + 2 doc-comment tweaks) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | RED test + GREEN literals for Neutra voice prose | PR 1 (single-pr, requires size:exception per single-pr strategy if any threshold questioned) | `go test ./jarvis-cli/internal/persona/... -run TestNeutraPresentationRendersAuthoredVoice -v` | N/A — pure text rendering, no shell/process/runtime scenario to exercise | Revert the 5 map entries + new test; `proseFor` fallback restores raw-token rendering, no schema/yaml/generated impact |

## Phase 1: RED — Failing Test

- [x] 1.1 In `jarvis-cli/internal/persona/v2_test.go`, add `TestNeutraPresentationRendersAuthoredVoice`: load the built-in `neutra` preset, call both `RenderLayer2` and `RenderOutputStyle`.
- [x] 1.2 Assert both outputs contain label-prefix substrings: `- Vocabulary: Neutral, standard vocabulary`, `- Humor: No humor as a device`, `- Phrase pack: Plain, clear, neutral phrasing`, `- Address pack: Address the user as a professional peer`, `- Anti-caricature: Stay genuinely neutral and professional`.
- [x] 1.3 Assert Portability affirmation clause present and dialect-gating clause absent (per Requirement: Neutra Portability Without Dialect Gating).
- [x] 1.4 Assert forbidden Layer-1 strings (mentor rules / supremacy clause wording) are absent from both renders (per Requirement: Neutra Voice-Only and Cross-Agent Parity).
- [x] 1.5 Run `go test ./jarvis-cli/internal/persona/... -run TestNeutraPresentationRendersAuthoredVoice -v` and confirm it FAILS (raw tokens render, not prose).

## Phase 2: GREEN — Populate Locked Literals

- [x] 2.1 In `jarvis-cli/internal/persona/loader.go`, set `vocabularyProse["neutral-spanish"]` to the locked literal (design.md Locked Literals table).
- [x] 2.2 Set `humorProse["none"]` to the locked literal.
- [x] 2.3 Set `phrasePackProse["neutral"]` to the locked literal.
- [x] 2.4 Set `addressPackProse["neutral"]` to the locked literal.
- [x] 2.5 Set `antiCaricatureProse["neutral"]` to the locked literal.
- [x] 2.6 Do NOT modify `presentationRegister`, `isBoundDialect`, schema (`preset_v2.go`), or `neutra.yaml`.
- [x] 2.7 Refresh the 2 stale prose-map doc comments in `loader.go` that still say "empty" to reflect the now-populated Neutra entries.

## Phase 3: Verification

- [x] 3.1 Run `go test ./jarvis-cli/internal/persona/... -run TestNeutraPresentationRendersAuthoredVoice -v` and confirm it now PASSES.
- [x] 3.2 Run `go test ./...` and confirm full suite green, including `bridge_test.go:140` and `:236` (register RAW fixture) and the raw-ID fallback test for unpopulated keys.
- [x] 3.3 Run `go vet ./...` and confirm no issues.
