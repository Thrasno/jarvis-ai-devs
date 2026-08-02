# Apply Progress: Distributed Project Quarantine

**Status**: complete — #474–#477 and delivery evidence are complete.
**Mode**: Strict TDD
**Delivery**: `size:exception` approved for one PR up to 3,000 changed code/test lines; verified actual code/test review surface: 2,607 lines; native attempt `full-implementation`.

## Completed Tasks

- [x] 5.1 Maintainer approved the size exception before implementation.
- [x] 1.1 RED: contract tests cover historical `export_marker`/`purge_intent` reads, `purge_intent` no-mutation rejection, and truthful audit facts.
- [x] 1.2 GREEN: additive migration 017 is registered; contract reads retain history and generation; handlers/routes/services use BLOCK/UNBLOCK only.
- [x] 1.3 REFACTOR/verify: focused and complete Hive API suites pass.

### #477 Completion

- [x] 4.1 RED: route, privacy, accessibility, release, polling, terminal-unsupported, and rollback scenarios were added in the supported focused dashboard test surface: `Quarantine.test.ts`, `QuarantineRoute.test.ts`, client, and fixture tests. No `hive-dashboard/e2e/quarantine.spec.ts` exists or is required.
- [x] 4.2 GREEN: Quarantine Center route, client, domain, navigation, fixtures, styles, and view implement the admin-only, username-only, generation-safe behavior.
- [x] 4.3 REFACTOR/verify: focused dashboard tests pass 48/48; the full dashboard suite passes 384/384; TypeScript lint, production dependency audit, complete API/daemon test and vet suites, PostgreSQL Testcontainers scenarios, and `git diff --check` pass.
- [x] 5.2 Delivery evidence records focused tests, supported runtime proof, changed-line scope, and rollback boundaries for all units.

### #475 Completion

