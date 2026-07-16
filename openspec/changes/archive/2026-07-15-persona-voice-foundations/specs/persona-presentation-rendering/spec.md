# Delta for Persona Presentation Rendering

No prior spec exists for this domain; requirements below are ADDED as the
first pinned baseline for `renderPresentation` (shared by `RenderLayer2` and
`RenderOutputStyle`) and the Layer 1 contract (`technical-contract.md`).

## ADDED Requirements

### Requirement: Contract Supremacy Clause

Layer 1 render output MUST include an ABSOLUTE-precedence clause that
enumerates, by name, the protected rules: verify before asserting;
distinguish confirmed facts from assumptions; ask one question then stop.
The clause MUST state that voice/persona styling affects delivery only and
MUST NOT be interpreted as overriding these rules. Layer 2 presentation
output (both `RenderLayer2` and the presentation section of
`RenderOutputStyle`) MUST NOT contain this clause or its enumerated rule
text (forbidden-string invariant).

#### Scenario: Supremacy clause renders in Layer 1

- GIVEN Layer 1 technical-contract content is rendered for any agent
- WHEN the rendered output is inspected
- THEN it contains an absolute-precedence statement naming verification,
  fact-vs-assumption distinction, and the one-question-then-stop rule

#### Scenario: Supremacy clause absent from Layer 2

- GIVEN a preset is rendered via `RenderLayer2` or `RenderOutputStyle`
- WHEN the presentation section is inspected
- THEN none of the Layer 1 supremacy-clause strings appear

### Requirement: Authoritative Reply-Language Rule

The system MUST render a single, authoritative rule stating that replies
always follow the user's language and that persona voice never forces a
different reply language. The legacy imperative bullet (`- Language: <X>`)
MUST NOT appear in rendered output. The rule MUST be single-sourced in
Layer 1 and MUST NOT duplicate or contradict the SDD orchestrator preflight
reply-language rule.

#### Scenario: Reply-language rule renders, imperative removed

- GIVEN any preset is rendered
- WHEN the output is inspected
- THEN it contains the authoritative reply-follows-user-language rule
- AND it does NOT contain a `- Language: <value>` bullet

#### Scenario: No contradiction with orchestrator preflight

- GIVEN both the Layer 1 rule and the SDD orchestrator preflight rule exist
- WHEN both are compared
- THEN they express the same reply-language behavior without conflicting
  wording

### Requirement: Persona Portability Classification

The renderer MUST classify each preset as `bound` or `portable` from the
in-memory `Presentation` struct only, without any schema or YAML change.
Presets combining a regional Spanish language value (`es-rioplatense`,
`es-asturian`, `es-galician`) with a regional pack MUST classify as
`bound`. All other valid presentation combinations MUST classify as
`portable`.

#### Scenario: Regional builtins classify bound

- GIVEN the `argentino`, `asturiano`, or `galleguinho` builtin preset
- WHEN portability is derived
- THEN the preset classifies as `bound`

#### Scenario: Neutral and en-us builtins classify portable

- GIVEN the `neutra`, `yoda`, `sargento`, or `tony-stark` builtin preset
- WHEN portability is derived
- THEN the preset classifies as `portable`

### Requirement: Dialect-Gating and Portability-Affirmation Clauses

For a `bound` preset, the render MUST include a dialect-gating clause
stating the dialect layer (regional markers such as voseo/lunfardo,
retranca, Asturian markers) is active only in that persona's native Spanish
variant, and that outside that variant, ONLY dialect markers are dropped
while register and mentor soul are KEPT. For a `portable` preset, the
render MUST include an affirmation clause stating the persona's character
applies in any reply language.

#### Scenario: Bound preset renders dialect-gating clause

- GIVEN the `argentino` preset is rendered
- WHEN the output is inspected
- THEN it states the dialect layer applies only in `es-rioplatense` replies
- AND it states that outside that variant, register and mentor soul are
  kept while only dialect markers are dropped

#### Scenario: Portable preset renders affirmation clause

- GIVEN the `yoda` preset is rendered
- WHEN the output is inspected
- THEN it states the persona's character applies regardless of reply
  language

### Requirement: Exhaustive Prose Fallback

Prose maps for enumerated presentation fields MUST ship empty in this
change (no concrete voice prose). For every value in
`v2AllowedPresentationValues`, rendering MUST resolve to a non-empty
string: either mapped prose (added in a future change) or the raw enum
value as fallback. Rendered output MUST NEVER contain an empty value for
any selected presentation field.

#### Scenario: Unmapped enum value falls back to raw ID

- GIVEN a presentation field whose value has no prose map entry
- WHEN the field is rendered
- THEN the output shows the raw enum value, never an empty string

#### Scenario: All builtins render non-empty fields

- GIVEN each of the 7 builtin presets
- WHEN fully rendered
- THEN every selected presentation field has non-empty rendered text

### Requirement: Claude/OpenCode Parity

All four behaviors above MUST live in `renderPresentation` (shared) or
Layer 1 content, never in `RenderOutputStyle`-only frontmatter, so that
`RenderLayer2` (OpenCode path) and `RenderOutputStyle` (Claude path) stay
behaviorally identical for supremacy, reply-language, portability, and
prose-fallback content.

#### Scenario: Identical persona behavior across agents

- GIVEN the same preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the reply-language rule, portability clause, and prose fallback are
  compared
- THEN they are behaviorally identical (Layer 2 additionally omits the
  Layer 1-only supremacy clause and `keep-coding-instructions: true`
  frontmatter, which remains Claude-specific)
