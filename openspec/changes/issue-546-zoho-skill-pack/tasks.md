# Implementation Tasks: Zoho Skills Pack

## Review Workload Forecast

| Field | Value |
| ------- | ------- |
| Estimated production lines | 265–330 |
| Estimated test lines | 390–470 |
| Estimated artifact/generated lines | 0 |
| Estimated changed lines | 655–800 total review lines (additions + deletions) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (catalog + TUI/non-TUI convergence) → PR 2 (sync expansion + post-verification persistence + reporting) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: No — resolved by the maintainer
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

The maintainer selected two chained PRs using `stacked-to-main`. PR 1 must be merged to `master` before PR 2 begins.

### Maintainer-approved PR 1 exceptions

- The cohesive PR 1 source-and-test slice is authorized up to 600 changed lines; its validated actual size is 441 lines.
- All OpenSpec planning and progress artifacts are explicitly included in PR 1 rather than delivered separately or retained only locally.
- At the delivery decision point, the OpenSpec artifacts contain 1,153 lines, making the complete PR 1 candidate approximately 1,594 changed lines.
- This documentation-inclusive exception does not expand implementation scope and does not authorize PR 2 work before PR 1 merges.

## Resolved Review Slices

- **PR 1 — catalog + setup convergence** (estimated 300–370 lines): begins from `main`; target is `main` for `stacked-to-main`, or the feature/tracker branch for `feature-branch-chain`. It delivers the catalog-derived `ZohoPack`, one pack prompt, and identical TUI/non-TUI concrete desired-state reduction. It is independently testable with `internal/skills` and `internal/tui` package tests.
- **PR 2 — replay convergence + durable reporting** (estimated 355–395 lines): depends on PR 1. For `stacked-to-main`, it begins after PR 1 merges and targets `main`; for `feature-branch-chain`, it begins from PR 1 and targets the immediate PR 1 branch. It delivers copied-state replay expansion, post-verification locked persistence, concurrency protection, and command reporting. It is independently testable with `internal/sync`, `cmd/jarvis`, and the complete `jarvis-cli` suite.

Each slice keeps behavior, tests, and regression evidence together. Roll back PR 2 without deleting installed files or contracting persisted concrete IDs; roll back PR 1 without changing unrelated desired-state IDs. Issue #547 nested-reference behavior and Claude Code/OpenCode reference or parity work are explicitly out of scope for both slices.

## PR 1 — Catalog Contract and Setup Convergence

### 1. Catalog-derived Zoho contract and interactive classification

**Dependencies:** None.
**Allowed edit surfaces:** `jarvis-cli/internal/skills/zoho_pack.go`, `jarvis-cli/internal/skills/zoho_pack_test.go`, `jarvis-cli/internal/skills/interactive.go`, and `jarvis-cli/internal/skills/interactive_test.go`; add focused assertions only in `jarvis-cli/internal/sync/ownership_test.go` if its existing ownership fixture is the narrow regression seam.
**Expected evidence:** table-driven tests prove sorted, unique catalog membership; future `zoho-*` inclusion; non-Zoho exclusion; defensive member copies; `zoho-deluge` as the sole enrollment anchor; deterministic select/deselect behavior; preservation of unrelated IDs; idempotency; and unchanged non-Zoho interactive classification.
**Rollback boundary:** Revert this work unit to remove the pack value and restore prior classifier behavior; no durable state or filesystem migration is performed by this unit.

- [x] RED — Add failing table-driven `internal/skills` tests for the V0 ordered member list, duplicate/unsorted synthetic catalog entries, a future `zoho-expense` entry, non-Zoho exclusion, and defensive-copy semantics; run `cd jarvis-cli && go test ./internal/skills`. <!-- sdd-owner: implementation -->
- [x] RED — Add failing `ZohoPack` selection/expansion cases proving that only `zoho-deluge` is eligible, isolated non-anchor `zoho-*` IDs do not enroll the pack, selection preserves catalog-absent and unrelated IDs, deselection removes every recorded Zoho ID, and repeat operations are duplicate-free; run `cd jarvis-cli && go test ./internal/skills`. <!-- sdd-owner: implementation -->
- [x] GREEN — Implement `ZohoPack`, the shared private prefix predicate, sorted/deduplicated catalog membership, selection reduction, and expansion in `jarvis-cli/internal/skills/zoho_pack.go`; run `cd jarvis-cli && go test ./internal/skills`. <!-- sdd-owner: implementation -->
- [x] TRIANGULATE — Add focused interactive/ownership regression cases for a future catalog Zoho ID and existing non-Zoho interactive IDs, then update `IsInteractive` to delegate every `zoho-*` decision to the shared prefix rule while retaining only non-Zoho static entries; run `cd jarvis-cli && go test ./internal/skills ./internal/sync`. <!-- sdd-owner: implementation -->
- [x] REFACTOR — Simplify duplicate filtering/order helpers only within the allowed surfaces, run `gofmt` on changed Go files, and rerun `cd jarvis-cli && go test ./internal/skills ./internal/sync`. <!-- sdd-owner: implementation -->

