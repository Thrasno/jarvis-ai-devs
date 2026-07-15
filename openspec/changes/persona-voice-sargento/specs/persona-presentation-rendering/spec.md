# Delta for Persona Presentation Rendering — Sargento Voice

Scope: `jarvis-cli/internal/persona/loader.go` (`renderPresentation`, `proseFor`, `presentationRegister`, `isBoundDialect`, prose maps). Pure ADDED — no existing Sargento rendering behavior to modify/remove.

## ADDED Requirements

### Requirement: Sargento's Dedicated Prose Packs Render Authored Voice

The system MUST render `vocabularyProse["military"]`, `phrasePackProse["sergeant"]`, and `addressPackProse["sergeant"]` as authored, non-raw prose when rendering the `sargento` preset via `renderPresentation`.

#### Scenario: Vocabulary, phrase, and address bullets render authored prose

- GIVEN the `sargento` preset with `Vocabulary: "military"`, `PhrasePack: "sergeant"`, `AddressPack: "sergeant"`
- WHEN `RenderLayer2` or `RenderOutputStyle` renders the preset
- THEN the "Vocabulary", "Phrase pack", and "Address pack" bullets each render authored prose distinct from the raw IDs `"military"`/`"sergeant"`
- AND none of the three bullets falls back to the raw enum ID via `proseFor`'s empty-map fallback

### Requirement: Mission-Briefing Register Renders Authored Prose

`presentationRegister` MUST render the `mission-briefing` register value as `"clipped, terse, and mission-focused"` instead of the raw ID.

#### Scenario: Mission-briefing register renders authored text

- GIVEN the `sargento` preset with `Register: "mission-briefing"`
- WHEN the preset is rendered
- THEN the "Register" bullet renders exactly `"clipped, terse, and mission-focused"`

### Requirement: Humor Bullet Stays Out of Scope on This Branch

The Humor bullet (`humorProse["dry"]`) MUST NOT be authored or asserted by this change. `humorProse` is owned by the Yoda change; on this isolated branch it MUST continue to render the raw fallback ID.

#### Scenario: Humor bullet renders raw and is not asserted

- GIVEN the `sargento` preset with `Humor: "dry"` on this isolated branch (Yoda's `humorProse["dry"]` not yet merged)
- WHEN the preset is rendered
- THEN the "Humor" bullet renders the raw ID `"dry"` via the `proseFor` fallback
- AND no test introduced by this change MUST assert the Humor bullet's content

### Requirement: Sargento Classifies Portable — No Dialect Gating

`isBoundDialect` MUST return `false` for the `sargento` preset's `Presentation`, and the rendered "Language Behavior" section MUST include only the portability affirmation clause, never a dialect-gating clause.

#### Scenario: Sargento renders portability-only language behavior

- GIVEN the `sargento` preset (`Language` not in `regionalLanguages`)
- WHEN the preset is rendered
- THEN `isBoundDialect` returns `false`
- AND "Language Behavior" contains the portability affirmation sentence
- AND "Language Behavior" MUST NOT contain a "Dialect gating:" line

### Requirement: Anti-Caricature Guardrail Forbids Disrespect While Allowing a Brusque Edge

`antiCaricatureProse["sergeant"]` MUST render prose that permits a gruff, terse delivery style while explicitly forbidding insults, humiliation, shouting down, or real disrespect toward the user, and MUST state that brevity/discipline never substitutes for verification.

#### Scenario: Anti-caricature bullet pins the disrespect boundary

- GIVEN the `sargento` preset with `AntiCaricature: "sergeant"`
- WHEN the preset is rendered
- THEN the "Anti-caricature" bullet renders authored prose (not the raw ID `"sergeant"`)
- AND the rendered prose conveys: gruff/terse delivery is style only; no insults, humiliation, shouting down, or real disrespect; discipline serves clarity/momentum, never intimidation; brevity never replaces verifying facts

### Requirement: Authored Packs Are Dedicated to Sargento

The 4 authored prose entries (`military`, `sergeant` × 3) MUST be dedicated keys not shared with, or overwritten by, any other persona's prose entries in the same maps.

#### Scenario: Keys are disjoint from other personas

- GIVEN the prose maps after this change is applied
- WHEN inspecting `vocabularyProse`, `phrasePackProse`, `addressPackProse`, `antiCaricatureProse`
- THEN the keys `"military"` and `"sergeant"` map only to Sargento's authored literals
- AND no other persona's key in those maps is altered by this change

### Requirement: Voice-Only Rendering — No Layer-1 Leak, Claude/OpenCode Parity

Authored Sargento prose MUST NOT restate Layer-1 mentor content and MUST NOT leak `display_name` into voice bullets. `RenderOutputStyle` MUST preserve `keep-coding-instructions: true`. `RenderLayer2` and `RenderOutputStyle` MUST render identical "Presentation" and "Language Behavior" bodies for `sargento`.

#### Scenario: No Layer-1 leak and Claude/OpenCode parity hold

- GIVEN the `sargento` preset
- WHEN rendering via `RenderLayer2` and via `RenderOutputStyle`
- THEN neither authored bullet contains Layer-1 strings (e.g. "CONCEPTS > CODE", "AI IS A TOOL") or the raw `display_name` value
- AND `RenderOutputStyle`'s output includes `keep-coding-instructions: true`
- AND the "Presentation" and "Language Behavior" sections are identical between both render paths (only the output-style front matter differs)
