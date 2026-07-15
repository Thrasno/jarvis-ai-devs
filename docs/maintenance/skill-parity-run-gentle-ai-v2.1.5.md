# Gentle AI v2.1.5 Skill Parity Ledger

## Provenance

| Field | Value |
|---|---|
| Upstream tag | `v2.1.5` |
| Tag object | `0b4532b5a73c12b7347c1954ef37cb372056c914` |
| Peeled commit | `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17` |
| Scope | Selective source-of-truth sync; generated user-machine configuration is excluded. |

Every row below has an explicit disposition and rationale. Deferred authority is not implemented by this parity run: #365 owns ledger/4R/refuter, #366 transaction/snapshot/receipt/facade, #367 Judgment Day, #363 final routing and complete status contract, #420 verification/archive authority, #421 remediation lineage, and #422 semantic validity.

## Invokable Skills (19)

| Skill | Disposition | Rationale |
|---|---|---|
| `branch-pr` | Metadata only | Existing behavior is equivalent; do not rewrite the body. |
| `chained-pr` | Metadata only | Preserve review-budget behavior without authority clauses. |
| `cognitive-doc-design` | Metadata only | Existing behavior is equivalent; do not rewrite the body. |
| `comment-writer` | Adopt upstream body | Keep the generic English body; repository bilingual policy remains external. |
| `go-testing` | Adopt upstream body and examples | Keep generic upstream references with no Jarvis-specific examples. |
| `hermes-ephemeral-delegation` | Excluded | Hermes runtime is unavailable; a future generic alternative belongs to #363. |
| `issue-creation` | Repository-neutral adaptation | Discover repository templates, labels, blank-issue policy, approval workflow, and Discussions capability. |
| `judgment-day` | Excluded | Deferred to #367; do not import ledger or transaction authority. |
| `sdd-explore` | Neutral mechanics | Keep executor, language, persistence, and envelope behavior only. |
| `sdd-propose` | Neutral mechanics | Keep question, capability, persistence, and limit behavior only. |
| `sdd-spec` | Neutral mechanics | Keep complete deltas and migration behavior with four stores. |
| `sdd-design` | Neutral mechanics | Keep applicable threat and planned RED behavior only. |
| `sdd-tasks` | Neutral mechanics | Keep focused tests, runtime evidence, rollback, and authored-line treatment. |
| `sdd-init` | Compatible v3 mechanics | Preserve Hive, four stores, canonical `.jarvis`, and legacy `.atl` fallback. |
| `sdd-onboard` | Neutral mechanics | Keep artifact-language and walkthrough improvements only. |
| `sdd-apply` | Compatible v3 mechanics | Preserve bounded work-unit evidence and negative executor review/remediation boundary. |
| `sdd-verify` | Compatible v3 mechanics | Preserve independent evidence; do not import transaction, receipt, or exit-125 authority. |
| `sdd-archive` | Neutral mechanics | Keep task/spec/current-verify reconciliation only. |
| `work-unit-commits` | Neutral mechanics | Keep focused-result, runtime-or-N/A, rollback, and authored-line rules. |

## Meta-Tooling Packages (3)

| Package | Disposition | Rationale |
|---|---|---|
| `skill-creator` | Adopt upstream body and guide | Packaging metadata only; no product-specific adaptation. |
| `skill-improver` | Minimal adaptation | Use canonical `.jarvis` registry, `.atl` legacy fallback, refresh, approval, and artifact-language safety. |
| `skill-registry` | Hive/Jarvis adaptation | Keep the canonical, auto-refreshed, gitignored local cache decision. |

## Shared Support Files (8)

| File | Disposition | Rationale |
|---|---|---|
| `_shared/SKILL.md` | Excluded | Support-only package is not an invokable skill. |
| `engram-convention.md` | Hive adaptation | Adopt neutral naming and automated-save behavior using Hive terminology. |
| `openspec-convention.md` | Preserve | Existing compatible OpenSpec convention remains source-compatible. |
| `persistence-contract.md` | Hive adaptation | Preserve Hive and `hive | openspec | hybrid | none` behavior without authority state. |
| `review-ledger-contract.md` | Excluded | Ledger authority is deferred to #365 and #366. |
| `sdd-phase-common.md` | Selective adoption | Keep loading, persistence, response ordering, workload, and golden treatment only. |
| `sdd-status-contract.md` | Excluded | Complete status authority is deferred to #363; retain Go-owned current-state rows. |
| `skill-resolver.md` | Hive/Jarvis adaptation | Use Hive retrieval, `.jarvis` canonical path, and `.atl` legacy fallback. |

## Non-Goals Confirmed

This change does not add positive review routing, a transaction, ledger, snapshot, receipt, `reviewGate`, remediation, generation, exit-125 denial protocol, semantic-validity authority, archive authority, or a new invokable skill.
