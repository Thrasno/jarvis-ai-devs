# Tasks: `jarvis sync` replays the last installation's desired state

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

| Field | Value |
|---|---|
| Estimated changed lines | ~2,900 (additions+deletions, TDD-inflated) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 → PR 5 → PR 6 |
| Delivery strategy | auto-chain |
| Chain strategy | pending — orchestrator resolves stacked-to-main vs feature-branch-chain before PR 1 lands |

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | `state.yaml` store, tri-state, migration, `config.yaml` v3 | PR 1 | `go test ./internal/state/... ./internal/config/...` | N/A — pure load/validate/migrate, no live agent | Revert `internal/state/`, config.go schema bump |
| 2 | Read-only planner: render, ownership, skill lifecycle | PR 2 | `go test ./internal/sync/... -run Plan\|Ownership` | N/A — planner never mutates | Revert `internal/sync/plan.go`, `ownership.go` |
| 3 | Extract `internal/agentapply` from `tui/agent_setup.go` | PR 3 | `go test ./internal/tui/... ./internal/agentapply/...` | `jarvis` wizard non-TUI smoke run | Revert extraction; `tui` keeps its own copy |
| 4 | Machine-scoped replay applier + component order | PR 4 | `go test ./internal/sync/... -run Apply\|Order` | `jarvis sync` against `t.TempDir()` home | Revert `internal/sync/apply.go`; sync stays no-op |
| 5 | Snapshot/diff, backup targets, verify, bookkeeping | PR 5 | `go test ./internal/sync/... ./internal/lifecycle/...` | Two consecutive `jarvis sync` runs, zero-diff assertion | Revert `snapshot.go`, backup.go target-list change |
| 6 | `cmd_sync.go` wiring, flag rejection, docs | PR 6 | `go test ./cmd/jarvis/...` | `jarvis sync`, `jarvis sync --dry-run` (expect usage error) | Revert `cmd_sync.go`; restore no-op message |

## Phase 1: Desired-State Store and Migration (PR 1)

- [x] 1.1 RED: `internal/state/state_test.go` — schema round-trip holds agents, skills, models, persona, scope, statusline (desired-state-manifest: State Store Schema).
- [x] 1.2 GREEN: `internal/state/state.go` — `StatuslineState{Decided, Enabled bool}` + full manifest struct, YAML load/save.
- [x] 1.3 RED: table-driven tri-state test — not-decided / decided-disabled / decided-enabled resolve correctly (Statusline Tri-State Consent).
- [x] 1.4 GREEN: tri-state resolution helper in `state.go`.
- [x] 1.5 RED: fail-closed load test — missing file OK, corrupt/incompatible-version/whitespace/unrecognized value all abort, zero writes (Fail-Closed State Load).
- [x] 1.6 GREEN: `Load()` validation in `state.go`.
- [x] 1.7 RED: `internal/state/migrate_test.go` — v2 `config.yaml` fixture → migration writes `state.yaml`, bumps `config.yaml` to `schema_version: 3`, fields absent from `config.yaml` after (One-Way Field Migration, Store Disjointness).
- [x] 1.8 RED: migration runs before validation early-return; notice withheld until write is durable (Migration precedes validation blocking; Notice withheld).
- [x] 1.9 GREEN: `internal/state/migrate.go` — one-way move, durable-write-gated notice, runs pre-return.
- [ ] 1.10 GREEN: `internal/config/config.go` — `schema_version: 3`, remove migrated fields. **BLOCKED — see Phase 1 Blocker below; deferred to its own slice.**
- [x] 1.11 Refactor: dedupe shared YAML helpers between `state.go`/`config.go` once green.

### Phase 1 Blocker: task 1.10 does not fit this slice

`state.Migrate()` strips the replay fields from `config.yaml` on disk, so the
store-level disjointness the spec requires is already satisfied and proven by
`TestMigrate_StoresAreDisjointAfterMigration`. Deleting the same fields from the
`AppConfig` **struct** is a separate, much larger change:

| Evidence | Value |
|---|---|
| Production references to the removed fields | ~200 across ~55 files (`internal/tui`, `internal/persona`, `internal/sddruntime`, `internal/agent`, `cmd/jarvis`) |
| Test references | ~200 more |
| Compile errors inside `internal/config` alone | 40 (`go build -gcflags=-e`) |
| PR 1 budget in this plan | ~600 lines, two paths |

It is also **not safe to land alone**: nothing reads `state.yaml` until PR 2-6, so
removing the fields from `AppConfig` today would regress persona, skill selection,
per-phase model assignment, and scope persistence for every existing user.

Consequence to schedule before `Migrate()` is wired into a command (PR 6): while
`internal/config` still persists these keys, a `config.Save()` after a migration
would rewrite them into `config.yaml` and break disjointness at runtime. The
consumer cutover onto `internal/state` is therefore a hard prerequisite for the
slice that calls `Migrate()`, not optional cleanup.

## Phase 2: Read-Only Planner (PR 2)

