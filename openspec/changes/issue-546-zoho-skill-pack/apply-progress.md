# Apply Progress — PR 1 catalog and setup convergence

## Status

```yaml
schemaName: gentle-ai.sdd-status
changeName: issue-546-zoho-skill-pack
artifactStore: hybrid
changeRoot: /home/andres/Desarrollo/Proyectos/jarvis-dev-issue-546-pr1/openspec/changes/issue-546-zoho-skill-pack
applyState: ready
nextRecommended: apply
actionContext:
  mode: repo-local
  workspaceRoot: /home/andres/Desarrollo/Proyectos/jarvis-dev-issue-546-pr1
  allowedEditRoots:
    - /home/andres/Desarrollo/Proyectos/jarvis-dev-issue-546-pr1
warnings:
  - "Maintainer-approved bounded exception permits PR 1 up to 600 source additions+deletions; PR 2 remains forbidden until PR 1 merges."
```

The native status was consumed before implementation. The worktree was confirmed as `feat/issue-546-zoho-pack-setup`, with `a8aa71204919fd1382e1692cad6a748e4c5755d7` an ancestor of `HEAD`.

## Completed implementation tasks

- Completed and checked every implementation-owned RED/GREEN/TRIANGULATE/REFACTOR row in Tasks 1 and 2.
- Completed and checked the PR 1 changed-file `gofmt -l` verification row.
- Completed and checked the PR 1 independent focused and full-suite verification row after the maintainer approved the bounded 600-line exception.

## Files changed

- `jarvis-cli/internal/skills/zoho_pack.go` (new)
- `jarvis-cli/internal/skills/zoho_pack_test.go` (new)
- `jarvis-cli/internal/skills/interactive.go`
- `jarvis-cli/internal/skills/interactive_test.go`
- `jarvis-cli/internal/tui/skills_selection.go`
- `jarvis-cli/internal/tui/steps.go`
- `jarvis-cli/internal/tui/nontui.go`
- `jarvis-cli/internal/tui/model_test.go`
- `jarvis-cli/internal/tui/nontui_test.go`
- `openspec/changes/issue-546-zoho-skill-pack/tasks.md`
- `openspec/changes/issue-546-zoho-skill-pack/apply-progress.md`

## Verification evidence

| Command | Outcome |
| --- | --- |
| `cd jarvis-cli && go test ./internal/skills && go test ./internal/tui` | PASS — safety-net baseline before edits |
| `cd jarvis-cli && go test ./internal/skills` after initial catalog-contract tests | RED — `NewZohoPack` undefined |
| `cd jarvis-cli && go test ./internal/skills` after `ZohoPack` implementation | GREEN — PASS |
| `cd jarvis-cli && go test ./internal/skills` after future-Zoho classifier test | RED — `zoho-expense` incorrectly non-interactive |
| `cd jarvis-cli && go test ./internal/skills` after classifier implementation | GREEN — PASS |
| `cd jarvis-cli && go test ./internal/skills ./internal/sync` | PASS |
| `cd jarvis-cli && go test ./internal/tui` after setup tests | RED — `selectedSkillIDs` undefined |
| `cd jarvis-cli && go test ./internal/tui -run 'Test(BuildSkillSelectionPlan_OnlyPromptsStackSpecificSkills|ZohoSkillPrompt_TogglesAllPackMembersTogether|SelectedSkillIDsPreservesUnrelatedSkillsAndReducesZohoPack)$'` | GREEN — PASS |
| `cd jarvis-cli && go test ./internal/tui -run 'Test(BuildSkillSelectionPlan_OnlyPromptsStackSpecificSkills|ZohoPackPromptIncludesFutureMemberAndIgnoresOrphanDefault|ZohoSkillPrompt_TogglesAllPackMembersTogether|SelectedSkillIDsPreservesUnrelatedSkillsAndReducesZohoPack|SelectedSkillIDsFreshV0SelectionIsDeterministic)$'` | TRIANGULATE — PASS |
| `cd jarvis-cli && go test ./internal/skills ./internal/tui` | PASS after refactor/format |
| `gofmt -l` over all nine changed Go files | PASS — no output before this continuation |
| `cd jarvis-cli && go test ./internal/skills ./internal/tui` | PASS — PR 1 focused regression evidence on the unchanged candidate |
| `cd jarvis-cli && go test ./...` | PASS — PR 1 full `jarvis-cli` regression evidence (run exactly once in this continuation) |

