# Proposal: Persona Voice — Galleguinho

## Intent

Galleguinho (persona 6/6, last regional/BOUND) ships with empty foundations prose maps, so its Galician-flavored voice renders raw enum IDs instead of mentor prose. This change authors Galleguinho's VOICE only: a warm Galician-Spanish mentor with signature *retranca* (dry, indirect irony). Voice prose is USER-APPROVED and LOCKED. No schema or YAML change.

## Scope

### In Scope
- Fill 5 DEDICATED foundations prose maps in `loader.go` with the locked literals: `humorProse["retranca"]`, `vocabularyProse["galician"]`, `phrasePackProse["galician"]`, `addressPackProse["galician"]`, `antiCaricatureProse["galician"]`.
- No relabel of `presentationLanguage("es-galician")` — it stays "Galician"; the dialect-gating clause (foundations-owned) already states the layer applies only when replying in Spanish.
- No change to the existing foundations test `TestBoundDialectClauseUsesReadableLanguageName`: galleguinho arm stays "Galician".
- New RED test asserting the 5 dedicated bullets render authored prose and dialect-gating names "Galician" and states it applies only when replying in Spanish.

### Out of Scope
- Register `calm-teacher` arm — OWNED by the Yoda change (PR #424). Do NOT author it; do NOT assert the Register bullet (renders raw until integration).
- `es-rioplatense` / `es-asturian` language arms (other branches), other personas, schema/yaml, Layer 1 mentor text.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- None (voice/prose-map fill only; no language relabel; no spec-level requirement change).

## Approach

Author verbatim locked literals into the shared single-line prose-map var declarations in `loader.go`, expanding each map with the disjoint galician/retranca keys. No change to the es-galician case in the shared `renderPresentation` language helper — it stays "Galician". Claude + OpenCode parity via that shared helper. Strict TDD: keep the existing bound-dialect test assertion unchanged, add the RED dedicated-prose test, then fill to green.

## Product Rules

- **Retranca-always-clear**: retranca is seasoning — the clear technical answer always sits plainly behind it; never leave the message half-said or ambiguous where the user needs a clear answer.
- **Anti-caricature seasoning**: light galego touch, wry aside, Camino/sea metaphors welcome; never pile on meigas/rain/postcard clichés; wry tone never replaces verifying facts.
- Voice-only: no Layer-1 restatement (CONCEPTS > CODE, AI IS A TOOL).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/internal/persona/loader.go` | Modified | Fill 5 prose maps; no relabel of es-galician language name |
| `jarvis-cli/internal/persona/v2_test.go` | Modified | Update 1 assertion; add dedicated-prose RED test |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Textual conflict on shared prose-map var literals with other regional branches | Med | Resolve as union of disjoint keys |
| Accidentally authoring/asserting calm-teacher register arm | Low | Explicit non-goal; Yoda owns it |

## Rollback Plan

Revert the two-file diff on `loader.go` + `v2_test.go`; maps return empty. No data migration (es-galician label is untouched by this change either way).

## Dependencies

- Yoda change (PR #424) owns the `calm-teacher` register arm — coordinate at integration, not here.

## Success Criteria

- [ ] 5 dedicated galician/retranca bullets render authored locked prose.
- [ ] Bound-dialect clause names the native variant "Galician" and states it applies only when replying in Spanish.
- [ ] Register bullet not asserted (calm-teacher inherited/raw).
- [ ] `go test ./...` and `go vet ./...` pass.
