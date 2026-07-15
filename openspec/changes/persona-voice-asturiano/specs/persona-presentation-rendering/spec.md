# Delta for Persona Presentation Rendering

Builds on the foundations baseline (Exhaustive Prose Fallback, Dialect-Gating
Clause, Claude/OpenCode Parity). No schema or YAML change. Fills the 4
dedicated `asturian` prose keys; `presentationLanguage("es-asturian")` keeps
its readable label "Asturian" (no relabel), since the corrected foundations
dialect-gating clause now applies only when replying in Spanish. Humor `dry`
is out of scope (owned separately) and MUST NOT be asserted here.

## ADDED Requirements

### Requirement: Asturiano Dedicated Prose Bullets

For the `asturiano` builtin preset, rendering the vocabulary, phrase-pack,
and address-pack fields MUST resolve to authored prose registered under the
`asturian` key, not the raw enum ID `asturian`. The prose MUST read as
warm, Asturian-flavored Spanish (light bable lexicon, mining metaphors,
retranca) woven into clear Spanish, never as an obstacle to understanding.

#### Scenario: Vocabulary bullet renders authored prose

- GIVEN the `asturiano` preset is rendered
- WHEN the vocabulary field is inspected
- THEN it shows authored Asturian-flavored-Spanish prose, not the literal
  string `asturian`

#### Scenario: Phrase-pack bullet renders authored prose

- GIVEN the `asturiano` preset is rendered
- WHEN the phrase-pack field is inspected
- THEN it shows authored retranca/mining-metaphor prose, not the literal
  string `asturian`

#### Scenario: Address-pack bullet renders authored prose

- GIVEN the `asturiano` preset is rendered
- WHEN the address-pack field is inspected
- THEN it shows authored warm-peer address prose, not the literal string
  `asturian`

### Requirement: Asturiano Anti-Caricature Guardrail

Rendering the `asturiano` preset's anti-caricature field MUST resolve to
authored prose stating that Asturian warmth and retranca are seasoning, not
a costume, and MUST warn against regional-cliché performance while
affirming that a lively tone never replaces verification.

#### Scenario: Anti-caricature bullet renders authored guardrail

- GIVEN the `asturiano` preset is rendered
- WHEN the anti-caricature field is inspected
- THEN it shows authored prose forbidding regional-cliché performance and
  affirming verification is never replaced by tone

### Requirement: Asturiano Dialect-Gating Clause

The dialect-gating clause for the `asturiano` preset MUST name the dialect
layer "Asturian" and MUST gate activation on replying in Spanish, matching the
corrected foundations mechanism.

#### Scenario: Gating clause names the Asturian layer and gates on Spanish

- GIVEN the `asturiano` preset is rendered
- WHEN the dialect-gating clause is inspected
- THEN it contains the string "- Dialect gating: the Asturian dialect layer"
- AND it contains the string "applies only when replying in Spanish"

## MODIFIED Requirements

### Requirement: Exhaustive Prose Fallback

Prose maps for enumerated presentation fields MUST ship empty in this
change (no concrete voice prose). For every value in
`v2AllowedPresentationValues`, rendering MUST resolve to a non-empty
string: either mapped prose (added in a future change) or the raw enum
value as fallback. Rendered output MUST NEVER contain an empty value for
any selected presentation field. The `asturian` vocabulary, phrase-pack,
address-pack, and anti-caricature keys MUST resolve to authored prose
(not raw-ID fallback) as of this change; the `asturian` humor key remains
unmapped and continues to fall back to the raw ID.
(Previously: all prose maps shipped empty; every field, including
`asturian`, rendered via raw-ID fallback.)

#### Scenario: Unmapped enum value falls back to raw ID

- GIVEN a presentation field whose value has no prose map entry
- WHEN the field is rendered
- THEN the output shows the raw enum value, never an empty string

#### Scenario: All builtins render non-empty fields

- GIVEN each of the 7 builtin presets
- WHEN fully rendered
- THEN every selected presentation field has non-empty rendered text

#### Scenario: Asturiano humor bullet still falls back to raw ID

- GIVEN the `asturiano` preset is rendered
- WHEN the humor field is inspected
- THEN it shows the raw enum value `dry` (unmapped in this change), never
  an empty string, and no dedicated prose is asserted for it

## Voice-Only and Parity Invariants (unchanged, restated for pin)

- Layer 1 supremacy clause and forbidden-string invariants (per
  foundations spec) MUST continue to hold: no Asturiano prose bullet
  leaks into Layer 1, and no Layer 1 rule text appears in Asturiano's
  Layer 2 output.
- No `display_name` value MUST leak into rendered prose.
- `RenderLayer2` and `RenderOutputStyle` MUST render the 4 Asturiano
  bullets and the "applies only when replying in Spanish" gating clause
  identically (Claude/OpenCode parity), per the foundations Claude/OpenCode
  Parity requirement.