- [x] 2.1 RED: lifecycle tests cover authenticated inbox delivery, inbox-before-sync ordering, stale/replay generation handling, archive restoration, HTTP 423, ACK retry, and response-loss reconnect.
- [x] 2.2 GREEN: migration 018 and the API/daemon lifecycle paths provide immutable generations, immediate cloud release, reversible local archives, and durable ACK delivery.
- [x] 2.3 REFACTOR/verify: legacy daemon HTTP harnesses explicitly accept only an authenticated inbox barrier before their existing `/sync` assertions; focused and full daemon/API suites plus vet pass.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `hive-api/internal/repository/postgres_project_block_test.go`, `internal/service/project_governance_test.go` | Integration/unit | model, handler, service, and migrations baseline PASS; repository baseline was run separately | Historical-read test failed because migration 017 left new rows without a generation; audit test failed because legacy export metadata was emitted | Passed after migration default, repository generation scan, and truthful audit metadata | Historical `export_marker` and `purge_intent`; rejected `purge_intent` with no repository call | `gofmt`; focused API suite passed |
| 1.2 | `hive-api/migrations/distributed_quarantine_test.go`, `cmd/server/main_test.go` | Unit | Existing migration/startup tests PASS | Existing partial migration/register test evidence retained | Passed after additive migration and ordered startup registration | Nullable legacy marker, generation backfill/default, and historical legacy actions | `gofmt`; full API suite passed |
| 1.3 | Focused Hive API package suites | Integration/unit | N/A — verification task | N/A — no new production behavior | Focused and full API suites passed | Repository integration tests run against PostgreSQL containers | No further refactor required |
| #474 slice | `hive-api/migrations/distributed_quarantine_test.go`, `hive-api/cmd/server/main_test.go` | Unit | `go test ./migrations` passed (no prior test files); API package baseline above | Both focused commands failed before their migration and startup registration changes | Both passed after implementation | Legacy nullable field and generation backfill; ordered migration registration | `gofmt`; focused tests passed |
| #474 contract correction | `hive-dashboard/src/views/Projects.test.ts`, `hive-dashboard/src/api/client.test.ts`, API service/handler contract tests | Unit | Dashboard 377/377 and focused API packages passed before edits | Updated Project view expectations failed while `export_marker` and `quarantine` remained in the form | `Projects.test.ts` 14/14; dashboard 377/377 + lint; API service/handler focused tests passed | BLOCK payload without legacy marker; exact confirmation still rejects padded input | TypeScript and Go formatting; full API suite passed |
| #475 daemon storage slice | `hive-daemon/internal/db/project_block_test.go` | Unit | `go test ./internal/db` passed before edits | `TestDB_RecordProjectBlockAppliesNewerUnblockWithoutDeletingHistory` failed to compile because action/generation were absent | Focused daemon DB test passed; full daemon suite passed | BLOCK generation 1 then UNBLOCK generation 2, with stale generation protected by `ON CONFLICT` predicate | `gofmt`; focused tests passed |
| #475 generation/history partial | `hive-daemon/internal/db/project_block_test.go`, `hive-api/migrations/distributed_quarantine_test.go`, `hive-api/internal/repository/postgres_project_block_test.go` | Unit/integration | Existing daemon DB/sync suites passed; API repository suite required Testcontainers | Duplicate generation test failed because `>=` replaced the command; migration test did not compile before 018 existed; UNBLOCK integration test initially exposed the actor column mismatch | Daemon duplicate/stale test, migration contract test, and PostgreSQL BLOCK→UNBLOCK→history test passed after the minimum changes | Older generation and same-generation divergent replay; BLOCK generation 1 then immediate cloud UNBLOCK generation 2 with two retained commands | `gofmt`; API and daemon full suites plus `go vet ./...` passed |
| #475 harness convergence | `hive-daemon/internal/sync/syncer_test.go` | Integration | Full daemon sync baseline was RED: legacy servers rejected `/project-blocks/inbox` before `/sync` | Existing lifecycle/order test was strengthened first; legacy harnesses then failed until they modeled the inbox barrier | `go test ./internal/sync -count=1 -timeout 90s` PASS after explicit authenticated inbox handling | Inbox auth and exact `inbox → ACK → ACK → sync` sequence; response-loss reconnect and ACK retry scenarios | Shared helper preserves existing `/sync` payload assertions and rejects unexpected paths |

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused tests | `cd hive-api && go test ./internal/model ./internal/repository ./internal/service ./internal/handler ./migrations -count=1` — PASS (5 packages; repository integration suite PASS in 187s). |
| Full API test | `cd hive-api && go test ./...` — PASS (9 packages; repository integration suite PASS in 188s). |
| Runtime harness | `go test ./internal/repository -count=1` — PASS (187s); repository contract tests execute against PostgreSQL Testcontainers. |
| Changed lines | #474 Hive API objective: 248 additions + 38 deletions tracked, plus 37 untracked migration/test lines = 323 code/test lines; within the 350-line unit cap. Existing daemon/dashboard partial bytes are preserved and excluded. |
| Rollback boundary | Revert #474 Hive API contract files and additive migration 017 together; retained historical `project_blocks` rows remain readable and are not rewritten. |
| #475 partial focused tests | `cd hive-daemon && go test ./internal/db -run 'TestDB_RecordProjectBlock(AppliesNewerUnblockWithoutDeletingHistory|IgnoresDuplicateOrStaleGeneration)' -count=1` — PASS; `cd hive-api && go test ./migrations ./internal/repository -count=1` — PASS. |
| #475 partial runtime harness | `cd hive-api && go test ./internal/repository -run TestPostgresProjectBlockRepository_UnblockAdvancesGenerationAndReleasesCloud -count=1` — PASS (PostgreSQL Testcontainers). |
| #475 partial full suites | `cd hive-api && go test ./...` — PASS; `cd hive-daemon && go test ./...` — PASS; both `go vet ./...` — PASS. |
| #475 partial rollback boundary | Revert migration 018, its startup registration, repository generation/release update, and the daemon strict-generation predicate together; migration history is retained and no dashboard files are touched. |
| #475 final-attempt focused tests | `cd hive-daemon && go test ./internal/db -run TestDB_QuarantineProjectRestoresOnlyArchiveOwnedByBlock -count=1` — PASS; `cd hive-daemon && go test ./internal/sync -run 'TestSyncer_Sync_(ProjectBlockCommandQuarantinesAndDoesNotMarkRowsSynced|ProjectBlockAckFailureRecordedInAttemptLog|RetriesPendingProjectBlockAcksBeforeNormalSync|PollsInboxBeforeSyncAndDropsDelayedOlderBlock)' -count=1` — PASS; `cd hive-api && go test ./internal/handler -run 'TestProjectGovernance_(InboxDeliversOnlyAuthenticatedAccountCommands|AckProjectBlockAllowsAuthenticatedDaemonUser)' -count=1` — PASS; focused service/repository generation tests — PASS. |
| #475 harness focused tests | `cd hive-daemon && go test ./internal/sync -run 'TestSyncer_Sync_(PollsInboxBeforeSyncAndDropsDelayedOlderBlock|ProjectBlockCommandQuarantinesAndDoesNotMarkRowsSynced|ProjectBlockAckFailureRecordedInAttemptLog|RetriesPendingProjectBlockAcksBeforeNormalSync|ResponseLossLeavesMutationRetryableThenAcksDuplicate|MutationProtocolV2SendsDeleteAfterCreateResponseLoss)' -count=1` — PASS. API inbox/ACK/generation repository runtime scenarios — PASS (PostgreSQL Testcontainers). |
| #475 full suites and vet | `cd hive-daemon && go test ./... -count=1 && go vet ./...` — PASS; `cd hive-api && go test ./... -count=1 && go vet ./...` — PASS (repository integration suite 189s). |
| #475 rollback boundary | Revert the lifecycle migration/API/daemon changes together; the harness-only portion is `hive-daemon/internal/sync/syncer_test.go`, and reverting it restores prior test fixture behavior without deleting archives or cloud command history. |
| #477 focused dashboard tests | `cd hive-dashboard && npm test -- src/views/Quarantine.test.ts src/views/QuarantineRoute.test.ts src/api/client.test.ts src/fixtures/hive-dashboard/dashboardFixtures.test.ts` — PASS (4 files, 48 tests). |
| #477 full dashboard and lint | `cd hive-dashboard && npm test` — PASS (26 files, 384 tests); `cd hive-dashboard && npm run lint` — PASS (`tsc --noEmit`). |
| #477 supported runtime evidence | Focused jsdom route/client/controller path — PASS. Dashboard E2E is N/A because `hive-dashboard/package.json` has no E2E script; this is the repository-supported behavioral proof. |
| #477 API/daemon regression | `cd hive-api && go test ./... && go vet ./...` — PASS (including PostgreSQL Testcontainers repository scenarios); `cd hive-daemon && go test ./... && go vet ./...` — PASS. |
| #477 audit and normalizers | `cd hive-dashboard && npm audit --omit=dev` — PASS, 0 vulnerabilities; `git diff --check` — PASS. |
| #477 changed lines | Current dashboard/task surface: 164 additions + 21 deletions; including the new Quarantine view/tests, approximately 448 authored changed lines, within the approved 2,000-line `size:exception`. |
| #477 rollback boundary | Revert only Quarantine Center dashboard client/domain/route/sidebar/fixture/style/view/tests and task marks; retain #474–#476 migrations, lifecycle, audit, commands, and stored cloud state. |

