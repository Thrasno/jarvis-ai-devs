# Design: persona-voice-neutra (Neutra baseline VOICE)

## Technical Approach

Approach A: fill ONLY the 5 renderer-owned prose maps in
`jarvis-cli/internal/persona/loader.go` with the LOCKED user-approved
literals. Change 1 already shipped the routing: `renderPresentation` sends
Vocabulary/Humor/Address pack/Phrase pack/Anti-caricature through `proseFor`,
which returns the map value when present and falls back to the raw enum ID
otherwise. Populating the 5 keys flips those bullets from raw tokens to
finished prose with zero routing, schema, or yaml change. Claude
(`RenderOutputStyle`) and OpenCode (`RenderLayer2`) reach the same
`renderPresentation`, so parity is automatic.

Neutra presentation uses language `es-neutral` with neutral packs, so
`isBoundDialect` returns false → only the Portability clause renders, no
dialect gating. Register `friendly-professional` is NOT in
`presentationRegister`'s `warm-direct` arm, so it renders RAW — untouched
(Approach A), keeping `bridge_test.go:140/:236` fixture assertions green.

## Architecture Decisions

| Decision | Choice | Rationale (vs. rejected) |
|---|---|---|
| Prose home | 5 renderer maps in `loader.go` | Reuses shipped `proseFor` routing; no schema/yaml touch. Rejected: yaml prose fields (violates freeze). |
| Register handling | Leave `presentationRegister` unchanged (RAW `friendly-professional`) | Approach A avoids the shared `validPresetV2` fixture in `bridge_test.go:140/:236`. Rejected: add register arm (breaks shared assertions, out of scope). |
| Neutra portability | Rely on existing `isBoundDialect` = false for `es-neutral` + neutral packs | Baseline is portable by design; portability clause only, no dialect gating. Rejected: any dialect wording (contaminates generic baseline). |
| Prose flavor | Verbatim locked literals, genuinely generic | Neutra is the inherited default/custom fallback; any flavor leaks into every persona. |

## Locked Literals (record VERBATIM as final Go)

| Map key | Literal | Rationale |
|---|---|---|
| `vocabularyProse["neutral-spanish"]` | `Neutral, standard vocabulary — no regional markers, slang, or jargon beyond what the task needs; plain, precise, and widely understood in whatever language you reply in.` | Generic vocabulary, portable across languages, zero regional marker. |
| `humorProse["none"]` | `No humor as a device — keep it straightforward and professional; warmth comes from clarity and helpfulness, not from jokes.` | Baseline carries no comedic device; warmth via clarity only. |
| `phrasePackProse["neutral"]` | `Plain, clear, neutral phrasing — straightforward sentences, no ornament and no stylized turns; communicate directly and professionally.` | Unornamented phrasing so inherited default stays flavorless. |
| `addressPackProse["neutral"]` | `Address the user as a professional peer — courteous, direct, and helpful; neither deferential nor overly casual.` | Peer register, neither servile nor slangy. |
| `antiCaricatureProse["neutral"]` | `Stay genuinely neutral and professional — never adopt a regional, theatrical, or exaggerated voice; clarity comes first, and a measured tone never replaces verifying facts and doing the work right.` | Guards baseline against drift into any flavored voice; defers to Layer-1 verification. |

## Data Flow

    neutra preset ──→ renderPresentation ──→ proseFor(map, id)
                          │                       │ hit → locked prose
                          │                       └ miss → raw id (other keys)
                          ├─→ RenderLayer2 (OpenCode)
                          └─→ RenderOutputStyle (Claude)

## File Changes

| File | Action | Description |
|---|---|---|
| `jarvis-cli/internal/persona/loader.go` | Modify | Populate 5 prose-map literals (keys above). No other edit. |
| `jarvis-cli/internal/persona/*_test.go` | Modify | Add RED `TestNeutraPresentationRendersAuthoredVoice` first. |

`presentationRegister`, `isBoundDialect`, schema, yaml, generated `~/.claude/*`: UNCHANGED.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit (RED→GREEN) | `TestNeutraPresentationRendersAuthoredVoice` | Load built-in neutra preset; assert BOTH `RenderLayer2` and `RenderOutputStyle` contain label-prefix substrings: `- Vocabulary: Neutral, standard vocabulary`, `- Humor: No humor as a device`, `- Phrase pack: Plain, clear, neutral phrasing`, `- Address pack: Address the user as a professional peer`, `- Anti-caricature: Stay genuinely neutral and professional`. Assert Portability clause present, NO dialect-gating clause, and forbidden Layer-1 strings absent. |
| Regression | `bridge_test.go:140/:236` + raw-ID fallback test | Confirm green — register stays RAW, other empty keys still fall back to raw IDs. |

Stable label-prefix substrings (`- <Label>: <lead words>`) survive minor
literal edits while proving the bullet is authored, not a raw token.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file
classification, or process-integration boundary. Pure text rendering.

## Migration / Rollout

No migration. Revert the 5 map entries + new test = empty maps restore
raw-token fallback. No schema/yaml/generated impact.

## Open Questions

- None. Literals are user-approved and LOCKED.
