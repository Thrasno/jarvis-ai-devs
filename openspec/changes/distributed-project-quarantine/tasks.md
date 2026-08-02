# Tasks: Distributed Project Quarantine

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 850–1,100 additions + deletions |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | #474 → #475 → #476 → #477; each unit keeps tests with behavior |
| Delivery strategy | exception-ok |
| Chain strategy | not-applicable — maintainer approved `size:exception` for one PR |

Decision needed before apply: No — resolved by maintainer-approved `size:exception`.
Chained PRs recommended: Yes
Chain strategy: not-applicable
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| #474 | Contract/migration compatibility | PR 1 | `go test ./hive-api/internal/model ./hive-api/internal/repository ./hive-api/internal/service ./hive-api/internal/handler` | API contract tests; no external runtime | `hive-api/migrations/017_quarantine_contract.sql` plus contract files |
| #475 | Generations, inbox, daemon convergence | PR 2 | `go test ./hive-api/internal/... ./hive-daemon/internal/...` | short sync scenario; skip integrations with `-short` | lifecycle/inbox/daemon files; retain schema/history |
| #476 | Admin snapshot read model | PR 3 | `go test ./hive-api/internal/handler ./hive-api/internal/repository` | authenticated admin API scenario | read-model DTO/repository/service/routes |
| #477 | Quarantine Center UI | PR 4 | `npm test --prefix hive-dashboard` | N/A: `hive-dashboard/package.json` has no E2E script | Quarantine route/view/style files |

## Phase 1: #474 Contract Foundation

- [x] 1.1 RED: add tests in `hive-api/internal/handler/project_governance_test.go`, `hive-api/internal/repository/postgres_project_block_test.go`, and `hive-api/migrations/distributed_quarantine_test.go` for contract scenarios.
- [x] 1.2 GREEN: add `hive-api/migrations/017_quarantine_contract.sql`; update `hive-api/internal/model/project_block.go`, repository/service/handler/router, and `hive-api/migrations/migrations.go` without rewriting history.
- [x] 1.3 REFACTOR/verify: record focused API and migration-test evidence. Commit creation is a later delivery concern.

## Phase 2: #475 Distributed Lifecycle

- [x] 2.1 RED: add race/inbox/stale/replay/archive/423/ACK-retry tests in `hive-daemon/internal/db/project_block_test.go` and `hive-daemon/internal/sync/syncer_test.go` plus API tests.
- [x] 2.2 GREEN: add `hive-api/migrations/018_distributed_quarantine.sql`; update `hive-api/internal/repository/{tx,postgres_tx,project_block,postgres_project_block}.go`, services/handlers, and `hive-daemon/internal/{db,sync}` paths.
- [x] 2.3 REFACTOR/verify: record API and daemon suite evidence, including short integrations. Commit creation is a later delivery concern.

## Phase 3: #476 Read Model

- [x] 3.1 RED: add admin/auth, aggregation, duplicate, cursor, and snapshot tests in `hive-api/internal/handler/project_governance_test.go` and `hive-api/internal/repository/postgres_project_block_test.go`.
- [x] 3.2 GREEN: implement DTOs, `REPEATABLE READ` queries, redaction, ordering, and generation-pinned pagination in `hive-api/internal/repository/{project_block,postgres_project_block}.go`, service, and `project_governance.go`/`router.go`.
- [x] 3.3 REFACTOR/verify: record handler/repository and API-integration evidence. Commit creation is a later delivery concern.

## Phase 4: #477 Dashboard

- [x] 4.1 RED: add route/privacy/accessibility/release/polling/state/unsupported/rollback tests in `hive-dashboard/src/views/Quarantine.test.ts`, `hive-dashboard/src/views/QuarantineRoute.test.ts`, and existing client/app/sidebar/fixture tests; prove the dashboard runtime path through the repository-supported focused Vitest suite.
- [x] 4.2 GREEN: add `hive-dashboard/src/views/Quarantine.ts`; wire `src/api/client.ts`, `src/domain/dashboard.ts`, `src/main.ts`, `src/components/Sidebar.ts`, fixtures, and `src/styles.css` behind a capability flag.
- [x] 4.3 REFACTOR/verify: record the focused dashboard suite (48/48), full dashboard suite (384/384), TypeScript lint, `npm audit --omit=dev` (0 vulnerabilities), full API/daemon `go test ./...` and `go vet ./...`, PostgreSQL Testcontainers scenarios, and `git diff --check`. No E2E command exists in `hive-dashboard/package.json`; the focused route/client/controller tests are the supported dashboard runtime evidence. Commit creation is a later delivery concern.

