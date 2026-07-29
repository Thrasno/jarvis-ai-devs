# Apply Progress: Replace Healthy Daemons with Syncing Users

## Status
Complete — tasks 1.1 through 3.2 are complete; focused remediation evidence is pending parent finish.

## Completed Tasks
- [x] 1.1 Repository/service RED coverage for canonical projection and status mapping.
- [x] 1.2 Projection model, repository/mock contract, migration 016, and embedding.
- [x] 1.3 Shared sync-status and last-success mapping.
- [x] 2.1 API DTOs, overview/admin services, wiring, and authorization-safe admin users contract.
- [x] 2.2 Dashboard API/domain/view contracts for syncing users and user sync context.
- [x] 3.1 Remove daemon-health repository residue and stale assertions.
- [x] 3.2 Final cleanup and full multi-module verification.

## TDD Cycle Evidence
| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| 1.1 | `TestPostgresSyncAttemptRepository_UserSyncProjectionUsesCanonicalCompletedAttempts` did not compile: `SyncAttemptRepository.UserSyncProjection` was missing. | `go test ./internal/repository ./internal/service ./cmd/server ./migrations` — PASS. | Triangulated canonical-only, completed-attempt, deterministic-tie, and one-query repository cases with service-table coverage; `gofmt`. |
| 1.2 | Projection tests did not compile: `model.UserSyncProjectionRow` and repository/mock `UserSyncProjection` were missing; migration 016 was not embedded. | `go test ./internal/repository ./internal/service ./cmd/server ./migrations` — PASS. | Triangulated model/interface compilation, PostgreSQL projection integration, mock conformance, and migration embedding; `gofmt`. |
| 1.3 | `TestUserSyncStatus` did not compile: `userSyncStatus` and `userSyncLastSyncAt` were undefined. | `go test ./internal/repository ./internal/service ./cmd/server ./migrations` — PASS. | Triangulated inclusive 24-hour boundaries, latest-failure, future/incomplete, inactive, and retained-success mapping through the shared mapper; `gofmt`. |
| 2.1 | `TestAdminService_ListUsersAddsSyncContextWithTwoRepositoryCalls` failed: constructor/DTO absent | Focused Go service/handler/server command passed | Captured `now` once per Overview request; `gofmt` |
| 2.2 | Focused Overview/Users Vitest tests failed on absent KPI/columns | Dashboard lint and full suite passed: 23 files, 361 tests | Const-derived sync statuses; no navigation added |
| 3.1 | `TestSyncAttemptRepositoryDoesNotRetainDaemonHealthContract` failed for interface, PostgreSQL SQL, and mock residue | PASS after removing all three, focused repository test passed | Updated stale Overview test setup without weakening account/role/Audit Log coverage; `gofmt` |
| 3.2 | N/A — verification-only task after source normalization | All module tests, affected-module vet, dashboard tests/lint, and `git diff --check` passed | Triangulation skipped: no production behavior added |

## Work Unit Evidence
- Focused cleanup: `go test ./internal/repository -run '^TestSyncAttemptRepositoryDoesNotRetainDaemonHealthContract$' -count=1` — PASS (1 test).
- Source proof: `daemon_health` has no Go source occurrences; the dashboard's retained negative contract assertion verifies it is never serialized.
- Full Go tests: `go test ./...` passed in all modules — hive-api 809, hive-daemon 1,271, jarvis-cli 2,289, hivederive 25 passing test events (4,394 total; 43 packages; 0 failures).
- Vet: `go vet ./...` — PASS in affected module `hive-api`.
- Dashboard: `npm test -- --run && npm run lint` — PASS; 361 tests in 23 files; lint passed.
- Diff hygiene: `git diff --check` — PASS.
- Runtime harness: N/A — Gin handler and DOM contracts exercise the exposed HTTP/UI boundaries; no new process boundary.
- Rollback: revert this unit's daemon-health removal and stale-test normalization together with the API/dashboard contract; migration 016 may remain.
- Workload: final cleanup unit is 155 changed lines, within its 500-line budget; cumulative worktree is 1,132 changed lines, within the approved 1,600-line limit.

## Focused Remediation: Native Ordinal 8

- Safety net and affected service suite: `go test ./internal/service -count=1` — PASS (before 2.385s; after 2.406s).
| Task | RED | GREEN | REFACTOR |
|---|---|---|---|
| Old retained success | Test written first; existing mapper already passed, so no production change was needed. | `go test ./internal/service -run '^TestUserSyncStatus_OldRetainedSuccessRemainsInactiveWithExactLastSync$' -count=1` — PASS. | `gofmt -w internal/service/user_sync_test.go` — PASS. |
- Dashboard boundary unchanged: `npm test -- --run src/views/Users.test.ts` — PASS (25 tests).
- Docs: spec/design define operational `unknown` as `Unavailable` on projection failure, distinct from successful-projection `last_24h|inactive|never`.
- Rollback: revert the mapper test and synchronized spec/design/apply-progress evidence only.

## Remaining Tasks
None.
