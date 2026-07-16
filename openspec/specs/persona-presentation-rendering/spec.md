# Persona Presentation Rendering Specification

## Purpose

Defines the behavior of `renderPresentation` (shared by `RenderLayer2` for
OpenCode and `RenderOutputStyle` for Claude output styles) and the Layer 1
technical contract (`embed/technical-contract.md`) for rendering a persona's
presentation/voice. Covers the shared rendering mechanism (supremacy clause,
reply-language rule, portability/dialect-gating, prose fallback) plus the
authored voice content for each of the 7 built-in presets
(`neutra`, `argentino`, `yoda`, `tony-stark`, `sargento`, `asturiano`,
`galleguinho`).

Implementation lives in `jarvis-cli/internal/persona/loader.go`
(`renderPresentation`, `proseFor`, `presentationRegister`, `isBoundDialect`,
`presentationLanguage`, and the 5 dedicated prose maps: `vocabularyProse`,
`humorProse`, `phrasePackProse`, `addressPackProse`, `antiCaricatureProse`).

## Requirements

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
in-memory `Presentation` struct only, without any schema or YAML change,
via `isBoundDialect`. Classification is `true` (bound) only when the
preset's `Language` is one of the explicit regional Spanish values
(`es-rioplatense`, `es-asturian`, `es-galician`) AND the preset also selects
the matching regional pack (via `Vocabulary`, `PhrasePack`, or
`AddressPack`) for that same language — an explicit language-to-pack
pairing, not an OR across independent sets. All other valid presentation
combinations MUST classify as `portable`.

#### Scenario: Regional builtins classify bound

- GIVEN the `argentino`, `asturiano`, or `galleguinho` builtin preset
- WHEN portability is derived
- THEN the preset classifies as `bound`

#### Scenario: Neutral and en-us builtins classify portable

- GIVEN the `neutra`, `yoda`, `sargento`, or `tony-stark` builtin preset
- WHEN portability is derived
- THEN the preset classifies as `portable`

### Requirement: Dialect-Gating and Portability-Affirmation Clauses

For a `bound` preset, the render MUST include a `### Language Behavior`
dialect-gating clause naming the persona's native regional variant using
its readable label (`Rioplatense (voseo)`, `Asturian`, or `Galician`) and
stating that the dialect layer — regional markers such as voseo/lunfardo,
retranca, Asturian bable — applies only when replying in Spanish. For any
other reply language, ONLY dialect markers are dropped while register and
Layer 1 mentor approach are KEPT. For a `portable` preset, the render MUST
include a portability-affirmation clause stating the persona's character
and register apply in whatever language the user writes, with no
dialect-gating clause present.

#### Scenario: Bound preset renders dialect-gating clause gated on Spanish

- GIVEN the `argentino`, `asturiano`, or `galleguinho` preset is rendered
- WHEN the "Language Behavior" section is inspected
- THEN it contains a "Dialect gating:" line naming the persona's native
  variant (`Rioplatense (voseo)`, `Asturian`, or `Galician` respectively)
- AND it states the dialect layer applies only when replying in Spanish
- AND it states that outside that condition, register and mentor soul are
  kept while only dialect markers are dropped

#### Scenario: Portable preset renders affirmation clause only

- GIVEN the `neutra`, `yoda`, `sargento`, or `tony-stark` preset is rendered
- WHEN the "Language Behavior" section is inspected
- THEN it contains the Portability affirmation clause
- AND it does NOT contain a "Dialect gating:" line

### Requirement: Exhaustive Prose Fallback

For every value in `v2AllowedPresentationValues`, rendering MUST resolve to
a non-empty string via `proseFor`: authored prose where a dedicated map
entry exists, otherwise the raw enum value as fallback. Rendered output
MUST NEVER contain an empty value for any selected presentation field. All
7 built-in presets' selected presentation values (vocabulary, humor, phrase
pack, address pack, anti-caricature, and the `warm-direct`/`calm-teacher`/
`fast-witty`/`mission-briefing` register arms) now resolve to authored
prose; any other, currently-unmapped enum value (e.g. a custom preset
selecting an unauthored combination) continues to fall back to the raw
enum ID.

#### Scenario: Unmapped enum value falls back to raw ID

- GIVEN a presentation field whose value has no prose map entry
- WHEN the field is rendered
- THEN the output shows the raw enum value, never an empty string