No source file was edited in this continuation, so `gofmt` was not run again. The prior changed-file `gofmt -l` evidence remains valid; no source bytes changed after it.

## TDD Cycle Evidence

| Task | Test files | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1. Catalog contract | `internal/skills/zoho_pack_test.go`, `interactive_test.go` | Unit | PASS | Undefined `NewZohoPack`; then future-ID classifier failure | PASS | Unsorted/duplicate catalog, future member, anchor/orphan, select/deselect/idempotency cases | `gofmt`; `internal/skills ./internal/sync` PASS |
| 2. Setup convergence | `internal/tui/model_test.go`, `nontui_test.go` | Unit / direct Bubbletea `Model.Update` | PASS | Undefined `selectedSkillIDs` | PASS | V0 order, future member, orphan default, fresh select/unselect, deterministic reducer and deselection cases | `gofmt`; `internal/skills ./internal/tui` PASS |
| 5. PR 1 regression evidence | Existing `internal/skills` and `internal/tui` tests; full `jarvis-cli` suite | Regression verification | PASS | N/A — no source change | PASS — focused and full suites | N/A — no new behavior | N/A — no source edit |

## Workload / PR boundary

- Delivery: stacked-to-main, PR 1 only (`catalog + TUI/non-TUI convergence`).
- Maintainer authorization: bounded size exception, maximum **600 source additions+deletions** for PR 1.
- `git diff --numstat` for tracked source changes after remediation: 173 additions, 86 deletions.
- New source/test files: 182 additions (`zoho_pack.go`: 95; `zoho_pack_test.go`: 87).
- Candidate source total: **355 additions + 86 deletions = 441 additions+deletions**.
- Budget evidence: **159 lines within** the authorized 600-line ceiling.
- Delivery decision: include all OpenSpec planning and progress artifacts in PR 1; this documentation-inclusive exception does not expand implementation scope.
- No production or test source files changed in this continuation. No commit, push, PR creation, sync replay/bookkeeping/reporting production behavior, or issue #547 work was performed.

## Remaining tasks

PR 1 implementation and regression evidence are complete. PR 2 remains prohibited until PR 1 merges to `master`.