Split for the 400-line budget: **PR 2a** = ownership (2.3-2.8), **PR 2b** = target
rendering and the block path (2.1, 2.2, 2.9, 2.10). PR 2a resolved the
`interactiveSkillIDs` layering problem by moving the list from `internal/tui`
into `internal/skills.IsInteractive`, so `internal/sync` reads the same single
source without importing Bubbletea.

- [x] 2.1 RED: `internal/sync/plan_test.go` — rendered targets match installed binary's embedded assets only (Target Rendering).
- [x] 2.2 GREEN: `internal/sync/plan.go` — render `PlannedArtifact{Identity, Location, Bytes, Proof}` from embedded assets.
- [x] 2.3 RED: `internal/sync/ownership_test.go` — frontmatter `scope:` never decides ownership; only catalog/manifest membership (Identity-Based Ownership Classification).
- [x] 2.4 GREEN: `internal/sync/ownership.go` — `IdentityProof` two-list membership check.
- [x] 2.5 RED: table-driven skill lifecycle over all 5 rows (manifest×catalog×interactive) — update/delete/install/skip/untouched (Skill Lifecycle Rules).
- [x] 2.6 GREEN: skill lifecycle resolver in `ownership.go`.
- [x] 2.7 RED: manifest `skills` write never filters against catalog — dropped skill stays listed until deleted (Manifest Skills List Is Never Filtered on Write). **Already satisfied by PR 1** — `TestSave_RetainsSkillIDsAbsentFromCurrentCatalog` (`jarvis-cli/internal/state/state_test.go:141`).
- [x] 2.8 GREEN: manifest writer preserves unfiltered `skills` list. **Already satisfied by PR 1** — same test; `state.Save` writes `Skills` verbatim.
- [x] 2.9 RED: agent-less manifest blocks, names `jarvis` recovery command, zero writes (No Filesystem Redetection).
- [x] 2.10 GREEN: block path in `plan.go`.

## Phase 3: Extract `internal/agentapply` (PR 3 — lands before PR 4)

- [x] 3.1 Confirm `internal/tui` test suite is green before touching `agent_setup.go:77-202`.
- [x] 3.2 Move `configureWizardAgent` (`agent_setup.go:77-150`) and `reconcileWizardMCPs` (`:169-202`) verbatim into `internal/agentapply/apply.go`.
- [x] 3.3 `internal/tui/agent_setup.go` delegates to `internal/agentapply`; `configureWizardAgents` (live caller) unchanged in behavior.
- [x] 3.4 GREEN: re-run existing `internal/tui` tests unmodified — must stay green (regression gate for the extraction).
- [x] 3.5 RED: `internal/agentapply/apply_test.go` — statusline decision derived from tri-state is never `nil`, and `confirm()` is not invoked when undecided (Decided/Enabled table, claude.go:868).
- [x] 3.6 GREEN: statusline decision switch in `agentapply/apply.go` per design's tri-state snippet.

### Phase 3 PR boundary: one slice, two PRs

A verbatim move is counted twice by `git diff --numstat` (deletion at the source
plus addition at the destination), so Phase 3 cannot land as a single PR under
the 400-line budget. The two commits split exactly on that line and each is
autonomous:

| PR | Commit | Tasks | Changed lines |
|---|---|---|---|
| 3a | `refactor(agentapply): extract the wizard agent pipeline into its own package` | 3.1-3.4 | 354 |
| 3b | `feat(agentapply): derive the statusline decision from the persisted tri-state` | 3.5-3.6 | 153 |

Trimming will not rescue a single PR: 147 deletions and ~150 addition lines of
moved code are irreducible, putting the floor near 432 before any statusline
test. PR 3a is green on its own (`internal/tui` unmodified and passing), so 3b
stacks on it cleanly.

## Phase 4: Machine-Scoped Replay Applier (PR 4)

- [x] 4.1 RED: `internal/sync/apply_test.go` — recorder fake asserts exact ordered component-ID slice: models → skills → orchestrator/agents/hooks → MCPs → persona+instructions → statusline (Component Application Order Contract).
- [x] 4.2 GREEN: `internal/sync/apply.go` — ordered applier calling `internal/agentapply`, persona/instructions last.
- [x] 4.3 RED: sentinel-loss regression — CLAUDE.md with no sentinels → full order → assert sentinels, all installed skills, Hive protocol, orchestrator import present after.
- [x] 4.4 GREEN: wire fresh-render fallback per `WriteInstructions` (claude.go:350-356, opencode.go:445-452).
- [x] 4.5 RED: sentinel-bearing file preserves content outside managed sections byte-for-byte (Managed Instruction File Ownership Scope).
- [x] 4.6 RED: unowned-path file is never read/modified/replaced.
- [x] 4.7 GREEN: ownership-scope guard in `apply.go`.
- [ ] 4.8 RED: Jarvis-managed MCPs replaced unconditionally, never treated as a persisted user choice (Machine-Scoped Artifact Replay).
- [ ] 4.9 GREEN: unconditional MCP replacement call into existing `reconcile`/executor seams.
- [ ] 4.10 RED: statusline drift — decided-enabled + script absent on disk → reinstalled, manifest unchanged (Statusline Reinstallation on Drift).
- [ ] 4.11 GREEN: drift reinstall path in `apply.go`.
- [x] 4.12 RED: D1 partial failure — agent A converges, agent B fails midway; A's changes remain, report names both outcomes, non-zero exit, no global-convergence claim, no cross-agent rollback (Partial Failure Reporting Across Agents).
- [x] 4.13 GREEN: per-agent `ReconcileInstallRequest` scoping + loop-continue-on-failure in `apply.go`.

