# Design: Persona Voice — Tony Stark (Change 2, persona 3 — portable, supremacy stress-test)

## Technical Approach

Author Tony's VOICE on the foundations mechanism only. Fill 5 dedicated prose entries in `loader.go` (all keys are Tony-exclusive per explore) and add one `fast-witty` arm to `presentationRegister`. Everything routes through the existing `renderPresentation`, giving Claude output-style and OpenCode Layer-2 parity for free. Tony is `en-us` → `isBoundDialect` returns false → he stays PORTABLE (Portability affirmation only, NO dialect gating). No schema/yaml/`preset_v2.go` change; enums already validate.

Confidence is treated as delivery STYLE. The load-bearing guardrail (`antiCaricatureProse["engineer"]`) keeps wit/confidence from eroding the Layer-1 supremacy rules, which live only in Layer 1 and are never restated here.

## The authored prose (final Go string literals)

Each value is a single string rendered inline after its `- <Label>: ` bullet.

| Map key / token | Exact literal | Rationale |
|---|---|---|
| `vocabularyProse["engineering"]` | `engineering and systems vocabulary — talk in terms of components, interfaces, tolerances, and failure modes; name the moving parts precisely and keep the phrasing sharp and technical.` | Precise systems lexicon; sharp, punchy framing. |
| `phrasePackProse["engineer"]` | `fast, punchy delivery with sharp one-liners that still teach the underlying idea; occasional light engineering-hero nods (reactor cores, blueprints, suiting up) recontextualized to the real technical problem, never quoted verbatim, out of context, or as parody.` | Fast delivery that still TEACHES; soft recontextualized character nods, never parody/verbatim. |
| `addressPackProse["engineer"]` | `address the user as a capable engineering peer whose competence you assume; energetic, direct, and collaborative — never talk down, never condescend.` | Confident peer collaborator; assumes competence; never condescending. |
| `antiCaricatureProse["engineer"]` | `keep the wit and confidence as delivery style only: never let them tip into arrogance, false certainty, or skipped verification; when something is not verified, say so plainly; aim every joke or bit of ribbing at the problem, the code, or the situation, never at the user, and never condescend or talk down to them; confidence is how you talk, never a substitute for doing the work correctly.` | THE guardrail: wit/confidence never become arrogance, false certainty, skipped verification, or condescension (explicitly forbidden here per spec); ribbing lands on the problem, not the user. |
| `humorProse["witty"]` | `quick, dry, clever wit delivered in one-liners; always aimed at the problem or the situation, never at the user's expense, and never mean or sarcastic toward the user.` | Quick dry-sharp one-liners aimed at the problem, never at the user. |
| register token `fast-witty` | returns `fast, witty, and confident` | Short readable register, parallel to the warm-direct arm. |

None restate Layer-1 philosophy — forbidden strings (`CONCEPTS > CODE`, `AI IS A TOOL`, `Technical Behavior`) never appear.

## Architecture Decisions

| Decision | Choice | Rationale (vs. rejected) |
|---|---|---|
| Prose home | 5 dedicated keys in `loader.go` maps | Same shape as foundations scaffolding; Tony-only keys, no generic authoring. Rejected: shared/generic prose (no other persona uses these keys). |
| Register helper shape | Convert `if` to `switch`, add `fast-witty` case | Parallel arms, clean union with Yoda's `calm-teacher` branch. Warm-direct arm stays byte-identical. Rejected: chained `if` (worse merge surface). |
| Portability | Rely on existing `isBoundDialect(en-us)==false` | Tony portable by construction; no new gating. Rejected: any dialect logic. |
| Guardrail placement | Voice expressed inside `antiCaricatureProse` prose | Keeps supremacy rules in Layer 1; Layer 2 only styles delivery. |

## presentationRegister change (if → switch)

```go
func presentationRegister(register string) string {
	switch register {
	case "warm-direct":
		return "warm, energetic, and direct" // byte-identical to prior arm
	case "fast-witty":
		return "fast, witty, and confident"
	}
	return register
}
```

Integration note: Yoda's change adds a `calm-teacher` case to the same `switch`. Different branch, disjoint token → trivial union at integration time. Prose-map keys are also disjoint across Tony/Argentino/Yoda → union with no conflict.

## File Changes

| File | Action | Description |
|---|---|---|
| `jarvis-cli/internal/persona/loader.go` | Modify | Fill 5 prose entries; `if`→`switch` with `fast-witty` arm |
| `jarvis-cli/internal/persona/v2_test.go` | Modify | NEW RED tests (below) |

`preset_v2.go`, `embed/personas/*.yaml`, generated `~/.claude/*` — unchanged.

## Testing Strategy (TDD — RED first)

New test `TestTonyStarkPresentationRendersAuthoredVoice`: load the built-in `tony-stark` preset; for BOTH `RenderLayer2(preset)` and `RenderOutputStyle(preset)`, assert these stable label-prefix + first-clause substrings are present:

- `- Register: fast, witty, and confident`
- `- Vocabulary: engineering and systems vocabulary`
- `- Humor: quick, dry, clever wit`
- `- Address pack: address the user as a capable engineering peer`
- `- Phrase pack: fast, punchy delivery with sharp one-liners`
- `- Anti-caricature: keep the wit and confidence as delivery style only`

Same test asserts ABSENCE of `CONCEPTS > CODE`, `AI IS A TOOL`, `Technical Behavior`.

No existing assertion breaks: `TestPresentationValuesResolveNonEmptyWithRawIDFallback` (maps stay non-empty), `TestBuiltinProfilesV2MatchPresentationMatrix` (checks struct + forbidden strings, not prose), and `TestBuiltinPresetsRenderPortabilityAndGateDialectOnlyWhenBound` (tony-stark stays `false`; register arm adds no dialect gating) all pass unchanged. No existing exact-match assertion targets Tony's packs.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. Pure text rendering.

## Migration / Rollout

No migration. Revert `loader.go` + test edits = clean git revert. No schema/yaml/generated-file change.

## Open Questions

- [ ] None blocking. Yoda `switch` union is a known trivial integration merge.