## Delivery Gate

- [x] 5.1 Before apply, obtain maintainer choice: approve `size:exception` for one PR or select a chained strategy; do not begin oversized single-PR implementation without that decision.
- [x] 5.2 Recorded focused tests, supported runtime result, changed-line count, and exact rollback boundary for every unit; rollback preserves schema and history.

## Corrective Apply: Quarantine Center Runtime

- [x] 4.1 corrective RED: added deterministic controller/client tests for 15-second route polling, hidden-tab pause/resume, abort-on-destroy, cursor and scroll retention, stale response/release protection, and `401` session termination.
- [x] 4.2 corrective GREEN: wired the Quarantine Center controller to the dashboard route lifecycle, browser visibility, abortable progress requests, and the existing session logout convention; the client now forwards polling abort signals.
- [x] 4.3 corrective REFACTOR/verify: focused Vitest (3 files, 32 tests), full dashboard Vitest (26 files, 388 tests), TypeScript lint, and `git diff --check` pass. Rollback is limited to `hive-dashboard/src/{main.ts,api/client.ts,views/Quarantine.ts}` and their focused tests.

## Final Maintainer-Authorized Route Correction

- [x] 4.1 final RED: added actual-route Vitest scenarios that failed before production wiring for cursor pagination, scroll restoration, guarded release teardown, and release `401` logout.
- [x] 4.2 final GREEN: the production Quarantine route now calls `setCursor` from its next-page control and `setScrollTop` from its scroll handler; it restores scroll after controller rerenders and routes release only through the abortable, sequence- and generation-guarded controller.
- [x] 4.3 final REFACTOR/verify: focused Vitest passes 35/35, full dashboard Vitest passes 391/391, TypeScript lint and `git diff --check` pass. Rollback is limited to the Quarantine route/controller/client wiring and focused route tests.

## Test-only Final Race Proof

- [x] 4.1 final race RED: replaced the invalid controller-only release assertion (no loaded detail, so no release could start) with a deterministic `startDashboardApp` route test that starts a real release, selects a competing project at generation 15, resolves the stale release, and asserts the competing route remains rendered.
- [x] 4.2 final race GREEN: made Quarantine Center query selection cache-sensitive so the real route reloads the selected project instead of retaining stale cached detail; no release-controller behavior changed.
- [x] 4.3 final race REFACTOR/verify: focused Vitest passes 36/36, full dashboard Vitest passes 392/392, TypeScript lint and `git diff --check` pass. Rollback is limited to `hive-dashboard/src/main.ts` and `hive-dashboard/src/views/{Quarantine.test.ts,QuarantineRoute.test.ts}`.

## Maintainer-Authorized Verification Remediation

- [x] R1 RED/GREEN: added a real PostgreSQL concurrent-transition service scenario; two administrators commit one canonical project and preserve commands/generations `1,2` plus the generation-2 head.
- [x] R2 RED/GREEN: added inbox-specific missing-auth and accountless-auth tests; accountless callers receive `403` without service/repository access or command/project disclosure.
- [x] R3 RED/GREEN: added a real PostgreSQL/Testcontainers `ReadOnlyRepeatableRead` scenario that holds the admin snapshot while another connection creates generation 2, then proves the transaction remains generation 1 and a later read sees generation 2.
- [x] R4 RED/GREEN: covered admin list handler/service/repository center-load behavior, replaced the hard-coded list `pending` state with the latest active-account current-generation outcome, and proved disabled→re-enabled dashboard reload of retained state.
- [x] R5 REFACTOR/verify: synchronized OpenSpec and Engram design/tasks/apply-progress, preserved the failed verify report as historical evidence, and ran focused/runtime/full API, daemon, dashboard, and diff checks.

## Delivery Authorization Reconciliation — 2026-08-02

The maintainer explicitly approved `size:exception` up to 3,000 changed code/test lines. The verified code/test review surface is 2,607 lines, so the previous 2,000-line authorization blocker is resolved. This authorization reconciliation preserves every checkbox and forecast above; the current `verify-report.md` remains historical `FAIL` evidence until a fresh `sdd-verify` issues a new verdict.