## Remaining Tasks

None.

## Corrective Runtime Rerun — 2026-08-02

The prior artifact history is retained. A failed apply gate found that the original Quarantine Center controller was not connected to route lifecycle behavior. This permitted corrective rerun closes that gap without changing the lifecycle/read-model contracts.

### Completed Corrective Tasks

- [x] Route-scoped polling starts only after the authenticated Quarantine Center route has loaded a selected detail, uses a 15-second schedule, stops on navigation/logout/destroy, and pauses/resumes from `visibilitychange`.
- [x] Progress requests receive an `AbortSignal`; superseded and destroyed requests are aborted, sequence-checked, and cannot update newer route state.
- [x] The controller retains selected project, filter, current pagination cursor, next cursor, and scroll context. It sends the preserved current cursor rather than advancing polling with the response's `next_cursor`.
- [x] `401` invokes the dashboard's established `actions.onLogout()` path, which clears the session through `SessionStore.logout`; `403` remains a denied/stopped state.
- [x] Release work is abortable and generation/request-sequence safe; late responses cannot clear newer state.

### Corrective TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | Triangulate | Refactor |
|---|---|---|---|---|---|---|---|
| Runtime lifecycle | `hive-dashboard/src/views/Quarantine.test.ts` | jsdom unit/integration | Existing 4 controller tests passed | 3 new tests failed because polling, cursor, lifecycle, and unauthorized APIs did not exist | 7/7 passed after minimum controller lifecycle implementation | Visible/hidden/resumed, destroyed/stale, `401`/state retention paths | Separated `cursor` (requested page) from `nextCursor` (server continuation) with tests green |
| Abortable client | `hive-dashboard/src/api/client.test.ts` | client unit | Existing 22 tests passed | Signal-forwarding assertion failed | 23/23 passed after optional progress signal forwarding | Existing cursor URL test plus explicit signal test | No further behavior refactor required |
| Route integration | `hive-dashboard/src/main.ts` via focused controller/route suite | jsdom integration | Quarantine route/controller/client baseline passed | Controller runtime contract was red before route wiring | Focused 3-file suite passed (32 tests) after route lifecycle and session wiring | Navigation/logout/destroy guards share the route lifecycle | Kept controller state independent from dashboard cache |

