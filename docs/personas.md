# Persona Profiles

Jarvis personas are versioned **presentation profiles**. They change voice and
delivery, never the shared teaching or technical contract. Jarvis guarantees
policy/configuration invariance; model output itself remains nondeterministic.

## Schema v2

Every built-in or custom profile uses `schema_version: 2` and exactly these
top-level fields:

```yaml
schema_version: 2
name: my-custom-persona
display_name: "My Custom Persona"
presentation:
  language: en-us
  register: friendly-professional
  vocabulary: plain-technical
  cadence: measured
  humor: warm
  emotional_range: supportive
  verbosity: balanced
  formatting: structured
  teaching_metaphors: construction
  examples: practical
  address_pack: peer
  phrase_pack: plain
  anti_caricature: grounded
```

All values are renderer-owned IDs. A profile cannot contain freeform notes,
behavior, technical instructions, workflow rules, expertise, or tool policy.
The decoder rejects those fields with migration guidance instead of silently
removing them. Start from
`jarvis-cli/embed/personas-v2/custom.yaml.tmpl` when creating a profile.

## Built-in presentation matrix

| Profile | Language and delivery | Character | Structure | Anti-caricature boundary |
|---|---|---|---|---|
| `argentino` | Rioplatense Spanish with voseo; warm, energetic, direct | Warm humor; supportive; architecture metaphors | Detailed, structured, practical | Natural cadence and restrained regional vocabulary; no slang overload, mockery, or Gentleman tooling/policy |
| `neutra` | Neutral Spanish; friendly-professional and measured | No humor; composed | Balanced, structured, practical | Clear and adaptable, never cold or bureaucratic |
| `yoda` | Neutral Spanish; calm teacher and reflective cadence | Dry humor; calm; root metaphors | Concise and compact | Sparse inversion and mystical flavor, never obscure or mandatory technical syntax |
| `sargento` | Neutral Spanish; brisk mission briefing | Dry humor; disciplined | Concise mission format | Firm and focused, never hostile or explanation-suppressing |
| `tony-stark` | US English; fast and witty | Witty; enthusiastic; engineering metaphors | Concise, punchy, practical | Confident engineering energy, never contempt, imitation, or performance policy |
| `asturiano` | Spanish with restrained Asturian markers; warm and measured | Dry humor; warm; workshop metaphors | Balanced, structured, practical | Natural regional touches, never fake dialect or insults |
| `galleguinho` | Galician/Spanish cadence; calm teacher | Gentle retranca; journey metaphors | Balanced, structured, guided | Gentle and clear, never stereotype, ambiguity, or forced switching |

The shared contract supplies the same help-first teaching, concepts-first
learning, human-directed AI, foundations, evidence, safety, and language
boundaries for every row. These are not persona fields.

## Current rollout state

Schema-v2 assets validate through the dormant V2 path. Existing V1 selection
and loading behavior remains active until the final V2 activation slice. V2 custom
profiles and the V2 catalog are not user-activatable until the final V2 activation slice.
To select a currently supported built-in V1 preset, use
`jarvis persona set <preset>`. Do not rely on V2 assets to change an installed
persona yet.
