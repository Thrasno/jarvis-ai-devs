# Apply Progress: Replace Open Conflicts with Degraded Projects

## Cumulative Status

- Completed tasks: 1.1–3.4.
- Pending tasks: 4.1–4.2.

## Partial Work

Completed portal identity persistence. The repository persists nullable portal identity/provenance; migration 015 backfills only exact legacy email matches, constrains provenance, and adds the projection index. The handler passes JWT subject and level to the service; device metadata is never identity input.

Completed the canonical repository projection. It ranks attempts per `(project, portal_user_id)` using the specified stable ordering, joins active users at read time, excludes blocked and nonparticipating projects, and derives both rows and totals from one result.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `hive-api/internal/{service,repository}/sync_attempt_test.go` | Unit/integration | service/handler baseline passed | service RED: missing actor/provenance API; repository RED: missing migration symbol | focused suites passed | member, admin exact-email, unresolved admin, legacy exact-email | None needed |
| 1.2 | `hive-api/internal/repository/postgres_sync_attempt_test.go` | Integration | repository focused baseline passed | migration/backfill test failed to compile without 015 registration | PostgreSQL migration test passed | legacy match and non-null provenance persistence | None needed |
| 1.3 | `hive-api/internal/handler/sync_attempt_test.go` | HTTP unit | handler focused baseline passed | actor propagation assertion failed (`actor == nil`) | handler/service/repository focused suites passed | admin actor and repository persisted provenance | None needed |
| 2.1 | `hive-api/internal/repository/postgres_sync_attempt_overview_test.go` | PostgreSQL integration | overview safety net initially failed because fixture omitted 015 | missing `ProjectSyncHealth` API failed to compile | projection scenario suite passed | multiple users, stale outcome, equal timestamp, inactive, blocked, and empty cases | None needed |
| 2.2 | `hive-api/internal/{model,repository}/{sync_attempt,postgres_sync_attempt,mock_sync_attempt}.go` | PostgreSQL integration | same fixture baseline | projection test failed without interface/implementation | focused repository suite passed | empty projection added | None needed |
| 2.3–2.4 | `hive-api/internal/{service,handler}/{overview,project}_test.go` | Unit/HTTP integration | service/handler baseline passed after fake repair | overview contract failed on legacy audit call; filter tests failed to pass/reject `health` | focused tests and package safety net passed | canonical healthy/degraded participants and nullable nonparticipant paths | canonical Project-service projection wiring complete |

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test | `go test ./internal/repository -run 'Test(PostgresSyncAttemptRepository_UpsertBatchIsIdempotentByDevAndAttempt|SyncAttemptPortalUsersMigration_BackfillsExactEmail)' -count=1` PASS (3.134s); service and handler focused tests PASS. |
| Runtime harness | Same repository command: real PostgreSQL Testcontainers migration/backfill and INSERT path passed. |
| Rollback boundary | Revert migration 015, its registration, and sync-attempt handler/repository persistence changes; existing audit rows and additive columns remain safe. |

## Work Unit 2 Evidence

| Evidence | Result |
|---|---|
| Focused test | `go test ./internal/repository -run 'TestPostgresSyncAttemptRepository_(SyncHealthByProject|ProjectSyncHealth)' -count=1` PASS (13.502s). |
| Runtime harness | Same command: real PostgreSQL Testcontainers applies migrations 007, 015, and project blocks; projection exercises persistence and canonical SQL. |
| Rollback boundary | Revert `ProjectSyncHealth` model/interface/mock/PostgreSQL query and its repository tests; Phase 1 identity persistence remains intact. |

## Native Attempt Evidence

- Ordinal: 4
- Revision: `sha256:3639cf3eba36b6f14de118b480b2435b72ee3903d42279f339e0e26c8de2bc20`
- Work unit: `complete-identity-persistence`
- Reset baseline: `458860...`; cumulative changed lines at entry: 0.
- Objective-relative changed lines: 84 added/modified, manually tallied from this actor's file edits only; pre-baseline partial tree excluded.

## Native Attempt Evidence

- Ordinal: 5
- Revision: `sha256:4431cd79a43de18e35af23de5f2a3acb1c20e42199db5dd1b251da9faaba65e0`
- Work unit: `projection-and-api-contract`
- Reset baseline: tree after Phase 1; objective-relative implementation changed lines: 0/200.
- Blocked before RED: the required repository safety-net suite fails because `startPostgresWithSyncAttemptsAndSessions` applies only `SyncAttemptLogsSQL`, while `UpsertBatch` now writes the Phase 1 `portal_user_id` column added by migration 015. The test container therefore lacks that column. No task 2.1–2.4 production or test change was made.

## Native Attempt Evidence