#### Scenario: All 7 builtins render fully authored, non-empty fields

- GIVEN each of the 7 builtin presets (`neutra`, `argentino`, `yoda`,
  `tony-stark`, `sargento`, `asturiano`, `galleguinho`)
- WHEN fully rendered via `RenderLayer2` or `RenderOutputStyle`
- THEN every selected presentation field renders authored, non-raw prose
  text (register bullet included for the personas that authored a register
  arm)

### Requirement: Claude/OpenCode Parity

All rendering behaviors above — supremacy clause exclusion, reply-language
rule, portability/dialect-gating clauses, and every persona's authored
prose — MUST live in `renderPresentation` (shared) or Layer 1 content,
never in `RenderOutputStyle`-only frontmatter, so that `RenderLayer2`
(OpenCode path) and `RenderOutputStyle` (Claude path) render byte-identical
`### Presentation` and `### Language Behavior` bodies for every preset,
differing only in the Claude-specific frontmatter block (which retains
`keep-coding-instructions: true`).

#### Scenario: Identical persona behavior across agents

- GIVEN any built-in preset rendered via `RenderLayer2` and via
  `RenderOutputStyle`
- WHEN the reply-language rule, portability/dialect-gating clause, and
  authored prose content are compared
- THEN they are behaviorally identical (Layer 2 additionally omits the
  Layer 1-only supremacy clause and the Claude-only frontmatter block)

### Requirement: Neutra Dedicated Voice (portable baseline)

The `neutra` preset MUST render authored, non-raw prose for its 5
dedicated presentation values (`vocabulary=neutral-spanish`, `humor=none`,
`phrase_pack=neutral`, `address_pack=neutral`, `anti_caricature=neutral`).
Its `friendly-professional` register value remains an intentionally raw,
unauthored fallback. The authored prose MUST read as plain, professional,
and generic — no regional vocabulary, no character/persona flavor, no
dialect markers — since Neutra is the inherited fallback voice for default
and custom personas. Its anti-caricature prose MUST state that the voice
stays neutral/professional, clarity comes first, and tone never replaces
verification. Neutra classifies `portable` (no dialect-gating clause).

#### Scenario: Neutra's five dedicated bullets render authored, generic prose

- GIVEN the `neutra` preset is rendered
- WHEN the Vocabulary, Humor, Address pack, Phrase pack, and Anti-caricature
  bullets are inspected
- THEN each shows authored prose distinct from the raw token
- AND none contains regional vocabulary, slang, or persona-specific voice
  markers

#### Scenario: Neutra register stays raw

- GIVEN the `neutra` preset is rendered
- WHEN the Register bullet is inspected
- THEN it reads exactly `- Register: friendly-professional`

### Requirement: Argentino Dedicated Voice (bound, Rioplatense)

The `argentino` preset MUST render authored, non-raw prose for its 5
dedicated presentation values keyed `rioplatense` (vocabulary), `warm`
(humor), `peer` (address pack), `plain` (phrase pack), and `grounded`
(anti-caricature). The `rioplatense` vocabulary entry MUST carry an
Argentina-specific voice signature (voseo forms and Rioplatense lexicon as
warm-mentor seasoning) and MUST NOT contain the literal enum strings
`es-rioplatense`, `es-asturian`, or `es-galician`. The `warm`, `peer`,
`plain`, and `grounded` entries are SHARED, generic packs (also inherited
by other personas selecting the same pack IDs) and MUST carry no
Argentina-specific vocabulary. Argentino classifies `bound` and renders the
dialect-gating clause naming the native variant `Rioplatense (voseo)`.

#### Scenario: Argentino's 5 fields render authored prose, not raw IDs

- GIVEN the `argentino` preset is rendered via `RenderLayer2` or
  `RenderOutputStyle`
- WHEN the vocabulary, humor, address, phrase, and anti-caricature bullets
  are inspected
- THEN each bullet shows authored prose text
- AND none of the bullets show the raw enum literal (`rioplatense`, `warm`,
  `peer`, `plain`, `grounded`) as the entire bullet value

#### Scenario: Rioplatense prose omits raw dialect enum literals

- GIVEN the `argentino` preset is rendered
- WHEN the vocabulary bullet is inspected
- THEN it contains voseo and/or Rioplatense lexical markers
- AND it does not contain the strings `es-rioplatense`, `es-asturian`, or
  `es-galician`

