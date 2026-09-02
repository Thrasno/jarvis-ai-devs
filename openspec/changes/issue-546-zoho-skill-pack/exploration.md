# Exploration: `issue-546-zoho-skill-pack`

## Executive finding

Issue #546 is an approved, bounded selection-and-replay change, not a request to add or alter the embedded Zoho skill content. The repository already embeds all seven V0 IDs, but the current selection model only groups `zoho-deluge` and `zoho-crm`; the remaining embedded Zoho skills are treated as ordinary auto-installed optional skills. This violates both pack completeness and deselection semantics.

The recommended direction is a catalog-derived Zoho-pack helper, used by both wizard selection paths and replay migration. It must treat every embedded `zoho-*` ID as a pack member, preserve a deterministic membership order, expand the released legacy `zoho-deluge` manifest only in memory for planning, and commit the expanded desired state only after a converged sync. No production code was changed during exploration.

## Fixed product decisions

- The wizard exposes exactly one interactive choice: **Zoho Skills Pack**. It never offers per-Zoho-skill choices.
- Membership is every current and future embedded catalog ID beginning `zoho-`.
- V0 members are `zoho-deluge`, `zoho-crm`, `zoho-books`, `zoho-people`, `zoho-projects`, `zoho-creator`, and `zoho-analytics`.
- A fresh selected pack persists every concrete current member in deterministic order.
- The only released legacy selected state is `zoho-deluge`. A successful `jarvis sync` expands it to the current pack and persists every installed member. A failed or blocked run leaves the prior desired state unchanged.
- Future embedded `zoho-*` members are installed and persisted only after a successful convergence.
- Deselecting the pack removes every Zoho ID from desired state and stops future management, but does not uninstall copies already present.
- Managed selected files retain existing atomic/idempotent overwrite behavior and symlink refusal.
- `jarvis sync` remains flagless and noninteractive and must not call `config.Save()` because manifest locking is non-reentrant.
- #547 owns final nested-reference and Claude/OpenCode E2E parity; this change must not duplicate or preempt that scope.

## Verified current implementation and failures

### Catalog and selection

- `jarvis-cli/internal/skills/registry.go` discovers embedded `SKILL.md` files under `embed/skills` and derives IDs from directories. Existing contract tests prove the V0 application skills are embedded and individually installable.
- `jarvis-cli/internal/tui/skills_selection.go` hard-codes the Zoho prompt to only `zoho-deluge` and `zoho-crm`. It includes only template IDs that exist in the catalog.
- `jarvis-cli/internal/skills/interactive.go` marks only those two Zoho IDs as interactive. `buildSkillSelectionPlan` auto-selects every non-core skill that is not interactive.
- Consequently, `zoho-books`, `zoho-people`, `zoho-projects`, `zoho-creator`, and `zoho-analytics` are currently auto-selected on a fresh install, while the visible Zoho pack controls only two IDs. A pack deselection therefore cannot remove all Zoho desired-state IDs.
- `jarvis-cli/internal/tui/nontui.go` uses the same `buildSkillSelectionPlan` and applies one answer to each prompt member, so the defect affects both TUI and non-TUI setup.
- `jarvis-cli/internal/tui/steps.go:buildSelectedIDs` persists selected IDs in catalog traversal order. The implementation needs an explicit deterministic Zoho ordering rule rather than relying on embed traversal incidentally.

### Desired state and wizard persistence

- `jarvis-cli/internal/state/state.go` makes `State.Skills` the durable desired-state list in `~/.jarvis/state.yaml`; it deliberately retains catalog-absent IDs as ownership proof.
- The wizard writes selected IDs through `recordWizardDesiredState` in `internal/tui/model.go`, which uses `state.Update` under the manifest lock. TUI and no-TUI paths respectively assign `manifest.Skills` from `buildSelectedIDs` and their equivalent selected-ID loop.
- The installer only writes selected/core files. It does not remove unselected skill directories; this already matches the requested non-uninstall deselection behavior.

### Sync and transaction boundary