- Ordinal: 6
- Revision: `sha256:3131128ade4f654a57192ab4b9260cfbc845bb2eacc95095bcef3de9d26cc9ca`
- Work unit: `projection-and-api-contract`
- Reset baseline: tree after Phase 1; entry ledger: 8/200.
- Objective-relative implementation delta: 102 changed lines (49 integration-test lines including the fixture migration, plus 53 model/repository lines); cumulative ledger: 110/200; remainder: 90 lines.
- Completed the Phase 1 fixture correction and Phase 2 repository projection only; API wiring remains deliberately out of scope for this bounded batch.

## Native Attempt Evidence

- Ordinal: 7
- Revision: `sha256:ec5c2c2d1926a233b3c3ce81650a4bcb72674ba01a570a52e5e5c5d1c117c51b`
- Work unit: `atomic-api-contract-and-filter`
- Reset baseline: current projection baseline; objective budget: 200 lines.
- Blocked before RED: the required `hive-api` service/handler safety-net command fails to compile because `fakeSyncAttemptRepo` in `internal/service/sync_attempt_test.go` no longer implements `SyncAttemptRepository` after task 2.2 added `ProjectSyncHealth`. Tasks 2.3–2.4 were not edited, so no task can be marked complete under strict TDD.

## Native Attempt Evidence

- Ordinal: 8
- Revision: `sha256:fb85c0c54f7992175a30fc9f8387edd6ad2b7d791cc9e57e4fc5d866c8e20eae`
- Work unit: `atomic-api-contract-and-filter`; delivery: `size:exception`.
- Repaired `fakeSyncAttemptRepo.ProjectSyncHealth` first; the service/handler safety net then passed.
- RED→GREEN: overview emits `degraded_projects` from `ProjectSyncHealthProjection` and has no `conflicts` JSON field; `GET /projects?health=degraded` accepts only the canonical filter and rejects unsupported values with `400` behind existing authentication.
- Focused verification passed: service canonical-contract/status tests and handler filter/auth tests; `go test ./internal/service ./internal/handler -count=1` passed.
- Not marked complete: the filtered Project service still maps its legacy per-project aggregate instead of consuming `ProjectSyncHealthProjection`, so task 2.4's canonical project-summary/filter wiring (including nullable nonparticipant health) remains. Dashboard work was not touched.

## Native Attempt Evidence

- Ordinal: 9
- Revision: `sha256:34af0f6526542bc279ce1e8eb1bfb70eb5c0fbf8f298c3e6042eac376f0b06e8`
- Work unit: `canonicalize-project-service-health`; delivery: `size:exception`, bounded to the remaining Phase 2 API work.
- RED: `go test ./internal/service -run 'TestProjectService_List' -count=1` failed before production changes because `NewProjectService` had no health projection dependency and `ProjectSummary.SyncHealth` was non-nullable.
- GREEN: ProjectService now reads `ProjectSyncHealthProjection` for canonical health and the degraded filter; nonparticipants have omitted `syncHealth`. Server wiring injects the sync-attempt repository, while overview retains the existing `degraded_projects` projection contract.
- REFACTOR: kept a narrow projection-reader interface at the service boundary and reused `latestTime` for activity timestamps.

## Work Unit 3 Evidence

| Evidence | Result |
|---|---|
| Focused test | `go test ./internal/service -run 'TestProjectService_List' -count=1` PASS (0.012s); `go test ./cmd/server -run 'TestWireServices_(InjectsAuditRepositoryIntoAdminAndSyncServices|ProjectBlockAckWiredWhenAdminMutationDisabledByDefault|WiresProjectRepositoryIntoRouterDeps|WiresActivityServiceFromMemoryRepository)$' -count=1` PASS (0.002s); `go test ./internal/service ./internal/handler -count=1` PASS (service 2.539s; handler 0.016s). |
| Runtime harness | `go test ./internal/handler -count=1` passed the Gin/httptest HTTP boundary, including authenticated `GET /projects?health=degraded` and invalid-filter `400` coverage. |
| Rollback boundary | Revert ProjectService projection injection, nullable `ProjectSummary.SyncHealth`, and server-factory wiring in `hive-api/internal/{model,service}/project.go` and `hive-api/cmd/server`; the canonical repository projection and overview contract remain intact. |

## Known Verification Note

- `go test ./cmd/server -count=1` still fails only on four pre-existing migration-count assertions that expect 14 startup migrations although migration 015 is now registered. The focused server wiring tests pass; this batch did not alter those migration assertions.

## Native Attempt Evidence

