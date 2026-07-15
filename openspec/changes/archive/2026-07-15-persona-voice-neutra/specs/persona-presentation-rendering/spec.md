# Delta for Persona Presentation Rendering

Builds on the Change 1 baseline (`renderPresentation`, `proseFor`, empty
prose maps, portability classification). No main spec exists yet in
`openspec/specs/` (Change 1 unarchived), so these are ADDED requirements
scoped to the `neutra` preset only.

## ADDED Requirements

### Requirement: Neutra Prose Renders Authored Text

For the `neutra` preset, rendering MUST resolve the following presentation
bullets to authored prose (via `proseFor`), never the raw enum token:
Vocabulary (`neutral-spanish`), Humor (`none`), Address pack (`neutral`),
Phrase pack (`neutral`), Anti-caricature (`neutral`).

#### Scenario: All five Neutra bullets render prose

- GIVEN the `neutra` preset is rendered via `RenderLayer2` or
  `RenderOutputStyle`
- WHEN the Vocabulary, Humor, Address pack, Phrase pack, and Anti-caricature
  bullets are inspected
- THEN each shows authored prose text
- AND none shows the raw token (`neutral-spanish`, `none`, or `neutral`)

### Requirement: Register Stays Raw for Neutra

The `friendly-professional` register value MUST continue to render as the
raw token (`- Register: friendly-professional`). This change MUST NOT add a
register arm or otherwise modify `presentationRegister`.

#### Scenario: Neutra register renders raw

- GIVEN the `neutra` preset is rendered
- WHEN the Register bullet is inspected
- THEN it reads exactly `- Register: friendly-professional`

#### Scenario: Shared fixture assertions unaffected

- GIVEN the `validPresetV2` fixture used by `bridge_test.go`
- WHEN its existing register assertions are run
- THEN they pass unchanged

### Requirement: Neutra Portability Without Dialect Gating

The `neutra` preset MUST classify as `portable` and its render MUST include
only the portability-affirmation clause. It MUST NOT include a
dialect-gating clause.

#### Scenario: Neutra renders affirmation, no gating

- GIVEN the `neutra` preset is rendered
- WHEN the Language Behavior section is inspected
- THEN it contains the portability-affirmation clause
- AND it does NOT contain a dialect-gating clause

### Requirement: Neutra Prose Stays Genuinely Generic

Authored Neutra prose MUST read as plain, professional, and generic — no
regional vocabulary, no character/persona flavor, no dialect markers. This
is the inherited fallback voice for default and custom personas, so any
flavor would contaminate that baseline.

#### Scenario: Neutra prose contains no regional or character markers

- GIVEN the five authored Neutra prose bullets
- WHEN their text is inspected
- THEN none contains regional vocabulary, slang, or persona-specific voice
  markers

### Requirement: Neutra Anti-Caricature Guardrail

The Neutra anti-caricature prose MUST state that the voice stays genuinely
neutral and professional, never adopts a regional or theatrical voice, that
clarity comes first, and that tone never replaces verifying facts and doing
the work correctly.

#### Scenario: Anti-caricature bullet states the guardrail

- GIVEN the `neutra` preset is rendered
- WHEN the Anti-caricature bullet is inspected
- THEN it states the voice stays neutral/professional, never regional or
  theatrical, with clarity first and tone never substituting for
  verification

### Requirement: Neutra Voice-Only and Cross-Agent Parity

Neutra prose additions MUST NOT restate or duplicate any Layer 1
technical-contract text (mentor rules, supremacy clause, verification
rules). Rendered Neutra output MUST NOT leak the preset's YAML
`display_name` field. `RenderLayer2` and `RenderOutputStyle` MUST render
identical Neutra presentation content (Claude/OpenCode parity), and
`RenderOutputStyle` frontmatter MUST still declare
`keep-coding-instructions: true`.

#### Scenario: Neutra prose contains no Layer 1 text

- GIVEN the `neutra` preset presentation section is rendered
- WHEN it is inspected for Layer 1 forbidden strings (mentor rules,
  supremacy clause language)
- THEN none of those strings appear

#### Scenario: No display_name leak

- GIVEN the `neutra` preset is rendered
- WHEN the output is inspected
- THEN the YAML `display_name` field value does not appear verbatim

#### Scenario: Claude and OpenCode parity for Neutra

- GIVEN the `neutra` preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the presentation sections are compared
- THEN the five prose bullets, register, and language-behavior clauses are
  identical
- AND `RenderOutputStyle` additionally includes
  `keep-coding-instructions: true` frontmatter