- `cmd/jarvis/cmd_sync.go:runSync` first calls `state.Migrate`, loads the manifest, builds a `sync.ReplayInput`, then plans and runs replay. `newSyncCommand` rejects every supplied flag, and the command has no prompts.
- `replayInput` reads the embedded catalog and builds instruction-file `SkillInfo` from `manifest.Skills`; the planner and runner receive the same `ReplayInput`. An expansion must therefore occur before `replayInput` renders desired assets, but initially only in an in-memory copy of the manifest.
- `internal/sync/plan.go:renderSkills` renders every `State.Skills` member for each configured agent and tracks each rendered file. `internal/sync/runner.go` passes the same IDs to `agentapply.ConfigureAgent`, which installs selected skills.
- `internal/sync/backup.go:Run` measures the tracked plan, backs it up before mutation, applies, enforces modes, takes a closing snapshot, attributes changes, and invokes bookkeeping. This is the correct convergence boundary for committing an expanded pack.
- `internal/sync/bookkeeping.go` currently records only `ManagedAssetDigest`, under `state.WithLock`, after re-reading the manifest. It has no facility to commit upgraded skill IDs. A new pack-commit path must re-read under the lock and preserve concurrent non-Zoho state; it must not call `config.Save()`.
- Existing bookkeeping is invoked before `verifyApplied` is returned from `Run`. Design must explicitly establish that Zoho desired-state expansion is conditional on a successful final verification/converged report, not merely on a closing snapshot.

### Installation and ownership behavior

- `internal/skills/installer.go` recursively selects trees by top-level ID, preserves `_shared`, skips byte-identical files, atomically renames changed files, replaces a final-file symlink without following it, and rejects symlink directories/ancestors. These behaviors already satisfy the selected-file requirements.
- `sync.Plan.Tracked` is the shared source for backup and measurement; adding pack members through the existing renderer includes every managed embedded file in both protections.
- Generated agent skill copies and instruction files are outputs. The source of truth remains `jarvis-cli/embed/skills` and the manifest; do not edit user-machine generated files.

## Exact seams and focused test targets

| Concern | Primary source seam | Existing test seam | Required new coverage |
| --- | --- | --- | --- |
| Catalog-derived pack membership | `internal/skills/interactive.go`, `internal/tui/skills_selection.go` | `internal/skills/interactive_test.go`, `internal/tui/model_test.go` | V0 membership; future `zoho-*` discovery; non-Zoho exclusion; deterministic order; one prompt only |
| TUI selection/deselection | `internal/tui/skills_selection.go`, `internal/tui/steps.go` | `internal/tui/model_test.go` | toggle selects/removes all current members while preserving unrelated selections; fresh selected persistence order |
| No-TUI parity | `internal/tui/nontui.go` | `internal/tui/nontui_test.go` | one answer controls the entire pack with identical desired IDs |
| Legacy/future sync migration | `cmd/jarvis/cmd_sync.go`, `internal/sync/runner.go`, `internal/sync/bookkeeping.go` | `cmd/jarvis/cmd_sync_e2e_test.go`, `internal/sync/bookkeeping_test.go` | legacy Deluge plans all current members; success commits expanded IDs; failed/blocked sync retains prior IDs; future catalog member converges then persists |
| File safety/idempotency | `internal/skills/installer.go`, `internal/sync/plan.go` | `internal/skills/installer_test.go`, `internal/sync/idempotency_test.go` | reuse existing guarantees; add only scoped evidence needed for pack-driven selection |

Tests should be table-driven, use `t.TempDir()` for filesystem state, and assert state transitions and observable files rather than helper internals. Run the narrow `jarvis-cli` package tests before the module suite. Final nested-reference and cross-agent E2E parity remains #547 work.

## Recommended bounded architecture

