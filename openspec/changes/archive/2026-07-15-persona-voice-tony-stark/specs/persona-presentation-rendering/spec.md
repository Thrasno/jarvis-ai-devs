# Delta for Persona Presentation Rendering

## ADDED Requirements

### Requirement: Tony Stark dedicated prose renders authored text

`renderPresentation` MUST resolve Tony Stark's 5 dedicated presentation values
— `vocabulary=engineering`, `humor=witty`, `phrase_pack=engineer`,
`address_pack=engineer`, `anti_caricature=engineer` — to authored,
human-readable prose via `proseFor`, never falling back to the raw enum ID.

#### Scenario: Tony's dedicated packs render prose, not raw IDs

- GIVEN the `tony-stark` built-in preset with `vocabulary=engineering`,
  `humor=witty`, `phrase_pack=engineer`, `address_pack=engineer`,
  `anti_caricature=engineer`
- WHEN `renderPresentation` renders the profile (Claude output-style or
  OpenCode Layer 2)
- THEN the "Vocabulary", "Humor", "Address pack", "Phrase pack", and
  "Anti-caricature" bullets each contain non-empty prose distinct from the
  raw enum ID (`"engineering"`, `"witty"`, `"engineer"`)
- AND none of those 5 bullets render the bare raw ID as their entire value

### Requirement: `fast-witty` register renders prose

`presentationRegister` MUST resolve the `fast-witty` register to readable
prose, matching the existing pattern used for `warm-direct`, instead of
returning the raw token.

#### Scenario: fast-witty register renders as prose

- GIVEN a profile with `register=fast-witty`
- WHEN `renderPresentation` renders the "Register" bullet
- THEN the bullet contains authored prose, not the literal string
  `"fast-witty"`

### Requirement: Tony Stark classifies as portable, no dialect gating

Tony Stark's presentation (`language=en-us`, non-regional packs) MUST
classify as portable under `isBoundDialect`, so the render includes only the
Portability affirmation clause and MUST NOT include a Dialect gating clause.

#### Scenario: Tony renders portability-only Language Behavior section

- GIVEN the `tony-stark` built-in preset
- WHEN `renderPresentation` renders the "Language Behavior" section
- THEN `isBoundDialect(p)` returns `false`
- AND the output contains the Portability affirmation sentence
- AND the output does NOT contain a "Dialect gating:" line

### Requirement: Confidence-vs-verification prose never encodes false certainty

Tony's authored prose (vocabulary, humor, phrase pack, address pack) MUST
convey a confident, witty, technically sharp voice that is explicitly humble
on unverified facts — confidence is presented as a stylistic trait, never as
license to assert without verification.

#### Scenario: Confident-engineer prose stays verification-humble

- GIVEN Tony's rendered `vocabularyProse["engineering"]`,
  `humorProse["witty"]`, `phrasePackProse["engineer"]`, and
  `addressPackProse["engineer"]` bullets
- WHEN inspecting their content
- THEN at least one of these bullets communicates that confidence/wit is a
  delivery style and does not substitute for verifying facts before
  asserting them
- AND none of the bullets state or imply that Tony asserts unverified claims
  as fact

### Requirement: Wit targets the problem, never the user

Tony's authored humor and phrase-pack prose MUST direct wit and ribbing at
the technical problem, code, or situation, and MUST NOT direct it at the
user in a demeaning or teasing way.

#### Scenario: Humor prose is problem-directed

- GIVEN Tony's rendered `humorProse["witty"]` and `phrasePackProse["engineer"]`
  bullets
- WHEN inspecting their content
- THEN the prose describes wit aimed at the problem/code/situation
- AND the prose does not describe wit aimed at, or at the expense of, the
  user

### Requirement: Character nods stay soft and recontextualized

Any Tony Stark character-flavor references in the authored prose MUST be
soft, recontextualized nods to the real technical situation, and MUST NOT be
verbatim movie quotes or parody.

#### Scenario: No verbatim or parody character quoting

- GIVEN Tony's authored dedicated prose bullets
- WHEN inspecting their content for character references
- THEN any character nod is paraphrased/recontextualized to the technical
  situation at hand
- AND no bullet reproduces a verbatim movie line or plays the character for
  parody

### Requirement: Anti-caricature guardrail forbids arrogance and skipped verification

`antiCaricatureProse["engineer"]` MUST explicitly guard against wit/confidence
degrading into arrogance, false certainty, skipped verification, or
condescension toward the user.

#### Scenario: Anti-caricature bullet blocks all four failure modes

- GIVEN Tony's rendered "Anti-caricature" bullet
  (`antiCaricatureProse["engineer"]`)
- WHEN inspecting its content
- THEN it explicitly forbids arrogance
- AND it explicitly forbids false/unverified certainty
- AND it explicitly forbids skipping verification before asserting
- AND it explicitly forbids condescension toward the user

### Requirement: Tony's dedicated prose stays voice-only, no Layer-1 leak

Tony's authored prose MUST NOT restate Layer-1 mentor philosophy or
supremacy-rule wording, and MUST NOT leak the internal `display_name` field
into the presentation output. `keep-coding-instructions: true` MUST remain
present in the Claude output-style frontmatter.

#### Scenario: No forbidden Layer-1 strings in Tony's render

- GIVEN Tony's rendered presentation (Claude output-style and OpenCode
  Layer 2)
- WHEN scanning the full rendered text
- THEN it does not contain Layer-1 philosophy strings (e.g. "CONCEPTS >
  CODE", "AI IS A TOOL", "Technical Behavior")
- AND it does not contain the raw `display_name` value
- AND the Claude output-style variant still contains
  `keep-coding-instructions: true`

### Requirement: Claude and OpenCode parity for Tony's render

`RenderOutputStyle` (Claude) and `RenderLayer2` (OpenCode) MUST render
identical Presentation and Language Behavior bodies for the `tony-stark`
profile, differing only in the Claude-specific frontmatter block.

#### Scenario: Presentation and Language Behavior bodies match across targets

- GIVEN the `tony-stark` built-in preset
- WHEN rendering via `RenderOutputStyle` and via `RenderLayer2`
- THEN the "### Presentation" and "### Language Behavior" sections are
  identical text in both renders
- AND only `RenderOutputStyle`'s output includes the leading Claude
  frontmatter block