#### Scenario: Shared packs render generically for any persona

- GIVEN the `warm`, `peer`, `plain`, or `grounded` prose is rendered for any
  preset (including non-Argentine personas)
- WHEN the bullet text is inspected
- THEN it contains no Argentina-specific vocabulary or regional markers

### Requirement: Yoda Dedicated Voice (portable)

The `yoda` preset MUST render authored, non-raw prose for its 4 dedicated
presentation values (`vocabulary=yoda`, `phrase_pack=yoda`,
`address_pack=yoda`, `anti_caricature=yoda`) and MUST resolve its
`calm-teacher` register arm to `"calm, patient, and reassuring"`. Yoda's
dedicated prose MUST communicate: clause inversion as a strong but
clarity-capped stylistic signature (dropped whenever it would bury the
technical point); "Hmm." as a sporadic, non-constant thinking beat; any
movie/character nods as soft, recontextualized echoes (never verbatim
quotation or parody); roots-and-patience metaphor imagery; and an
anti-caricature guardrail stating clarity beats mysticism and calm must
never become vagueness or false certainty. Yoda's `language=es-neutral` is
absent from the regional-language set, so it classifies `portable`
(`isBoundDialect(yoda) == false`) and renders only the portability
affirmation clause.

#### Scenario: Yoda's dedicated bullets and register render authored prose

- GIVEN the built-in `yoda` preset
- WHEN `RenderLayer2` or `RenderOutputStyle` renders the `### Presentation`
  section
- THEN the Vocabulary, Phrase pack, Address pack, Anti-caricature, and
  Register bullets each contain authored prose, not the raw enum ID
  (`yoda` or `calm-teacher`)

#### Scenario: Yoda's prose caps clause inversion with a clarity rule

- GIVEN Yoda's dedicated prose set
- WHEN the prose describing clause inversion is read
- THEN it explicitly pairs the inversion signature with a clarity/teaching
  cap — it does not present inversion as unconditional
- AND "Hmm." is described as sparing/occasional, and movie nods as
  recontextualized/soft echoes, never verbatim or parody

#### Scenario: Yoda renders as portable

- GIVEN the built-in `yoda` preset
- WHEN its presentation is rendered
- THEN the "Language Behavior" section contains the Portability
  affirmation line and does NOT contain a "Dialect gating:" line

### Requirement: Tony Stark Dedicated Voice (portable)

The `tony-stark` preset MUST render authored, non-raw prose for its 5
dedicated presentation values (`vocabulary=engineering`, `humor=witty`,
`phrase_pack=engineer`, `address_pack=engineer`, `anti_caricature=engineer`)
and MUST resolve its `fast-witty` register arm to
`"fast, witty, and confident"`. The authored prose MUST convey a confident,
witty, technically sharp voice that stays explicitly humble on unverified
facts (confidence is a delivery style, never license to assert without
verification); wit MUST target the technical problem/code, never the user;
any character-flavor nods MUST be soft, recontextualized, never verbatim
movie quotes or parody; and `antiCaricatureProse["engineer"]` MUST
explicitly forbid arrogance, false/unverified certainty, skipped
verification, AND condescension toward the user. Tony Stark's
`language=en-us` with non-regional packs classifies `portable` (no
dialect-gating clause).

#### Scenario: Tony's dedicated packs and register render authored prose

- GIVEN the `tony-stark` built-in preset
- WHEN `renderPresentation` renders the profile
- THEN the Vocabulary, Humor, Address pack, Phrase pack, Anti-caricature,
  and Register bullets each contain non-empty prose distinct from the raw
  enum ID (`engineering`, `witty`, `engineer`, `fast-witty`)

#### Scenario: Anti-caricature bullet blocks all four failure modes

- GIVEN Tony's rendered "Anti-caricature" bullet
- WHEN inspecting its content
- THEN it explicitly forbids arrogance, false/unverified certainty, skipping
  verification before asserting, AND condescension toward the user

#### Scenario: Tony renders portability-only Language Behavior section

- GIVEN the `tony-stark` built-in preset
- WHEN `renderPresentation` renders the "Language Behavior" section
- THEN `isBoundDialect(p)` returns `false`
- AND the output contains only the Portability affirmation sentence

### Requirement: Sargento Dedicated Voice (portable)

