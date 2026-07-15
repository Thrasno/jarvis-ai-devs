# Design: persona-voice-asturiano (Change 2, persona 5 — BOUND regional)

## Technical Approach

Foundations (Change 1) shipped the render mechanism: 5 empty prose maps with
raw-ID fallback via `proseFor`, the `isBoundDialect` classifier, and the
`### Language Behavior` dialect-gating clause. This change is VOICE-ONLY: fill
the 4 dedicated `asturian` keys with USER-APPROVED, LOCKED literals and relabel
the `es-asturian` arm of `presentationLanguage`. No schema, no YAML, no
generated `~/.claude/*`. Source of truth: `internal/persona/loader.go` only.

Both `RenderLayer2` (OpenCode Layer-2) and `RenderOutputStyle` (Claude
output-style) call the shared `renderPresentation`, so filling the maps yields
Claude+OpenCode parity automatically. `asturiano.yaml` already selects
`vocabulary/address_pack/phrase_pack/anti_caricature = asturian` and
`language = es-asturian`, so the keyed literals fire with no asset edit.

## Architecture Decisions

| Decision | Choice | Rationale (vs. rejected) |
|---|---|---|
| Prose home | 4 dedicated `asturian` keys in existing renderer maps | Matches foundations scaffolding; keyed by pack enum ID. Rejected: YAML prose field (violates schema freeze). |
| Dialect firing model | Option A — Asturian-flavored **Spanish** | Flavor fires when replying in Spanish, parallel to Argentino voseo. Rejected: pure bable (would gate to a language users rarely write). |
| Language label | `presentationLanguage("es-asturian")` stays `"Asturian"` | Corrected foundations gates the clause on "Spanish" directly, so the label only names the dialect layer. Earlier relabel reverted as redundant. |
| Humor `dry` | Leave `humorProse` empty → renders raw ID `dry` | Out of scope (owned by Yoda PR #424). Not authored, not asserted. |
| Register | `warm-direct` arm unchanged | Already expanded to "warm, energetic, and direct" in foundations; no new arm. |

## Exact locked literals (author VERBATIM as final Go)

**`presentationLanguage`** — es-asturian arm stays `"Asturian"` (no relabel);
`es-rioplatense` and `es-galician` untouched:

```go
case "es-asturian":
    return "Asturian"
```

**`vocabularyProse["asturian"]`** — Asturian-flavored Spanish; light bable as
seasoning, clarity always wins:
> "Asturian-flavored Spanish — weave warm Asturian lexicon and turns of phrase into clear Spanish (light bable touches like 'ho', 'guaje', 'prestar', 'ñeru'), always kept light enough that the message stays perfectly clear; the flavor is seasoning, never an obstacle to understanding."

**`phrasePackProse["asturian"]`** — measured phrasing, retranca wit, mining
imagery for metaphors:
> "Warm, measured phrasing with a wink of Asturian retranca — dry, understated regional wit and the easygoing cadence of someone who'd settle a debate over a few sidras. Reach for mining imagery when a metaphor helps (digging into the seam, propping the tunnel, bringing the ore up), since Asturias is mining country. Keep the levity light; the point always lands."

**`addressPackProse["asturian"]`** — warm close peer, a paisanu:
> "Address the user as a warm, close peer — a paisanu you'd share a table and a sidra with; direct, honest, and welcoming, never deferential or distant."

**`antiCaricatureProse["asturian"]`** — seasoning not costume; flavor never
blocks verification:
> "The Asturian warmth and retranca are seasoning, not a costume — light bable and the odd sidra or mining aside are welcome, but never pile on regional clichés or perform a postcard Asturias; the flavor serves warmth and clarity, and a lively tone never replaces verifying facts and doing the work right."

## Render behavior (unchanged mechanism)

- Asturiano stays **BOUND** (`es-asturian` + `asturian` packs) → dialect-gating
  clause renders: `- Dialect gating: the Asturian dialect layer (regional
  vocabulary and phrasing) applies only when replying in Spanish...`.
- Bullets Vocabulary / Address pack / Phrase pack / Anti-caricature now render
  the locked prose instead of raw enum IDs; Humor bullet still renders raw `dry`.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/persona/loader.go` | Modify | Fill 4 `asturian` prose keys; es-asturian arm stays "Asturian" |
| `internal/persona/v2_test.go` | Modify | Update `TestBoundDialectClauseUsesReadableLanguageName` asturiano expectation; add new authored-voice test |

## Testing Strategy (TDD — RED first)

| Test | Change |
|---|---|
| `TestBoundDialectClauseUsesReadableLanguageName` (v2_test.go) | asturiano `readable` expectation stays `"Asturian"` (matches corrected foundations). `rawEnums` still asserts `es-asturian` absent. GREEN. |
| NEW `TestAsturianoPresentationRendersAuthoredVoice` | Load built-in `asturiano`; for BOTH `RenderLayer2` and `RenderOutputStyle` assert stable label-prefix substrings: `- Vocabulary: Asturian-flavored Spanish`, `- Phrase pack: Warm, measured phrasing with a wink of Asturian retranca`, `- Address pack: Address the user as a warm, close peer`, `- Anti-caricature: The Asturian warmth and retranca are seasoning`, `- Dialect gating: the Asturian dialect layer`, and `applies only when replying in Spanish`. Assert absence of forbidden Layer-1 strings (supremacy / reply-language). Do NOT assert the Humor bullet. |

No other existing assertion breaks: no test elsewhere asserts asturiano→"Asturian"
(only line 420, being updated). Verify: `go test ./...` and `go vet ./...` green.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file
classification, or process-integration boundary. Pure text rendering.

## Migration / Rollout

No migration. Revert the 4 prose additions + the one-arm relabel + test edits =
clean git revert. No schema/YAML/generated-artifact change.

## Open Questions

None. Voice prose USER-APPROVED and LOCKED; recorded verbatim.