### Corrective Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command | `cd hive-dashboard && npm test -- src/views/Quarantine.test.ts src/views/QuarantineRoute.test.ts src/api/client.test.ts` — PASS (3 files, 32 tests). |
| Runtime harness | Focused jsdom controller + dashboard route/client path — PASS; no E2E script exists in `hive-dashboard/package.json`. |
| Full dashboard verification | `cd hive-dashboard && npm test` — PASS (26 files, 388 tests); `npm run lint` — PASS (`tsc --noEmit`). |
| Integrity | `git diff --check` — PASS. |
| Rollback boundary | Revert only the corrective dashboard lifecycle/client/controller changes in `hive-dashboard/src/main.ts`, `hive-dashboard/src/api/client.ts`, `hive-dashboard/src/views/Quarantine.ts`, and their focused tests; retain #474–#476 schema/history and the existing Quarantine Center surface. |

**Next recommended**: `sdd-verify`.

## Test-only Final Route Race Correction — 2026-08-02

This cumulative artifact preserves every prior completion above. An independent gate identified that the claimed stale-release controller proof never loaded a detail and therefore never initiated release work. The route cache also treated Quarantine Center project selection as query-insensitive, which let a competing selection retain the stale detail.

### Completed Final Race-Proof Tasks

- [x] Replaced the invalid controller assertion with a real `startDashboardApp` route scenario: it starts release for `Jarvis Dev`, changes selection to `Other Project` at generation 15, resolves the original release late, and proves the route still renders `Other Project` and its generation-15 progress.
- [x] Made `quarantines` query-sensitive in the existing route cache guard so a project-selection URL change reloads the detail; this is the minimum production fix required for the real route proof.
- [x] Preserved teardown and release-401 coverage; no client/controller release semantics were changed.

### Final Race-Proof TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | Triangulate | Refactor |
|---|---|---|---|---|---|---|---|
| Final route race proof | `hive-dashboard/src/views/Quarantine.test.ts`, `hive-dashboard/src/views/QuarantineRoute.test.ts` | jsdom route integration | Focused 35/35 PASS | The corrected controller assertion failed 0 calls, proving the previous test had no loaded detail and never started release; the new route test then failed because cached `Jarvis Dev` detail survived competing selection. | Focused 3-file suite PASS (36/36) after marking `quarantines` query-sensitive. | Real release + competing selected project/generation + late stale resolution; existing teardown and 401 route paths remain covered. | Removed the false-positive release assertion; retained the focused refresh-abort test and added no abstraction. |

### Final Race-Proof Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command | `cd hive-dashboard && npm test -- src/views/Quarantine.test.ts src/views/QuarantineRoute.test.ts src/api/client.test.ts` — PASS (3 files, 36 tests). |
| Runtime harness | `startDashboardApp` jsdom route path — PASS: a real `blockProject` UNBLOCK request starts, competing `other-project` generation 15 selection aborts its signal, and its late resolution cannot replace the rendered competing detail. No E2E script exists in `hive-dashboard/package.json`. |
| Full dashboard and lint | `cd hive-dashboard && npm test` — PASS (26 files, 392 tests); `npm run lint` — PASS. |
| Integrity | `git diff --check` — PASS. |
| Rollback boundary | Revert only `hive-dashboard/src/main.ts` query-sensitive cache change and the two focused test files; all #474–#476 schema, history, daemon lifecycle, and existing route wiring remain intact. |

