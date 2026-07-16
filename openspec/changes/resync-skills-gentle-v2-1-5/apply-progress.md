# Apply Progress: Selectively Resync Embedded Skills to gentle-ai v2.1.5

## Status

**State:** complete. All 13 task checkboxes are reconciled in `tasks.md` and the full `jarvis-cli` suite plus `go vet` are green.

## Completed Tasks

1.1–1.3, 2.1–2.3, 3.1–3.4, and 4.1–4.3.

## Regression Fixed in This Batch

A prior WIP state left four catalog-contract tests RED (generic assets drifted
from their pinned v2.1.5 contract). Fixed as part of tasks 1.2/1.3:

- `comment-writer/SKILL.md`: restored the `v1.26.5` packaging source stamp and the
  upstream-generic `Match thread language` Voice Rules label. Replaced the
  persona/language-contract wording (`Match target context language` + Rioplatense
  voseo) with neutral, non-leaking English, per spec "Adaptation is not inferred".
- `skill-creator/SKILL.md`: restored the compact LLM-first contract. Removed the
  non-resolvable `docs/skill-style-guide.md` markdown reference and the forbidden
  `description: >` example block; restored the `[references/quality-loop.md]` link
  and added a `[references/skill-style-guide.md]` reference (design intent).
- `go-testing/references/examples.md` and `skill-creator/references/skill-style-guide.md`:
  added the required `gentle-ai v2.1.5 selective sync` provenance marker.
- Re-synced the four pinned fixtures under
  `internal/skills/testdata/gentle-ai-v2.1.5/` so `GentleAIV215GenericAssetsMatchPinnedFixtures`
  stays byte-exact.

RED evidence: `go test ./internal/skills/ -run TestCatalogContract` failed on
`ComplementarySkillsMatchUpstreamContract/comment-writer`,
`SkillCreatorUsesCompactLLMFirstContract`, `EmbeddedSkillMarkdownReferencesResolve`,
and `GentleAIV215AssetsPreserveApprovedBoundaries`. GREEN after the fixes above.

## TDD Cycle Evidence

| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 1.1 ledger portion | `TestCatalogContract_GentleAIV215LedgerRecordsCompleteDispositionInventory` failed when the ledger was absent | Passed after `docs/maintenance/skill-parity-run-gentle-ai-v2.1.5.md` was created | None needed |
| 1.2–1.3 | `TestCatalogContract_GentleAIV215AssetsPreserveApprovedBoundaries` initially failed on absent v2.1.5 assets | Passed after generic assets, examples, style guide, and neutral issue contract were added | Restored required packaging metadata/stamps while keeping generic bodies neutral |
| 2.1–2.3 | Existing catalog and runtime contract checks were exercised as the safety net | Focused skills/runtime suites passed | Added only neutral selective-sync annotations; no authority contract added |
| 3.1–3.4 | Existing all-store/grant/diagnostic tests were exercised as the safety net | `go test ./internal/agent ./internal/sddruntime ./internal/lifecycle ./internal/project ./internal/projectregistry -run 'Hive|Store|Verify|Doctor|Registry|SDD' -count=1` passed | None needed; existing Go runtime already provided exact five-tool and read-only behavior |
| 4.1–4.3 | Catalog contract initially exposed old repository-coupled issue assumptions | Full suite passed after neutral assertion and registry/docs alignment | None needed |

## Work Unit Evidence

| Unit | Focused test command and exact result | Runtime harness command/scenario and exact result | Rollback boundary |
|---|---|---|---|
| Provenance and generic assets | `go test ./internal/skills -run 'TestCatalogContract_(GentleAIV215|GentleAIParity|SDD)' -count=1` — PASS | N/A: embedded source contracts only | Revert ledger, generic skill files, and `catalog_contract_test.go` |
| Hive grants and diagnostics | `go test ./internal/agent ./internal/sddruntime ./internal/lifecycle ./internal/project ./internal/projectregistry -run 'Hive|Store|Verify|Doctor|Registry|SDD' -count=1` — PASS | Existing temp-home renderer/observer tests for Claude/OpenCode grants and doctor plans — PASS | Revert only source skill/shared docs; existing runtime grant implementation remains independently revertible |
| Registry/docs/install | `go test ./...` — PASS from `jarvis-cli` module | Embedded installation/rendering contract tests across supported agents — PASS | Revert registry/improver/resolver and three public docs; regenerate from restored embedded sources |

## Final Verification

- `go test ./...` from `jarvis-cli` — PASS (all 22 packages).
- `go vet ./...` from `jarvis-cli` — PASS (exit 0).
- `go test ./internal/skills/ -run TestCatalogContract -count=1` — PASS (previously RED).
- `hive-api` and `hive-daemon` modules — PASS (untouched, confirmed green).
- Repository-root `go test ./...` and `go vet ./...` are not runnable because the root has no Go module; there are three modules (`jarvis-cli`, `hive-api`, `hive-daemon`). Per-module commands above are authoritative.

## Changed Files

- `docs/maintenance/skill-parity-run-gentle-ai-v2.1.5.md` — pinned 19/3/8 provenance/disposition ledger.
- `jarvis-cli/embed/skills/{comment-writer,go-testing,issue-creation}/` — generic/neutral asset updates and Go references.
- `jarvis-cli/embed/skills/{sdd-*,work-unit-commits,skill-improver,skill-registry,_shared}/` — neutral selective-sync and registry/cache contract updates.
- `docs/{configuration,security-privacy,generated-artifacts}.md` — canonical local-cache and gitignore policy.
- `jarvis-cli/internal/skills/catalog_contract_test.go` — provenance, boundary, and neutral issue assertions.
- `openspec/changes/resync-skills-gentle-v2-1-5/tasks.md` — all completed checkboxes.

## Deferred Authority Confirmation

No ledger runtime, transaction, snapshot, receipt, `reviewGate`, remediation/generation, exit-125, semantic validity, archive authority, or positive review routing was added. Hive, all four stores, Go-owned model rows, and source-only editing remain intact.

## Delivery

- Strategy: one atomic PR with maintainer-approved `size:exception`; no commit was created.
- Actual authored lines: approximately 800 additions/deletions; generated outputs excluded.
- Rollback: revert this complete work unit and regenerate installations from restored embedded sources.
