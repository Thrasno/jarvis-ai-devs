# Design: Persona Voice Foundations (Change 1 — mechanism + correctness)

## Technical Approach

Four fixes across two source-of-truth surfaces:

- **Layer 1** (`embed/technical-contract.md`): supremacy clause (#1) + reply-language rule (#2). Reaches BOTH agents via `config.Layer1Content()` → `WriteInstructions(layer1,…)` → CLAUDE.md and AGENTS.md. It does **not** flow through `renderPresentation`, so it stays out of the Layer-2 presentation block and the forbidden-string invariants hold automatically.
- **Renderer** (`internal/persona/loader.go`, shared by Claude output-style and OpenCode Layer-2): drop the `- Language:` bullet (#2), portability classifier + Language-Behavior clauses (#3), empty prose maps with raw-ID fallback (#4).

Schema frozen: `preset_v2.go` and every `personas/*.yaml` untouched.

## Architecture Decisions

| Decision | Choice | Rationale (vs. rejected) |
|---|---|---|
| Home of supremacy + reply-language | Layer 1 `technical-contract.md` | Single source; reaches Claude+OpenCode via `Layer1Content`. Rejected: emit in `renderPresentation` (would duplicate rule and risk Layer-1 policy leaking into Layer-2 presentation). |
| Portability derivation | Renderer-side classifier over in-memory `Presentation` | No schema change, no yaml/injection surface. Rejected: yaml field (violates freeze). |
| Bound-dialect test | `regionalLanguage AND regionalPack` | Protects custom personas that set a regional language with generic packs (stay portable — no dialect layer to gate). |
| Prose scaffolding | 5 empty maps + `proseFor` raw-ID fallback | Same shape as existing `presentationLanguage/Register`; ships empty so current bullets render unchanged. |

## Interfaces (loader.go)

```go
var regionalLanguages = map[string]bool{"es-rioplatense": true, "es-asturian": true, "es-galician": true}
var regionalPacks     = map[string]bool{"rioplatense": true, "asturian": true, "galician": true}

// bound iff regional Spanish language paired with a regional pack; else portable.
func isBoundDialect(p Presentation) bool {
    if !regionalLanguages[p.Language] { return false }
    return regionalPacks[p.Vocabulary] || regionalPacks[p.PhrasePack] || regionalPacks[p.AddressPack]
}

var vocabularyProse, humorProse, phrasePackProse, addressPackProse, antiCaricatureProse = map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}

func proseFor(t map[string]string, id string) string {
    if v, ok := t[id]; ok && strings.TrimSpace(v) != "" { return v }
    return id // graceful: never empty
}
```

Classification table: argentino / asturiano / galleguinho ⇒ **bound**; neutra / yoda / sargento (es-neutral) / tony-stark (en-us) ⇒ **portable**. `presentationRegister` and `presentationLanguage` helpers stay (real non-empty mappings); the 5 new maps ship empty. `presentationLanguage` moves from the deleted bullet into the dialect clause.

## Exact wording — Layer 1 (`technical-contract.md`)

New `## Contract Supremacy` section (place after `Evidence, Certainty, and Safety`):

```
## Contract Supremacy

This contract holds absolute precedence over every persona. When persona voice, tone, or presentation would conflict with a rule here, this contract wins and the voice yields. Persona voice shapes only how a reply is delivered; it never changes what must be verified, claimed, or asked. These protected rules can never be softened, overridden, or bypassed by any persona:

- Verify technical claims with evidence before asserting them as fact.
- Distinguish confirmed facts from assumptions, and state uncertainty explicitly.
- Ask one focused clarifying question when a decision is blocked, then stop and wait for the answer.
- Persona voice styles delivery only; it is never a substitute for verification and never a source of certainty.
```

New `## Reply Language` section (place before `Persona Scope and Artifact Language`):

```
## Reply Language

Reply in the language the user writes in. The active persona's character and register still apply — expressed in that language. This does not change artifact language: generated technical artifacts still default to English (see below).
```

No contradiction with `sdd-orchestrator.md:111-115` (that scopes reply-language to the SDD preflight — a specific application of this general rule). No forbidden phrase is reused.

## Exact wording — renderer (`### Language Behavior`, emitted by `renderPresentation` after the bullets)

Affirmation clause (ALL personas):

```
- Portability: this character and its register apply in whatever language the user writes; the reply always follows the user's language.
```

Dialect-gating clause (bound personas only; `{NATIVE}` = `presentationLanguage(p.Language)`, e.g. `Rioplatense Spanish (voseo)`):

```
- Dialect gating: the {NATIVE} dialect layer (regional vocabulary and phrasing) applies only when replying in {NATIVE}. In any other language, drop only the dialect markers and keep the register and the Layer 1 mentor approach — never collapse into a generic, character-less voice.
```

## Render format (order)

`## Persona: <TitleCase(slug)>` → `### Presentation` (12 bullets, `- Language:` REMOVED, order otherwise unchanged; Vocabulary/Humor/Address pack/Phrase pack/Anti-caricature routed through `proseFor`) → `### Language Behavior` (Portability always; Dialect gating only if `isBoundDialect`).

## File Changes

| File | Action | Description |
|---|---|---|
| `embed/technical-contract.md` | Modify | Add Contract Supremacy + Reply Language sections |
| `internal/persona/loader.go` | Modify | Remove `- Language:` bullet; add classifier, prose maps, `proseFor`, Language-Behavior block |
| `internal/persona/{v2,bridge,apply}_test.go` | Modify | Update exact-match assertions FIRST (TDD) |
| `internal/config/templates_test.go` | Modify | Add RED test: contract contains supremacy + reply-language |

Repo-root `AGENTS.md`/`CLAUDE.md` are unaffected — this touches product Layer 1 (`embed/`), not the repo agent-instruction files.

## Testing Strategy (TDD — tests first)

| Test | Change |
|---|---|
| `TestRenderV2PresentationRendersEverySelectedTrait` (bridge_test.go:219) | Drop `- Language: en-us` from wantTraits; add `Portability:` clause assertion |
| `TestRenderV2PresentationKeepsPolicyOutOfPresentationSurfaces` (:139/147/172) | Replace `wantLanguage` bullet with Language-Behavior clause; rioplatense case asserts dialect-gating text, custom en-us asserts Portability + NO dialect gating |
| `TestArgentinePresentationKeepsSharedLayer1OutOfRenderedSurfaces` (:201) | Assert dialect-gating clause (contains `Rioplatense Spanish (voseo)`) instead of bullet |
| New Layer-1 tests | `technical-contract` contains supremacy + reply-language; supremacy/reply-language ABSENT from `RenderLayer2`/`RenderOutputStyle` |
| Preserved invariants | All forbidden-string checks (bridge:165/175, v2:350/420, apply:121); `keep-coding-instructions: true`; slug heading (no display_name leak); `TestBuiltinProfilesV2MatchPresentationMatrix` (asset tuples) — unchanged |

Empty prose maps ⇒ Vocabulary/Address/Phrase/Anti-caricature bullets fall back to raw IDs ⇒ existing exact-match strings for those bullets still pass.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. Pure text rendering + embedded-asset edit.

## Migration / Rollout

No migration. Revert `technical-contract.md`, `loader.go`, and test edits = clean git revert. No schema/yaml/generated `~/.claude/*` change.

## Open Questions

- [ ] es-neutral personas (yoda/sargento/neutra) classify as portable (no gated dialect layer). Confirmed intended per proposal rule #4 — flagged for validation only.