### Phase 4 PR boundary: three slices

Phase 4's 13 tasks cannot fit the 400-line budget as one PR, so it is split on
behaviour rather than file type. Each slice is autonomous and stacks on the
previous one:

| PR | Tasks | Scope |
|---|---|---|
| 4a | 4.1, 4.2, 4.12, 4.13 | Applier spine: locked component order, per-agent isolation, `AgentResult`/`Report` |
| 4b | 4.3-4.7 | Instruction-file ownership scope and sentinel handling |
| 4c | 4.8-4.11 | Unconditional MCP replacement and statusline drift reinstall |

PR 4a fixes the sequencing and reporting contract behind `ComponentRunner`, so
4b and 4c add component behaviour without reshaping the applier.

## Phase 5: Lifecycle Safety and Measured Idempotency (PR 5)

- [ ] 5.1 RED: `internal/sync/snapshot_test.go` — diff compares content+mode, never mtime (idempotency correctness given `InstallStatusline` rewrites unconditionally, claude.go:882-885).
- [ ] 5.2 GREEN: `internal/sync/snapshot.go` — content+mode snapshot/diff over the plan's own path list.
- [ ] 5.3 RED: threat-matrix mode assertion — statusline stays 0755, skills/instructions stay 0644 after sync, including reinstall-after-delete.
- [ ] 5.4 GREEN: mode assertion (not inheritance) in snapshot/apply write paths.
- [ ] 5.5 RED: backup precedes first mutation; backup failure blocks all mutation and is reported (Backup Precedes Mutation).
- [ ] 5.6 GREEN: `internal/lifecycle/backup.go` — accept explicit target list sourced from sync's plan (same list feeding the diff).
- [ ] 5.7 RED: second consecutive run against unchanged manifest/version reports zero changed files, zero writes (Measured Idempotency).
- [ ] 5.8 GREEN: zero-diff short-circuit wired through `apply.go`.
- [ ] 5.9 RED: changed-path report is required output, not optional (Required Changed-Path Output).
- [ ] 5.10 RED: no-op run writes no bookkeeping; changed run writes bookkeeping under lock (Bookkeeping Under Lock).
- [ ] 5.11 GREEN: locked bookkeeping writer, changed-path reporter.
- [ ] 5.12 RED: post-apply verification names `jarvis` as recovery command for an agent-less manifest (Post-Apply Verification and Recovery Naming).
- [ ] 5.13 GREEN: verification pass + recovery-command naming.

## Phase 6: Compatibility and Docs (PR 6)

- [ ] 6.1 RED: `cmd/jarvis/cmd_sync.go` — any flag is a usage error, zero mutation (Domain and CLI Boundary Exclusions).
- [ ] 6.2 RED: sync never calls Hive memory sync; call graph contains no Hive reference.
- [ ] 6.3 RED: `local+cloud` scope with missing/unparseable `sync.json` reports `jarvis login` for the cloud portion without aborting local scope.
- [ ] 6.4 GREEN: `cmd_sync.go` — replace no-op with planner+applier wiring, flag rejection, scope-partial reporting.
- [ ] 6.5 Update `docs/` — sync behavior, upgrade notes, `state.yaml` vs `config.yaml` split, recovery command.
- [ ] 6.6 Update `AGENTS.md`/`CLAUDE.md` parity note if sync behavior is referenced there.

## Review Workload Forecast

- Estimated changed lines: ~2,900 (additions + deletions, TDD roughly doubles production line count)
- 400-line budget risk: High
- Chained PRs recommended: Yes
- Decision needed before apply: No — `auto-chain` delivery strategy proceeds with PR 1 once the orchestrator resolves chain strategy
- PR boundaries and per-slice estimate:
  - PR 1 (state store + migration): ~600 lines — `internal/state/`, `internal/config/config.go`
  - PR 2 (planner + ownership): ~500 lines — `internal/sync/plan.go`, `ownership.go`
  - PR 3 (agentapply extraction): ~250 lines — `internal/agentapply/apply.go`, `internal/tui/agent_setup.go` delegation
  - PR 4 (replay applier): ~700 lines — `internal/sync/apply.go`
  - PR 5 (lifecycle safety): ~600 lines — `internal/sync/snapshot.go`, `internal/lifecycle/backup.go`
  - PR 6 (CLI + docs): ~280 lines — `cmd/jarvis/cmd_sync.go`, `docs/`
