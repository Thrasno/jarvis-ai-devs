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
`jarvis-cli/embed/personas/custom.yaml.tmpl` when creating a profile.

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

### Argentine trait contract

`argentino` has one deterministic presentation tuple:
`address_pack: peer`, `phrase_pack: plain`, and `anti_caricature: grounded`.
Any other renderer-owned ID for these fields is rejected at load time, including
Gentle-specific or stereotype-risk IDs. Correct the named field to its canonical
value; keep Gentle technical policy in Layer 1. This is typed-ID validation only:
Jarvis does not inspect free text, perform NLP, or rewrite a profile. Other
profiles, including custom profiles, remain governed by the general schema-v2 ID
allowlist.

The shared contract supplies the same help-first teaching, concepts-first
learning, human-directed AI, foundations, evidence, safety, and language
boundaries for every row. These are not persona fields.

## Active catalog

The schema-v2 catalog is active. To select a built-in presentation profile, use
`jarvis persona set <preset>`. Existing V1 profiles and custom YAML require an
explicit migration to schema version 2 before they can be applied.
