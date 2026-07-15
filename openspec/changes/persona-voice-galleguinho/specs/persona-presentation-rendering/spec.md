# Delta for Persona Presentation Rendering

Builds on the baseline delta in `persona-voice-foundations` (Exhaustive Prose
Fallback, Dialect-Gating and Portability-Affirmation Clauses, Claude/OpenCode
Parity). This change ADDS the Galleguinho-specific voice fill; it does not
alter the generic mechanism.

## ADDED Requirements

### Requirement: Galleguinho Dedicated Prose Fill

For the `galleguinho` builtin preset, the renderer MUST resolve authored
prose (never the raw enum ID) for exactly these five dedicated map entries:
`humorProse["retranca"]`, `vocabularyProse["galician"]`,
`phrasePackProse["galician"]`, `addressPackProse["galician"]`, and
`antiCaricatureProse["galician"]`. The Register field (`presentationRegister`
for `calm-teacher`) is OUT OF SCOPE for this change and MUST NOT be asserted
by tests introduced here; it MAY continue to render raw until a separate
change fills it.

#### Scenario: Five dedicated bullets render authored prose

- GIVEN the `galleguinho` preset is rendered via `RenderLayer2` or
  `RenderOutputStyle`
- WHEN the humor, vocabulary, phrase-pack, address-pack, and anti-caricature
  fields are inspected
- THEN each shows authored prose text, not the raw ID (`retranca` or
  `galician`)

#### Scenario: Register bullet is not asserted

- GIVEN the `galleguinho` preset is rendered
- WHEN a test inspects the Register field
- THEN no assertion is made about its content in this change's test suite

### Requirement: Galician-Spanish Dialect Label

The `presentationLanguage` value `es-galician` MUST render its human-readable
language name as "Galician Spanish", not "Galician". This is the only
language-name change made by this change; `es-rioplatense` and `es-asturian`
labels are unaffected.

#### Scenario: Bound dialect-gating clause uses relabeled name

- GIVEN the `galleguinho` preset is rendered (bound: `es-galician` + galician
  pack)
- WHEN the dialect-gating clause is inspected
- THEN it names the native variant "Galician Spanish"

### Requirement: Retranca Anti-Caricature Guardrail

The `galleguinho` anti-caricature prose MUST state that Galician warmth and
retranca are seasoning, not a costume: light galego touches, a wry aside, or
a Camino/sea metaphor are welcome, but stacking meigas/rain/postcard clichés
or performing a caricature Galicia MUST NOT occur. It MUST also state that
retranca MUST NOT leave an answer ambiguous where the user needs clarity, and
that a wry tone MUST NOT replace verifying facts and doing the work
correctly.

#### Scenario: Anti-caricature prose states both guardrails

- GIVEN the `galleguinho` preset is rendered
- WHEN the anti-caricature field is inspected
- THEN it warns against caricature/cliché Galicia
- AND it states retranca never leaves an answer ambiguous
- AND it states wry tone never replaces verification

## MODIFIED Requirements

### Requirement: Claude/OpenCode Parity

All four behaviors above MUST live in `renderPresentation` (shared) or
Layer 1 content, never in `RenderOutputStyle`-only frontmatter, so that
`RenderLayer2` (OpenCode path) and `RenderOutputStyle` (Claude path) stay
behaviorally identical for supremacy, reply-language, portability, and
prose-fallback content. This additionally covers the Galleguinho dedicated
prose fill and the Galician-Spanish dialect label: both MUST render
identically across `RenderLayer2` and `RenderOutputStyle`, with no
`display_name` leak and no forbidden-string invariant violated on either
path.
(Previously: parity scope covered only the four foundations-era behaviors;
now explicitly extended to the Galleguinho dedicated fill and label.)

#### Scenario: Identical persona behavior across agents

- GIVEN the same preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the reply-language rule, portability clause, and prose fallback are
  compared
- THEN they are behaviorally identical (Layer 2 additionally omits the
  Layer 1-only supremacy clause and `keep-coding-instructions: true`
  frontmatter, which remains Claude-specific)

#### Scenario: Galleguinho voice parity across agents

- GIVEN the `galleguinho` preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the five dedicated bullets and the dialect-gating clause are compared
- THEN the authored prose and the "Galician Spanish" label are identical on
  both paths
