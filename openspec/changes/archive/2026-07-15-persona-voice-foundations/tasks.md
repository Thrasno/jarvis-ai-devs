# Tasks: Persona Voice Foundations (Change 1)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~220-320 (2 source files + 3 test files + 1 config test) |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Contract supremacy clause (Layer 1) | single PR | `go test ./internal/config/... -run TestTechnicalContract` | N/A — pure text render, no shell/process | `embed/technical-contract.md` + `internal/config/templates_test.go` |
| 2 | Reply-language rule + drop `- Language:` bullet | single PR | `go test ./internal/persona/... -run TestRenderV2Presentation` | N/A — pure text render | `embed/technical-contract.md` + `loader.go` renderPresentation bullet |
| 3 | Portability classifier + Language-Behavior clauses | single PR | `go test ./internal/persona/... -run TestRenderV2PresentationKeepsPolicyOutOfPresentationSurfaces` | N/A — pure text render | `loader.go` (isBoundDialect, clause block) |
| 4 | Empty prose maps + proseFor fallback | single PR | `go test ./internal/persona/... -run TestBuiltinProfilesV2MatchPresentationMatrix` | N/A — pure text render | `loader.go` (5 maps, proseFor call sites) |

Since size-exception is required (Medium risk, single-pr strategy), maintainer approval is needed before apply; all 4 units land in one PR but tasks stay independently revertable per the rollback boundaries above.

## Phase 1: Contract Supremacy Clause (Fix #1, Layer 1)

- [x] 1.1 RED: add test in `jarvis-cli/internal/config/templates_test.go` asserting `Layer1Content()`/rendered CLAUDE.md+AGENTS.md contain `## Contract Supremacy` with the three named rules (verify-before-asserting, confirmed-vs-assumption, one-question-then-stop) and the voice-styles-delivery-only statement.
- [x] 1.2 RED: add test asserting `RenderLayer2`/`RenderOutputStyle` presentation output does NOT contain the supremacy clause (forbidden-string invariant).
- [x] 1.3 GREEN: add `## Contract Supremacy` section to `jarvis-cli/embed/technical-contract.md` (after Evidence, Certainty, and Safety) per design exact wording.
- [x] 1.4 Run `go test ./internal/config/... ./internal/persona/...` and `go vet ./...`; confirm both new tests pass and no existing Layer1/Layer2 test breaks.

## Phase 2: Authoritative Reply-Language Rule (Fix #2)

- [x] 2.1 RED: add test in `jarvis-cli/internal/config/templates_test.go` asserting rendered contract contains `## Reply Language` rule (reply follows user's language; persona voice never forces different reply language; artifact language unaffected).
- [x] 2.2 RED: update exact-match assertions in `jarvis-cli/internal/persona/bridge_test.go` (TestRenderV2PresentationRendersEverySelectedTrait, line ~219) to remove expectation of `- Language: en-us` bullet.
- [x] 2.3 RED: update `jarvis-cli/internal/persona/v2_test.go` and `apply_test.go` exact-match assertions that reference the `- Language:` bullet to drop it.
- [x] 2.4 GREEN: add `## Reply Language` section to `jarvis-cli/embed/technical-contract.md` (before Persona Scope and Artifact Language) per design exact wording; confirm no conflict with `sdd-orchestrator.md:111-115`.
- [x] 2.5 GREEN: remove the `- Language: <X>` bullet emission from `renderPresentation` in `jarvis-cli/internal/persona/loader.go`.
- [x] 2.6 Run `go test ./internal/config/... ./internal/persona/...` and `go vet ./...`; confirm RED tests from 2.1-2.3 now pass.

## Phase 3: Portability Classifier + Language-Behavior Clauses (Fix #3)

- [x] 3.1 RED: update `jarvis-cli/internal/persona/bridge_test.go` TestRenderV2PresentationKeepsPolicyOutOfPresentationSurfaces (lines ~139/147/172) — replace old language-bullet assertions with Language-Behavior clause assertions: rioplatense expects dialect-gating clause naming "Rioplatense Spanish (voseo)"; custom en-us preset expects Portability clause and asserts absence of dialect-gating text.
- [x] 3.2 RED: update `jarvis-cli/internal/persona/bridge_test.go` TestArgentinePresentationKeepsSharedLayer1OutOfRenderedSurfaces (line ~201) to assert the dialect-gating clause text instead of the old bullet.
- [x] 3.3 RED: add test asserting portable presets (neutra/yoda/sargento/tony-stark) render the Portability affirmation clause and do NOT render a dialect-gating clause.
- [x] 3.4 GREEN: add `regionalLanguages` and `regionalPacks` sets + `isBoundDialect(p Presentation) bool` to `jarvis-cli/internal/persona/loader.go` per design (false unless regional language AND matching regional pack in Vocabulary/PhrasePack/AddressPack).
- [x] 3.5 GREEN: add `### Language Behavior` render block in `renderPresentation` (after the 12 trait bullets): always emit Portability clause; emit Dialect-gating clause only when `isBoundDialect` is true, using `presentationLanguage(p.Language)` for `{NATIVE}`.
- [x] 3.6 Run `go test ./internal/persona/...` and `go vet ./...`; confirm bound presets (argentino/asturiano/galleguinho) show dialect-gating and portable presets show only affirmation.

## Phase 4: Empty Prose Maps + Raw-ID Fallback (Fix #4)

- [x] 4.1 RED: add test asserting every value in `v2AllowedPresentationValues` resolves to a non-empty string in the rendered Vocabulary/Humor/AddressPack/PhrasePack/AntiCaricature bullets (falls back to raw enum ID when unmapped).
- [x] 4.2 RED: confirm `TestBuiltinProfilesV2MatchPresentationMatrix` still asserts current asset tuples unchanged (no new prose expected since maps ship empty).
- [x] 4.3 GREEN: add 5 empty maps (`phrasePackProse`, `addressPackProse`, `vocabularyProse`, `humorProse`, `antiCaricatureProse`) and `proseFor(t map[string]string, id string) string` (returns `t[id]` if present and non-blank, else `id`) to `jarvis-cli/internal/persona/loader.go`.
- [x] 4.4 GREEN: wire `proseFor` into the Vocabulary/Humor/AddressPack/PhrasePack/AntiCaricature bullet emission sites in `renderPresentation`.
- [x] 4.5 Run `go test ./internal/persona/... -run TestBuiltinProfilesV2MatchPresentationMatrix` and full `go test ./...`; confirm existing exact-match bullet strings still pass (empty maps = no behavior change yet).

## Phase 5: Verification & Parity

- [x] 5.1 Run full `go test ./...` and `go vet ./...` across `jarvis-cli` module; all green.
- [x] 5.2 Verify Claude/OpenCode parity: confirm `RenderLayer2` and `RenderOutputStyle` produce identical Language Behavior + prose-fallback output for the same preset (no OpenCode-only frontmatter divergence).
- [x] 5.3 Verify preserved invariants: forbidden-string checks (bridge_test.go:165/175, v2_test.go:350/420, apply_test.go:121), `keep-coding-instructions:true` behavior, no `display_name` leak in slug heading.
- [x] 5.4 Confirm `preset_v2.go` and all `personas/*.yaml` files are untouched (schema freeze).
- [x] 5.5 Run `gofmt -l` on all changed Go files; format if needed.
