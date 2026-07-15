# Tasks: Persona Voice — Galleguinho

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~60-90 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Fill galleguinho voice (2 test files + loader.go) | PR 1 (single-pr, needs `size:exception` or explicit go-ahead per single-pr strategy) | `go test ./jarvis-cli/internal/persona/... -run 'Galleguinho|BoundDialectClauseUsesReadableLanguageName'` | N/A — pure Go unit test rendering, no shell/process/CLI harness applicable | Revert `jarvis-cli/internal/persona/loader.go` + `jarvis-cli/internal/persona/v2_test.go`; clean two-file revert |

## Phase 1: RED — Update Existing Assertion

- [x] 1.1 In `jarvis-cli/internal/persona/v2_test.go` `TestBoundDialectClauseUsesReadableLanguageName` (~line 421), change `"galleguinho": "Galician"` to `"galleguinho": "Galician Spanish"`.
- [x] 1.2 Run `go test ./jarvis-cli/internal/persona/... -run TestBoundDialectClauseUsesReadableLanguageName` and confirm it now FAILS (loader.go not yet updated).

## Phase 2: RED — New Voice Test

- [x] 2.1 In `jarvis-cli/internal/persona/v2_test.go`, add `TestGalleguinhoPresentationRendersAuthoredVoice`: load the built-in `galleguinho` preset, render via both `RenderLayer2` and `RenderOutputStyle`.
- [x] 2.2 Assert both renders contain stable substrings for: `"- Vocabulary: Galician-flavored Spanish"`, `"- Humor: Galician retranca"`, `"- Phrase pack: Calm, unhurried, warm phrasing with a touch of morriña"`, `"- Address pack: Address the user as a warm, close paisano"`, `"- Anti-caricature: The retranca and Galician warmth are seasoning"`, `"- Dialect gating: the Galician Spanish dialect layer"` (per spec Requirement: Galleguinho Dedicated Prose Fill, Requirement: Galician-Spanish Dialect Label, Requirement: Retranca Anti-Caricature Guardrail).
- [x] 2.3 Assert absence of forbidden Layer-1 strings on both render paths (per spec Requirement: Claude/OpenCode Parity).
- [x] 2.4 Do NOT assert the Register bullet (calm-teacher is out of scope; Requirement: Galleguinho Dedicated Prose Fill, Scenario "Register bullet not asserted").
- [x] 2.5 Run the new test and confirm it FAILS (prose maps still empty / raw enum IDs render).

## Phase 3: GREEN — Fill Loader Voice Literals

- [x] 3.1 In `jarvis-cli/internal/persona/loader.go` `presentationLanguage` `case "es-galician"` (~line 182), change return value from `"Galician"` to `"Galician Spanish"`. Do NOT touch `es-rioplatense`/`es-asturian` arms.
- [x] 3.2 In `jarvis-cli/internal/persona/loader.go` prose-map vars (~lines 168-173), set `humorProse["retranca"]`, `vocabularyProse["galician"]`, `phrasePackProse["galician"]`, `addressPackProse["galician"]`, `antiCaricatureProse["galician"]` to the exact LOCKED literals from design (verbatim, no paraphrase).
- [x] 3.3 Do NOT edit `presentationRegister` or add a `"calm-teacher"` arm (Yoda PR #424 owns it).
- [x] 3.4 Refresh the stale doc comments at loader.go ~156-158 and ~166-167 ("Prose maps ship empty...") if they still describe all-empty state, to note galleguinho's 5 keys are now filled.

## Phase 4: Verification

- [x] 4.1 Run `go test ./...` and confirm all green, including both Phase 1 and Phase 2 tests.
- [x] 4.2 Run `go vet ./...` and confirm clean.
- [x] 4.3 Confirm no schema/yaml diff and no generated `~/.claude/*` artifact changed (source-of-truth only: `jarvis-cli/internal/persona/loader.go`, `jarvis-cli/internal/persona/v2_test.go`).