- Ordinal: 10
- Revision: `sha256:a81e25f5ad1effa59c78abc9825e548a99e1cea247b4e01191a34b0660f45570`
- Work unit: `fix-migration-count-and-validate-backend`; delivery: `size:exception`, bounded to the migration-count assertions.
- RED: `go test ./cmd/server -count=1` failed only because the four candidate assertions still expected 14 startup migrations while `startupMigrationSQL` registered migration 015 and returned 15 entries.
- GREEN: updated only those four assertions from 14 to 15; `go test ./cmd/server -count=1` passed.
- REFACTOR: none needed; this is a structural expectation correction.

## Work Unit 4 Evidence

| Evidence | Result |
|---|---|
| Focused test | `go test ./cmd/server -count=1` PASS (0.003s). |
| Runtime harness | `go test ./internal/handler -count=1` PASS (0.015s), exercising the Gin/httptest HTTP boundary; no runtime boundary exists for the startup-migration slice itself. |
| Backend safety net | `go test ./internal/service ./internal/handler -count=1` PASS (service 2.610s; handler 0.015s); `go test ./internal/repository -count=1` PASS (200.418s). |
| Rollback boundary | Revert only the four `startupMigrationSQL` length expectations in `hive-api/cmd/server/main_test.go`; migration 015 registration and all production behavior remain unchanged. |

## TDD Cycle Evidence: Work Unit 4

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Migration-count correction | `hive-api/cmd/server/main_test.go` | Unit | Candidate suite failed only on the four stale counts | Existing assertions failed: actual 15, expected 14 | `go test ./cmd/server -count=1` PASS | Skipped: one structural count determined solely by the registered migration list | None needed |

## Native Attempt Evidence

- Ordinal: 11
- Revision: `sha256:d19ebae6d8178f6e9fd1f3da79f65576c2bbb56cbff48743df8e387399c678ce`
- Work unit: `dashboard-contract-and-routing`; delivery: `size:exception`.
- Objective-relative dashboard delta: 158 changed lines (124 additions, 34 deletions), within the 200-line budget from the current backend baseline.
- Completed tasks 3.1–3.4. The Dashboard consumes `degraded_projects`, renders exact `DEGRADED PROJECTS` `N / total`, serializes `health=degraded`, and restores direct/shared/refresh/popstate project routes.

## Work Unit 5 Evidence

| Evidence | Result |
|---|---|
| Focused test | `npm test -- src/api/client.test.ts src/api/urlFilters.test.ts src/views/Projects.test.ts src/views/Overview.test.ts src/app.test.ts` PASS (5 files, 213 tests). |
| Runtime harness | jsdom app harness in `src/app.test.ts` PASS: direct degraded URL and `popstate` reload asserted against the API client. |
| Full dashboard suite | `npm test` PASS (23 files, 359 tests). `npm run lint` PASS (`tsc --noEmit`). |
| Rollback boundary | Revert only the Phase 3 dashboard API/query/domain/view/fixture files listed in the implementation report; backend `degraded_projects` contract remains unchanged. |

## TDD Cycle Evidence: Work Unit 5

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 3.1–3.2 | `src/api/client.test.ts`, `src/app.test.ts` | API/app integration | `npm test` baseline: 23 files, 355 tests PASS | New query and URL-restoration assertions failed: `/projects` omitted health and app passed no filter | Focused suite PASS | Direct degraded route and unfiltered popstate path | Query parsing shared by API reload and visible state |
| 3.3–3.4 | `src/views/Overview.test.ts`, `src/views/Projects.test.ts` | DOM integration | Same baseline | KPI and filter-link assertions failed: no degraded metric or controls | Focused suite PASS | Active degraded and all links, empty state, and card accessibility/no nested controls | None needed |

## Known Verification Note

- The planned `npm test -- --runInBand` command is invalid for this Vitest version (`Unknown option --runInBand`); the repository script `npm test` is the passing dashboard command used above.

## Attempt 15 Remediation

- Revision: `sha256:0cbc80f822e19961837030fd6056a662ce44ecaf4964bb0e39bf539863984526`; remediation delta before this evidence: 7 changed lines.
- The maintainer explicitly approved retained single-PR `size:exception`; the approval is recorded in `tasks.md`.
- Limitation: historical RED commands/output were unavailable and were not reconstructed. The new regression entered GREEN because the existing DTO already excluded device fields.

## TDD Cycle Evidence: Attempt 15

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Serialized health contract | `internal/handler/overview_test.go` | Gin/httptest | focused test PASS | N/A — no historical RED invented | focused test PASS | health plus three forbidden device keys | None needed |

## Work Unit Evidence: Attempt 15

- Focused/runtime: `go test ./internal/handler -run '^TestOverviewHandler_GetStats_AdminJWT_Returns200$' -count=1` PASS; Gin/httptest serializes authenticated admin stats.
- Rollback: revert the four serialization assertions and populated health row in `overview_test.go`; no production behavior changed.