The `sargento` preset MUST render authored, non-raw prose for its 3
dedicated presentation values (`vocabulary=military`, `phrase_pack=sergeant`,
`address_pack=sergeant`) and MUST resolve its `mission-briefing` register
arm to `"clipped, terse, and mission-focused"`. `antiCaricatureProse["sergeant"]`
MUST permit a gruff, terse delivery style while explicitly forbidding
insults, humiliation, shouting down, or real disrespect toward the user,
and MUST state brevity/discipline never substitutes for verification. The
Humor bullet (shared `humorProse["dry"]`, owned by the Yoda voice fill)
falls back to the raw ID whenever unmapped for a given branch state but
resolves to the shared authored `dry` prose once merged. Sargento's
`language=es-neutral` classifies `portable` (no dialect-gating clause).

#### Scenario: Vocabulary, phrase, and address bullets render authored prose

- GIVEN the `sargento` preset with `Vocabulary: "military"`,
  `PhrasePack: "sergeant"`, `AddressPack: "sergeant"`
- WHEN `RenderLayer2` or `RenderOutputStyle` renders the preset
- THEN the Vocabulary, Phrase pack, and Address pack bullets each render
  authored prose distinct from the raw IDs `"military"`/`"sergeant"`

#### Scenario: Mission-briefing register renders authored text

- GIVEN the `sargento` preset with `Register: "mission-briefing"`
- WHEN the preset is rendered
- THEN the Register bullet renders exactly
  `"clipped, terse, and mission-focused"`

#### Scenario: Anti-caricature bullet pins the disrespect boundary

- GIVEN the `sargento` preset with `AntiCaricature: "sergeant"`
- WHEN the preset is rendered
- THEN the rendered prose conveys: gruff/terse delivery is style only; no
  insults, humiliation, shouting down, or real disrespect; brevity never
  replaces verifying facts

#### Scenario: Sargento renders portability-only language behavior

- GIVEN the `sargento` preset (`Language` not in the regional-language set)
- WHEN the preset is rendered
- THEN `isBoundDialect` returns `false`
- AND "Language Behavior" contains only the portability affirmation
  sentence

### Requirement: Asturiano Dedicated Voice (bound, Asturian)

The `asturiano` preset MUST render authored, non-raw prose for its 4
dedicated presentation values keyed `asturian` (vocabulary, phrase pack,
address pack, anti-caricature). The prose MUST read as warm,
Asturian-flavored Spanish (light bable lexicon, mining metaphors,
retranca) woven into clear Spanish, never as an obstacle to understanding.
The anti-caricature prose MUST state that Asturian warmth and retranca are
seasoning, not a costume, warning against regional-cliché performance while
affirming that a lively tone never replaces verification. The Humor bullet
stays unmapped and continues to fall back to the raw ID `dry` (out of
scope, owned by the shared Yoda/sargento pack). `presentationLanguage`
keeps `es-asturian`'s readable label as `"Asturian"` (no relabel).
Asturiano classifies `bound` and its dialect-gating clause names the
native variant `"Asturian"`, gated on replying in Spanish.

#### Scenario: Asturiano's dedicated bullets render authored prose

- GIVEN the `asturiano` preset is rendered
- WHEN the vocabulary, phrase-pack, and address-pack fields are inspected
- THEN each shows authored, warm Asturian-flavored-Spanish prose, not the
  literal string `asturian`

#### Scenario: Anti-caricature bullet renders authored guardrail

- GIVEN the `asturiano` preset is rendered
- WHEN the anti-caricature field is inspected
- THEN it shows authored prose forbidding regional-cliché performance and
  affirming verification is never replaced by tone

#### Scenario: Gating clause names the Asturian layer and gates on Spanish

- GIVEN the `asturiano` preset is rendered
- WHEN the dialect-gating clause is inspected
- THEN it contains the string "- Dialect gating: the Asturian dialect layer"
- AND it contains the string "applies only when replying in Spanish"

#### Scenario: Asturiano humor bullet still falls back to raw ID

- GIVEN the `asturiano` preset is rendered
- WHEN the humor field is inspected
- THEN it shows the raw enum value `dry` (unmapped for Asturiano), never an
  empty string, and no dedicated prose is authored for it in this pack

### Requirement: Galleguinho Dedicated Voice (bound, Galician)

