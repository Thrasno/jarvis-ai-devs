# Apply Progress: Persona Voice Foundations (Change 1)

**Mode**: Strict TDD (active). Test runner: `go test ./...` + `go vet ./...`.
**Delivery**: single-pr, size-exception accepted. Batch 1 (first + only batch).
**Result**: 26/26 tasks complete. All 5 phases done.

## Final Verification
- `go test ./...` → 23 packages, ALL PASS (no FAIL/panic).
- `go vet ./...` → clean.
- `gofmt -l` on all changed Go files → clean.
- Diff: 209 insertions, 28 deletions (237 lines) — within 400-line budget.

## TDD Cycle Evidence
| Task | RED (test first) | GREEN (impl passes) | REFACTOR |
|------|------------------|---------------------|----------|
| 1 Contract Supremacy | templates_test.go supremacy test failed ("missing ## Contract Supremacy") | Added `## Contract Supremacy` to technical-contract.md → pass | none |
| 2 Reply Language + drop `- Language:` | templates_test.go reply-language test failed; bridge/claude exact-match failed | Added `## Reply Language`; removed `- Language:` bullet in loader.go → pass | none |
| 3 Portability classifier + clauses | bridge_test.go behavior-clause assertions failed; builtin portability test | Added regionalLanguages/regionalPacks + isBoundDialect + Language Behavior block → pass | none |
| 4 Empty prose maps + proseFor | v2_test.go proseFor test = compile failure (undefined) | Added 5 empty maps + proseFor raw-ID fallback, wired into bullets → pass | none |

## Work Unit Evidence
| Evidence | Value |
|---|---|
| Focused test command / result | `go test ./internal/persona/ ./internal/config/` → ok |
| Runtime harness | N/A — pure text rendering + embedded-asset edit; no shell/process/routing boundary (per design Threat Matrix) |
| Rollback boundary | `git revert` of the 6 changed files; no schema/yaml/generated-file change |

## Files Changed
| File | Action | What |
|------|--------|------|
| jarvis-cli/embed/technical-contract.md | Modify | Added `## Contract Supremacy` (absolute precedence + 3 enumerated protected rules) and `## Reply Language` (reply follows user's language). |
| jarvis-cli/internal/persona/loader.go | Modify | Removed `- Language:` bullet; added regionalLanguages/regionalPacks + isBoundDialect(Presentation); added `### Language Behavior` block (Portability always, Dialect gating if bound); added 5 empty prose maps + proseFor raw-ID fallback wired into Vocabulary/Humor/Address pack/Phrase pack/Anti-caricature. |
| jarvis-cli/internal/config/templates_test.go | Modify | RED tests: contract contains supremacy clause + enumerated rules; contract contains reply-language rule. |
| jarvis-cli/internal/persona/bridge_test.go | Modify | Updated exact-match assertions: dropped `- Language:` bullet; added Portability/Dialect-gating clause assertions; made rioplatense/argentine fixtures genuinely bound (vocabulary: rioplatense). |
| jarvis-cli/internal/persona/v2_test.go | Modify | Added Layer-2-absence invariant test, builtin portability/dialect-gating test, proseFor raw-ID-fallback test. |
| jarvis-cli/internal/agent/claude_test.go | Modify | Updated affected rendered-output assertion (argentino output-style) from `- Language:` bullet to dialect-gating clause. |

## Invariants Preserved
- Forbidden-string checks (bridge:165/175, v2:350/420, apply:121) still pass — Layer-1 clauses live in technical-contract.md, never flow through renderPresentation.
- `keep-coding-instructions: true` frontmatter unchanged.
- No `display_name` leak: slug-heading test passes.
- Schema freeze: `preset_v2.go` and `personas/*.yaml` untouched (not in diff).
- Claude/OpenCode parity: all behavior in shared renderPresentation; RenderLayer2 and RenderOutputStyle identical for Language Behavior + prose fallback.

## Deviations from Design
- Design test strategy listed the rioplatense/argentine fixtures for dialect-gating, but those fixtures only altered language+register (generic packs), which classify as portable under isBoundDialect (regional language AND regional pack). Resolved by adding `vocabulary: rioplatense` to those RED fixtures so they exercise the genuinely-bound path — consistent with the design's bound-dialect rule.
- Added an extra builtin-preset test (TestBuiltinPresetsRenderPortabilityAndGateDialectOnlyWhenBound) to satisfy task 3.3 against real assets.

## Open Question (from design, confirmed)
- es-neutral personas (yoda/sargento/neutra) classify portable (no gated dialect layer) — validated: they render Portability only, no dialect gating.
