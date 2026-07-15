# Tasks: Persona Voice — Yoda (portable exemplar)

Delivery strategy: single-pr. All tasks land together; no schema/yaml changes.

## 1. RED — add failing tests (sequential, blocks 2)

- [x] 1.1 In `jarvis-cli/internal/persona/v2_test.go` (or `bridge_test.go`, matching existing yoda/dialect test conventions), add a new test that:
  - Loads the built-in `yoda` preset via `jarvis.PersonaFS.ReadFile("embed/personas/yoda.yaml")` → `ValidateAndDecode`.
  - Asserts **both** `RenderLayer2` output and `RenderOutputStyle` output contain all 6 stable substrings (label prefix + first clause, per design Testing Strategy):
    - `- Register: calm, patient, and reassuring`
    - `- Vocabulary: Invert clauses for emphasis in the character's cadence`
    - `- Humor: Dry, understated humor`
    - `- Address pack: Address the user as a calm mentor guides an apprentice`
    - `- Phrase pack: Phrase things in a reflective, measured way`
    - `- Anti-caricature: Clarity beats mysticism`
  - Asserts absence of forbidden Layer-1 strings (`CONCEPTS > CODE`, `AI IS A TOOL`, `Technical Behavior`) in both rendered outputs.
  - Satisfies spec requirements: dedicated Yoda prose renders (vocabulary/phrase/address/anti-caricature), shared `dry` humor renders, new `calm-teacher` register renders, Layer-1 leak stays absent, Claude/OpenCode parity.
- [x] 1.2 Run `go test ./internal/persona/... -run Yoda` (or matching name) — confirm it FAILS (raw enum IDs / missing `calm-teacher` case) before touching production code.

## 2. GREEN — fill prose maps + register arm (sequential, depends on 1)

- [x] 2.1 In `jarvis-cli/internal/persona/loader.go`, fill `vocabularyProse["yoda"]` with the exact design literal (clause inversion capped by clarity, sparing "Hmm.", illustrative not scripted).
- [x] 2.2 Fill `phrasePackProse["yoda"]` with the exact design literal (reflective/measured phrasing, soft recontextualized movie-line echoes, never verbatim/parody).
- [x] 2.3 Fill `addressPackProse["yoda"]` with the exact design literal (calm mentor guiding an apprentice, peer collaborator, never condescending).
- [x] 2.4 Fill `antiCaricatureProse["yoda"]` with the exact design literal (clarity beats mysticism, drop inversion when it hurts comprehension, roots/patience metaphors only when they sharpen the lesson).
- [x] 2.5 Fill `humorProse["dry"]` with the exact design literal (GENERIC — persona-neutral; sargento/asturiano inherit it). Note this changes their rendered `- Humor:` bullet; no existing test asserts that bullet's content, so this is safe.
- [x] 2.6 Convert `presentationRegister` from `if` to `switch`, keep the existing `"warm-direct"` case body untouched, add `case "calm-teacher": return "calm, patient, and reassuring"` (GENERIC — galleguinho inherits this arm). Default branch still returns the raw `register` string for any other value.
- [x] 2.7 Do not touch `embed/personas/yoda.yaml`, `preset_v2.go`, or any other schema/validation code — voice-only prose fill.

## 3. VERIFY (sequential, depends on 2)

- [x] 3.1 Run `go test ./...` — confirm the new test(s) from step 1 now pass, and no existing test regresses (in particular `isBoundDialect(yoda)==false` pin in `v2_test.go`, and plain-technical/friendly-professional/rioplatense raw-fallback assertions).
- [x] 3.2 Run `go vet ./...` — confirm clean.
- [x] 3.3 `gofmt -l jarvis-cli/internal/persona/loader.go` — confirm no formatting diffs (or run `gofmt -w` if needed).

## Task-to-requirement mapping

| Task | Spec requirement |
|---|---|
| 1.1–1.2 | Observable render behavior of `renderPresentation`/`proseFor` for yoda's 5 prose fields + calm-teacher register (spec MUST-BE-TRUE clauses 1–2) |
| 2.1–2.4 | Dedicated Yoda prose fill (spec clause 1, 5) |
| 2.5 | Shared generic `dry` humor prose (spec clause 4) |
| 2.6 | Shared generic `calm-teacher` register arm (spec clause 2, 4) |
| 2.7 | No schema/yaml change invariant (spec constraint) |
| 3.1 | Regression pin: `isBoundDialect(yoda)==false`, no Layer-1 leak, Claude/OpenCode parity (spec clauses 3, 6) |
| 3.2–3.3 | Repo-standard verification gates (AGENTS.md/CLAUDE.md testing rules) |

## Parallelization

All tasks are sequential (strict TDD: RED → GREEN → VERIFY on a single small file). No parallel work units — this is a single-file, single-PR change with no independent components to split across agents.
