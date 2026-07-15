# Spec Delta: Persona Presentation Rendering — Yoda Voice

Change: `persona-voice-yoda`
Capability: `persona-presentation-rendering` (no new capability; voice-only prose fill on the
Change-1 foundations mechanism — `renderPresentation` / `proseFor` / `presentationRegister` /
`isBoundDialect` in `jarvis-cli/internal/persona/loader.go`).

## MUST-BE-TRUE (post-change)

### Requirement: Yoda's dedicated packs render authored prose, not raw enum IDs

After this change, rendering the built-in `yoda` preset through `RenderLayer2` or
`RenderOutputStyle` MUST resolve each of the following presentation values to non-empty,
human-readable prose via `proseFor`, not fall back to the raw enum ID string:

- `- Vocabulary:` — resolved from `vocabularyProse["yoda"]`.
- `- Phrase pack:` — resolved from `phrasePackProse["yoda"]`.
- `- Address pack:` — resolved from `addressPackProse["yoda"]`.
- `- Anti-caricature:` — resolved from `antiCaricatureProse["yoda"]`.
- `- Humor:` — resolved from `humorProse["dry"]` (shared pack; see Requirement below).

#### Scenario: Rendering yoda no longer emits raw enum IDs

- **GIVEN** the built-in `yoda` preset resolved via `ResolveProfile`
- **WHEN** `RenderLayer2(preset)` (or `RenderOutputStyle(preset)`) renders the `### Presentation`
  section
- **THEN** the `- Vocabulary:`, `- Phrase pack:`, `- Address pack:`, `- Anti-caricature:`, and
  `- Humor:` bullets each contain their authored prose string
- **AND** none of those five bullets render the bare enum ID (`yoda` or `dry`) as its entire value

### Requirement: calm-teacher register renders readable prose

`presentationRegister` MUST resolve the `calm-teacher` register token to non-empty, generic
readable prose, following the same pattern already used for the `warm-direct` arm.

#### Scenario: Rendering yoda's Register bullet is readable

- **GIVEN** the built-in `yoda` preset (`presentation.register == "calm-teacher"`)
- **WHEN** `renderPresentation` renders the `- Register:` bullet
- **THEN** the bullet's value is authored prose, not the literal string `calm-teacher`

### Requirement: Yoda classifies as portable — no dialect-gating clause

Yoda's `presentation.language` is `es-neutral`, which is absent from `regionalLanguages`, so
`isBoundDialect` MUST return `false` for Yoda's presentation. Rendering Yoda MUST include the
Portability affirmation clause and MUST NOT include the Dialect gating clause.

#### Scenario: Yoda renders as portable

- **GIVEN** the built-in `yoda` preset
- **WHEN** its presentation is rendered
- **THEN** the `### Language Behavior` section contains the Portability affirmation line
  ("this character and its register apply in whatever language the user writes...")
- **AND** the rendered output does NOT contain a "Dialect gating:" line

This is a regression-pinning scenario: `isBoundDialect(yoda) == false` is already asserted at
`jarvis-cli/internal/persona/v2_test.go:364` prior to this change and MUST remain true after it.

### Requirement: shared `dry` and `calm-teacher` prose stay generic (no Yoda flavor)

`humorProse["dry"]` and the `calm-teacher` arm of `presentationRegister` are shared across
personas (`dry` is also selected by `sargento` and `asturiano`; `calm-teacher` is also selected
by `galleguinho`). Their authored prose MUST read as generic, persona-neutral English/Spanish
description of the trait — it MUST NOT reference Yoda-specific flavor (no clause inversion, no
"Hmm.", no movie nods, no roots/patience metaphor, no Yoda naming) so that inheriting personas'
rendered bullets remain sensible for their own voice.

#### Scenario: dry humor prose is Yoda-agnostic

- **GIVEN** `humorProse["dry"]`
- **WHEN** it is inspected or rendered for any persona selecting `humor: dry` (Yoda, sargento,
  asturiano)
- **THEN** its text describes the dry/understated humor trait generically
- **AND** it contains no Yoda-specific vocabulary (no inversion cue, no "Hmm.", no movie
  reference, no roots/patience metaphor)

#### Scenario: calm-teacher register prose is Yoda-agnostic

- **GIVEN** the `calm-teacher` arm of `presentationRegister`
- **WHEN** it is inspected or rendered for any persona selecting `register: calm-teacher` (Yoda,
  galleguinho)
- **THEN** its text describes the calm-teacher register generically
- **AND** it contains no Yoda-specific vocabulary (no inversion cue, no "Hmm.", no movie
  reference, no roots/patience metaphor)

### Requirement: Yoda's dedicated prose encodes clause inversion capped by clarity

