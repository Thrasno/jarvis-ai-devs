# Design: Persona Voice — Yoda (portable exemplar)

## Technical Approach

Fill the empty Layer-2 prose maps in `jarvis-cli/internal/persona/loader.go:168-174` with
final English descriptive prose and extend `presentationRegister` with a `calm-teacher`
arm. DEDICATED `yoda` keys (vocabulary/phrase/address/anti-caricature) encode the product
voice rules; SHARED generic `humorProse["dry"]` and the `calm-teacher` register stay
persona-neutral so sargento/asturiano (dry) and galleguinho (calm-teacher) inherit clean
prose. No schema/YAML change (`yoda.yaml`, `preset_v2.go` frozen). Both Claude
(`RenderOutputStyle`) and OpenCode (`RenderLayer2`) route through the shared
`renderPresentation` → parity is automatic. Each value renders as one bullet
`- Label: <prose>`. Prose is VOICE-ONLY: no Layer-1 restatement, no enum literals.

## Architecture Decisions

### Decision: DEDICATED yoda prose vs SHARED generic fills

**Choice**: Author `yoda` keys with full Yoda-specific voice; author `dry` + `calm-teacher`
generically. **Alternatives**: fold Yoda flavor into `dry`/`calm-teacher` (rejected — bleeds
into sargento/asturiano/galleguinho). **Rationale**: keys are the sharing boundary; disjoint
`yoda`/`dry` keys also give a clean textual merge with the Argentino branch
(`rioplatense`/`warm`/`peer`/`plain`/`grounded`).

### Decision: extend `presentationRegister` for `calm-teacher`

**Choice**: convert the single `if` to a `switch` and add a `calm-teacher` arm returning a
short readable register phrase, matching the `warm-direct` pattern. **Alternatives**: leave
`calm-teacher` rendering raw (rejected — inconsistent, unreadable bullet). **Rationale**:
mirrors the existing mechanism; the `warm-direct` arm is untouched; galleguinho inherits it.

### Decision: clarity HARD CAP on inversion

**Choice**: encode inversion as a strong signature bounded by an explicit clarity cap in the
prose itself. **Rationale**: the proposal's central risk is inversion burying the lesson;
the cap plus the `yoda` anti-caricature entry keep comprehension first.

## Final Go literals (exact strings)

`vocabularyProse["yoda"]` (DEDICATED signature):
"Invert clauses for emphasis in the character's cadence — put the object or complement first and let the verb land last on short and medium statements (for example, 'un fallo en tu código veo, corregir el índice del array debes'). Clarity and the lesson are a hard cap: if inversion would bury the technical point or force deep nesting, straighten the sentence so the lesson always lands — never sacrifice comprehension for style. An occasional 'Hmm.' can mark a genuine thinking beat, sparingly, never as a verbal tic. Treat these phrases as illustrations of the flavor, not a script to repeat."
> Rationale: strong inversion + hard clarity cap + calibrated OK example + sporadic "Hmm." + illustration disclaimer.

`phrasePackProse["yoda"]` (DEDICATED):
"Phrase things in a reflective, measured way — short sentences and deliberate pauses carry more weight than exclamations. Any echo of the character's famous lines must be soft and recontextualized to the actual technical situation, adapting their spirit to the problem at hand; never quote them verbatim, out of context, or as parody."
> Rationale: measured cadence over exclamations; movie nods only soft and recontextualized.

`addressPackProse["yoda"]` (DEDICATED):
"Address the user as a calm mentor guides an apprentice — patient, encouraging, and steady, taking the time to let understanding grow. Stay a peer collaborator who shares ownership of the problem; guidance and encouragement never tip into condescension or talking down."
> Rationale: mentor-master stance toward a learner, still a peer, never condescending.

`antiCaricatureProse["yoda"]` (DEDICATED):
"Clarity beats mysticism — drop the clause inversion the moment it hurts comprehension, and keep the calm tone from sliding into vagueness or false certainty. Metaphors of roots and patience serve the lesson and only appear when they sharpen it, never as decoration for its own sake."
> Rationale: clarity > mysticism; calm never becomes vagueness/false certainty; metaphors serve the lesson.

`humorProse["dry"]` (GENERIC — sargento/asturiano inherit):
"Dry, understated humor — subtle and delivered with a light touch, the kind that rewards a second read. Never slapstick, never sarcastic at the user's expense; the wit stays gentle and keeps the collaboration comfortable."
> Rationale: dry, subtle wit; no persona flavor; not slapstick/sarcastic-at-user.

`presentationRegister` `calm-teacher` arm (GENERIC — galleguinho inherits) returns:
"calm, patient, and reassuring"
> Rationale: short register phrase parallel to "warm, energetic, and direct"; unhurried, reassuring; no Yoda flavor.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `jarvis-cli/internal/persona/loader.go` | Modify | 5 prose map entries (4 yoda + dry) as one-key-per-line literals; add `calm-teacher` arm to `presentationRegister` (if→switch, `warm-direct` unchanged) |
| `jarvis-cli/internal/persona/bridge_test.go` (or v2_test.go) | Modify | NEW RED tests for yoda rendered bullets + calm-teacher register |

## Rendering after change

`isBoundDialect(yoda)==false` (es-neutral not in `regionalLanguages`), so Yoda renders ONLY
the Portability clause — NO dialect-gating clause. Confirmed portable; no dialect logic added.

## Testing Strategy (RED-first)

New test loads the built-in preset via `jarvis.PersonaFS` → `ValidateAndDecode(yoda.yaml)` →
asserts `RenderLayer2` and `RenderOutputStyle` both contain these stable substrings
(label prefix + first clause):
- `- Register: calm, patient, and reassuring`
- `- Vocabulary: Invert clauses for emphasis in the character's cadence`
- `- Humor: Dry, understated humor`
- `- Address pack: Address the user as a calm mentor guides an apprentice`
- `- Phrase pack: Phrase things in a reflective, measured way`
- `- Anti-caricature: Clarity beats mysticism`

| Layer | What | Approach |
|-------|------|----------|
| Unit | yoda 5 fields + calm-teacher register render authored prose | substring asserts on both render paths |
| Unit | no forbidden Layer-1 strings | assert absence of `CONCEPTS > CODE`, `AI IS A TOOL`, `Technical Behavior` |

No existing assertion breaks: all target `validPresetV2` (uses `plain-technical`/`friendly-professional`,
no prose entry) or the rioplatense variant — never yoda keys. `dry` fill changes the rendered
`- Humor:` bullet for sargento/asturiano (safe — no test asserts it). `warm-direct` arm untouched.

## Threat Matrix

N/A — pure in-memory string data; no routing, shell, subprocess, VCS/PR automation,
executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration. Rollback: revert the 5 map literals to empty and remove the `calm-teacher`
arm + new tests; renderer falls back to raw enum IDs.

## Open Questions

None.
