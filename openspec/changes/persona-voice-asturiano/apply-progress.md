# Apply Progress: persona-voice-asturiano

**Change**: persona-voice-asturiano (Change 2, persona 5 — BOUND regional)
**Mode**: Strict TDD
**Status**: All 15 tasks complete. Ready for verify.
**Delivery**: single-pr, size:exception acknowledged (~40 changed lines, within budget).

## TDD Cycle Evidence

| Task | RED (test first) | GREEN (impl passes) | REFACTOR |
|------|------------------|---------------------|----------|
| 1.x relabel es-asturian | v2_test.go readable map "Asturian"→"Asturian Spanish"; ran focused test → FAIL as expected | loader.go presentationLanguage es-asturian arm → "Asturian Spanish"; test GREEN | Doc comments refreshed |
| 2.x authored voice | Added TestAsturianoPresentationRendersAuthoredVoice; ran → FAIL (raw enum IDs) | 4 prose-map literals filled; test GREEN | gofmt applied |

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test | `go test ./internal/persona/... -run 'Asturian\|BoundDialectClause'` → ok |
| Runtime harness | N/A — pure text-rendering unit test, no runtime/shell/subprocess boundary |
| Rollback boundary | Revert confined to loader.go + v2_test.go (4 prose literals + 1 relabel + 2 doc comments + 2 test edits) |

## Files Changed
| File | Action | What |
|------|--------|------|
| `jarvis-cli/internal/persona/loader.go` | Modified | es-asturian arm → "Asturian Spanish"; filled vocabulary/phrasePack/addressPack/antiCaricature asturian prose keys; refreshed 2 stale doc comments |
| `jarvis-cli/internal/persona/v2_test.go` | Modified | Updated TestBoundDialectClauseUsesReadableLanguageName expectation; added TestAsturianoPresentationRendersAuthoredVoice |
| `openspec/changes/persona-voice-asturiano/tasks.md` | Modified | All tasks marked [x] |

## Verification Results
- `go test ./...` → all packages ok (persona 0.012s), no regressions
- `go vet ./...` → clean
- `gofmt -l loader.go v2_test.go` → no diffs

## Constraints Honored
- No schema/YAML change: preset_v2.go, personas/*.yaml untouched.
- humorProse["dry"] NOT authored (Yoda owns it); Humor bullet not asserted.
- Only es-asturian arm of presentationLanguage changed.
- presentationRegister warm-direct arm untouched.
- Prose is voice-only; no Layer-1 restatement leaked.
- Claude + OpenCode parity via shared renderPresentation (both asserted).
- Source of truth embed/ + internal/ only; no generated ~/.claude/* edits.

## Deviations
None — implementation matches design (locked literals verbatim).
