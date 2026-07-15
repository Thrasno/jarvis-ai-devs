# Proposal: Persona Voice Foundations (Change 1 of 2 — mechanism + correctness)

## Intent

Persona rendering today emits a terse `- Language: <X>` bullet that reads as an
imperative, has no authoritative "reply in the user's language" rule, no
contract-supremacy clause protecting the epistemic soul, and no portability
semantics. This makes persona behavior ambiguous and lets voice appear to
override verification rules. This change fixes the *mechanism* so correct
language behavior ships now. Concrete persona voice authoring is Change 2.

## Scope

### In Scope
- Fix #1: contract-supremacy clause in Layer 1 (`embed/technical-contract.md`).
- Fix #2: create + single-source an authoritative reply-language rule (Layer 1); remove the `- Language: X` imperative bullet from the render.
- Fix #3: renderer-side portability derivation (no schema change) + render language-behavior clauses (portability / dialect-gating).
- Fix #4: renderer-owned prose scaffolding in `loader.go` (maps keyed by pack ID), shipped EMPTY with graceful raw-ID fallback.

### Out of Scope (Change 2 / post-MVP)
- Concrete persona voice prose (Argentino, Yoda, Tony Stark).
- Regional prose (Gallego/Asturiano), data-driven pack expansion, weak-model validation.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None (renderer + Layer-1 correctness; no spec-level requirement change, no schema change).

## Approach

All four fixes live in `renderPresentation` (shared) + `technical-contract.md`, so
Claude (output-style) and OpenCode (Layer 2 in AGENTS.md) get identical behavior.
Schema frozen: `preset_v2.go` and every `personas/*.yaml` untouched.

**Product rules (authoritative):**
1. Portability MUST: ALL personas portable by default — character, register, Layer-1 mentor soul apply in whatever language the user writes; reply language ALWAYS follows the user.
2. The ONLY language-gated element is a regional persona's DIALECT layer (voseo/lunfardo, retranca, Asturian markers), active only when reply language is that persona's native Spanish variant.
3. Out-of-native-language: drop ONLY dialect markers; KEEP warm/energetic register + mentor soul. Never collapse to generic Neutra.
4. es-neutral personas (Yoda, Sargento, Neutra) are fully language-independent (no gated dialect layer).
5. Supremacy clause: ABSOLUTE precedence AND ENUMERATES protected rules by name — verify before asserting; distinguish confirmed facts from assumptions; ask one question then stop. Voice styles delivery only, never substitutes for verification.

**Change-1 Definition of Done:** supremacy clause + reply-language rule + portability/dialect-gating clauses all render. Prose maps ship EMPTY with raw-ID fallback (no concrete voice prose).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/embed/technical-contract.md` | Modified | Fix #1 supremacy clause + Fix #2 reply-language rule |
| `jarvis-cli/internal/persona/loader.go` | Modified | Fix #2 (drop imperative), #3 (portability + clauses), #4 (empty prose maps) |
| `jarvis-cli/internal/persona/{v2,bridge,apply}_test.go` | Modified | Update exact trait-bullet assertions FIRST (TDD); keep forbidden-string invariants |
| `preset_v2.go`, `personas/*.yaml` | Unchanged | Schema/enum frozen |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Exact-match trait assertions break on bullet-format change | High | Update tests FIRST (strict TDD) |
| Prose map missing key renders empty | Med | Graceful raw-ID fallback for every enum value |
| Reply-language rule duplicated/contradicts orchestrator preflight | Low | Single-source in Layer 1 |
| Voice leaks into artifacts / Layer-1 policy leaks into Layer 2 | Low | Preserve forbidden-string test invariants |

## Rollback Plan

Revert the two source files (`technical-contract.md`, `loader.go`) and the test
edits. No schema, yaml, or generated `~/.claude/*` files change, so revert is a
clean git revert with no migration.

## Dependencies

- Exploration artifact `sdd/persona-voice-foundations/explore` (id 4453). None external.

## Success Criteria

- [ ] Supremacy clause renders in Layer 1 surfaces, ABSENT from Layer 2.
- [ ] Single authoritative reply-language rule renders; `- Language: X` imperative removed.
- [ ] Portability + dialect-gating clauses render for all 7 builtins (bound vs portable correct).
- [ ] Prose maps ship empty; every enum value falls back to raw ID (never empty).
- [ ] Claude + OpenCode parity holds; `go test ./...` and `go vet ./...` green.