### 2. One pack prompt and shared concrete desired-state reducer

**Dependencies:** Task 1.
**Allowed edit surfaces:** `jarvis-cli/internal/tui/skills_selection.go`, `jarvis-cli/internal/tui/steps.go`, `jarvis-cli/internal/tui/nontui.go`, `jarvis-cli/internal/tui/model_test.go`, and `jarvis-cli/internal/tui/nontui_test.go`.
**Expected evidence:** direct `Model.Update` and reducer tests prove one `Zoho Skills Pack` prompt carrying all V0 IDs in lexicographic order; no individual Zoho prompt/control; all current/future Zoho IDs excluded from optional auto-selection; TUI/non-TUI parity; selected and deselected outcomes preserve unrelated IDs; selected Zoho IDs persist deterministically; and orphan non-anchor state does not default the pack on.
**Rollback boundary:** Revert this work unit to restore the prior setup reduction without deleting installed files; only later user setup actions may alter desired state.

- [x] RED — Add failing `model_test.go` cases for exactly one Zoho pack prompt, its ordered V0 IDs, direct `Model.Update` toggling of all pack IDs, and preservation of unrelated selection-map entries; run `cd jarvis-cli && go test ./internal/tui`. <!-- sdd-owner: implementation -->
- [x] RED — Add failing table-driven reducer and non-TUI parity cases for fresh selected/unselected setup, existing pack deselection, unrelated and catalog-absent non-Zoho preservation, ordered concrete Zoho persistence, and an orphan non-anchor that does not default the pack on; run `cd jarvis-cli && go test ./internal/tui`. <!-- sdd-owner: implementation -->
- [x] GREEN — Derive the single prompt from `ZohoPack.MemberIDs`, exclude all Zoho catalog entries from independent optional selection, and implement the shared `selectedSkillIDs` reducer in `skills_selection.go`; run `cd jarvis-cli && go test ./internal/tui`. <!-- sdd-owner: implementation -->
- [x] GREEN — Route `buildSelectedIDs` in `steps.go` and the non-TUI final selection/persistence path in `nontui.go` through the same reducer, removing unordered map-based desired-state persistence; run `cd jarvis-cli && go test ./internal/tui`. <!-- sdd-owner: implementation -->
- [x] TRIANGULATE — Extend the prompt/reducer fixture with a future `zoho-*` catalog member and verify both setup paths produce the same concrete outcome without adding a per-application control; run `cd jarvis-cli && go test ./internal/tui`. <!-- sdd-owner: implementation -->
- [x] REFACTOR — Keep prompt construction and reduction cohesive without a generic pack framework, format changed Go files, and rerun `cd jarvis-cli && go test ./internal/skills ./internal/tui`. <!-- sdd-owner: implementation -->

## PR 2 — Sync Expansion, Verified Persistence, and Reporting

### 3. Expanded replay input and post-verification bookkeeping transaction

**Dependencies:** PR 1 / Tasks 1–2.
**Allowed edit surfaces:** `jarvis-cli/cmd/jarvis/cmd_sync.go`, `jarvis-cli/internal/sync/backup.go`, `jarvis-cli/internal/sync/bookkeeping.go`, `jarvis-cli/internal/sync/bookkeeping_test.go`, `jarvis-cli/internal/sync/backup_test.go`, and focused plan-tracking assertions in the existing applicable `jarvis-cli/internal/sync/*_test.go` file. Do not edit `state/state.go`, `sync/plan.go`, `sync/runner.go`, `sync/verify.go`, installer code, embedded skill content, or generated user-machine files.
**Expected evidence:** replay uses one copied expanded state before rendering/planning; selected pack files reach `Plan.Tracked`; legacy anchor and future-member expansion are in-memory only until final verification; already-matching runs can commit verified additions without backup/apply; all pre-verification failures avoid locking/persistence; the locked fresh-state merge preserves unrelated changes; concurrent deselection prevents resurrection; save/lock failure yields no additions; and repeat success is idempotent.
**Rollback boundary:** Revert this unit to remove expansion/persistence behavior while leaving installed files and previously durable concrete IDs intact; retain existing backups as the recovery path.

