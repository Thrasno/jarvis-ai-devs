# Apply Progress: persona-voice-neutra (Neutra baseline VOICE)

**Mode**: Strict TDD
**Status**: done — 15/15 tasks complete
**Delivery**: single-pr (~30 authored lines added, within 400-line budget)

## Completed Tasks

- [x] 1.1 Added `TestNeutraPresentationRendersAuthoredVoice` in v2_test.go (loads built-in neutra, calls RenderLayer2 + RenderOutputStyle)
- [x] 1.2 Asserts 5 label-prefix voice substrings in both renders
- [x] 1.3 Asserts Portability clause present, dialect-gating clause absent
- [x] 1.4 Asserts forbidden Layer-1 strings absent (CONCEPTS > CODE, AI IS A TOOL, Technical Behavior)
- [x] 1.5 Confirmed RED (raw tokens rendered before fix)
- [x] 2.1 vocabularyProse["neutral-spanish"] set to locked literal
- [x] 2.2 humorProse["none"] set to locked literal
- [x] 2.3 phrasePackProse["neutral"] set to locked literal
- [x] 2.4 addressPackProse["neutral"] set to locked literal
- [x] 2.5 antiCaricatureProse["neutral"] set to locked literal
- [x] 2.6 presentationRegister / isBoundDialect / preset_v2.go / neutra.yaml UNCHANGED
- [x] 2.7 Refreshed 2 stale "empty" prose-map doc comments
- [x] 3.1 Focused test PASS
- [x] 3.2 `go test ./...` green (jarvis-cli module)
- [x] 3.3 `go vet ./...` clean

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| Neutra voice prose | `TestNeutraPresentationRendersAuthoredVoice` failed: rendered `- Vocabulary: neutral-spanish` (raw token) | 5 prose-map literals populated → test PASS | Doc comments refreshed; no logic refactor needed |

## Work Unit Evidence

| Evidence | Value |
|----------|-------|
| Focused test | `go test ./internal/persona/... -run TestNeutraPresentationRendersAuthoredVoice -v` → PASS |
| Runtime harness | N/A — pure text rendering, no shell/process/runtime boundary |
| Rollback boundary | Revert 5 map entries + new test; proseFor fallback restores raw-token rendering. No schema/yaml/generated impact |

## Verification Results

- `go test ./...` (jarvis-cli): all packages ok, internal/persona 0.012s
- `go vet ./...` (jarvis-cli): exit 0, clean

## Files Changed

- `jarvis-cli/internal/persona/loader.go` — populated 5 prose-map literals + refreshed 2 doc comments
- `jarvis-cli/internal/persona/v2_test.go` — added `TestNeutraPresentationRendersAuthoredVoice`

## Deviations from Design

None — implementation matches design exactly. Literals verbatim from locked design.

## Risks

None. Neutra stays portable (isBoundDialect=false), register friendly-professional renders raw (bridge_test.go:140/:236 green), raw-ID fallback test green.