1. Introduce one cohesive pack-membership/normalization API near the embedded skill catalog, not separate hard-coded lists in TUI, no-TUI, and sync. It should filter current catalog IDs by the `zoho-` prefix and return deterministic IDs. Preserve the supplied V0 order explicitly if it is the required compatibility order; append future IDs deterministically.
2. Make the wizard construct the single Zoho prompt from that API and classify every `zoho-*` member as pack-controlled rather than auto-installed. The remaining PHP and Go prompts retain their current independent behavior.
3. Make fresh selection store all current pack IDs in the defined order. On pack deselection, remove all current/recorded Zoho IDs from desired state while leaving installation copies untouched.
4. In sync, recognize legacy selected state through `zoho-deluge`, produce an expanded in-memory manifest before instruction rendering/planning, and replay from that one expanded value.
5. Extend the existing post-convergence bookkeeping transaction to atomically persist the expanded IDs only when the run has fully converged and final verification succeeds. Re-read the manifest while holding `state.WithLock`; preserve unrelated skills and respect a concurrent pack deselection rather than resurrecting it. Do not use `config.Save()`.

This reuses catalog discovery, planning, installer, backup, mode assertion, and state locking instead of adding a generic installer or agent abstraction.

## Viable alternatives

- **Preferred: catalog-derived pack helper plus conditional sync bookkeeping.** Correctly includes future IDs and keeps all three callers aligned. It is localized to selection/state/replay seams.
- **Hard-code the seven V0 IDs in the wizard and sync.** Smaller immediately, but fails the fixed future-member requirement and recreates the current drift hazard. Reject.
- **Persist expansion before replay.** Simpler, but violates the requirement that blocked/failed sync retain the prior desired state. Reject.
- **Uninstall deselected skill directories.** Conflicts with established installer behavior and explicit product semantics. Reject.

## Likely changed files

Production changes are likely limited to:

- `jarvis-cli/internal/skills/interactive.go` and/or a new cohesive catalog/pack helper in that package
- `jarvis-cli/internal/tui/skills_selection.go`
- `jarvis-cli/internal/tui/steps.go`
- `jarvis-cli/internal/tui/nontui.go`
- `jarvis-cli/cmd/jarvis/cmd_sync.go`
- `jarvis-cli/internal/sync/backup.go` and `jarvis-cli/internal/sync/bookkeeping.go` (only if needed to make the successful-convergence commit atomic)

Likely tests:

- `jarvis-cli/internal/skills/interactive_test.go`
- `jarvis-cli/internal/tui/model_test.go`
- `jarvis-cli/internal/tui/nontui_test.go`
- `jarvis-cli/internal/sync/bookkeeping_test.go`
- focused additions to `jarvis-cli/cmd/jarvis/cmd_sync_e2e_test.go`

The proposal/design phase should minimize this list after deciding the exact API and transaction proof.

## Risks and technical unknowns for proposal/design

1. **Deterministic order contract:** product fixes V0 members but does not explicitly say whether their listed order or lexical order is canonical. Design should select and test one stable policy; explicit V0 order plus sorted future members is the safest compatibility choice.
2. **Concurrent manifest mutation:** the success-time transaction must not overwrite unrelated skills or reverse a simultaneous pack deselection. Define the exact eligibility check against the freshly re-read manifest.
3. **Success definition:** `Run` currently records bookkeeping before its final verification error is returned. Define the precise point at which pack expansion may commit and prove failure/partial outcomes leave `State.Skills` untouched.
4. **Legacy marker scope:** only `zoho-deluge` is released legacy state. Do not infer pack enrollment from arbitrary orphan `zoho-*` IDs without an approved compatibility rule.
5. **Review size:** selection, sync, and test changes may approach the 400-line review budget. Tasks must forecast actual changed lines and request the cached `ask-on-risk` decision if needed.

## Non-goals

- No new or changed Zoho skill content, remote Zoho calls, credentials, or runtime integration.
- No generated agent configuration or user-machine file edits.
- No new generic installer/router framework.
- No changes to flagless/noninteractive sync semantics.
- No final nested-reference or Claude/OpenCode E2E parity work owned by #547.

## Ready for proposal

Yes. Proposal should state the fixed pack contract, its safe migration/convergence boundary, and the explicit #547 ownership boundary. Design must resolve the deterministic-order and success-time transaction details before tasks are written.