**Next recommended**: `sdd-verify`.

## Resolved Dependency Blocker

The prior `dompurify` dependency-resolution blocker was resolved by the maintainer-approved `npm ci` restoration before this apply run. No source or manifest was changed for that restoration.

| Command | Result |
|---|---|
| `go test ./...` in `hive-api` | PASS |
| `go test ./...` in `hive-daemon` | PASS |
| `npm test && npm run lint` in `hive-dashboard` | Prior #474 baseline PASS: 377/377 tests and TypeScript lint; final #477 evidence is recorded above as 384/384 plus lint. |

#474–#477 task checkboxes and delivery evidence are complete. Preserve the completed lifecycle and read-model contracts for `sdd-verify`.

## Final Maintainer-Authorized Route Correction — 2026-08-02

This focused correction preserves all prior history above. It addresses only the independent gate's three remaining route-wiring blockers.

### Completed Final Corrective Tasks

- [x] The real Quarantine route provides a next-page control that calls the controller's `setCursor` with the API-provided opaque cursor before refreshing.
- [x] The real Quarantine route reports user scroll events to `setScrollTop` and restores the retained value onto its replacement view after controller refresh/rerender.
- [x] The real release control invokes only the guarded controller path. The controller owns an abort signal and release sequence, rejects stale generation/selection completions, aborts on teardown, and sends release `401` through the established logout action.
- [x] Route-level Vitest exercises cursor pagination, scroll restoration, release teardown race, and `401` session termination through `startDashboardApp`, not only controller methods.

### Final Corrective TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | Triangulate | Refactor |
|---|---|---|---|---|---|---|---|
| Final route wiring | `hive-dashboard/src/views/QuarantineRoute.test.ts` | jsdom route integration | Existing route file: 2/2 PASS | Three new real-route tests failed: next-page/release controls were absent and release `401` was unhandled | 5/5 PASS after minimum route/controller/client wiring | Pagination + scroll rerender; teardown late completion; release `401` logout | Reused controller state as the single runtime source; 35 focused tests remain green |

### Final Corrective Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command | `cd hive-dashboard && npm test -- src/views/Quarantine.test.ts src/views/QuarantineRoute.test.ts src/api/client.test.ts` — PASS (3 files, 35 tests). |
| Runtime harness | `startDashboardApp` jsdom route path — PASS for cursor propagation, scroll restoration, guarded release abort/late completion, and release `401` logout. No E2E script exists in `hive-dashboard/package.json`. |
| Full dashboard | `cd hive-dashboard && npm test` — PASS (26 files, 391 tests); `npm run lint` — PASS. |
| Integrity | `git diff --check` — PASS. |
| Rollback boundary | Revert the final wiring in `hive-dashboard/src/{main.ts,api/client.ts,views/Quarantine.ts}` and `hive-dashboard/src/views/{Quarantine.test.ts,QuarantineRoute.test.ts}`; all #474–#476 schema, history, and daemon lifecycle work remains intact. |

**Next recommended**: `sdd-verify`.

## #476 Completion