The `galleguinho` preset MUST render authored, non-raw prose for exactly 5
dedicated map entries: `humorProse["retranca"]`,
`vocabularyProse["galician"]`, `phrasePackProse["galician"]`,
`addressPackProse["galician"]`, and `antiCaricatureProse["galician"]`. The
Register field (`calm-teacher`, owned by the shared Yoda register fill)
resolves to the shared authored register prose once merged. The
anti-caricature prose MUST state that Galician warmth and retranca are
seasoning, not a costume (light galego touches, a wry aside, or a
Camino/sea metaphor welcome; stacking meigas/rain/postcard clichés or
performing a caricature Galicia forbidden), that retranca MUST NOT leave an
answer ambiguous where the user needs clarity, and that a wry tone MUST NOT
replace verifying facts. `presentationLanguage` keeps `es-galician`'s
readable label as `"Galician"` (no relabel). Galleguinho classifies `bound`
and its dialect-gating clause names the native variant `"Galician"`, gated
on replying in Spanish.

#### Scenario: Five dedicated bullets render authored prose

- GIVEN the `galleguinho` preset is rendered via `RenderLayer2` or
  `RenderOutputStyle`
- WHEN the humor, vocabulary, phrase-pack, address-pack, and anti-caricature
  fields are inspected
- THEN each shows authored prose text, not the raw ID (`retranca` or
  `galician`)

#### Scenario: Anti-caricature prose states both guardrails

- GIVEN the `galleguinho` preset is rendered
- WHEN the anti-caricature field is inspected
- THEN it warns against caricature/cliché Galicia
- AND it states retranca never leaves an answer ambiguous
- AND it states wry tone never replaces verification

#### Scenario: Bound dialect-gating clause names the native variant and gates on Spanish

- GIVEN the `galleguinho` preset is rendered (bound: `es-galician` +
  galician pack)
- WHEN the dialect-gating clause is inspected
- THEN it names the native variant "Galician"
- AND it states the dialect layer applies only when replying in Spanish

### Requirement: Shared Register and Prose Packs Stay Persona-Agnostic

Prose entries and register arms shared across multiple personas
(`humorProse["dry"]` used by yoda/sargento/asturiano;
`presentationRegister`'s `calm-teacher` arm used by yoda/galleguinho; the
`warm`/`peer`/`plain`/`grounded` packs used by argentino and available to
any future preset) MUST be authored as generic, persona-neutral content.
They MUST NOT reference any one persona's specific flavor (no clause
inversion, no "Hmm.", no movie nods, no roots/patience metaphor, no
military/engineering/regional vocabulary) so that every inheriting
persona's rendered bullet remains sensible for its own voice.

#### Scenario: Shared dry humor prose is persona-agnostic

- GIVEN `humorProse["dry"]`
- WHEN it is rendered for any persona selecting `humor: dry` (yoda,
  sargento, asturiano)
- THEN its text describes the dry/understated humor trait generically with
  no Yoda-specific vocabulary

#### Scenario: Shared calm-teacher register prose is persona-agnostic

- GIVEN the `calm-teacher` arm of `presentationRegister`
- WHEN it is rendered for any persona selecting `register: calm-teacher`
  (yoda, galleguinho)
- THEN its text describes the calm-teacher register generically with no
  Yoda-specific vocabulary

### Requirement: Voice-Only Rendering Boundary

Authored Layer-2 prose for every persona MUST remain voice-only. It MUST
NOT restate or duplicate Layer 1 mentor-philosophy or supremacy-rule
wording (forbidden strings such as `"CONCEPTS > CODE"`, `"AI IS A TOOL"`,
`"Technical Behavior"`), MUST NOT leak the preset's internal `display_name`
field into rendered output (rendering uses `toTitleCase(preset.Name)`
instead), and MUST NOT alter `keep-coding-instructions: true` frontmatter
behavior in `RenderOutputStyle`.

#### Scenario: Forbidden-string invariants hold for every persona

- GIVEN any built-in preset rendered via `RenderLayer2` or
  `RenderOutputStyle`
- WHEN the rendered presentation text is inspected
- THEN it does not contain any Layer-1 forbidden string or the raw
  `display_name` value

#### Scenario: Keep-coding-instructions frontmatter unaffected

- GIVEN any built-in preset rendered via `RenderOutputStyle`
- WHEN the frontmatter is inspected
- THEN `keep-coding-instructions: true` is still present and unchanged
