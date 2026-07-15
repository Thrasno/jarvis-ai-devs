# Proposal: persona-voice-neutra (Neutra baseline VOICE)

## Intent

Change 1 shipped the `proseFor` mechanism but all 5 prose maps ship EMPTY, so every Neutra presentation bullet renders its raw enum token (e.g. `neutral-spanish`, `none`). Neutra is the LAST persona (7/7) and the generic/plain-professional BASELINE that default and custom personas inherit as fallback. It must read as finished, plain, professional prose — not raw tokens — while staying genuinely generic (zero flavor/character).

## Scope

### In Scope
- Fill 5 Neutra-only prose entries in `jarvis-cli/internal/persona/loader.go` with the LOCKED user-approved literals (verbatim):
  - `vocabularyProse["neutral-spanish"]`
  - `humorProse["none"]`
  - `phrasePackProse["neutral"]`
  - `addressPackProse["neutral"]`
  - `antiCaricatureProse["neutral"]`
- TDD: Neutra render assertions (RED) for each of the 5 bullets + preserved portability / no-dialect-gating invariants, then populate maps (GREEN).

### Out of Scope (Non-goals)
- Other personas (yoda, tony, sargento, argentino, asturiano, galleguinho).
- Schema (`preset_v2.go`) and yaml (`neutra.yaml`) — both frozen, already match matrix.
- The `friendly-professional` register arm / any `presentationRegister` edit (Approach A — left RAW; shared with `validPresetV2` fixture).
- Layer 1 (Mentor: "CONCEPTS > CODE", "AI IS A TOOL", Technical Behavior) — prose is VOICE-ONLY.
- Generated `~/.claude/*`, docs.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- None (prose-fill only; no spec-level requirement change).

## Approach

Approach A (recommended in exploration): touch ONLY the 5 prose-map literals. Keys are disjoint from sibling persona branches → trivial union merge. `renderPresentation` already routes these bullets through `proseFor`; Claude + OpenCode parity is automatic via shared `renderPresentation`. Register stays raw, avoiding the shared `bridge_test.go:140`/`:236` fixture assertions.

## Product Rules
- Prose must stay GENUINELY GENERIC — these are the inherited default/custom fallback packs; any flavor would contaminate the baseline.
- Prose is VOICE-ONLY; never restate Layer 1.
- Neutra is PORTABLE (es-neutral) → portability affirmation only, no dialect gating.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Register assertions break | Low | Approach A never touches `presentationRegister` |
| Prose too flavored, contaminates baseline | Med | Use locked plain literals verbatim; review for neutrality |
| Adjacent-line merge conflict with sibling branches | Med | Keys disjoint → trivial union resolution |

## Rollback Plan

Revert the loader.go prose-map additions and the new Neutra test assertions. Empty maps restore raw-token fallback; no schema/yaml/generated-artifact impact.

## Dependencies

- Change 1 (persona-voice-foundations) `proseFor` mechanism — already merged.

## Success Criteria

- [ ] All 5 Neutra bullets render the locked prose (not raw tokens).
- [ ] Portability affirmation present; no dialect gating.
- [ ] `friendly-professional` register still renders RAW; `bridge_test.go` unchanged.
- [ ] `go test ./...` and `go vet ./...` green; no schema/yaml/generated changes.
