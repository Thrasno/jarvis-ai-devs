# Apply Progress: Persona Voice — Argentino

**Change**: persona-voice-argentino (Change 2, persona 1 of 2)
**Mode**: Strict TDD
**Batch**: 1 (first and only — all tasks complete)
**Status**: 18/18 tasks complete — ready for verify

## Completed Tasks

### 1. RED — update the 6 exact-match assertions
- [x] 1.1 bridge_test.go:239 `- Humor: warm` → `- Humor: Warmth and humor that come from genuinely caring`
- [x] 1.2 bridge_test.go:245 `- Address pack: peer` → `- Address pack: Address the user as a capable colleague`
- [x] 1.3 bridge_test.go:246 `- Phrase pack: plain` → `- Phrase pack: Plain, clear, direct phrasing`
- [x] 1.4 bridge_test.go:247 `- Anti-caricature: grounded` → `- Anti-caricature: Express character and regional color authentically`
- [x] 1.5 internal/agent/claude_test.go:156 `- Address pack: peer` → `- Address pack: Address the user as a capable colleague`
- [x] 1.6 internal/agent/opencode_test.go:87 `- Address pack: peer` → `- Address pack: Address the user as a capable colleague`
- [x] 1.7 bridge_test.go:237 (`- Vocabulary: plain-technical`) left untouched (raw-ID fallback preserved)
- [x] 1.8 RED confirmed: assertions failed against empty prose maps

### 2. GREEN — fill the 5 prose-map literals in loader.go
- [x] 2.1 vocabularyProse["rioplatense"] — signature voseo + lexicon prose (no es-* literals)
- [x] 2.2 humorProse["warm"] — generic warm caring energy
- [x] 2.3 addressPackProse["peer"] — generic peer stance
- [x] 2.4 phrasePackProse["plain"] — generic plain unadorned phrasing
- [x] 2.5 antiCaricatureProse["grounded"] — generic authentic-color prose
- [x] 2.6 renderPresentation / proseFor / isBoundDialect / Language Behavior untouched — no schema/yaml/mechanism change

### 3. Verify
- [x] 3.1 `go test ./...` — all packages OK (persona + agent green, incl. TestBoundDialectClauseUsesReadableLanguageName)
- [x] 3.2 `go vet ./...` — clean (exit 0)
- [x] 3.3 Forbidden-string spot-check: prose block (loader.go:168-183) contains no `es-rioplatense`/`es-asturian`/`es-galician`, no `CONCEPTS > CODE`, no `AI IS A TOOL`, no `display_name`. (es-* matches at 127-129/189-193 are pre-existing, outside the prose block.)
- [x] 3.4 Claude/OpenCode parity: shared renderPresentation → identical prose in both paths (agent tests green).

## Files Changed
| File | Action | What |
|------|--------|------|
| jarvis-cli/internal/persona/loader.go | Modified | Filled 5 prose-map literals (rioplatense, warm, peer, plain, grounded) verbatim from design |
| jarvis-cli/internal/persona/bridge_test.go | Modified | Updated 4 substring assertions (Humor, Address pack, Phrase pack, Anti-caricature) |
| jarvis-cli/internal/agent/claude_test.go | Modified | Updated Address pack substring assertion (line 156) |
| jarvis-cli/internal/agent/opencode_test.go | Modified | Updated Address pack substring assertion (line 87) |

## TDD Cycle Evidence
| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1–1.6 (assertions) | bridge_test.go, agent/claude_test.go, agent/opencode_test.go | Unit | ✅ persona+agent green | ✅ Written | ✅ Failed as expected then passed | ➖ Fixed literals (design table) | ➖ None needed |
| 2.1–2.5 (prose fill) | (same assertions) | Unit | ✅ baseline | ✅ RED via updated assertions | ✅ Passed | ➖ 6 assertions across 3 files exercise both render paths | ➖ None needed |

Note: design fixed exact literals + exact substring diffs; triangulation is the multi-field/multi-render-path coverage (6 assertions, 2 paths). No branching logic to generalize.

## Work Unit Evidence
| Evidence | Value |
|---|---|
| Focused test command + result | `go test ./internal/persona/... ./internal/agent/...` → both `ok` |
| Runtime harness | N/A — pure in-memory string data, no runtime/shell/subprocess boundary |
| Rollback boundary | Revert loader.go 5 map literals to empty + restore 6 assertions; renderer falls back to raw IDs |

## Deviations from Design
None — implementation matches design literals and substring table exactly. Only note: design abbreviated `claude_test.go`/`opencode_test.go` paths; actual location is `jarvis-cli/internal/agent/` (not persona). Line numbers matched.

## Verification Output
- `go test ./...`: all packages `ok` (no FAIL)
- `go vet ./...`: exit 0, no findings
