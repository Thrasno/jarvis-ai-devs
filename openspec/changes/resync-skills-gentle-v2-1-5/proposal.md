# Proposal: Selectively Resync Embedded Skills to gentle-ai v2.1.5

> **Status: APPROVED by the user after two clarification rounds.**

## Intent

Synchronize useful upstream mechanics from annotated tag `v2.1.5` (tag object `0b4532b5a73c12b7347c1954ef37cb372056c914`, peeled commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`) without importing runtime authorities Jarvis does not yet implement.

## Scope

### In Scope
- Classify 19 invokable skills, 3 meta-tooling packages, and 8 `_shared` support files.
- Update existing Jarvis equivalents; absent `hermes-ephemeral-delegation` remains excluded. No new invokable skill is added.
- For generic skills, follow the exact adoption decisions in [proposal-appendix.md](proposal-appendix.md).
- For SDD/shared assets, adopt neutral evidence, work-unit, runtime, rollback, persistence, language, and negative executor-boundary mechanics only.
- Verify source contracts and full CLI installation/regeneration; prepare but do not apply the bilingual issue #362 rewrite.

### Out of Scope
- Positive transaction, ledger, snapshot, receipt, `reviewGate`, native remediation/generation, archive-receipt, and authority-only verification protocols.
- CodeGraph lifecycle, generated user-machine edits, and GitHub mutation.

## Capabilities

### New Capabilities
- `embedded-skill-upstream-sync`: pinned provenance, dispositions, selective-adoption boundaries, and installation-level verification.

### Modified Capabilities
- `sdd-subagent-hive-tools`: synchronized SDD contracts preserve Hive tools, four stores, regeneration diagnostics, and safe executor boundaries.

## Approach

Port file by file. Generic bodies stay upstream-generic unless explicitly adapted. Split neutral mechanics from authority-bearing text; defer the latter to #365–#367 and #420–#422. #363 owns final phase routing and the complete `jarvis.sdd-status` contract.

## Affected Areas

| Area | Impact |
|---|---|
| `jarvis-cli/embed/skills/` | Skills, references, shared contracts, provenance |
| `jarvis-cli/embed/templates/`, `embed/hive-protocol.md` | Conditional source-contract parity |
| `jarvis-cli/internal/{skills,sddruntime,agent,project}` | Catalog, render/install, runtime, registry verification |

## Review Budget

Forecast: **900–1,300 authored changed lines**. Use the approved `size:exception` and single PR: this contract sync is safer atomically than with mixed phase versions. Goldens are excluded only from authored-line risk, never from normal generated-output verification.

## Risks

| Risk | Mitigation |
|---|---|
| Prompt contracts require unavailable authority | Negative-boundary tests and exact deferral map |
| Installed output drifts from sources | Full CLI install/regeneration checks |
| Registry cache/shareability remains contradictory | Mandatory design decision before implementation |

## Rollback Plan

Revert the sync PR/commit and regenerate installations from restored embedded sources.

## Dependencies

- #365 ledger/4R/refuter; #366 transaction/snapshot/receipt/facade; #367 Judgment Day; #363 routing/status; #420 verification/archive authority; #421 remediation; #422 semantic validity.

## Success Criteria

- [ ] Every row records pinned provenance, disposition, and adaptation/defer rationale.
- [ ] No unsupported positive authority protocol ships.
- [ ] Hive, four stores, registry legacy behavior, Go-owned model rows, persona separation, template parity, and source-only editing remain intact.
- [ ] Full source-to-installed CLI flow is verified.
