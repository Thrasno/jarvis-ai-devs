# Apply Progress: persona-voice-sargento (Change 2, persona 4, portable)

**Mode**: Strict TDD (RED -> GREEN -> refactor-green)
**Status**: All tasks complete (7/7). Ready for verify.
**Delivery**: single-pr (small, ~50 lines, well under 400-line budget).

## Completed Tasks

- [x] 1.1 RED: added `TestSargentoPresentationRendersAuthoredVoice` in v2_test.go
- [x] 2.1 GREEN: filled 4 dedicated prose-map literals (military / sergeant x3)
- [x] 2.2 GREEN: converted `presentationRegister` if -> switch, added mission-briefing arm
- [x] 2.3 Verified no schema/YAML change; isBoundDialect untouched (Sargento portable)
- [x] 3.1 `go test ./...` green in all 3 modules
- [x] 3.2 `go vet ./...` clean
- [x] 3.3 Parity spot-check: shared renderPresentation drives both surfaces

## Files Changed

| File | Action | What |
|------|--------|------|
| `jarvis-cli/internal/persona/v2_test.go` | Modified | New RED test `TestSargentoPresentationRendersAuthoredVoice`: loads built-in sargento preset, asserts 5 authored-voice substrings on BOTH RenderLayer2 + RenderOutputStyle, asserts absence of Layer-1 strings. Humor bullet NOT asserted (dry owned by Yoda PR #424). |
| `jarvis-cli/internal/persona/loader.go` | Modified | Filled `vocabularyProse["military"]`, `phrasePackProse["sergeant"]`, `addressPackProse["sergeant"]`, `antiCaricatureProse["sergeant"]` verbatim from design Locked Literals. Converted `presentationRegister` if -> switch; warm-direct byte-identical; added `case "mission-briefing": return "clipped, terse, and mission-focused"`. humorProse left empty. |

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| 1.1 / 2.1 / 2.2 | Test failed: rendered raw enum IDs (`- Register: mission-briefing`, `- Vocabulary: military`, `- Address pack: sergeant`, etc.) | After filling maps + register arm, test passes on both surfaces | None needed (minimal additive change; if->switch is the structural refactor, output preserved) |

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command + result | `cd jarvis-cli && go test ./internal/persona/` -> ok (0.007s); target test green |
| Runtime harness | N/A — pure in-memory string rendering, no runtime/routing/shell boundary (design Threat Matrix: N/A) |
| Rollback boundary | Revert 4 prose entries + register switch in loader.go and the new test in v2_test.go; proseFor fallback restores raw enum IDs |

## Verification Results

- `go test ./...` (jarvis-cli): all packages ok, including persona 0.007s
- `go test ./...` (hive-daemon, hive-api): all ok — unaffected
- `go vet ./...` (jarvis-cli): clean, exit 0
- `git diff --stat`: only loader.go (+23/-6) and v2_test.go (+33) — no schema/YAML change

## Deviations from Design

- Dropped the optional `display_name` absence assertion mentioned in tasks.md 1.1.
  Reason: `renderPresentation` emits `## Persona: Sargento` via `toTitleCase(preset.Name)`,
  which is byte-identical to `display_name: "Sargento"`, so a `display_name` absence check
  would always fail even with correct authored prose. The direct apply instruction listed
  only the 3 Layer-1 forbidden strings (CONCEPTS > CODE, AI IS A TOOL, Technical Behavior),
  which are asserted. Voice-only / no-Layer-1-leak intent is preserved.

## Integration Note

`presentationRegister` now a 2-arm switch (warm-direct, mission-briefing). Yoda
(`calm-teacher`) and Tony (`fast-witty`) each add a disjoint arm in sibling changes —
trivial conflict-free 3-way union. `humorProse["dry"]` intentionally NOT authored here
(Yoda owns it, PR #424) to avoid a merge conflict.