- [x] 3.1 RED: model cursor binding/privacy and malformed-cursor handler tests failed before `CursorID` and typed cursor validation existed; the prior partial RED remains the evidence for the read-model DTO/repository surface.
- [x] 3.2 GREEN: progress remains in a read-only `REPEATABLE READ` transaction; rows are username-only, active-account scoped, generation-pinned, deterministically ordered, and duplicate ACKs collapse to one account result. The cursor binds project/generation and serializes a hash ordering key instead of an account ID.
- [x] 3.3 REFACTOR/verify: focused model/handler/repository/service tests and the complete Hive API suite plus vet pass. PostgreSQL Testcontainers proves duplicate collapse, next-page cursor validation, and old/current generation separation.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 3.1 | `hive-api/internal/model/project_block_test.go`, `hive-api/internal/handler/project_governance_test.go`, `hive-api/internal/repository/postgres_project_block_test.go` | Unit/integration | Existing handler passed; focused package baseline completed before final changes | Cursor round-trip failed because the user-ID ordering field was omitted; invalid-cursor handler test failed to compile before the typed error existed | Model/handler tests pass after opaque hash cursor and client-error mapping | Cross-project/generation cursor rejection; malformed cursor; admin/no-identity projection; duplicate ACK state | Removed raw account ID from the cursor payload; `gofmt` and focused suites pass |
| 3.2 | Same as 3.1 | Integration | Existing read-model partial coverage retained | Covered by 3.1 cursor contract RED and prior missing-repository-surface RED | PostgreSQL projection passes with one row per active account | Duplicate ACK retry, next page, old generation ACK, new generation pending | Kept response DTO limited to username/state/timestamp |
| 3.3 | Same as 3.1 | Integration | N/A — verification task | N/A | Focused and full suites pass | PostgreSQL Testcontainers runtime path | No behavior-only refactor required |
| 4.1 | `hive-dashboard/src/views/Quarantine.test.ts`, `QuarantineRoute.test.ts` | jsdom integration | client, app, and Projects baseline: 179/179 | Missing Quarantine module and non-admin route assertions failed first | Focused 48/48 pass | Username privacy, pending text, lower-generation discard, release failure, authorization stop, and backoff | Extracted controller and reusable render helpers; focused suite remains green |
| 4.2 | `hive-dashboard/src/api/client.test.ts`, `fixtures/hive-dashboard/dashboardFixtures.test.ts` | jsdom/client | Existing dashboard suite baseline | New list/detail client and navigation expectations were introduced | Client paths, route, sidebar, and fixtures pass | List/detail URL, selected generation, and non-admin denial | Kept the projection closed to username, state, and timestamp only |
| 4.3 | Dashboard suite and Go suites | Integration | Focused suite green | Full dashboard suite initially found stale navigation expectations | 384 dashboard tests and both Go suites pass | Live jsdom route/client/runtime behavior; E2E command absent | `git diff --check` clean |

## Work Unit Evidence — #476

| Evidence | Result |
|---|---|
| Focused tests | `cd hive-api && go test ./internal/model ./internal/handler ./internal/repository ./internal/service -count=1` — PASS (4 packages; repository PostgreSQL suite PASS in 198.304s). |
| Runtime harness | `cd hive-api && go test ./internal/repository -run 'TestPostgresProjectBlockRepository_QuarantineProgress(CollapsesDuplicateAcknowledgementsAndPagesSafely|KeepsOlderGenerationConsistentAfterNewGeneration)' -count=1` — PASS (PostgreSQL Testcontainers, 3.167s). |
| Full API + vet | `cd hive-api && go test ./... -count=1 && go vet ./...` — PASS (9 packages; repository suite 197.395s). |
| Changed lines | Tracked workspace diff is 1,389 additions + deletions, within the maintainer-approved 2,000-line `size:exception`; this #476 completion adds model/handler/repository tests and cursor validation to the existing read-model slice. |
| Rollback boundary | Revert only #476 DTO/read-only transaction/project-block repository/service/handler/router changes and their tests; retain migrations and #474/#475 lifecycle history. |

## Maintainer-Authorized Verification Remediation — 2026-08-02

The failed `verify-report.md` is retained unchanged as historical evidence (`verdict: fail`, evidence revision `sha256:37430cfb80d0c6bc1f1210e3abf1360eca055a9528f16a3d16154fbb2654b567`). This apply transaction addresses its five CRITICAL findings; a fresh verdict remains owned by `sdd-verify`.

### Completed Remediation Tasks

- [x] R1: `TestProjectGovernanceService_ConcurrentTransitionsPreserveStrictGenerationHistory` runs two simultaneous real PostgreSQL service transitions and proves unique ordered generations, retained command history, and current head truth.
- [x] R2: `TestProjectGovernance_InboxRejectsUnauthenticatedAndAccountlessCallersWithoutDisclosure` proves `401` and `403` rejection without command/project fields; accountless callers do not invoke `Inbox`.
- [x] R3: `TestPostgresTxManager_ReadOnlyRepeatableReadKeepsAdminSnapshotDuringConcurrentTransition` executes the production read-only transaction manager against Testcontainers and proves generation-pinned list/detail consistency during a committed concurrent transition.
- [x] R4: `ListQuarantines` now derives the latest active-account current-generation outcome rather than hard-coding `pending`; handler/service/repository and dashboard disabled→re-enabled retained-state paths are covered.
- [x] R5: OpenSpec and Engram design/tasks/apply-progress have identical current remediation status; verify-report remains explicitly historical and failing pending re-verification.

