# Delta for Persona Presentation Rendering

Builds on the foundations baseline. No schema/YAML/mechanism change; only the
5 prose-map entries (`rioplatense`, `warm`, `peer`, `plain`, `grounded`) gain
authored content. `argentino`'s portability classification (`bound`) and its
dialect-gating clause are inherited unchanged from the foundations spec.

## MODIFIED Requirements

### Requirement: Exhaustive Prose Fallback

Prose maps for enumerated presentation fields MUST resolve to a non-empty
string for every rendered field: mapped prose where an entry exists, else the
raw enum value. Rendered output MUST NEVER contain an empty value for any
selected presentation field. The 5 prose-map entries used by `argentino`
(`rioplatense`, `warm`, `peer`, `plain`, `grounded`) now have authored
mappings and MUST render that authored prose, not the raw enum ID.
(Previously: all 5 maps shipped empty; every field fell back to the raw
enum ID such as `- Humor: warm`.)

#### Scenario: Unmapped enum value falls back to raw ID

- GIVEN a presentation field whose value has no prose map entry
- WHEN the field is rendered
- THEN the output shows the raw enum value, never an empty string

#### Scenario: All builtins render non-empty fields

- GIVEN each of the 7 builtin presets
- WHEN fully rendered
- THEN every selected presentation field has non-empty rendered text

#### Scenario: Argentino's 5 fields render authored prose, not raw IDs

- GIVEN the `argentino` preset is rendered via `RenderLayer2` or
  `RenderOutputStyle`
- WHEN the vocabulary, humor, address, phrase, and anti-caricature bullets
  are inspected
- THEN each bullet shows authored prose text
- AND none of the bullets show the raw enum literal (`rioplatense`, `warm`,
  `peer`, `plain`, `grounded`) as the entire bullet value

### Requirement: Claude/OpenCode Parity

All four behaviors above MUST live in `renderPresentation` (shared) or
Layer 1 content, never in `RenderOutputStyle`-only frontmatter, so that
`RenderLayer2` (OpenCode path) and `RenderOutputStyle` (Claude path) stay
behaviorally identical for supremacy, reply-language, portability, and
prose content — including the newly authored `argentino` prose.
(Previously: parity covered prose-fallback content only in the abstract,
before any pack had authored prose.)

#### Scenario: Identical persona behavior across agents

- GIVEN the same preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the reply-language rule, portability clause, and prose content are
  compared
- THEN they are behaviorally identical (Layer 2 additionally omits the
  Layer 1-only supremacy clause and `keep-coding-instructions: true`
  frontmatter, which remains Claude-specific)

#### Scenario: Argentino prose matches across both render paths

- GIVEN the `argentino` preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the authored prose for vocabulary, humor, address, phrase, and
  anti-caricature is compared between the two outputs
- THEN the prose text is identical in both

## ADDED Requirements

### Requirement: Rioplatense Signature Prose

The `rioplatense` vocabulary pack MUST carry Argentina-specific voice
signature: voseo forms and Rioplatense lexicon as warm-mentor seasoning. The
authored prose MUST NOT contain the literal enum strings `es-rioplatense`,
`es-asturian`, or `es-galician`.

#### Scenario: Rioplatense prose carries the Argentine signature

- GIVEN the `argentino` preset is rendered
- WHEN the vocabulary bullet is inspected
- THEN it contains voseo and/or Rioplatense lexical markers

#### Scenario: Rioplatense prose omits raw dialect enum literals

- GIVEN the `argentino` preset is rendered
- WHEN the vocabulary bullet is inspected
- THEN it does not contain the strings `es-rioplatense`, `es-asturian`, or
  `es-galician`

### Requirement: Shared Pack Generic Neutrality

The `warm`, `peer`, `plain`, and `grounded` prose entries MUST be authored as
generic content with no Argentina-specific vocabulary, so any current or
future preset reusing these pack IDs inherits neutral, persona-agnostic
prose.

#### Scenario: Shared packs render generically

- GIVEN the `warm`, `peer`, `plain`, or `grounded` prose is rendered for any
  preset (including non-Argentine personas)
- WHEN the bullet text is inspected
- THEN it contains no Argentina-specific vocabulary or regional markers

### Requirement: Layer-2 Voice-Only Boundary

Authored Layer-2 prose MUST remain voice-only. It MUST NOT contain Layer-1
forbidden-invariant strings (e.g. "CONCEPTS > CODE", "AI IS A TOOL"), MUST
NOT leak the preset's internal `display_name`, and MUST NOT alter
`keep-coding-instructions: true` frontmatter behavior.

#### Scenario: Forbidden-string invariants hold after authoring

- GIVEN the `argentino` preset is rendered via either render path
- WHEN the Layer-2 presentation section is inspected
- THEN none of the Layer-1 forbidden strings or the `display_name` value
  appear

#### Scenario: Keep-coding-instructions frontmatter unaffected

- GIVEN the `argentino` preset is rendered via `RenderOutputStyle`
- WHEN the frontmatter is inspected
- THEN `keep-coding-instructions: true` is still present and unchanged