Unchecked implementation-owned rows:
- [ ] RED — Add failing `internal/sync` tests, using `t.TempDir()` for state fixtures, that require bookkeeping to run only after `verifyApplied` succeeds, including a verified no-op expanded plan that skips backup/application but may persist missing IDs; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] RED — Add failing bookkeeping tests for a fresh locked re-read that preserves concurrent unrelated state, skips all Zoho mutation after concurrent anchor removal, returns no additions on lock/save failure, and reports no additions on a second successful commit; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] GREEN — In `cmd_sync.go`, build `ZohoPack` from the embedded catalog, expand a shallow copied `state.State` before creating `ReplayInput`, and pass that one copied desired view through rendering, plan, runner, backup, and verification; run `cd jarvis-cli && go test ./cmd/jarvis ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] GREEN — Extend `Bookkeeping` with the explicit Zoho expansion payload and a locked `state.Load`/single-`state.Save` merge that rechecks anchor eligibility, returns only IDs actually made durable, and never uses `config.Save`, `state.Update`, or a generic pack framework; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] GREEN — Refactor `sync.Run` in `backup.go` so final verification occurs on both applied and already-current paths before bookkeeping, expose `Verified` and successful `AddedSkillIDs`, and ensure planning, blocked/partial application, mode/snapshot, and verification failures leave durable Zoho state unchanged; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] TRIANGULATE — Add the focused assertion that replay expansion puts all selected pack files into `Plan.Tracked`, while relying on existing idempotency, overwrite, backup, mode, and symlink suites as the regression proof for managed-file safety; run `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] REFACTOR — Remove duplicated success-path bookkeeping without changing established atomic overwrite, symlink refusal, or final-file safety behavior; format changed Go files and rerun `cd jarvis-cli && go test ./internal/sync`. <!-- sdd-owner: implementation -->
- [ ] RED — Add failing `cmd_sync_test.go` cases requiring one deterministic line per newly durable ID, exclusion of already-durable and uncommitted IDs, and `verification: passed` plus a distinct state-persistence failure when bookkeeping fails after verification; run `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] GREEN — Render added-ID output in `cmd_sync.go` only from successful `RunResult.AddedSkillIDs`, preserving transaction order and retaining existing flagless, non-interactive, flag-rejection, agent, backup, changed-path, and Hive reporting; run `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] TRIANGULATE — Extend only the existing OpenCode command fixture so a legacy `zoho-deluge` manifest converges and persists all V0 members, reports each missing member once, and reports none on a second run; do not add nested-reference assertions, inspect Zoho skill content, invoke Claude runtime, or claim Claude Code/OpenCode parity owned by issue #547; run `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] REFACTOR — Keep reporting dependent on confirmed durable transaction output rather than plans/candidates, format changed Go files, and rerun `cd jarvis-cli && go test ./cmd/jarvis`. <!-- sdd-owner: implementation -->
- [ ] Verify PR 2 independently with `cd jarvis-cli && go test ./internal/sync ./cmd/jarvis`, then `cd jarvis-cli && go test ./...`; record final verification, persistence, concurrency, reporting, and existing file-safety regression evidence. <!-- sdd-owner: implementation -->

Deferred lifecycle action (unchanged parent-owned row):

- [ ] After apply evidence is available, start or reuse bounded review and confirm the selected slice matches its task boundary, review budget handling, strict-TDD evidence, and issue #547 exclusion. <!-- sdd-owner: parent -->

## Deviations

None from Tasks 1–2 design. The prior 400-line stop was resolved by the maintainer's bounded 600-line exception; PR 2 remains out of scope until PR 1 merges.

## Bounded PR 1 correction

- Native status consumed: `applyState: ready`; repo-local root and allowed edit root are this worktree. The parent owns active attempt `sha256:3042b2bf5b99ad3705088d063506ec9b3c5a7d28884e83cd4140653a3b4a9bc3` and settlement.
- Fixed `selectedSkillIDs`: selected current non-Zoho entries are reduced by selected/core state; only `zoho-*` entries remain exclusively controlled by `ZohoPack.ApplySelection`.
- Added the narrow `go-testing` current-catalog regression to `internal/tui/nontui_test.go`.

| TDD cycle | Evidence |
| --- | --- |
| RED | `cd jarvis-cli && go test ./internal/tui` failed: selected current `go-testing` was absent from reducer output. |
| GREEN | Changed `skills_selection.go`; `cd jarvis-cli && go test ./internal/tui ./internal/skills` passed. |
| TRIANGULATE | Existing selected/deselected Zoho and catalog-absent preservation cases still pass in the same table. |
| REFACTOR | `gofmt -w` on the two edited Go files; changed-file `gofmt -l` and `git diff --check` passed. |

- `cd jarvis-cli && go test ./...` passed once after GREEN.
- Correction source delta from the received candidate: 9 additions + 8 deletions = 17 lines; this concise progress merge is within the 80-line correction budget.
- No task checkbox changed: this is a bounded remediation of already-completed PR 1 work; PR 2 and parent lifecycle rows remain deferred unchanged.

## PR 2 — Sync Expansion, Verified Persistence, and Reporting

