# Design: Persona Voice — Galleguinho (Change 2, persona 6/6, BOUND)

## Technical Approach

VOICE-ONLY change. Galleguinho already renders through the Change 1 mechanism
(`renderPresentation` → `proseFor` → empty prose maps → raw enum IDs). This
change fills the 5 dedicated prose-map keys galleguinho consumes with
USER-APPROVED, LOCKED literals, and relabels the `es-galician` arm of
`presentationLanguage` so the bound-dialect clause reads naturally.

No schema/yaml change. No `presentationRegister` edit. Two source files only:
`internal/persona/loader.go` (voice literals + one relabel) and
`internal/persona/v2_test.go` (1 assertion update + 1 new RED test). Claude
(`RenderOutputStyle`) and OpenCode (`RenderLayer2`) share `renderPresentation`,
so both surfaces get the voice from one edit — parity is automatic.

galleguinho.yaml enum IDs (verified) route to exactly these keys: `vocabulary:
galician`, `humor: retranca`, `phrase_pack: galician`, `address_pack: galician`,
`anti_caricature: galician`. `register: calm-teacher` has NO helper arm and
falls back to raw — intentionally, Yoda (PR #424) owns that arm.

## Architecture Decisions

| Decision | Choice | Rationale (vs. rejected) |
|---|---|---|
| Where voice lives | Renderer prose maps in `loader.go` | Same mechanism Change 1 shipped; disjoint keys, no schema touch. Rejected: yaml field (violates freeze). |
| Shared keys `"galician"` | Union of disjoint keys across regional branches | vocabulary/phrase/address/anti-caricature all key on `"galician"`; each map is independent, no collision with rioplatense/asturian. |
| Language relabel | `"Galician"` → `"Galician Spanish"` (Option A) | Parallels "Rioplatense Spanish (voseo)"; keeps es-galician BOUND and the dialect clause readable. es-rioplatense/es-asturian arms UNCHANGED. |
| Register calm-teacher | OUT OF SCOPE — not authored, not asserted | Owned by Yoda PR #424; renders raw `calm-teacher` until integration. No `presentationRegister` edit here. |

## LOCKED literals (record VERBATIM — final Go)

**`presentationLanguage("es-galician")`**: `return "Galician Spanish"` — readable bound-dialect label; parallels voseo/Asturian.

**`humorProse["retranca"]`** = `"Galician retranca — dry, indirect irony and gentle ambiguity: answer a question with a question, understate, lean on the 'haberlas, haylas' spirit. Wry and warm, never at the user's expense. But the retranca is seasoning: the clear technical answer always sits plainly behind it — never leave the message half-said."` — signature humor, always-clear guardrail baked in.

**`vocabularyProse["galician"]`** = `"Galician-flavored Spanish — light galego lexicon and expressions woven into clear Spanish ('¿e logo?', 'morriña', 'colo', 'riquiño'), warm and understated, always kept light enough that the message stays perfectly clear; the flavor is seasoning, never an obstacle to understanding."` — dialect lexicon, clarity-first.

**`phrasePackProse["galician"]`** = `"Calm, unhurried, warm phrasing with a touch of morriña. Reach for Camino de Santiago imagery (the next waymarker, don't rush the stage, one step at a time) and the sea and rías (reading the tide, mending the nets) when a metaphor helps — that is Galicia's landscape. Measured cadence; the point always lands."` — cadence + Galician metaphor field.

**`addressPackProse["galician"]`** = `"Address the user as a warm, close paisano — gentle, welcoming, and unhurried; direct but never distant or deferential."` — paisano address stance.

**`antiCaricatureProse["galician"]`** = `"The retranca and Galician warmth are seasoning, not a costume — a light galego touch, a wry aside, a Camino or sea metaphor are welcome, but never pile on meigas/rain/postcard clichés or perform a caricature Galicia; the retranca never leaves an answer ambiguous where the user needs a clear one, and a wry tone never replaces verifying facts and doing the work right."` — anti-caricature + clarity/verification guardrail.

## File Changes

| File | Action | Description |
|---|---|---|
| `jarvis-cli/internal/persona/loader.go` | Modify | Populate 5 prose-map keys with locked literals; relabel es-galician arm to "Galician Spanish". No presentationRegister change. |
| `jarvis-cli/internal/persona/v2_test.go` | Modify | Update galleguinho assertion (:421) "Galician" → "Galician Spanish"; add new RED authored-voice test. |

## Testing Strategy (TDD — RED first)

| Test | Change |
|---|---|
| `TestBoundDialectClauseUsesReadableLanguageName` (:421) | galleguinho `"Galician"` → `"Galician Spanish"`. Only this line changes. |
| NEW `TestGalleguinhoPresentationRendersAuthoredVoice` | Load built-in galleguinho; assert BOTH `RenderLayer2` and `RenderOutputStyle` contain stable label-prefix substrings: `- Vocabulary: Galician-flavored Spanish`, `- Humor: Galician retranca`, `- Phrase pack: Calm, unhurried, warm phrasing with a touch of morriña`, `- Address pack: Address the user as a warm, close paisano`, `- Anti-caricature: The retranca and Galician warmth are seasoning`, `- Dialect gating: the Galician Spanish dialect layer`. Assert absence of forbidden Layer-1 strings. Do NOT assert the Register bullet. |

No other existing assertion breaks: the 5 keys were empty (raw-ID fallback), so populating them only affects galleguinho's rendered bullets; no other persona consumes `retranca` or `galician`.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. Pure text rendering.

## Migration / Rollout

No migration. Revert the two-file diff = clean git revert. No schema/yaml/generated `~/.claude/*` change.

## Open Questions

- [ ] Register `calm-teacher` renders raw until Yoda PR #424 integrates the arm. Confirmed intended non-goal — flagged for integration coordination only.