Yoda's dedicated prose (vocabulary, phrase pack, address pack, and/or anti-caricature) MUST
communicate, in substance, all of the following intents. Exact wording is an implementation
choice; these are the observable intents the authored text must convey:

1. Clause inversion (subject/object before verb) is a strong, frequent stylistic signature.
2. Clarity and teaching effectiveness are a hard cap on that signature: inversion is dropped or
   reduced whenever it would bury the technical point, so the lesson always lands.
3. "Hmm." is a sporadic thinking beat, explicitly described as occasional/sparing, never framed
   as a constant tic.
4. Any reference to movies/lines is framed as a soft, recontextualized echo — never described as,
   or encouraging, verbatim quotation, out-of-context quoting, or parody.
5. Roots-and-patience metaphor imagery is present somewhere in the dedicated prose set.
6. The anti-caricature prose states that clarity beats mysticism and that calm must not become
   vagueness or false certainty.

#### Scenario: Yoda's prose caps inversion with a clarity rule

- **GIVEN** Yoda's dedicated prose set (vocabulary/phrase/address/anti-caricature)
- **WHEN** the prose describing clause inversion is read
- **THEN** it explicitly pairs the inversion signature with a clarity/teaching cap — it does not
  present inversion as unconditional

#### Scenario: Yoda's prose keeps "Hmm." sporadic and movie nods non-verbatim

- **GIVEN** Yoda's dedicated prose set
- **WHEN** it references "Hmm." and movie-adjacent nods
- **THEN** "Hmm." is described as sparing/occasional (not constant)
- **AND** movie nods are described as recontextualized/soft echoes, never verbatim or parody

### Requirement: voice-only change preserves Layer-1 boundaries and render contract

This change fills renderer-owned prose maps only. It MUST NOT alter the schema (`Profile`,
`Presentation`, `preset_v2.go`), the `yoda.yaml` preset values, `isBoundDialect` logic, or the
`renderPresentation` control flow (only the map literals and the `calm-teacher` switch arm
change). Layer-1 mentor-philosophy strings remain out of scope and MUST NOT appear in any newly
authored prose.

#### Scenario: No forbidden Layer-1 strings leak into rendered output

- **GIVEN** the built-in `yoda` preset rendered via `RenderLayer2` or `RenderOutputStyle`
- **WHEN** the rendered text is inspected
- **THEN** it does not contain the strings `"CONCEPTS > CODE"`, `"AI IS A TOOL"`, or
  `"Technical Behavior"`
- **AND** it does not contain the raw `display_name` field value from the YAML preset (only
  `toTitleCase(preset.Name)` is used for display)

#### Scenario: keep-coding-instructions preserved for Claude output-style rendering

- **GIVEN** the built-in `yoda` preset rendered via `RenderOutputStyle`
- **WHEN** the frontmatter is inspected
- **THEN** it contains `keep-coding-instructions: true` unchanged

#### Scenario: Claude and OpenCode rendering stay in parity

- **GIVEN** the built-in `yoda` preset
- **WHEN** rendered via `RenderLayer2` (OpenCode/plain path) and via `RenderOutputStyle` (Claude
  output-style path)
- **THEN** both renders share the identical `### Presentation` and `### Language Behavior`
  sections (byte-identical Presentation/Language Behavior bodies), differing only in the Claude
  output-style frontmatter block

## Out of Scope (explicitly not covered by this spec)

- Any change to `preset_v2.go`, `yoda.yaml`, or other built-in preset YAML files.
- Dialect-gating logic itself (`isBoundDialect`, `regionalLanguages`, `regionalPacks`) — only
  pinned as a regression, not modified.
- Raw-rendered presentation bullets that stay unmapped: `- Cadence:`, `- Emotional range:`,
  `- Verbosity:`, `- Formatting:`, `- Teaching metaphors:`, `- Examples:`.
- Other personas' dedicated prose (sargento, asturiano, galleguinho, argentino, etc.) — this
  change only authors the two SHARED entries (`dry`, `calm-teacher`) generically for their
  benefit; their own dedicated packs are out of scope.

## Test Strategy (informative — see tasks phase for concrete list)

Strict TDD: author new RED tests in `jarvis-cli/internal/persona/*_test.go` for each scenario
above (non-empty/non-raw-ID assertions for Yoda's five bullets and the Register bullet;
portability assertions reusing/extending the existing `isBoundDialect` coverage; generic-content
assertions for the shared `dry`/`calm-teacher` prose; forbidden-string and parity assertions),
then implement the minimum `loader.go` prose-map and register-arm changes to turn them GREEN.
Verify with `go test ./...` and `go vet ./...`.