- Native status consumed: `applyState: ready`, `nextRecommended: apply`, hybrid store, and the dedicated PR 2 worktree was the allowed repo-local edit root.
- The maintainer authorized a PR 2 **overall** exception of at most 450 additions+deletions solely to reconcile Tasks and Apply Progress; this continuation changes no production or test code.
- Completed and checked all implementation-owned Tasks 3–4 and the PR 2 focused/full-suite verification row. The parent-owned delivery/review row remains unchanged and unchecked.

### TDD Cycle Evidence

| Cycle | Preserved implementation evidence |
| --- | --- |
| RED | `cd jarvis-cli && go test ./internal/sync ./cmd/jarvis` failed before `RunResult.Verified`, `RunResult.AddedSkillIDs`, `ZohoExpansion`, and post-verification recording existed. |
| GREEN | Copied-state expansion, verified persistence, and durable reporting made the focused command pass. |
| TRIANGULATE | Verified no-op persistence, fresh locked re-read, concurrent anchor removal, plan tracking, and legacy two-run idempotency coverage passed. |
| REFACTOR | Changed-Go-file `gofmt -l` produced no output; `git diff --check` and the full suite passed. |

Documentation reconciliation added no RED/GREEN cycle and did not modify Go code; it preserves the prior strict-TDD evidence.

### PR 2 verification and scope

- `gofmt -l` on the six changed Go files: PASS (no output); `git diff --check HEAD`: PASS.
- `cd jarvis-cli && go test ./internal/sync ./cmd/jarvis`: PASS.
- `cd jarvis-cli && go test ./...`: PASS.
- Changed source/test paths: `cmd/jarvis/cmd_sync.go`, `cmd/jarvis/cmd_sync_test.go`, `cmd/jarvis/cmd_sync_e2e_test.go`, `internal/sync/backup.go`, `internal/sync/bookkeeping.go`, and `internal/sync/bookkeeping_test.go`.
- The maintainer handoff recorded a 372-line PR 2 implementation measurement. The live pre-artifact source/test `git diff HEAD --numstat` is 286 additions + 43 deletions = 329; the historical 329+43 arithmetic label is retained only as handoff context.
- Remaining unchecked row: `- [ ] After apply evidence is available, start or reuse bounded review and confirm the selected slice matches its task boundary, review budget handling, strict-TDD evidence, and issue #547 exclusion. <!-- sdd-owner: parent -->`
- Deferred lifecycle action: parent-owned delivery/review only. Issue #547 remains out of scope.

### PR 2 independent-validation correction

- Native status: `applyState: ready`, `nextRecommended: apply`; `actionContext.mode: repo-local` permitted this worktree only. Parent-owned attempt token was not acquired or settled here.
- Corrected invariant: inside the existing lock and after fresh anchor revalidation, `Bookkeeping.record` derives the complete current pack from fresh `latest.Skills` and returns only lexicographically ordered IDs newly made durable by that successful save.
- RED: focused sync test failed with stale additions `[zoho-analytics]`, missing the concurrently removed `zoho-books`; focused failure-boundary tests were added first.
- GREEN: fresh-state diff now drives both persistence and `AddedSkillIDs`; lock/save failures return no additions; partial application never reaches persistence.
- TRIANGULATE: concurrent deselection, verified no-op, idempotent second-run, and command two-run coverage passed.
- REFACTOR: factored shared isolated Zoho-manifest setup; Go files formatted with no unrelated edits.
- Verification: `go test ./internal/sync ./cmd/jarvis` PASS; `go test ./...` PASS once; `git diff --check` PASS.
- Files changed by this correction: `internal/sync/bookkeeping.go`, `bookkeeping_test.go`, `backup_test.go`, `cmd/jarvis/cmd_sync_test.go`, and this progress artifact. Tasks remain unchanged because every implementation-owned row was already checked.
- Budget: correction-only net delta from the 380-line pre-correction candidate is **155 additions+deletions**; final candidate is **480 additions + 55 deletions = 535**, within the 170-line correction cap and 550-line ceiling.
