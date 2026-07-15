# Tasks: Persona Voice — Tony Stark (Change 2, persona 3)

Delivery strategy: single-pr. Strict TDD (RED → GREEN → verify) per repo convention.

## 1. RED — add failing presentation-voice test (sequential, blocks task 2)

**Satisfies**: spec requirements 1–9 (Tony's 5 dedicated packs render non-raw prose;
fast-witty register parity with warm-direct; portability/no dialect-gating stays
false; confidence-vs-verification; wit targets problem not user; soft character
nods; anti-caricature forbids arrogance/false-certainty/skipped-verification/
condescension; no Layer-1 string leak; Claude/OpenCode parity).

- [x] 1.1 In `jarvis-cli/internal/persona/v2_test.go`, add
      `TestTonyStarkPresentationRendersAuthoredVoice`.
  - Load the built-in `tony-stark` preset (same fixture/lookup pattern as
    `TestBuiltinProfilesV2MatchPresentationMatrix` / `TestBuiltinPresetsRenderPortabilityAndGateDialectOnlyWhenBound`).
  - Call both `RenderLayer2` and `RenderOutputStyle` on the loaded presentation.
  - For **both** render outputs, assert presence of the stable label-prefix +
    first-clause substrings (exact literals, from design):
    - `- Register: fast, witty, and confident`
    - `- Vocabulary: engineering and systems vocabulary`
    - `- Humor: quick, dry, clever wit`
    - `- Address pack: address the user as a capable engineering peer`
    - `- Phrase pack: fast, punchy delivery with sharp one-liners`
    - `- Anti-caricature: keep the wit and confidence as delivery style only`
  - Assert **absence** of forbidden Layer-1 strings in both outputs:
    `CONCEPTS > CODE`, `AI IS A TOOL`, `Technical Behavior`.
  - Run `go test ./jarvis-cli/internal/persona/... -run TestTonyStarkPresentationRendersAuthoredVoice`
    and confirm it FAILS (RED) because the prose maps are currently empty
    (`vocabularyProse`, `humorProse`, `phrasePackProse`, `addressPackProse`,
    `antiCaricatureProse` are all `map[string]string{}` at loader.go:169–173,
    so `proseFor` falls back to the raw enum ID) and `presentationRegister`
    has no `fast-witty` arm (loader.go:188–193, only handles `warm-direct`).

## 2. GREEN — fill Tony's prose literals and register arm (sequential, depends on 1)

**Satisfies**: same spec requirements as task 1 (this is the implementation
that makes the RED test pass).

- [x] 2.1 In `jarvis-cli/internal/persona/loader.go`, add dedicated Tony
      entries to the five prose maps (loader.go:168–174), keyed by the exact
      enum IDs used in Tony's preset (`"engineering"`, `"witty"`, `"engineer"`
      for phrase/address/anti-caricature — per design, all four packs keyed
      `"engineer"`). Use the exact authored strings from design (Engram #4496):
      - `vocabularyProse["engineering"]` = engineering/systems vocabulary clause.
      - `humorProse["witty"]` = quick/dry/clever wit clause.
      - `phrasePackProse["engineer"]` = fast/punchy delivery clause.
      - `addressPackProse["engineer"]` = capable engineering peer clause.
      - `antiCaricatureProse["engineer"]` = keep-wit-as-delivery-style-only clause.
  - Do not touch Argentino/Yoda keys already present or reserved — Tony's
    keys must be disjoint from other personas' keys per design (trivial
    union at integration).
- [x] 2.2 In `presentationRegister` (loader.go:188–193), convert the
      `if register == "warm-direct"` statement to a `switch register { ... }`
      statement. Keep the `warm-direct` case byte-identical
      (`"warm, energetic, and direct"`). Add a new case:
      `case "fast-witty": return "fast, witty, and confident"`.
      Leave the default fallthrough (`return register`) unchanged.
  - Do not add the Yoda `calm-teacher` case here — that is out of scope for
    this change and will be a separate disjoint-branch addition at Yoda's
    own integration (per design, trivial union with no conflict expected).

## 3. Verify (sequential, depends on 2)

**Satisfies**: overall spec correctness + repo testing rules (no regressions).

- [x] 3.1 Run `go test ./...` — confirm
      `TestTonyStarkPresentationRendersAuthoredVoice` passes and no existing
      test regresses, in particular:
      `TestPresentationValuesResolveNonEmptyWithRawIDFallback`,
      `TestBuiltinProfilesV2MatchPresentationMatrix`,
      `TestBuiltinPresetsRenderPortabilityAndGateDialectOnlyWhenBound`
      (Tony's `isBoundDialect` must remain `false`; the new register case
      introduces no dialect gating).
- [x] 3.2 Run `go vet ./...` — confirm clean.
- [x] 3.3 Run `gofmt -l jarvis-cli/internal/persona/loader.go jarvis-cli/internal/persona/v2_test.go`
      and confirm no output (files already formatted); if not, run `gofmt -w`
      on both.

## Parallelization

All three tasks are strictly sequential (TDD RED → GREEN → verify); no
parallel work units. Single file pair (`loader.go` + `v2_test.go`), single
writer, single PR.

## Out of scope (confirmed by design)

- No changes to `preset_v2.go`, `embed/personas/*.yaml`, or any generated
  `~/.claude/*` artifact.
- No Yoda `calm-teacher` register arm (separate change).
- No schema change.