### Remediation TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | Triangulate | Refactor |
|---|---|---|---|---|---|---|---|
| R1 concurrent generations | `hive-api/internal/service/memory_transaction_integration_test.go` | PostgreSQL integration | Existing focused service tests PASS | Scenario authored before implementation assessment; existing serialized implementation passed the first runtime execution. | PASS: commands `[1,2]`, both actions retained, head `2`. | Concurrent BLOCK/UNBLOCK plus existing sequential history coverage. | No production change required. |
| R2 inbox authorization | `hive-api/internal/handler/project_governance_test.go` | HTTP integration | Existing governance handler tests PASS | Accountless subject called service and returned `500`, not `403`. | PASS after handler rejects invalid subject before service access. | Missing auth (`401`) and accountless auth (`403`) both prove no disclosure. | Generic forbidden response retained. |
| R3 repeatable-read snapshot | `hive-api/internal/repository/postgres_project_block_test.go` | PostgreSQL/Testcontainers | Existing repository snapshot tests PASS | Scenario authored before implementation assessment; production transaction implementation passed the first runtime execution. | PASS: transaction sees generation 1 before/after mutation; later read sees 2. | Detail plus list snapshot and external UNBLOCK paths. | No production change required. |
| R4 truthful list and rollback | `hive-api/internal/repository/postgres_project_block_test.go`, `hive-dashboard/src/views/QuarantineRoute.test.ts` | PostgreSQL/jsdom route | Existing repository and route tests PASS | List returned hard-coded `pending` after an active account ACK. | PASS after current-generation outcome query; dashboard retained-state reload also passes. | Applied outcome, retained UNBLOCK, disabled→re-enabled route. | Closed DTO remains privacy-limited. |
| R5 artifact parity | OpenSpec + Engram artifacts | Artifact integration | Prior artifacts read in both stores | Prior Engram design/tasks/progress diverged from filesystem. | OpenSpec updated and mirrored to the same Engram topics. | Historical failed verify report retained rather than overwritten. | No verify report created. |

### Remediation Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused Go tests | `cd hive-api && go test ./internal/handler -run 'TestProjectGovernance_(InboxRejects|ListQuarantines)' -count=1`; `go test ./internal/repository -run 'TestPostgres(ProjectBlockRepository_ListQuarantinesDerivesCurrentGenerationOutcome|TxManager_ReadOnlyRepeatableReadKeepsAdminSnapshotDuringConcurrentTransition)' -count=1`; `go test ./internal/service -run 'TestProjectGovernanceService_(ConcurrentTransitionsPreserveStrictGenerationHistory|ListQuarantinesLoadsRetainedCurrentGenerationAfterRelease)' -count=1` — PASS. |
| Dashboard runtime | `cd hive-dashboard && npm test -- src/views/Quarantine.test.ts src/views/QuarantineRoute.test.ts` — PASS (2 files, 14 tests); the route proves disabled→re-enabled retained-state loading. |
| Full verification requested for apply | `go test ./... && go vet ./...` in both `hive-api` and `hive-daemon` — PASS; `npm test && npm run lint` in `hive-dashboard` — PASS (26 files, 393 tests); `git diff --check` — PASS. |
| Testcontainers runtime | Focused repository and service commands above execute PostgreSQL Testcontainers against production repository/transaction/service code — PASS. |
| Rollback boundary | Revert only the R1/R3 test additions, R2 handler guard/tests, R4 list SQL/tests/dashboard route test, and this remediation artifact section. Migrations, commands, archives, and already committed releases remain intact. |

**Next recommended**: `sdd-verify` for a fresh independent verdict; do not archive from this apply result.

## Delivery Authorization Reconciliation — 2026-08-02

The maintainer explicitly expanded the approved `size:exception` from 2,000 to 3,000 changed code/test lines. The verified actual code/test review surface is 2,607 lines, within that approval. This resolves the prior authorization blocker recorded in the historical failed `verify-report.md` without changing its `FAIL` verdict; only a fresh `sdd-verify` may issue a new verdict.
