# Tasks: Selectively Resync Embedded Skills to gentle-ai v2.1.5

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated authored lines | 900–1,300 |
| Estimated generated lines | Variable goldens/install outputs; excluded from authored budget, included in verification |
| 400-line budget risk | High |
| Chained PRs recommended | No — approved `size:exception` |
| Suggested split | One atomic PR; work-unit commits below |
| Delivery strategy | single-pr-default |
| Chain strategy | size-exception |
| Decision needed before apply | No — user approved explicit `size:exception` |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units (one PR)

| Unit | Goal | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|
| 1 | Provenance, dispositions, generic assets | `go test ./jarvis-cli/... -run 'Provenance|Catalog|Skill'` | CLI asset/catalog inspection; N/A runtime authority | Ledger, generic skill files, creator guide |
| 2 | SDD/shared contracts and current status-core | `go test ./jarvis-cli/... -run 'Contract|Phase|Status'` | N/A: prompt contracts only | SDD/shared assets; no new status contract |
| 3 | Hive grants and four-store behavior | `go test ./jarvis-cli/... -run 'Agent|Store|Degraded'` | Render Claude/OpenCode in temp tree | Grant/template/runtime contract files |
| 4 | Verifier, doctor, registry migration | `go test ./jarvis-cli/... -run 'Verify|Doctor|Registry'` | `jarvis init`/doctor against temp installation | Diagnostics/registry changes |
| 5 | Source-to-install and final verification | `go test ./... && go vet ./...` | Full CLI install/regeneration for supported agents | Entire atomic sync; regenerate from reverted sources |

## Phase 1: RED Contracts and Inventory

- [x] 1.1 RED: add exact 19/3/8 ledger assertions plus repository-selection cases (relative/absolute/`git -C`/nested cwd/unavailable/wrong repo, fail without mutation) in the parity/catalog tests; production: create `docs/maintenance/skill-parity-run-gentle-ai-v2.1.5.md`.
- [x] 1.2 RED: assert metadata-only rows (`branch-pr`, `chained-pr`, `cognitive-doc-design`) and no-op dispositions cause no body rewrites; production: update only catalog/provenance.
- [x] 1.3 RED: assert generic copies (`comment-writer`, `go-testing` plus examples, `skill-creator` plus style guide) remain upstream-generic; production: update assets without Jarvis/Gentleman.Dots leakage; separately adapt `issue-creation` repository-neutrally.

## Phase 2: SDD and Shared Contracts

- [x] 2.1 RED: assert selective extraction for `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-onboard`, `sdd-archive`, `work-unit-commits`; production: update neutral language, persistence, deltas/migrations, threat/RED, runtime, rollback and authored/golden rules.
- [x] 2.2 RED: assert `sdd-init`, `sdd-apply`, `sdd-verify` compatible v3 mechanics, Hive/four stores, and negative no-review/remediation boundary; production: update those contracts only.
- [x] 2.3 RED: assert shared dispositions for `hive-convention`, `engram-convention`, `openspec-convention`, `persistence-contract`, `sdd-phase-common`, `skill-resolver`; assert no `_shared/SKILL.md`, ledger contract, or complete `sdd-status-contract.md`; production: adapt Hive, `.jarvis` canonical/`.atl` fallback, and only required current-state status-core while preserving Go-owned rows/persona separation.

## Phase 3: Hive Runtime Contracts (RED before each change)

- [x] 3.1 RED: `agent/{claude_sdd_agents,opencode}_test.go` and `sddruntime/store_contract_test.go` cover all phases × four stores, five exact Hive tools, unchanged filesystem permissions, and non-Hive modes; production: update `sdd_phase_agents.go`, `opencode.go`, templates, `store_contract.go`.
- [x] 3.2 RED: `agent/{runtime_opencode_parse,runtime_integration}_test.go` and `sddruntime/{verify,verify_opencode}_test.go` cover missing/stale grants, exact deny/ask, all stores, and no writes; production: classify FAIL for Hive/hybrid, advisory for others, with regeneration guidance.
- [x] 3.3 RED: `lifecycle/engine_verify_test.go` and `agent/lifecycle_adapter_test.go` cover both providers, drift, `ReadOnly:true`, guidance, unchanged temp-tree bytes; production: update `lifecycle/engine.go` and `agent/lifecycle_adapter.go`.
- [x] 3.4 RED: `skills/catalog_contract_test.go` asserts Hive/hybrid missing capability returns blocked (no inline artifact or permission broadening), while openspec/none continue; production: update phase/persistence contracts.

## Phase 4: Registry, Docs, and Verification

- [x] 4.1 RED: assert canonical `.jarvis` auto-refreshed gitignored cache, `.atl` warned read fallback/import, untracking, and migration compatibility; production: adapt registry/improver/resolver and `docs/{configuration,security-privacy,generated-artifacts}.md`.
- [x] 4.2 RED: assert source-only editing, `AGENTS.md.tmpl`/`CLAUDE.md.tmpl` parity, full install/regeneration and installed drift across agents; production: update source/templates only, never generated user files.
- [x] 4.3 RED: assert every deferred authority remains absent (ledger, transaction, snapshot, receipt, reviewGate, remediation/generation, exit-125, semantic validity, positive review routing); do not implement deferred authorities. Run focused checks, `go test ./...`, `go vet ./...`; record one-PR work-unit commits and rollback via full revert/regeneration.