- [ ] RED — Add failing `internal/sync` tests, using `t.TempDir()` for state fixtures, that require bookkeeping to run only after `verifyApplied` succeeds, including a verified no-op expanded plan that skips backup/application but may persist missing IDs; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] RED — Add failing bookkeeping tests for a fresh locked re-read that preserves concurrent unrelated state, skips all Zoho mutation after concurrent anchor removal, returns no additions on lock/save failure, and reports no additions on a second successful commit; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] GREEN — In `cmd_sync.go`, build `ZohoPack` from the embedded catalog, expand a shallow copied `state.State` before creating `ReplayInput`, and pass that one copied desired view through rendering, plan, runner, backup, and verification; run `cd jarvis-cli && go test ./cmd/jarvis ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] GREEN — Extend `Bookkeeping` with the explicit Zoho expansion payload and a locked `state.Load`/single-`state.Save` merge that rechecks anchor eligibility, returns only IDs actually made durable, and never uses `config.Save`, `state.Update`, or a generic pack framework; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] GREEN — Refactor `sync.Run` in `backup.go` so final verification occurs on both applied and already-current paths before bookkeeping, expose `Verified` and successful `AddedSkillIDs`, and ensure planning, blocked/partial application, mode/snapshot, and verification failures leave durable Zoho state unchanged; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] TRIANGULATE — Add the focused assertion that replay expansion puts all selected pack files into `Plan.Tracked`, while relying on existing idempotency, overwrite, backup, mode, and symlink suites as the regression proof for managed-file safety; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] REFACTOR — Remove duplicated success-path bookkeeping without changing established atomic overwrite, symlink refusal, or final-file safety behavior; format changed Go files and rerun `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->

### 4. Durable addition reporting and command-level convergence coverage

**Dependencies:** Task 3.
**Allowed edit surfaces:** `jarvis-cli/cmd/jarvis/cmd_sync.go`, `jarvis-cli/cmd/jarvis/cmd_sync_test.go`, and the existing `jarvis-cli/cmd/jarvis/cmd_sync_e2e_test.go` OpenCode fixture only.
**Expected evidence:** command tests prove lexicographic one-line reporting exclusively from successfully durable `AddedSkillIDs`, no reporting for pre-verification or state-persistence failure, idempotent second sync silence, unchanged flag rejection/non-interactive behavior, and legacy `zoho-deluge` convergence through the existing command fixture. The fixture is a command wiring regression only, not issue #547 parity evidence.
**Rollback boundary:** Revert reporting and fixture assertions without changing sync flags, prompts, filesystem contents, or durable state established by a prior successful run.

- [ ] RED — Add failing `cmd_sync_test.go` cases requiring one deterministic line per newly durable ID, exclusion of already-durable and uncommitted IDs, and `verification: passed` plus a distinct state-persistence failure when bookkeeping fails after verification; run `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] GREEN — Render added-ID output in `cmd_sync.go` only from successful `RunResult.AddedSkillIDs`, preserving transaction order and retaining existing flagless, non-interactive, flag-rejection, agent, backup, changed-path, and Hive reporting; run `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] TRIANGULATE — Extend only the existing OpenCode command fixture so a legacy `zoho-deluge` manifest converges and persists all V0 members, reports each missing member once, and reports none on a second run; do not add nested-reference assertions, inspect Zoho skill content, invoke Claude runtime, or claim Claude Code/OpenCode parity owned by issue #547; run `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] REFACTOR — Keep reporting dependent on confirmed durable transaction output rather than plans/candidates, format changed Go files, and rerun `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->

### 5. Slice and regression evidence

**Dependencies:** Tasks 1–4; execute after the resolved delivery boundary has selected the applicable slice.
**Allowed edit surfaces:** No additional production or test files; formatting changes are limited to Go files edited by Tasks 1–4.
**Expected evidence:** each slice remains within its stated estimate, contains its behavior and tests together, and passes focused tests before the full `jarvis-cli` regression suite. The complete suite confirms existing managed-file safety regressions remain covered; no tests are added for issue #547.
**Rollback boundary:** Revert the bounded slice only; do not add automated uninstall or desired-state contraction.

- [x] Verify PR 1 independently with `cd jarvis-cli && go test ./internal/skills ./internal/tui`, then `cd jarvis-cli && go test ./...`; record the focused and full-suite outcomes with the slice evidence. <!-- sdd-owner: implementation -->
- [ ] Verify PR 2 independently with `cd jarvis-cli && go test ./internal/sync ./cmd/jarvis`, then `cd jarvis-cli && go test ./...`; record final verification, persistence, concurrency, reporting, and existing file-safety regression evidence. <!-- sdd-owner: implementation -->
- [x] Run `gofmt -l` over the Go files changed by the selected slice and require no output before handoff; do not reformat unrelated files. <!-- sdd-owner: implementation -->

## Parent Delivery Gate

- [x] Resolved ask-on-risk delivery handling: two chained PRs using `stacked-to-main`; PR 1 must merge to `master` before PR 2 begins. No size exception was authorized. <!-- sdd-owner: parent -->
- [ ] After apply evidence is available, start or reuse bounded review and confirm the selected slice matches its task boundary, review budget handling, strict-TDD evidence, and issue #547 exclusion. <!-- sdd-owner: parent -->
